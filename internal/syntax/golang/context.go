package golang

import (
	"errors"
	"go/ast"
	"go/token"
	"unicode/utf8"
)

var (
	// ErrMalformedRange means the supplied range is not a coherent physical
	// range of the source bytes.
	ErrMalformedRange = errors.New("comment range is malformed")

	// ErrCommentNotFound means the range is not exactly one comment group.
	ErrCommentNotFound = errors.New("comment range does not match a comment")

	// ErrAmbiguousComment means the range matches more than one comment group.
	ErrAmbiguousComment = errors.New("comment range matches more than one comment")
)

// Limits bound a structural context snippet.
//
// Lines is the maximum number of physical source lines. ExtraBytes is the
// maximum number of source bytes placed outside the comment. The comment's
// own bytes are not counted against ExtraBytes. A non-positive Lines value
// is treated as one line. A negative ExtraBytes value is treated as zero.
type Limits struct {
	Lines      int
	ExtraBytes int
}

// Snippet is a physical slice of the source passed to Context.
//
// Text is src[Start.Offset:End.Offset]. Start and End use physical
// coordinates, with //line adjustment disabled.
type Snippet struct {
	Start Position
	End   Position
	Text  string
}

// Context returns the bounded structural source around the comment at
// [start, end).
//
// src is the complete file. Context does not open a path and does not walk
// a directory. start and end must be the physical range of exactly one
// parsed comment group. The group is identified by that range. Comment text
// is not a key. A source that does not parse returns *Error and no snippet.
//
// The container is chosen from the parsed file:
//
//   - Package documentation, and any other comment on the package-clause
//     line, uses the package comment when one exists and the package clause
//     through the end of that physical line.
//   - A comment recorded on a function, method, general declaration, spec,
//     or field uses the enclosing function, method, or general declaration.
//     The span grows to include the comment when the comment sits outside
//     the declaration's own token extent.
//   - A comment physically inside a function, method, or general declaration
//     uses the innermost of those declarations.
//   - A comment that begins on the same physical line, after a declaration
//     ends, uses the declaration that ends nearest the comment.
//   - Any other comment uses the whole file as its neighborhood.
//
// When the container is at most limits.Lines physical lines and adds at most
// limits.ExtraBytes outside the comment, the snippet is the whole container.
// Otherwise the snippet is a contiguous slice of the container around the
// comment: at most limits.Lines lines, with at most limits.ExtraBytes added
// outside the comment. Spare room is split before and after the comment.
// One extra unit of an odd budget is placed before the comment. Room that
// one side cannot use is given to the other side. The slice stays inside
// the container. A comment that itself occupies more than limits.Lines
// keeps its starting line and the following lines that fit; the snippet
// still contains the comment's starting byte. The slice is copied from src
// and contains no inserted ellipsis.
func Context(src []byte, start, end Position, limits Limits) (Snippet, error) {
	if src == nil {
		src = []byte{}
	}
	if err := rangeShape(src, start, end); err != nil {
		return Snippet{}, err
	}
	src, tf, file, err := parseSource(src)
	if err != nil {
		return Snippet{}, err
	}
	if positionAt(tf, start.Offset) != start || positionAt(tf, end.Offset) != end {
		return Snippet{}, ErrMalformedRange
	}
	group, err := matchGroup(src, tf, file, start, end)
	if err != nil {
		return Snippet{}, err
	}
	comment, err := commentFromGroup(src, tf, group)
	if err != nil {
		return Snippet{}, err
	}
	container := containerOf(src, tf, file, group, comment.Start.Offset, comment.End.Offset)
	from, to := boundSnippet(src, tf, container, byteSpan{comment.Start.Offset, comment.End.Offset}, normalizeLimits(limits))
	if from < 0 || to > len(src) || from > to {
		return Snippet{}, ErrMalformedRange
	}
	return Snippet{
		Start: positionAt(tf, from),
		End:   positionAt(tf, to),
		Text:  string(src[from:to]),
	}, nil
}

