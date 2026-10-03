package ecmascript

import "strings"

// Normalization removes only delimiters and one ASCII space after a line
// delimiter. Block interiors and JSDoc tags stay intact; CRLF becomes LF in
// indexing text while Raw always keeps the original bytes.
func normalizeComment(raw string) string {
	switch {
	case strings.HasPrefix(raw, "//"), strings.HasPrefix(raw, "#!"):
		return strings.TrimPrefix(raw[2:], " ")
	case strings.HasPrefix(raw, "/*") && strings.HasSuffix(raw, "*/"):
		return strings.ReplaceAll(raw[2:len(raw)-2], "\r\n", "\n")
	default:
		return raw
	}
}
