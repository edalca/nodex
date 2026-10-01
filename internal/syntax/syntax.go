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
// A successful document contains every comment unit in physical order and
// the declaration facts from that same parse.
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
	lang, ok := Recognize(logicalPath)
	if !ok {
		return Snippet{}, &UnsupportedError{Path: logicalPath}
	}
	switch lang {
	case Go:
		return declarationContextGo(logicalPath, source, declRange)
	default:
		return Snippet{}, &UnsupportedError{Path: logicalPath}
	}
}

func declarationContextGo(logicalPath string, source []byte, declRange Range) (Snippet, error) {
	if source == nil {
		source = []byte{}
	}
	got, err := golang.DeclarationContext(source, golang.Position{
		Offset: declRange.Start.Offset,
		Line:   declRange.Start.Line,
		Column: declRange.Start.Column,
	}, golang.Position{
		Offset: declRange.End.Offset,
		Line:   declRange.End.Line,
		Column: declRange.End.Column,
	}, golang.Limits{
		Lines:      MaxContextLines,
		ExtraBytes: MaxContextExtraBytes,
	})
	if err != nil {
		return Snippet{}, declarationContextFailure(logicalPath, err)
	}
	return Snippet{
		Range: Range{
			Start: Position{Offset: got.Start.Offset, Line: got.Start.Line, Column: got.Start.Column},
			End:   Position{Offset: got.End.Offset, Line: got.End.Line, Column: got.End.Column},
		},
		Text: got.Text,
	}, nil
}

func declarationContextFailure(logicalPath string, err error) error {
	switch {
	case errors.Is(err, golang.ErrMalformedDeclaration):
		return fmt.Errorf("%s: %w", logicalPath, ErrMalformedDeclaration)
	case errors.Is(err, golang.ErrDeclarationNotFound):
		return fmt.Errorf("%s: %w", logicalPath, ErrDeclarationNotFound)
	case errors.Is(err, golang.ErrAmbiguousDeclaration):
		return fmt.Errorf("%s: %w", logicalPath, ErrAmbiguousDeclaration)
	default:
		return parseFailure(logicalPath, err)
	}
}

func parseGo(logicalPath string, source []byte) (*Document, error) {
	if source == nil {
		source = []byte{}
	}
	comments, decls, err := golang.ParseFile(source)
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
	declared := make([]Declaration, len(decls))
	for i, d := range decls {
		names := d.Names
		if names == nil {
			names = []string{}
		} else {
			names = append([]string(nil), d.Names...)
		}
		declared[i] = Declaration{
			Kind:  Kind(d.Kind),
			Names: names,
			Range: Range{
				Start: Position{Offset: d.Start.Offset, Line: d.Start.Line, Column: d.Start.Column},
				End:   Position{Offset: d.End.Offset, Line: d.End.Line, Column: d.End.Column},
			},
			HasDoc: d.HasDoc,
			Doc: Range{
				Start: Position{Offset: d.DocStart.Offset, Line: d.DocStart.Line, Column: d.DocStart.Column},
				End:   Position{Offset: d.DocEnd.Offset, Line: d.DocEnd.Line, Column: d.DocEnd.Column},
			},
		}
	}
	return &Document{Path: logicalPath, Language: Go, Comments: out, Declarations: declared}, nil
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
