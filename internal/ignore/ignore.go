// Package ignore compiles the project-local ignore document and matches
// logical project-relative paths against that policy.
//
// The document is .nodex/ignore.json. Schema 1 is a JSON object with three
// required fields: schema, the integer 1; presets, an array of preset
// identifier strings; and exclude, an array of pattern strings. Either array
// may be empty. Preset identifiers are a set. Duplicates are rejected and the
// canonical order is lexical. This package checks that shape only. It does
// not decide which identifiers a project may enable or what an identifier
// excludes. Patterns keep their order, including duplicates. JSON whitespace
// and object-field order do not change the compiled policy. Unknown object
// fields, a missing or null presets or exclude field, a schema other than
// the integer 1, and any data after the JSON value are rejected. If one
// preset or pattern is invalid, Parse returns an error and the zero Policy,
// which is not a partial compilation of the document.
//
// A pattern uses slash-separated elements:
//
//   - zero or more bytes inside one path element
//     ?       one UTF-8 rune inside one path element
//     **      zero or more path elements, only when ** is a whole element
//     name/   directory only
//     /name   anchored at the project root
//     a/b     a pattern that contains "/" is anchored at the project root
//     name    a pattern with no "/" matches at any depth
//     !pat    negation; the last matching rule re-includes pat
//     [abc]   one rune from the class
//     [a-z]   one rune from the inclusive range
//     [!abc]  one rune outside the class
//     \x      x is literal
//
// Consecutive * bytes inside one element are a single *. A final foo/**
// matches entries inside foo and does not match foo itself. Rules run in
// order and the last match decides. A path that no rule matches is included.
// An excluded directory seals its subtree: a later negation does not
// re-include a path whose ancestor directory is excluded. A negation can
// re-include a path only when every ancestor directory stays traversable.
//
//	vendor/             vendor and everything under it is excluded; the
//	!vendor/patched.go  negation cannot reach inside the sealed directory.
//
//	vendor/*            vendor itself matches no rule and stays traversable,
//	!vendor/patched.go  so the negation re-includes patched.go.
//
// Matching is exact. It does not fold case, normalize Unicode, or treat a
// leading "." as special. Candidate paths are non-empty, relative, and use
// "/" separators, with no leading or trailing "/" and no empty, ".", or ".."
// element. This package does not clean paths, resolve symbolic links, walk a
// tree, discover a project root, or open the document on disk.
//
// A Policy is immutable. The zero value enables no presets and excludes
// nothing. Match, Excluded, Presets, Excludes, and Identity are safe for
// concurrent callers of one policy. Identity is a SHA-256 digest of the
// canonical preset set and the compiled rules, kept distinct from a hash of
// the document bytes by a fixed Nodex domain header.
package ignore

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
)

const (
	// DocumentPath is the path of the ignore document relative to the project root.
	DocumentPath = ".nodex/ignore.json"

	// SchemaVersion is the only document schema Parse accepts.
	SchemaVersion = 1
)

var (
	// ErrInvalidUTF8 is returned when the document is not valid UTF-8.
	ErrInvalidUTF8 = errors.New("ignore document contains invalid UTF-8")

	// ErrNotObject is returned when the document is not a JSON object.
	ErrNotObject = errors.New("ignore document must be a JSON object")

	// ErrMissingSchema is returned when the document has no schema field.
	ErrMissingSchema = errors.New("ignore document is missing schema")

	// ErrUnsupportedSchema is returned when schema is not the integer 1.
	ErrUnsupportedSchema = errors.New("ignore document schema is unsupported")

	// ErrMissingPresets is returned when the document has no presets field.
	ErrMissingPresets = errors.New("ignore document is missing presets")

	// ErrNullPresets is returned when presets is JSON null.
	ErrNullPresets = errors.New("ignore document presets is null")

	// ErrPresetsType is returned when presets is not an array of strings.
	ErrPresetsType = errors.New("ignore document presets must be an array of strings")

	// ErrDuplicatePreset is returned when a preset identifier is repeated.
	ErrDuplicatePreset = errors.New("duplicate preset")

	// ErrMissingExclude is returned when the document has no exclude field.
	ErrMissingExclude = errors.New("ignore document is missing exclude")

	// ErrNullExclude is returned when exclude is JSON null.
	ErrNullExclude = errors.New("ignore document exclude is null")

	// ErrExcludeType is returned when exclude is not an array of strings.
	ErrExcludeType = errors.New("ignore document exclude must be an array of strings")

	// ErrTrailingData is returned when bytes after the JSON value remain.
	ErrTrailingData = errors.New("ignore document has trailing data")
)

// UnknownFieldError reports a top-level JSON field this schema does not define.
type UnknownFieldError struct {
	Name string
}

// Error reports the unknown field name.
func (e *UnknownFieldError) Error() string {
	return fmt.Sprintf("ignore document contains unknown field %q", e.Name)
}

// RuleError is the failure of one exclude entry.
//
// Index is the zero-based position of that entry. Pattern is the entry text
// when the entry was a string. The wrapped error is the cause.
type RuleError struct {
	Index   int
	Pattern string
	Err     error
}

