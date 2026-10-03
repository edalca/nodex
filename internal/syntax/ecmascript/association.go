package ecmascript

import (
	"bytes"
	"sort"
	"strings"
	"unicode"

	"github.com/edalca/nodex/internal/syntax/types"
)

// leadingDocs verifies only the CST-bounded trivia interval. A line break
// inside a trailing block cannot make it leading; the break must be in the
// whitespace between syntax/comments. Ordinary comments do not break the run.
func (a *adapter) leadingDocs(previous, start int) []types.Range {
	docs := []types.Range{}
	cursor := previous
	leading := previous == 0
	first := sort.Search(len(a.comments), func(i int) bool { return a.comments[i].Start.Offset >= previous })
	for _, c := range a.comments[first:] {
		if c.Start.Offset >= start {
			break
		}
		if c.End.Offset > start || c.Start.Offset < cursor {
			return []types.Range{}
		}
		gap := a.source[cursor:c.Start.Offset]
		if !onlyWhitespace(gap) {
			return []types.Range{}
		}
		if bytes.ContainsAny(gap, "\r\n\u2028\u2029") {
			leading = true
		}
		if leading && strings.HasPrefix(c.Raw, "/**") && c.Raw != "/**/" {
			docs = append(docs, types.Range{Start: c.Start, End: c.End})
		}
		cursor = c.End.Offset
	}
	if !onlyWhitespace(a.source[cursor:start]) {
		return []types.Range{}
	}
	return docs
}
func onlyWhitespace(b []byte) bool {
	return len(bytes.TrimFunc(b, func(r rune) bool { return unicode.IsSpace(r) || r == '\ufeff' })) == 0
}
