// Package golang parses Go source for the syntax root.
// New supplies one complete language capability, including recognition,
// parsing, structural context, and the Go preset catalog and semantics.
//
// Parse uses the standard library parser and returns one comment for every
// comment group, in physical lexical order. A group is a run of comments
// with no other tokens and no blank line between them. Parse collects
// package comments, documentation, trailing comments, comments inside
// function bodies, line comments, block comments, and directives written
// as comments. Text inside strings is left alone.
//
// ParseFile returns those comments and the declaration facts from the same
// parse. A declaration fact uses only the node's own Doc relationship. Docs
// is an empty non-nil collection when Doc is nil, or contains exactly that
// comment group's physical range. Import declarations are not part of that
// fact list.
//
// Context uses the same parser to select a structural container around one
// comment group and returns a bounded slice of the caller-supplied bytes.
// The container is a declaration, the package clause, or the file. The
// slice is not a syntax tree and it is not reformatted.
//
// The caller supplies the source bytes. Parse does not open a path, walk a
// directory, type-check, resolve imports, or read build constraints. A file
// whose path would be excluded by project policy is still parsed when its
// bytes are valid Go. A syntax error returns no comments.
//
// Positions come from go/token with //line adjustment disabled. Offset is a
// zero-based byte index. Line and Column are one-based, and Column counts
// bytes. A line comment runs from its opening slash up to, and not including,
// the '\n' that ends the line. A block comment runs from its opening slash
// through the bytes of the closing star-slash. Those ends are taken from the
// original source. The AST end is not used to slice the source: carriage
// returns are removed from AST comment text, so that end is short of the
// physical end.
//
// Raw is the original bytes. For a group it is the contiguous slice from the
// first comment through the last, including the bytes between them.
//
// Text is the indexing form of the same group. Each comment loses its
// delimiter. A line comment also loses one leading ASCII space when a space
// is present, and it loses trailing carriage returns from a CRLF line ending.
// A block comment keeps its interior, with each CRLF pair written as LF.
// Directive text, including go:generate and line, is kept. Bodies stay in
// source order and are joined with '\n'. Blank bodies, trailing spaces, and
// repeated blank lines stay as they are.
package golang

import (
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/scanner"
	"go/token"
	"sort"
	"strings"

	"github.com/edalca/nodex/internal/syntax/contracts"
	"github.com/edalca/nodex/internal/syntax/types"
)

type language struct{}

var _ contracts.Language = language{}

// New returns the complete, immutable Go syntax implementation.
func New() contracts.Language { return language{} }

// ID returns the stable Go language identifier.
func (language) ID() string { return "go" }

// Recognize reports whether the logical path ends in the case-sensitive .go suffix.
func (language) Recognize(logicalPath string) bool { return isGoPath(logicalPath) }

// Parse returns comments and declarations from one Go parse as neutral facts.
func (language) Parse(source []byte) (*contracts.Document, error) {
	comments, declarations, err := ParseFile(source)
	if err != nil {
		return nil, languageFailure(err)
	}
	return &contracts.Document{Comments: comments, Declarations: declarations}, nil
}

// Context builds bounded original source around exactly one comment group.
func (language) Context(source []byte, target types.Range, limits contracts.Limits) (types.Snippet, error) {
	snippet, err := Context(source, target.Start, target.End, limits)
	return snippet, languageFailure(err)
}

// DeclarationContext builds a bounded prefix of exactly one Go declaration.
func (language) DeclarationContext(source []byte, target types.Range, limits contracts.Limits) (types.Snippet, error) {
	snippet, err := DeclarationContext(source, target.Start, target.End, limits)
	return snippet, languageFailure(err)
}

// Presets returns the concrete Go exclusions and their aggregate selector.
func (language) Presets() contracts.PresetCatalog {
	return contracts.PresetCatalog{
		Concrete:   ConcretePresets(),
		Aggregates: []contracts.Aggregate{{ID: SelectorAll, Presets: ConcretePresets()}},
	}
}

