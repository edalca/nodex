package ignore

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

// rule is one compiled exclusion or negation.
type rule struct {
	negate  bool
	dirOnly bool
	segs    []segment
}

// segment is one path element of a compiled pattern.
// A double-star segment matches a run of elements. Every other segment is a
// glob over exactly one element.
type segment struct {
	doubleStar bool
	parts      []globPart
}

// globPart is one token of a single-element glob.
// Exactly one of star, any, lit, or class is set.
type globPart struct {
	star  bool
	any   bool
	lit   string
	class *charClass
}

// charClass matches exactly one rune.
type charClass struct {
	negate bool
	items  []classItem
}

// classItem is a single rune when lo == hi, otherwise an inclusive range.
type classItem struct {
	lo, hi rune
}

// compilePatterns compiles every pattern. The first failure rejects the set.
func compilePatterns(patterns []string) ([]rule, error) {
	if len(patterns) == 0 {
		return nil, nil
	}
	rules := make([]rule, 0, len(patterns))
	for i, pattern := range patterns {
		compiled, err := compileRule(pattern)
		if err != nil {
			return nil, &RuleError{Index: i, Pattern: pattern, Err: err}
		}
		rules = append(rules, compiled)
	}
	return rules, nil
}

// compileRule compiles one exclusion pattern.
//
// A leading "!" is negation. A trailing "/" that is not escaped makes the
// rule directory-only and is removed before the pattern is split. A leading
// "/" anchors the pattern at the project root; any remaining "/" does too.
// A pattern with no "/" matches at any depth, which is a leading "**".
// "/" separates elements before escapes are read, so a separator cannot be
// escaped. "**" is recursive only as a whole element. A final "**" matches
// inside a directory and not the directory itself, by requiring one more
// element. "*" bytes inside one element collapse to one "*".
func compileRule(pattern string) (rule, error) {
	var compiled rule
	body := pattern
	if strings.HasPrefix(body, "!") {
		compiled.negate = true
		body = body[1:]
	}
	if body == "" {
		return rule{}, fmt.Errorf("pattern is empty")
	}
	if unescapedTrailingSlash(body) {
		compiled.dirOnly = true
		body = strings.TrimSuffix(body, "/")
	}
	if body == "" {
		return rule{}, fmt.Errorf("pattern is empty")
	}

	anchored := false
	if strings.HasPrefix(body, "/") {
		anchored = true
		body = body[1:]
	} else if strings.Contains(body, "/") {
		anchored = true
	}
	if body == "" {
		return rule{}, fmt.Errorf("pattern is empty")
	}

	elements, err := splitPattern(body)
	if err != nil {
		return rule{}, err
	}
	segs := make([]segment, 0, len(elements)+2)
	if !anchored {
		segs = append(segs, segment{doubleStar: true})
	}
	for _, element := range elements {
		switch element {
		case "":
			return rule{}, fmt.Errorf("pattern has an empty element")
		case ".", "..":
			return rule{}, fmt.Errorf("pattern element %q is not allowed", element)
		case "**":
			segs = append(segs, segment{doubleStar: true})
		default:
			parts, err := compileGlob(element)
			if err != nil {
				return rule{}, err
			}
			segs = append(segs, segment{parts: parts})
		}
	}
	if len(segs) == 0 {
		return rule{}, fmt.Errorf("pattern is empty")
	}
	if segs[len(segs)-1].doubleStar {
		segs = append(segs, segment{parts: []globPart{{star: true}}})
	}
	compiled.segs = segs
	return compiled, nil
}

// unescapedTrailingSlash reports whether body ends in a separator that is
// not escaped. A backslash escapes the next byte, so an odd run of
// backslashes immediately before the final slash escapes that slash.
func unescapedTrailingSlash(body string) bool {
	if !strings.HasSuffix(body, "/") {
		return false
	}
	escapes := 0
	for i := len(body) - 2; i >= 0 && body[i] == '\\'; i-- {
		escapes++
	}
	return escapes%2 == 0
}

// splitPattern divides body into path elements.
//
// An unescaped "/" outside a character class is a separator. An unescaped
// "/" inside a class is rejected. An escaped "/" stays in its element so the
// glob compiler can treat it as a literal rune. A trailing backslash is kept
// for that compiler to reject as a dangling escape.
func splitPattern(body string) ([]string, error) {
	var elements []string
	var current strings.Builder
	inClass := false
	escaped := false
	for i := 0; i < len(body); i++ {
		c := body[i]
		if escaped {
			current.WriteByte('\\')
			current.WriteByte(c)
			escaped = false
			continue
		}
		if c == '\\' {
			escaped = true
			continue
		}
		if c == '[' && !inClass {
			inClass = true
			current.WriteByte(c)
			continue
		}
		if c == ']' && inClass {
			inClass = false
			current.WriteByte(c)
			continue
		}
		if c == '/' && inClass {
			return nil, fmt.Errorf("character class contains '/'")
		}
		if c == '/' {
			elements = append(elements, current.String())
			current.Reset()
			continue
		}
		current.WriteByte(c)
	}
	if escaped {
		current.WriteByte('\\')
	}
	elements = append(elements, current.String())
	return elements, nil
}

