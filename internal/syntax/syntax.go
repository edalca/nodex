// Package syntax interprets source text as a language-neutral structure.
//
// Recognize reports whether a logical path is a source file in a supported
// language. The decision uses the path suffix and is case-sensitive. File
// contents are not inspected. Parse then interprets the source bytes the
// caller supplies. It does not open the path, walk a directory, or apply
// project exclusion policy. A recognized path stays ordinary source until
// the caller applies an enabled preset. Parse itself still interprets any
// source the caller supplies.
//
// Optional language-aware source-exclusion presets are part of this package.
// The catalog, selector expansion, and the exclusion decision are declared
// with those functions. No preset is enabled here. Manual project exclusions
// are not decided here. Language implementations own what each preset means.
//
// A parsed document records the path, the language, the comment units in
// physical lexical order, and the declaration facts from that same parse.
// A comment unit carries its half-open physical range, the exact source
// bytes of that range, and normalized text for later indexing. Normalized
// text keeps directive content. Syntax does not decide whether a comment
// is useful, and it does not assign comment IDs.
//
// A declaration fact carries its kind, the names that node declares, its
// half-open physical range, and the physical range of its directly
// associated documentation comment when the parser recorded one. A missing
// documentation comment is a fact about that node. Syntax does not decide
// that the declaration requires documentation, and it does not assign
// declaration IDs.
//
// Context returns a bounded structural snippet around one comment range.
// DeclarationContext returns a bounded structural snippet of one declaration
// range. Each snippet is a physical slice of the caller-supplied bytes. It
// carries the range of that slice and the exact text. It does not carry a
// syntax tree or a judgment about the comment or the declaration.
//
// Language implementations are private details of this root. Callers import
// this semantic facade, whose results contain no parser-specific structures.
package syntax

import (
	"errors"
	"fmt"
	"strings"

	"github.com/edalca/nodex/internal/syntax/contracts"
	"github.com/edalca/nodex/internal/syntax/types"
)

// Language names a source language this package can interpret.
// The zero value is not a supported language.
type Language string

const (
	// MaxContextLines is the maximum number of physical source lines in a
	// structural snippet. A comment that itself occupies more lines keeps
	// its starting line and only as many following lines as fit in this
	// limit. A declaration snippet keeps the declaration's first physical
	// line and only as many following lines as fit in this limit.
	MaxContextLines = 40

	// MaxContextExtraBytes is the maximum number of source bytes added
	// around the anchored span inside a structural snippet. Bytes that
	// belong to the comment, or to the first physical line of a
	// declaration, are not counted against this budget.
	MaxContextExtraBytes = 8192
)

// Snippet is a bounded physical slice of source around one comment.
//
// Range is the half-open interval of Text in the source passed to Context.
// Text is the exact bytes of that interval. It is not reformatted, and a
// truncated slice contains no inserted ellipsis. The range is the record of
// which physical slice was selected.
type Snippet struct {
	Range Range
	Text  string
}

var (
	// ErrMalformedRange means commentRange is not a coherent physical range
	// of the supplied source.
	ErrMalformedRange = errors.New("comment range is malformed")

	// ErrCommentNotFound means commentRange is not exactly one comment unit.
	ErrCommentNotFound = errors.New("comment range does not match a comment")

	// ErrAmbiguousComment means commentRange matches more than one comment unit.
	ErrAmbiguousComment = errors.New("comment range matches more than one comment")

	// ErrMalformedDeclaration means declRange is not a coherent physical
	// range of the supplied source.
	ErrMalformedDeclaration = errors.New("declaration range is malformed")

	// ErrDeclarationNotFound means declRange is not exactly one declaration.
	ErrDeclarationNotFound = errors.New("declaration range does not match a declaration")

	// ErrAmbiguousDeclaration means declRange matches more than one declaration.
	ErrAmbiguousDeclaration = errors.New("declaration range matches more than one declaration")
)

// Position is a physical location in the source passed to Parse.
//
// Offset is a zero-based byte index into that source. Line and Column are
// one-based. Column counts bytes on the line, not runes, so Offset, Line,
// and Column name one byte. A line ends at '\n'. Line directives in the
// source do not move Position and do not change the file it refers to.
type Position struct {
	Offset int
	Line   int
	Column int
}

