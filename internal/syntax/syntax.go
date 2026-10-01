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
// A parsed document records the path, the language, and the comment units
// in physical lexical order. A unit carries its half-open physical range,
// the exact source bytes of that range, and normalized text for later
// indexing. Normalized text keeps directive content. Syntax does not decide
// whether a comment is useful, and it does not assign comment IDs.
//
// Context returns a bounded structural snippet around one comment range.
// The snippet is a physical slice of the caller-supplied bytes. It carries
// the range of that slice and the exact text. It does not carry a syntax
// tree, a node name, or a classification of the comment.
//
// The Go implementation is a private detail of this root. Callers import
// this package and do not import a language subpackage. The types returned
// here do not carry a Go syntax tree.
package syntax

import (
	"errors"
	"fmt"
	"strings"

	"github.com/edalca/nodex/internal/syntax/golang"
)

// Language names a source language this package can interpret.
// The zero value is not a supported language.
type Language string

const (
	// Go is the Go programming language. A logical path names Go when it
	// ends in the suffix ".go".
	Go Language = "go"
)

const (
	// MaxContextLines is the maximum number of physical source lines in a
	// structural snippet whose comment fits in that many lines. A comment
	// that itself occupies more lines keeps its starting line and only as
	// many following lines as fit in this limit.
	MaxContextLines = 40

	// MaxContextExtraBytes is the maximum number of source bytes added
	// around a comment inside a structural snippet. Bytes that belong to
	// the comment are not counted against this budget.
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
// A unit is one group of comments as the language defines a group. For Go,
// adjacent comments with no other tokens and no blank line between them are
// one unit. Raw is the exact source text covering Range, taken from the
// bytes passed to Parse. Text is the normalized content of the unit: comment
// delimiters are removed, directive text is kept, and the order of the
// comments in the group is preserved.
type Comment struct {
	Raw   string
	Text  string
	Range Range
}

// Document is one parsed source file.
//
// Path is the logical path the caller passed to Parse. Comments are in
// physical lexical order. The slice is empty when the file has no comments.
type Document struct {
	Path     string
	Language Language
	Comments []Comment
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
	if strings.HasSuffix(logicalPath, ".go") {
		return Go, true
	}
	return "", false
}

// Parse interprets source as the language of logicalPath.
//
// source is the complete file contents. Nil source is empty input. Parse
// does not open logicalPath. An unsupported path returns *UnsupportedError
// and a nil document, including when the bytes happen to be valid in some
// supported language. A syntax error returns *ParseError and a nil document.
// A successful document contains every comment unit in physical order.
func Parse(logicalPath string, source []byte) (*Document, error) {
	lang, ok := Recognize(logicalPath)
	if !ok {
		return nil, &UnsupportedError{Path: logicalPath}
	}
	switch lang {
	case Go:
		return parseGo(logicalPath, source)
	default:
		return nil, &UnsupportedError{Path: logicalPath}
	}
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
	lang, ok := Recognize(logicalPath)
	if !ok {
		return Snippet{}, &UnsupportedError{Path: logicalPath}
	}
	switch lang {
	case Go:
		return contextGo(logicalPath, source, commentRange)
	default:
		return Snippet{}, &UnsupportedError{Path: logicalPath}
	}
}

func contextGo(logicalPath string, source []byte, commentRange Range) (Snippet, error) {
	if source == nil {
		source = []byte{}
	}
	got, err := golang.Context(source, golang.Position{
		Offset: commentRange.Start.Offset,
		Line:   commentRange.Start.Line,
		Column: commentRange.Start.Column,
	}, golang.Position{
		Offset: commentRange.End.Offset,
		Line:   commentRange.End.Line,
		Column: commentRange.End.Column,
	}, golang.Limits{
		Lines:      MaxContextLines,
		ExtraBytes: MaxContextExtraBytes,
	})
	if err != nil {
		return Snippet{}, contextFailure(logicalPath, err)
	}
	return Snippet{
		Range: Range{
			Start: Position{Offset: got.Start.Offset, Line: got.Start.Line, Column: got.Start.Column},
			End:   Position{Offset: got.End.Offset, Line: got.End.Line, Column: got.End.Column},
		},
		Text: got.Text,
	}, nil
}

func contextFailure(logicalPath string, err error) error {
	switch {
	case errors.Is(err, golang.ErrMalformedRange):
		return fmt.Errorf("%s: %w", logicalPath, ErrMalformedRange)
	case errors.Is(err, golang.ErrCommentNotFound):
		return fmt.Errorf("%s: %w", logicalPath, ErrCommentNotFound)
	case errors.Is(err, golang.ErrAmbiguousComment):
		return fmt.Errorf("%s: %w", logicalPath, ErrAmbiguousComment)
	default:
		return parseFailure(logicalPath, err)
	}
}

func parseGo(logicalPath string, source []byte) (*Document, error) {
	if source == nil {
		source = []byte{}
	}
	comments, err := golang.Parse(source)
	if err != nil {
		return nil, parseFailure(logicalPath, err)
	}
	out := make([]Comment, len(comments))
	for i, c := range comments {
		out[i] = Comment{
			Raw:  c.Raw,
			Text: c.Text,
			Range: Range{
				Start: Position{Offset: c.Start.Offset, Line: c.Start.Line, Column: c.Start.Column},
				End:   Position{Offset: c.End.Offset, Line: c.End.Line, Column: c.End.Column},
			},
		}
	}
	return &Document{Path: logicalPath, Language: Go, Comments: out}, nil
}

func parseFailure(logicalPath string, err error) error {
	var ge *golang.Error
	if errors.As(err, &ge) {
		return &ParseError{Path: logicalPath, Detail: formatDiagnostics(logicalPath, ge.Diagnostics)}
	}
	return &ParseError{Path: logicalPath, Detail: fmt.Sprintf("%s: %s", logicalPath, err.Error())}
}

func formatDiagnostics(logicalPath string, diags []golang.Diagnostic) string {
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