// Error identifies the entry by index and includes the cause.
func (e *RuleError) Error() string {
	if e.Pattern == "" {
		return fmt.Sprintf("exclude[%d]: %v", e.Index, e.Err)
	}
	return fmt.Sprintf("exclude[%d] %q: %v", e.Index, e.Pattern, e.Err)
}

// Unwrap returns the cause of the entry failure.
func (e *RuleError) Unwrap() error { return e.Err }

// PresetError is the failure of one presets entry.
//
// Index is the zero-based position of that entry. ID is the entry text when
// the entry was a string. The wrapped error is the cause.
type PresetError struct {
	Index int
	ID    string
	Err   error
}

// Error identifies the entry by index and includes the cause.
func (e *PresetError) Error() string {
	if e.ID == "" {
		return fmt.Sprintf("presets[%d]: %v", e.Index, e.Err)
	}
	return fmt.Sprintf("presets[%d] %q: %v", e.Index, e.ID, e.Err)
}

// Unwrap returns the cause of the entry failure.
func (e *PresetError) Unwrap() error { return e.Err }

// Policy is an immutable compiled ignore policy.
//
// The zero value enables no presets and has no rules. presets are the
// canonical set of preset identifiers. excludes are the original manual
// patterns in document order. rules are those patterns compiled.
type Policy struct {
	presets  []string
	excludes []string
	rules    []rule
}

// Parse compiles one schema-1 ignore document.
//
// On error the returned Policy is the zero policy. That value enables no
// presets, excludes nothing, and is not a partial compilation of the document.
func Parse(data []byte) (Policy, error) {
	presets, patterns, err := decodeDocument(data)
	if err != nil {
		return Policy{}, err
	}
	return New(presets, patterns)
}

// New compiles a policy from preset identifiers and manual exclude patterns.
//
// Preset identifiers are copied and sorted lexically. An empty identifier or
// a duplicate is rejected. Exclude patterns keep their order and duplicates
// and are compiled as manual rules. The result does not share backing arrays
// with the caller. On error the returned Policy is the zero policy.
func New(presets, excludes []string) (Policy, error) {
	if err := validatePresetIDs(presets); err != nil {
		return Policy{}, err
	}
	rules, err := compilePatterns(excludes)
	if err != nil {
		return Policy{}, err
	}
	return Policy{
		presets:  canonicalPresets(presets),
		excludes: copyStrings(excludes),
		rules:    rules,
	}, nil
}

// Presets returns a copy of the enabled preset identifiers in canonical order.
func (p Policy) Presets() []string { return copyStrings(p.presets) }

// Excludes returns a copy of the original manual exclude patterns, in order.
func (p Policy) Excludes() []string { return copyStrings(p.excludes) }

func canonicalPresets(ids []string) []string {
	out := copyStrings(ids)
	sort.Strings(out)
	return out
}

// Encode returns the canonical schema-1 document.
//
// The object fields are schema, presets, and exclude, in that order, with
// two-space indentation. Preset identifiers are lexical. Manual exclude
// patterns keep their stored order. The result ends with one newline.
func (p Policy) Encode() ([]byte, error) {
	doc := encodedDocument{
		Schema:  SchemaVersion,
		Presets: p.Presets(),
		Exclude: p.Excludes(),
	}
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(doc); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

type encodedDocument struct {
	Schema  int      `json:"schema"`
	Presets []string `json:"presets"`
	Exclude []string `json:"exclude"`
}

func copyStrings(in []string) []string {
	out := make([]string, len(in))
	copy(out, in)
	return out
}

// RuleCount reports how many compiled rules the policy holds, duplicates included.
func (p Policy) RuleCount() int { return len(p.rules) }

// Match reports whether path is excluded.
//
// parentExcluded is the decision already made for the parent directory, and
// it is false for a child of the project root. An excluded directory seals
// every path under it, so a true parent decision stays excluded.
//
// isDir reports whether path is a directory. A directory-only pattern matches
// only when isDir is true.
//
// The last matching rule decides. A negation re-includes a path only when
// parentExcluded is false. A path with no matching rule is included.
func (p Policy) Match(path string, isDir, parentExcluded bool) bool {
	if parentExcluded {
		return true
	}
	if len(p.rules) == 0 {
		return false
	}
	segments := strings.Split(path, "/")
	excluded := false
	for _, rule := range p.rules {
		if rule.matches(segments, isDir) {
			excluded = !rule.negate
		}
	}
	return excluded
}

// Excluded reports whether path is excluded, including excluded ancestors.
//
// Each ancestor is matched as a directory. Once an ancestor is excluded, path
// stays excluded. isDir reports whether path itself is a directory.
func (p Policy) Excluded(path string, isDir bool) bool {
	if len(p.rules) == 0 {
		return false
	}
	segments := strings.Split(path, "/")
	parentExcluded := false
	for i := range segments {
		last := i == len(segments)-1
		parentExcluded = p.Match(strings.Join(segments[:i+1], "/"), !last || isDir, parentExcluded)
	}
	return parentExcluded
}