func rangeShape(src []byte, start, end Position) error {
	if start.Offset < 0 || end.Offset < 0 || end.Offset < start.Offset {
		return ErrMalformedRange
	}
	if start.Line < 1 || end.Line < 1 || start.Column < 1 || end.Column < 1 {
		return ErrMalformedRange
	}
	if start.Offset > len(src) || end.Offset > len(src) {
		return ErrMalformedRange
	}
	return nil
}

func matchGroup(src []byte, tf *token.File, file *ast.File, start, end Position) (*ast.CommentGroup, error) {
	var matched []*ast.CommentGroup
	for _, group := range file.Comments {
		got, err := commentFromGroup(src, tf, group)
		if err != nil {
			return nil, err
		}
		if got.Start == start && got.End == end {
			matched = append(matched, group)
		}
	}
	switch len(matched) {
	case 0:
		return nil, ErrCommentNotFound
	case 1:
		return matched[0], nil
	default:
		return nil, ErrAmbiguousComment
	}
}

type byteSpan struct {
	start int
	end   int
}

func (s byteSpan) extend(cStart, cEnd int) byteSpan {
	if cStart < s.start {
		s.start = cStart
	}
	if cEnd > s.end {
		s.end = cEnd
	}
	return s
}

func (s byteSpan) size() int {
	if s.end < s.start {
		return 0
	}
	return s.end - s.start
}

func containerOf(src []byte, tf *token.File, file *ast.File, group *ast.CommentGroup, cStart, cEnd int) byteSpan {
	if file.Doc == group {
		return clampSpan(packageSpan(src, tf, file, cStart, cEnd), len(src))
	}

	var decls []byteSpan
	var associated *byteSpan
	remember := func(span byteSpan) {
		span = clampSpan(span.extend(cStart, cEnd), len(src))
		if associated == nil || span.size() < associated.size() {
			associated = &span
		}
	}

	ast.Inspect(file, func(n ast.Node) bool {
		switch node := n.(type) {
		case *ast.FuncDecl:
			span := nodeSpan(tf, node.Pos(), node.End())
			decls = append(decls, span)
			if node.Doc == group {
				remember(span)
			}
		case *ast.GenDecl:
			span := nodeSpan(tf, node.Pos(), node.End())
			decls = append(decls, span)
			if node.Doc == group || specsMention(node, group) {
				remember(span)
			}
		case *ast.Field:
			if node.Doc != group && node.Comment != group {
				return true
			}
			if span, ok := innermostContaining(decls, offsetOf(tf, node.Pos())); ok {
				remember(span)
			}
		}
		return true
	})
	if associated != nil {
		return *associated
	}
	if span, ok := innermostContaining(decls, cStart); ok {
		return clampSpan(span.extend(cStart, cEnd), len(src))
	}
	if span, ok := trailingDecl(tf, decls, cStart); ok {
		return clampSpan(span.extend(cStart, cEnd), len(src))
	}
	if onPackageLine(tf, file, cStart) {
		return clampSpan(packageSpan(src, tf, file, cStart, cEnd), len(src))
	}
	return byteSpan{start: 0, end: len(src)}
}

func specsMention(decl *ast.GenDecl, group *ast.CommentGroup) bool {
	for _, spec := range decl.Specs {
		switch s := spec.(type) {
		case *ast.ValueSpec:
			if s.Doc == group || s.Comment == group {
				return true
			}
		case *ast.TypeSpec:
			if s.Doc == group || s.Comment == group {
				return true
			}
		case *ast.ImportSpec:
			if s.Doc == group || s.Comment == group {
				return true
			}
		}
	}
	return false
}

func nodeSpan(tf *token.File, pos, end token.Pos) byteSpan {
	return byteSpan{start: offsetOf(tf, pos), end: offsetOf(tf, end)}
}

func offsetOf(tf *token.File, pos token.Pos) int {
	if tf == nil || !pos.IsValid() {
		return 0
	}
	return tf.Offset(pos)
}

