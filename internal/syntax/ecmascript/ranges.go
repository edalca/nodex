package ecmascript

import (
	"sort"

	"github.com/edalca/nodex/internal/syntax/types"
	"github.com/odvcencio/gotreesitter"
)

func sourceLines(source []byte) []int {
	lines := []int{0}
	for i, b := range source {
		if b == '\n' {
			lines = append(lines, i+1)
		}
	}
	return lines
}
func position(lines []int, offset int) types.Position {
	i := sort.Search(len(lines), func(i int) bool { return lines[i] > offset }) - 1
	return types.Position{Offset: offset, Line: i + 1, Column: offset - lines[i] + 1}
}
func (a *adapter) physical(s span) types.Range {
	return types.Range{Start: position(a.lines, s.start), End: position(a.lines, s.end)}
}

// observe includes anonymous children and extras. Reverse traversal computes
// terminal bounds once per node, excluding comment extras from declaration ends.
func (a *adapter) observe(root *gotreesitter.Node) {
	a.comments = []types.Comment{}
	stack := []*gotreesitter.Node{root}
	for len(stack) > 0 {
		n := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if n == nil {
			continue
		}
		a.nodes = append(a.nodes, n)
		k := a.kind(n)
		if n.IsError() || n.IsMissing() || (n.IsExtra() && k != "comment" && k != "hash_bang_line") {
			a.unsafe = true
		}
		start, end := int(n.StartByte()), int(n.EndByte())
		if start < 0 || end < start || end > len(a.source) {
			a.unsafe = true
			continue
		}
		if k == "comment" || k == "hash_bang_line" {
			if end > start {
				r := a.physical(span{start, end})
				raw := string(a.source[start:end])
				a.comments = append(a.comments, types.Comment{Raw: raw, Text: normalizeComment(raw), Start: r.Start, End: r.End})
			}
			continue
		}
		for i := n.ChildCount() - 1; i >= 0; i-- {
			stack = append(stack, n.Child(i))
		}
	}
	for i := len(a.nodes) - 1; i >= 0; i-- {
		n := a.nodes[i]
		k := a.kind(n)
		if k == "comment" || k == "hash_bang_line" {
			continue
		}
		if n.ChildCount() == 0 {
			if n.EndByte() > n.StartByte() && int(n.EndByte()) <= len(a.source) {
				a.bounds[n] = span{int(n.StartByte()), int(n.EndByte())}
			}
			continue
		}
		var s span
		found := false
		for j := 0; j < n.ChildCount(); j++ {
			if b, ok := a.bounds[n.Child(j)]; ok {
				if !found {
					s = b
					found = true
				} else {
					s.end = b.end
				}
			}
		}
		if found {
			a.bounds[n] = s
		}
	}
	sort.Slice(a.comments, func(i, j int) bool {
		x, y := a.comments[i], a.comments[j]
		if x.Start.Offset != y.Start.Offset {
			return x.Start.Offset < y.Start.Offset
		}
		return x.End.Offset < y.End.Offset
	})
	// A repeated CST occurrence is still one physical unit. Distinct blocks,
	// including blocks with identical text, keep their separate identities.
	unique := a.comments[:0]
	for _, c := range a.comments {
		if len(unique) == 0 || unique[len(unique)-1].Start != c.Start || unique[len(unique)-1].End != c.End {
			unique = append(unique, c)
		}
	}
	a.comments = unique
}