// Range is the half-open physical interval [Start, End) of a comment unit.
// Start is the first byte of the unit. End is the first byte after it.
type Range struct {
	Start Position
	End   Position
}

// Comment is one comment unit.
//
// A unit is one group of comments as the language defines a group.
// Raw is the exact source text covering Range, taken from the
// bytes passed to Parse. Text is the normalized content of the unit: comment
// delimiters are removed, directive text is kept, and the order of the
// comments in the group is preserved.
type Comment struct {
	Raw   string
	Text  string
	Range Range
}

// Kind names a structural declaration form.
//
// The text is stable and language-neutral. It describes the form of the
// declaration node. It does not rank the declaration.
type Kind string

const (
	// KindPackage is a package clause.
	KindPackage Kind = "package"
	// KindFunction is a function declaration.
	KindFunction Kind = "function"
	// KindMethod is a function declaration with a receiver.
	KindMethod Kind = "method"
	// KindConstGroup is a parenthesized const declaration.
	KindConstGroup Kind = "const-group"
	// KindVarGroup is a parenthesized var declaration.
	KindVarGroup Kind = "var-group"
	// KindTypeGroup is a parenthesized type declaration.
	KindTypeGroup Kind = "type-group"
	// KindConst is one const declaration.
	KindConst Kind = "const"
	// KindVar is one var declaration.
	KindVar Kind = "var"
	// KindType is one type declaration.
	KindType Kind = "type"
	// KindField is a struct field or an interface field.
	KindField Kind = "field"
)

// Declaration is one structural declaration fact from a parse.
//
// Kind is the form of the node. Names are the identifiers that node
// declares, in source order. Names is empty when the node declares no
// identifier, including an embedded field. Range is the half-open physical
// extent of the declaration. HasDoc is true when the parser associated a
// documentation comment directly with this node. Doc is that comment's
// physical range, and it is meaningful only when HasDoc is true. A false
// HasDoc means the node's documentation field was empty. It does not mean
// that documentation is required, and it is not inferred from another node.
type Declaration struct {
	Kind   Kind
	Names  []string
	Range  Range
	HasDoc bool
	Doc    Range
}

// Document is one parsed source file.
//
// Path is the logical path the caller passed to Parse. Comments are in
// physical lexical order. The slice is empty when the file has no comments.
// Declarations are the declaration facts from the same parse. The slice is
// empty when the file has no declarations. Comments and Declarations are
// produced together. A failed parse returns neither.
type Document struct {
	Path         string
	Language     Language
	Comments     []Comment
	Declarations []Declaration
}

// UnsupportedError means logicalPath is not source in a language this
// package can interpret. The decision is the path suffix.
type UnsupportedError struct {
	Path string
}

// Error reports the unsupported path.
func (e *UnsupportedError) Error() string {
	if e == nil {
		return "unsupported source language"
	}
	return fmt.Sprintf("unsupported source language: %s", e.Path)
}

// ParseError means source is not valid in the language of Path.
//
// Path is the logical path supplied by the caller. Detail uses that path
// and physical line and column numbers. A ParseError has no document.
type ParseError struct {
	Path   string
	Detail string
}

// Error returns the parse diagnostic.
func (e *ParseError) Error() string {
	if e == nil {
		return "parse error"
	}
	if e.Detail == "" {
		return "parse " + e.Path
	}
	return e.Detail
}

// Recognize reports the language of logicalPath.
//
// The decision uses only the final suffix and is case-sensitive. Contents
// are not read. The second result is false when the suffix is not a
// supported language.
func Recognize(logicalPath string) (Language, bool) {
	implementation := languages.resolve(logicalPath)
	if implementation == nil {
		return "", false
	}
	return Language(implementation.ID()), true
}

// Parse interprets source as the language of logicalPath.
//
// source is the complete file contents. Nil source is empty input. Parse
// does not open logicalPath. An unsupported path returns *UnsupportedError
// and a nil document, including when the bytes happen to be valid in some
// supported language. A syntax error returns *ParseError and a nil document.
// A successful document contains every comment unit in physical order and
// the declaration facts from that same parse.
func Parse(logicalPath string, source []byte) (*Document, error) {
	return languages.parse(logicalPath, source)
}