// compileGlob tokenizes one path element.
func compileGlob(element string) ([]globPart, error) {
	var parts []globPart
	var lit strings.Builder
	flush := func() {
		if lit.Len() == 0 {
			return
		}
		parts = append(parts, globPart{lit: lit.String()})
		lit.Reset()
	}
	for i := 0; i < len(element); i++ {
		switch element[i] {
		case '\\':
			if i+1 >= len(element) {
				return nil, fmt.Errorf("pattern ends with a dangling escape")
			}
			i++
			lit.WriteByte(element[i])
		case '*':
			flush()
			if len(parts) == 0 || !parts[len(parts)-1].star {
				parts = append(parts, globPart{star: true})
			}
		case '?':
			flush()
			parts = append(parts, globPart{any: true})
		case '[':
			flush()
			class, next, err := compileCharClass(element, i)
			if err != nil {
				return nil, err
			}
			parts = append(parts, globPart{class: class})
			i = next - 1
		default:
			lit.WriteByte(element[i])
		}
	}
	flush()
	if len(parts) == 0 {
		return nil, fmt.Errorf("pattern element is empty")
	}
	return parts, nil
}

// compileCharClass compiles the class that opens at element[start].
// The returned index is one past the closing bracket.
func compileCharClass(element string, start int) (*charClass, int, error) {
	i := start + 1
	class := &charClass{}
	if i < len(element) && element[i] == '!' {
		class.negate = true
		i++
	}
	for {
		if i >= len(element) {
			return nil, 0, fmt.Errorf("character class is not terminated")
		}
		if element[i] == ']' {
			if len(class.items) == 0 {
				return nil, 0, fmt.Errorf("character class is empty")
			}
			return class, i + 1, nil
		}
		lo, next, err := classRune(element, i)
		if err != nil {
			return nil, 0, err
		}
		i = next
		if i < len(element) && element[i] == '-' && i+1 < len(element) && element[i+1] != ']' {
			hi, after, err := classRune(element, i+1)
			if err != nil {
				return nil, 0, err
			}
			if hi < lo {
				return nil, 0, fmt.Errorf("character class range %q-%q is inverted", lo, hi)
			}
			class.items = append(class.items, classItem{lo: lo, hi: hi})
			i = after
			continue
		}
		class.items = append(class.items, classItem{lo: lo, hi: lo})
	}
}

// classRune reads one class member at element[i], honoring a backslash.
func classRune(element string, i int) (rune, int, error) {
	escaped := false
	if element[i] == '\\' {
		if i+1 >= len(element) {
			return 0, 0, fmt.Errorf("character class ends with a dangling escape")
		}
		escaped = true
		i++
	}
	r, size := utf8.DecodeRuneInString(element[i:])
	if r == utf8.RuneError && size <= 1 {
		return 0, 0, fmt.Errorf("character class contains invalid UTF-8")
	}
	if !escaped && r == '/' {
		return 0, 0, fmt.Errorf("character class contains '/'")
	}
	return r, i + size, nil
}

// matches reports whether the class accepts r.
func (c *charClass) matches(r rune) bool {
	included := false
	for _, item := range c.items {
		if r >= item.lo && r <= item.hi {
			included = true
			break
		}
	}
	return included != c.negate
}

// matches reports whether the rule matches the path elements.
func (r rule) matches(segs []string, isDir bool) bool {
	if r.dirOnly && !isDir {
		return false
	}
	reached := r.reachable(segs)
	return reached[len(r.segs)]
}

// reachable runs the segment automaton.
// State i means the elements consumed so far match segs[:i]. A double-star
// stays in its own state, consuming one element, and the closure also steps
// over it without consuming one.
func (r rule) reachable(segs []string) []bool {
	n := len(r.segs)
	cur := make([]bool, n+1)
	r.closure(cur, 0)
	for _, seg := range segs {
		next := make([]bool, n+1)
		for i := 0; i < n; i++ {
			if !cur[i] {
				continue
			}
			if r.segs[i].doubleStar {
				r.closure(next, i)
				continue
			}
			if matchGlob(r.segs[i].parts, seg) {
				r.closure(next, i+1)
			}
		}
		cur = next
	}
	return cur
}

// closure marks state i and every later double-star state that consumes no element.
func (r rule) closure(set []bool, i int) {
	for i <= len(r.segs) {
		if set[i] {
			return
		}
		set[i] = true
		if i < len(r.segs) && r.segs[i].doubleStar {
			i++
			continue
		}
		return
	}
}

// matchGlob reports whether one path element matches the glob parts.
// A star tries every byte split. A question mark or a class consumes one rune.
func matchGlob(parts []globPart, s string) bool {
	if len(parts) == 0 {
		return s == ""
	}
	part := parts[0]
	switch {
	case part.star:
		for i := 0; i <= len(s); i++ {
			if matchGlob(parts[1:], s[i:]) {
				return true
			}
		}
		return false
	case part.any:
		if s == "" {
			return false
		}
		_, size := utf8.DecodeRuneInString(s)
		return matchGlob(parts[1:], s[size:])
	case part.class != nil:
		if s == "" {
			return false
		}
		r, size := utf8.DecodeRuneInString(s)
		if !part.class.matches(r) {
			return false
		}
		return matchGlob(parts[1:], s[size:])
	default:
		if !strings.HasPrefix(s, part.lit) {
			return false
		}
		return matchGlob(parts[1:], s[len(part.lit):])
	}
}