// PathExcluded applies enabled Go path presets without inspecting source.
func (language) PathExcluded(logicalPath string, enabled []string) bool {
	return PathExcluded(logicalPath, enabled)
}

// SourceExcluded applies enabled Go structural presets to the supplied bytes.
func (language) SourceExcluded(logicalPath string, source []byte, enabled []string) bool {
	return SourceExcluded(logicalPath, source, enabled)
}

func languageFailure(err error) error {
	var failure *Error
	if errors.As(err, &failure) {
		return &contracts.ParseError{Diagnostics: failure.Diagnostics}
	}
	return err
}

// parseMode collects comments and skips deprecated identifier resolution.
// Resolution is not structural comment extraction, and this package does not
// type-check.
const parseMode = parser.ParseComments | parser.SkipObjectResolution

// Error is a Go syntax failure. Parse returns no comments with an Error.
type Error struct {
	Diagnostics []types.Diagnostic
}

// Error returns the diagnostics using physical line and column numbers.
func (e *Error) Error() string {
	if e == nil || len(e.Diagnostics) == 0 {
		return "go syntax error"
	}
	parts := make([]string, len(e.Diagnostics))
	for i, d := range e.Diagnostics {
		if d.Position.Line > 0 {
			parts[i] = fmt.Sprintf("%d:%d: %s", d.Position.Line, d.Position.Column, d.Msg)
			continue
		}
		parts[i] = d.Msg
	}
	return strings.Join(parts, "\n")
}

// Parse interprets src as Go source.
//
// A nil src is empty input. On success the comment slice is non-nil and
// ordered by physical start offset. On failure the error is *Error and the
// slice is nil. Parse uses the same parse as ParseFile.
func Parse(src []byte) ([]types.Comment, error) {
	comments, _, err := ParseFile(src)
	if err != nil {
		return nil, err
	}
	return comments, nil
}

// parseSource parses src without reading a filesystem path.
//
// A nil src is empty input. The returned buffer is the buffer the parser
// saw. On success tf is the physical token file for that buffer.
func parseSource(src []byte) ([]byte, *token.File, *ast.File, error) {
	if src == nil {
		src = []byte{}
	}
	// The name is empty because the bytes are already in hand. ParseFile
	// reads the filesystem only when src is nil.
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "", src, parseMode)
	if err != nil {
		return src, nil, nil, parserFailure(tokenFile(fset, file), err)
	}
	tf := tokenFile(fset, file)
	if tf == nil {
		return src, nil, nil, &Error{Diagnostics: []types.Diagnostic{{Msg: "parsed file has no source position"}}}
	}
	return src, tf, file, nil
}

func tokenFile(fset *token.FileSet, file *ast.File) *token.File {
	if fset == nil {
		return nil
	}
	if file != nil {
		if tf := fset.File(file.FileStart); tf != nil {
			return tf
		}
	}
	var tf *token.File
	fset.Iterate(func(f *token.File) bool {
		tf = f
		return false
	})
	return tf
}

func parserFailure(tf *token.File, err error) error {
	var list scanner.ErrorList
	if !asErrorList(err, &list) {
		return &Error{Diagnostics: []types.Diagnostic{{Msg: err.Error()}}}
	}
	diags := make([]types.Diagnostic, len(list))
	for i, item := range list {
		diags[i] = types.Diagnostic{
			Position: positionAt(tf, item.Pos.Offset),
			Msg:      item.Msg,
		}
	}
	sort.SliceStable(diags, func(i, j int) bool {
		if diags[i].Position.Offset != diags[j].Position.Offset {
			return diags[i].Position.Offset < diags[j].Position.Offset
		}
		return diags[i].Msg < diags[j].Msg
	})
	return &Error{Diagnostics: diags}
}