func (r registry) parse(logicalPath string, source []byte) (*Document, error) {
	implementation := r.resolve(logicalPath)
	if implementation == nil {
		return nil, &UnsupportedError{Path: logicalPath}
	}
	got, err := implementation.Parse(source)
	if err != nil {
		return nil, parseFailure(logicalPath, err)
	}
	if got == nil {
		panic("syntax language returned no document on success")
	}
	out := make([]Comment, len(got.Comments))
	for i, c := range got.Comments {
		out[i] = Comment{Raw: c.Raw, Text: c.Text, Range: facadeRange(c.Start, c.End)}
	}
	declared := make([]Declaration, len(got.Declarations))
	for i, d := range got.Declarations {
		names := make([]string, len(d.Names))
		copy(names, d.Names)
		declared[i] = Declaration{Kind: Kind(d.Kind), Names: names,
			Range: facadeRange(d.Start, d.End), HasDoc: d.HasDoc,
			Doc: facadeRange(d.DocStart, d.DocEnd)}
	}
	return &Document{Path: logicalPath, Language: Language(implementation.ID()), Comments: out, Declarations: declared}, nil
}

// Context returns the bounded structural snippet surrounding the comment
// at commentRange.
//
// logicalPath selects the language. source is the complete file contents.
// Nil source is empty input. Context does not open logicalPath and does not
// walk a directory. commentRange must be the physical range of exactly one
// comment unit in a successful parse of source. The unit is identified by
// that range. Its text is not a key. An unsupported path returns
// *UnsupportedError. A syntax error returns *ParseError and no snippet.
// A range that is not exactly one comment unit returns an error wrapping
// ErrMalformedRange, ErrCommentNotFound, or ErrAmbiguousComment.
//
// The language implementation chooses the structural container: the
// declaration associated with the comment, the declaration that physically
// contains it, the package clause for a package comment, or a file
// neighborhood when no declaration applies. The snippet is that container
// when the container is at most MaxContextLines physical lines and adds at
// most MaxContextExtraBytes outside the comment. Otherwise the snippet is a
// contiguous slice of the container around the comment, bounded by those
// same limits. The slice stays inside the container. Text is copied from
// source.
func Context(logicalPath string, source []byte, commentRange Range) (Snippet, error) {
	return languages.context(logicalPath, source, commentRange, false)
}

// DeclarationContext returns the bounded structural snippet of the
// declaration at declRange.
//
// logicalPath selects the language. source is the complete file contents.
// Nil source is empty input. DeclarationContext does not open logicalPath
// and does not walk a directory. declRange must be the physical range of
// exactly one declaration in a successful parse of source. The declaration
// is identified by that range. Its names and its documentation text are not
// keys. An unsupported path returns *UnsupportedError. A syntax error
// returns *ParseError and no snippet. A range that is not exactly one
// declaration returns an error wrapping ErrMalformedDeclaration,
// ErrDeclarationNotFound, or ErrAmbiguousDeclaration.
//
// The snippet is a contiguous prefix of the declaration. It is the whole
// declaration when that fits in MaxContextLines and MaxContextExtraBytes.
// Otherwise it keeps the declaration's first physical line and as much of
// the following declaration text as those limits allow. Text is copied from
// source. It is not reformatted, and a truncated slice contains no inserted
// ellipsis.
func DeclarationContext(logicalPath string, source []byte, declRange Range) (Snippet, error) {
	return languages.context(logicalPath, source, declRange, true)
}

func (r registry) context(logicalPath string, source []byte, target Range, declaration bool) (Snippet, error) {
	implementation := r.resolve(logicalPath)
	if implementation == nil {
		return Snippet{}, &UnsupportedError{Path: logicalPath}
	}
	limits := contracts.Limits{Lines: MaxContextLines, ExtraBytes: MaxContextExtraBytes}
	var got types.Snippet
	var err error
	if declaration {
		got, err = implementation.DeclarationContext(source, internalRange(target), limits)
	} else {
		got, err = implementation.Context(source, internalRange(target), limits)
	}
	if err != nil {
		return Snippet{}, contextFailure(logicalPath, err)
	}
	return Snippet{Range: facadeRange(got.Start, got.End), Text: got.Text}, nil
}

