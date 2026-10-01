package main

import (
	"strings"

	"github.com/edalca/nodex/internal/index"
)

// comments returns the compact Markdown form of the current generated index.
//
// The index is loaded through the index package. A missing index, a corrupt
// index, and a stale index are errors. Nothing is regenerated. An empty
// current index returns an empty string.
func comments(dir string) (string, error) {
	idx, _, _, err := openCurrentIndex(dir)
	if err != nil {
		return "", err
	}
	return formatComments(idx.Entries()), nil
}

// formatComments renders entries as compact Markdown.
//
// Each block is the comment ID heading and the normalized text, copied
// unchanged. A block that does not already end in a newline is terminated
// so the following comment is separated by one blank line. No entries
// produce an empty result.
func formatComments(entries []index.Entry) string {
	if len(entries) == 0 {
		return ""
	}
	var b strings.Builder
	for i, entry := range entries {
		if i > 0 {
			b.WriteByte('\n')
		}
		b.WriteString("## ")
		b.WriteString(entry.ID.String())
		b.WriteString("\n\n")
		b.WriteString(entry.Text)
		if !strings.HasSuffix(entry.Text, "\n") {
			b.WriteByte('\n')
		}
	}
	return b.String()
}