func innermostContaining(decls []byteSpan, offset int) (byteSpan, bool) {
	found := false
	var best byteSpan
	for _, decl := range decls {
		if offset < decl.start || offset >= decl.end {
			continue
		}
		if !found || decl.size() < best.size() || (decl.size() == best.size() && decl.start > best.start) {
			best = decl
			found = true
		}
	}
	return best, found
}

func trailingDecl(tf *token.File, decls []byteSpan, cStart int) (byteSpan, bool) {
	if cStart <= 0 {
		return byteSpan{}, false
	}
	cLine := lineOf(tf, cStart)
	found := false
	var best byteSpan
	for _, decl := range decls {
		if decl.end <= 0 || cStart < decl.end {
			continue
		}
		if lineOf(tf, decl.end-1) != cLine {
			continue
		}
		if !found || decl.end > best.end {
			best = decl
			found = true
		}
	}
	return best, found
}

func onPackageLine(tf *token.File, file *ast.File, cStart int) bool {
	if file == nil || !file.Package.IsValid() {
		return false
	}
	return lineOf(tf, cStart) == lineOf(tf, tf.Offset(file.Package))
}

func packageSpan(src []byte, tf *token.File, file *ast.File, cStart, cEnd int) byteSpan {
	start := tf.Offset(file.Package)
	if file.Doc != nil && file.Doc.Pos().IsValid() {
		docStart := tf.Offset(file.Doc.Pos())
		if docStart < start {
			start = docStart
		}
	}
	end := start
	if file.Name != nil && file.Name.End().IsValid() {
		end = endOfLine(src, tf.Offset(file.Name.End()))
	}
	return (byteSpan{start: start, end: end}).extend(cStart, cEnd)
}

func endOfLine(src []byte, offset int) int {
	if offset < 0 {
		offset = 0
	}
	i := offset
	for i < len(src) && src[i] != '\n' {
		i++
	}
	if i < len(src) && src[i] == '\n' {
		i++
	}
	return i
}

func clampSpan(span byteSpan, size int) byteSpan {
	if span.start < 0 {
		span.start = 0
	}
	if span.end < span.start {
		span.end = span.start
	}
	if span.end > size {
		span.end = size
	}
	if span.start > span.end {
		span.start = span.end
	}
	return span
}

type normalizedLimits struct {
	lines      int
	extraBytes int
}

func normalizeLimits(limits Limits) normalizedLimits {
	lines := limits.Lines
	if lines < 1 {
		lines = 1
	}
	extra := limits.ExtraBytes
	if extra < 0 {
		extra = 0
	}
	return normalizedLimits{lines: lines, extraBytes: extra}
}

func boundSnippet(src []byte, tf *token.File, container, comment byteSpan, limits normalizedLimits) (int, int) {
	container = clampSpan(container.extend(comment.start, comment.end), len(src))
	comment = clampSpan(comment, len(src))
	if container.start > comment.start {
		container.start = comment.start
	}
	if container.end < comment.end {
		container.end = comment.end
	}
	if fits(tf, container, comment, limits) {
		return container.start, container.end
	}

	contFirst := lineOf(tf, container.start)
	contLast := lastLine(tf, container.start, container.end)
	commentFirst := lineOf(tf, comment.start)
	commentLast := lastLine(tf, comment.start, comment.end)
	winFirst, winLast := chooseWindow(contFirst, contLast, commentFirst, commentLast, limits.lines)
	start, end := lineBounds(tf, winFirst, winLast, len(src))
	if start < container.start {
		start = container.start
	}
	if end > container.end {
		end = container.end
	}
	if start > comment.start {
		start = comment.start
	}
	commentLines := commentLast - commentFirst + 1
	if commentLines <= limits.lines && end < comment.end {
		end = comment.end
		if end > container.end {
			end = container.end
		}
	}
	start, end = shrinkBytes(start, end, comment.start, comment.end, limits.extraBytes)
	anchorEnd := comment.end
	if anchorEnd > end {
		anchorEnd = end
	}
	start, end = snapRunes(src, start, end, comment.start, anchorEnd)
	start, end = clampPair(start, end, len(src))
	if start > comment.start {
		start = comment.start
	}
	if end < start {
		end = start
	}
	return start, end
}