func contextFailure(logicalPath string, err error) error {
	for _, pair := range []struct{ internal, external error }{
		{contracts.ErrMalformedRange, ErrMalformedRange},
		{contracts.ErrCommentNotFound, ErrCommentNotFound},
		{contracts.ErrAmbiguousComment, ErrAmbiguousComment},
		{contracts.ErrMalformedDeclaration, ErrMalformedDeclaration},
		{contracts.ErrDeclarationNotFound, ErrDeclarationNotFound},
		{contracts.ErrAmbiguousDeclaration, ErrAmbiguousDeclaration},
	} {
		if errors.Is(err, pair.internal) {
			return fmt.Errorf("%s: %w", logicalPath, pair.external)
		}
	}
	return parseFailure(logicalPath, err)
}

func facadeRange(start, end types.Position) Range {
	return Range{Start: Position(start), End: Position(end)}
}

func internalRange(r Range) types.Range {
	return types.Range{Start: types.Position(r.Start), End: types.Position(r.End)}
}

func parseFailure(logicalPath string, err error) error {
	var failure *contracts.ParseError
	if errors.As(err, &failure) {
		return &ParseError{Path: logicalPath, Detail: formatDiagnostics(logicalPath, failure.Diagnostics)}
	}
	return &ParseError{Path: logicalPath, Detail: fmt.Sprintf("%s: %s", logicalPath, err.Error())}
}

func formatDiagnostics(logicalPath string, diags []types.Diagnostic) string {
	if len(diags) == 0 {
		return "parse " + logicalPath
	}
	parts := make([]string, len(diags))
	for i, d := range diags {
		if d.Position.Line > 0 {
			parts[i] = fmt.Sprintf("%s:%d:%d: %s", logicalPath, d.Position.Line, d.Position.Column, d.Msg)
			continue
		}
		parts[i] = fmt.Sprintf("%s: %s", logicalPath, d.Msg)
	}
	return strings.Join(parts, "\n")
}

// Selectors returns the command-line preset selectors known to this build,
// in lexical order. The result includes concrete presets and aggregate
// selectors. It is a copy.
func Selectors() []string {
	return append([]string{}, languages.selectors...)
}

// ConcretePresets returns the preset identifiers this build can persist,
// in lexical order. Aggregate selectors are not included. The result is a copy.
func ConcretePresets() []string {
	return append([]string{}, languages.concrete...)
}

// ExpandSelector returns the concrete preset identifiers selected by selector.
//
// A concrete identifier selects itself. An aggregate selector expands to the
// concrete presets of that language known to this build. The result is a new
// slice in lexical order and never contains the aggregate selector. An
// unknown selector is rejected.
func ExpandSelector(selector string) ([]string, error) {
	return languages.expandSelector(selector)
}

// ValidatePresets reports whether every id is a concrete preset this build
// can persist. Aggregate selectors and unknown identifiers are rejected.
// An empty list is valid. The list is not reordered.
func ValidatePresets(ids []string) error {
	return languages.validatePresets(ids)
}

// PathExcluded reports whether enabled presets exclude logicalPath from its
// path alone.
//
// logicalPath is a slash-separated logical project path. enabled holds
// concrete preset identifiers. An unrecognized path is not excluded. Presets
// that need source text are not applied here, and source is not read.
func PathExcluded(logicalPath string, enabled []string) bool {
	return languages.pathExcluded(logicalPath, enabled)
}

// InspectExcluded reports whether enabled presets exclude logicalPath after
// structural inspection.
//
// source is the file bytes. It is parsed only when an enabled preset
// classifies files by structure. With no such preset, source is not
// inspected and may be nil. Path-only presets are also applied, so a caller
// that already dropped path exclusions can still use this as the full
// decision. An unrecognized path is not excluded.
func InspectExcluded(logicalPath string, source []byte, enabled []string) bool {
	return languages.inspectExcluded(logicalPath, source, enabled)
}
