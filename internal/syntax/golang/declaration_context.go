package golang

import (
	"go/ast"
	"go/token"
)

// DeclarationContext returns the bounded structural source of the declaration
// at [start, end).
//
// src is the complete file. DeclarationContext does not open a path and does
// not walk a directory. start and end must be the physical range of exactly
// one parsed declaration. The declaration is identified by that range.
// Names and comment text are not keys. A source that does not parse returns
// *Error and no snippet.
//
// The snippet is a contiguous prefix of that declaration, copied from src.
// When the declaration is at most limits.Lines physical lines and the bytes
// after its first physical line are at most limits.ExtraBytes, the snippet
// is the whole declaration. Otherwise the snippet keeps the declaration’s
// first physical line and as much of the following declaration text as those
// limits allow. The first line is the identifying portion and is kept even
// when the rest of the declaration is omitted. The slice is not reformatted
// and it contains no inserted ellipsis.
func DeclarationContext(src []byte, start, end Position, limits Limits) (Snippet, error) {
	if src == nil {
		src = []byte{}
	}
	if err := rangeShape(src, start, end); err != nil {
		return Snippet{}, ErrMalformedDeclaration
	}
	src, tf, file, err := parseSource(src)
	if err != nil {
		return Snippet{}, err
	}
	if positionAt(tf, start.Offset) != start || positionAt(tf, end.Offset) != end {
		return Snippet{}, ErrMalformedDeclaration
	}
	decl, err := matchDeclaration(src, tf, file, start, end)
	if err != nil {
		return Snippet{}, err
	}
	container := byteSpan{start: decl.Start.Offset, end: decl.End.Offset}
	focusEnd := endOfLine(src, container.start)
	if focusEnd > container.end {
		focusEnd = container.end
	}
	from, to := boundSnippet(src, tf, container, byteSpan{start: container.start, end: focusEnd}, normalizeLimits(limits))
	if from < container.start || to > container.end || from > to {
		return Snippet{}, ErrMalformedDeclaration
	}
	return Snippet{
		Start: positionAt(tf, from),
		End:   positionAt(tf, to),
		Text:  string(src[from:to]),
	}, nil
}

func matchDeclaration(src []byte, tf *token.File, file *ast.File, start, end Position) (Declaration, error) {
	decls, err := declarationsFrom(src, tf, file)
	if err != nil {
		return Declaration{}, err
	}
	var matched []Declaration
	for _, decl := range decls {
		if decl.Start == start && decl.End == end {
			matched = append(matched, decl)
		}
	}
	switch len(matched) {
	case 0:
		return Declaration{}, ErrDeclarationNotFound
	case 1:
		return matched[0], nil
	default:
		return Declaration{}, ErrAmbiguousDeclaration
	}
}