// asErrorList matches scanner.ErrorList. The concrete error is a slice, so
// errors.As needs a pointer to that slice type.
func asErrorList(err error, list *scanner.ErrorList) bool {
	if err == nil {
		return false
	}
	got, ok := err.(scanner.ErrorList)
	if !ok {
		return false
	}
	*list = got
	return true
}

func positionAt(tf *token.File, offset int) types.Position {
	if tf == nil {
		return types.Position{Offset: offset}
	}
	// false keeps the physical line and column. The default Position method
	// applies //line directives and can report another file name.
	p := tf.PositionFor(tf.Pos(offset), false)
	return types.Position{Offset: p.Offset, Line: p.Line, Column: p.Column}
}

func commentFromGroup(src []byte, tf *token.File, group *ast.CommentGroup) (types.Comment, error) {
	if group == nil || len(group.List) == 0 {
		return types.Comment{}, &Error{Diagnostics: []types.Diagnostic{{Msg: "empty comment group"}}}
	}
	parts := make([]string, 0, len(group.List))
	firstStart := -1
	lastEnd := -1
	for _, c := range group.List {
		if c == nil {
			return types.Comment{}, &Error{Diagnostics: []types.Diagnostic{{Msg: "empty comment"}}}
		}
		start := 0
		if tf != nil {
			start = tf.Offset(c.Slash)
		}
		end, err := commentEnd(src, start)
		if err != nil {
			return types.Comment{}, err
		}
		if firstStart < 0 {
			firstStart = start
		}
		lastEnd = end
		text, err := commentText(src[start:end])
		if err != nil {
			return types.Comment{}, err
		}
		parts = append(parts, text)
	}
	if firstStart < 0 || lastEnd < firstStart || lastEnd > len(src) {
		return types.Comment{}, &Error{Diagnostics: []types.Diagnostic{{Msg: "comment range is outside the source"}}}
	}
	return types.Comment{
		Raw:   string(src[firstStart:lastEnd]),
		Text:  strings.Join(parts, "\n"),
		Start: positionAt(tf, firstStart),
		End:   positionAt(tf, lastEnd),
	}, nil
}

// commentEnd returns the byte offset immediately after the comment that
// starts at start. The offset is found in src. It is not ast.Comment.End,
// whose length omits carriage returns that were present in src.
func commentEnd(src []byte, start int) (int, error) {
	if start < 0 || start+1 >= len(src) || src[start] != '/' {
		return 0, &Error{Diagnostics: []types.Diagnostic{{
			Position: types.Position{Offset: start},
			Msg:      "comment does not start at a slash",
		}}}
	}
	switch src[start+1] {
	case '/':
		i := start + 2
		for i < len(src) && src[i] != '\n' {
			i++
		}
		return i, nil
	case '*':
		for i := start + 2; i+1 < len(src); i++ {
			if src[i] == '*' && src[i+1] == '/' {
				return i + 2, nil
			}
		}
		return 0, &Error{Diagnostics: []types.Diagnostic{{
			Position: types.Position{Offset: start},
			Msg:      "unterminated block comment",
		}}}
	default:
		return 0, &Error{Diagnostics: []types.Diagnostic{{
			Position: types.Position{Offset: start},
			Msg:      "comment does not start at a slash",
		}}}
	}
}

func commentText(raw []byte) (string, error) {
	if len(raw) >= 2 && raw[0] == '/' && raw[1] == '/' {
		body := strings.TrimRight(string(raw[2:]), "\r")
		if len(body) > 0 && body[0] == ' ' {
			body = body[1:]
		}
		return body, nil
	}
	if len(raw) >= 4 && raw[0] == '/' && raw[1] == '*' && raw[len(raw)-2] == '*' && raw[len(raw)-1] == '/' {
		body := string(raw[2 : len(raw)-2])
		body = strings.ReplaceAll(body, "\r\n", "\n")
		return body, nil
	}
	return "", &Error{Diagnostics: []types.Diagnostic{{Msg: "comment bytes are not a Go comment"}}}
}