func fits(tf *token.File, container, comment byteSpan, limits normalizedLimits) bool {
	if container.end <= container.start {
		return true
	}
	lines := lastLine(tf, container.start, container.end) - lineOf(tf, container.start) + 1
	extra := 0
	if comment.start > container.start {
		extra += comment.start - container.start
	}
	if container.end > comment.end {
		extra += container.end - comment.end
	}
	return lines <= limits.lines && extra <= limits.extraBytes
}

func chooseWindow(contFirst, contLast, commentFirst, commentLast, maxLines int) (int, int) {
	if commentFirst < contFirst {
		commentFirst = contFirst
	}
	if commentLast > contLast {
		commentLast = contLast
	}
	if commentLast < commentFirst {
		commentLast = commentFirst
	}
	commentLines := commentLast - commentFirst + 1
	if commentLines >= maxLines {
		last := commentFirst + maxLines - 1
		if last > contLast {
			last = contLast
		}
		return commentFirst, last
	}
	before, after := splitBudget(maxLines-commentLines, commentFirst-contFirst, contLast-commentLast)
	return commentFirst - before, commentLast + after
}

func splitBudget(budget, beforeAvail, afterAvail int) (int, int) {
	if budget <= 0 {
		return 0, 0
	}
	if beforeAvail < 0 {
		beforeAvail = 0
	}
	if afterAvail < 0 {
		afterAvail = 0
	}
	if budget > beforeAvail+afterAvail {
		budget = beforeAvail + afterAvail
	}
	before := (budget + 1) / 2
	if before > beforeAvail {
		before = beforeAvail
	}
	after := budget - before
	if after > afterAvail {
		after = afterAvail
		before = budget - after
		if before > beforeAvail {
			before = beforeAvail
		}
	}
	return before, after
}

func shrinkBytes(start, end, commentStart, commentEnd, maxExtra int) (int, int) {
	if commentStart < start {
		commentStart = start
	}
	if commentEnd > end {
		commentEnd = end
	}
	if commentEnd < commentStart {
		commentEnd = commentStart
	}
	beforeAvail := commentStart - start
	afterAvail := end - commentEnd
	if beforeAvail+afterAvail <= maxExtra {
		return start, end
	}
	before, after := splitBudget(maxExtra, beforeAvail, afterAvail)
	return commentStart - before, commentEnd + after
}

func snapRunes(src []byte, start, end, anchorStart, anchorEnd int) (int, int) {
	if start < anchorStart {
		for start < anchorStart && start < len(src) && !utf8.RuneStart(src[start]) {
			start++
		}
	}
	for end > anchorEnd && end < len(src) && !utf8.RuneStart(src[end]) {
		end--
	}
	return start, end
}

func lineBounds(tf *token.File, first, last, size int) (int, int) {
	if first < 1 {
		first = 1
	}
	if last < first {
		last = first
	}
	count := tf.LineCount()
	if count < 1 {
		return 0, size
	}
	if first > count {
		first = count
	}
	if last > count {
		last = count
	}
	start := tf.Offset(tf.LineStart(first))
	end := size
	if last < count {
		end = tf.Offset(tf.LineStart(last + 1))
	}
	return start, end
}

func lineOf(tf *token.File, offset int) int {
	return positionAt(tf, offset).Line
}

func lastLine(tf *token.File, start, end int) int {
	if end <= start {
		return lineOf(tf, start)
	}
	return lineOf(tf, end-1)
}

func clampPair(start, end, size int) (int, int) {
	if start < 0 {
		start = 0
	}
	if end > size {
		end = size
	}
	if end < start {
		end = start
	}
	return start, end
}
