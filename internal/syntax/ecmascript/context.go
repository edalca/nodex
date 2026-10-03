package ecmascript

import (
	"unicode/utf8"

	"github.com/edalca/nodex/internal/syntax/contracts"
	"github.com/edalca/nodex/internal/syntax/types"
)

func validRange(source []byte, lines []int, r types.Range) bool {
	return r.Start.Offset >= 0 && r.End.Offset >= r.Start.Offset && r.End.Offset <= len(source) && position(lines, r.Start.Offset) == r.Start && position(lines, r.End.Offset) == r.End
}

// Context locates an exact physical comment and bounds its associated,
// containing, trailing, or whole-file source neighborhood.
func (l language) Context(source []byte, target types.Range, limits contracts.Limits) (types.Snippet, error) {
	lines := sourceLines(source)
	if !validRange(source, lines, target) {
		return types.Snippet{}, contracts.ErrMalformedRange
	}
	doc, err := l.Parse(source)
	if err != nil {
		return types.Snippet{}, err
	}
	matches := 0
	for _, c := range doc.Comments {
		if c.Start == target.Start && c.End == target.End {
			matches++
		}
	}
	if matches == 0 {
		return types.Snippet{}, contracts.ErrCommentNotFound
	}
	if matches > 1 {
		return types.Snippet{}, contracts.ErrAmbiguousComment
	}
	anchor := span{target.Start.Offset, target.End.Offset}
	container := span{0, len(source)}
	found := false
	// A direct relationship takes precedence over containment or trailing context.
	for _, d := range doc.Declarations {
		for _, r := range d.Docs {
			if r == target {
				s := span{min(d.Start.Offset, anchor.start), max(d.End.Offset, anchor.end)}
				if !found || s.end-s.start < container.end-container.start {
					container = s
					found = true
				}
			}
		}
	}
	if !found {
		for _, d := range doc.Declarations {
			if d.Start.Offset <= anchor.start && d.End.Offset >= anchor.end {
				s := span{d.Start.Offset, d.End.Offset}
				if !found || s.end-s.start < container.end-container.start {
					container = s
					found = true
				}
			}
		}
	}
	if !found {
		end := -1
		for _, d := range doc.Declarations {
			if d.End.Offset > 0 && d.End.Offset <= anchor.start && position(lines, d.End.Offset-1).Line == target.Start.Line && d.End.Offset > end {
				container = span{d.Start.Offset, anchor.end}
				end = d.End.Offset
			}
		}
	}
	s := bound(source, lines, container, anchor, limits)
	return snippet(source, lines, s), nil
}

// DeclarationContext returns a bounded prefix of an exact safe declaration.
// Its first physical line is the exempt identifying anchor.
func (l language) DeclarationContext(source []byte, target types.Range, limits contracts.Limits) (types.Snippet, error) {
	lines := sourceLines(source)
	if !validRange(source, lines, target) {
		return types.Snippet{}, contracts.ErrMalformedDeclaration
	}
	doc, err := l.Parse(source)
	if err != nil {
		return types.Snippet{}, err
	}
	matches := 0
	for _, d := range doc.Declarations {
		if d.Start == target.Start && d.End == target.End {
			matches++
		}
	}
	if matches == 0 {
		return types.Snippet{}, contracts.ErrDeclarationNotFound
	}
	if matches > 1 {
		return types.Snippet{}, contracts.ErrAmbiguousDeclaration
	}
	container := span{target.Start.Offset, target.End.Offset}
	anchor := span{container.start, min(lineEnd(source, container.start), container.end)}
	return snippet(source, lines, bound(source, lines, container, anchor, limits)), nil
}

func snippet(source []byte, lines []int, s span) types.Snippet {
	return types.Snippet{Start: position(lines, s.start), End: position(lines, s.end), Text: string(source[s.start:s.end])}
}
func lineEnd(source []byte, start int) int {
	for start < len(source) {
		start++
		if source[start-1] == '\n' {
			break
		}
	}
	return start
}
func lastLine(lines []int, s span) int {
	if s.end > s.start {
		return position(lines, s.end-1).Line
	}
	return position(lines, s.start).Line
}

// bound shares the neutral line/extra-byte semantics with the Go adapter.
// The anchor's own bytes are exempt; excess anchor lines retain its beginning.
// Spare capacity is balanced around a comment and goes forward for declarations.
func bound(source []byte, lines []int, container, anchor span, limits contracts.Limits) span {
	maxLines, extra := max(limits.Lines, 1), max(limits.ExtraBytes, 0)
	contFirst, contLast := position(lines, container.start).Line, lastLine(lines, container)
	if contLast-contFirst+1 <= maxLines && anchor.start-container.start+container.end-anchor.end <= extra {
		return container
	}
	first, last := position(lines, anchor.start).Line, lastLine(lines, anchor)
	if last-first+1 >= maxLines {
		last = min(contLast, first+maxLines-1)
	} else {
		before, after := split(maxLines-(last-first+1), first-contFirst, contLast-last)
		first -= before
		last += after
	}
	start, end := max(lines[first-1], container.start), container.end
	if last < len(lines) {
		end = min(end, lines[last])
	}
	start = min(start, anchor.start)
	clippedEnd := min(anchor.end, end)
	before, after := split(extra, anchor.start-start, end-clippedEnd)
	start = anchor.start - before
	end = clippedEnd + after
	for start < anchor.start && start < len(source) && !utf8.RuneStart(source[start]) {
		start++
	}
	for end > clippedEnd && end < len(source) && !utf8.RuneStart(source[end]) {
		end--
	}
	// Do not cut a CRLF pair when shrinking outside the anchored bytes.
	if start < anchor.start && start > 0 && source[start] == '\n' && source[start-1] == '\r' {
		start++
	}
	if end > clippedEnd && end < len(source) && end > 0 && source[end] == '\n' && source[end-1] == '\r' {
		end--
	}
	return span{start, end}
}
func split(budget, beforeAvail, afterAvail int) (int, int) {
	budget = min(budget, beforeAvail+afterAvail)
	before := min((budget+1)/2, beforeAvail)
	after := min(budget-before, afterAvail)
	before = min(budget-after, beforeAvail)
	return before, after
}
