package main

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/edalca/nodex/internal/index"
	"github.com/edalca/nodex/internal/project"
	"github.com/edalca/nodex/internal/syntax"
)

// showReadFile, when set, replaces Project.ReadFile while show is reading
// source for a current snapshot. Tests use it to observe a source change
// after currentness has already been decided. A nil hook reads through
// Project.ReadFile.
var showReadFile func(opened *project.Project, logical string) ([]byte, error)

// show resolves comment IDs from the current generated index.
//
// ids are the caller's arguments, in order, including duplicates. Every ID
// is parsed and resolved before any successful output is built. A missing
// index, a corrupt index, a stale index, a malformed ID, and an unknown ID
// are errors. Nothing is regenerated. Source is parsed only after the
// snapshot is current and every ID has been resolved. Each source file is
// read through the project, once per logical path, and its digest must still
// match the snapshot fingerprint.
func show(dir string, ids []string) (string, error) {
	if len(ids) == 0 {
		return "", fmt.Errorf("show requires a comment ID")
	}
	idx, snap, state, err := openCurrentIndex(dir)
	if err != nil {
		return "", err
	}
	entries := make([]index.Entry, len(ids))
	for i, text := range ids {
		id, err := index.ParseID(text)
		if err != nil {
			return "", err
		}
		entry, ok := idx.Lookup(id)
		if !ok {
			return "", fmt.Errorf("unknown comment ID %s", id)
		}
		entries[i] = entry
	}
	opened, err := project.Open(state.root)
	if err != nil {
		return "", err
	}
	bodies := make(map[string][]byte, len(entries))
	sections := make([]string, len(entries))
	for i, entry := range entries {
		body, err := cachedSource(opened, snap, bodies, entry.Path)
		if err != nil {
			return "", err
		}
		snippet, err := syntax.Context(entry.Path, body, entry.Range)
		if err != nil {
			return "", err
		}
		sections[i] = formatShow(entry, snippet)
	}
	return strings.Join(sections, "\n"), nil
}

func cachedSource(opened *project.Project, snap index.Snapshot, bodies map[string][]byte, logical string) ([]byte, error) {
	if body, ok := bodies[logical]; ok {
		return body, nil
	}
	body, err := readShowSource(opened, logical)
	if err != nil {
		return nil, err
	}
	digest, ok := fingerprintDigest(snap, logical)
	if !ok || index.DigestBytes(body) != digest {
		return nil, errStaleIndex
	}
	bodies[logical] = body
	return body, nil
}

func readShowSource(opened *project.Project, logical string) ([]byte, error) {
	if showReadFile != nil {
		return showReadFile(opened, logical)
	}
	return opened.ReadFile(logical)
}

func fingerprintDigest(snap index.Snapshot, logical string) (string, bool) {
	for _, src := range snap.Sources {
		if src.Path == logical {
			return src.Digest, true
		}
	}
	return "", false
}

func formatShow(entry index.Entry, snippet syntax.Snippet) string {
	var b strings.Builder
	b.WriteString("## ")
	b.WriteString(entry.ID.String())
	b.WriteString("\n\n")
	b.WriteString("file: `")
	b.WriteString(entry.Path)
	b.WriteString("`\n")
	b.WriteString("lines: ")
	b.WriteString(formatLines(entry.Range))
	b.WriteString("\n\n")
	b.WriteString("### Comment\n\n")
	b.WriteString(entry.Text)
	switch {
	case entry.Text == "":
	case strings.HasSuffix(entry.Text, "\n"):
		b.WriteByte('\n')
	default:
		b.WriteString("\n\n")
	}
	b.WriteString("### Context\n\n")
	b.WriteString(markdownFence(fenceInfo(entry.Language), snippet.Text))
	return b.String()
}

func formatLines(r syntax.Range) string {
	if r.Start.Line == r.End.Line {
		return strconv.Itoa(r.Start.Line)
	}
	return strconv.Itoa(r.Start.Line) + "-" + strconv.Itoa(r.End.Line)
}

func fenceInfo(lang syntax.Language) string {
	s := string(lang)
	if s == "" {
		return ""
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9':
		case c == '+' || c == '-' || c == '_' || c == '.':
		default:
			return ""
		}
	}
	return s
}

func markdownFence(info, body string) string {
	bar := strings.Repeat("`", fenceWidth(body))
	var b strings.Builder
	b.WriteString(bar)
	b.WriteString(info)
	b.WriteByte('\n')
	b.WriteString(body)
	if !strings.HasSuffix(body, "\n") {
		b.WriteByte('\n')
	}
	b.WriteString(bar)
	b.WriteByte('\n')
	return b.String()
}

// fenceWidth returns a backtick run longer than any line in body that would
// close a Markdown fence of that run or shorter.
func fenceWidth(body string) int {
	longest := 0
	rest := body
	for rest != "" {
		line := rest
		if i := strings.IndexByte(rest, '\n'); i >= 0 {
			line = rest[:i]
			rest = rest[i+1:]
		} else {
			rest = ""
		}
		if n := fenceCloserRun(line); n > longest {
			longest = n
		}
	}
	if longest < 3 {
		return 3
	}
	return longest + 1
}

// fenceCloserRun reports the backtick length of a line that CommonMark would
// accept as a closing fence. Other lines report zero.
func fenceCloserRun(line string) int {
	line = strings.TrimSuffix(line, "\r")
	i := 0
	for i < len(line) && i < 3 && line[i] == ' ' {
		i++
	}
	if i == 3 && i < len(line) && line[i] == ' ' {
		return 0
	}
	ticks := 0
	for i < len(line) && line[i] == '`' {
		i++
		ticks++
	}
	for i < len(line) && line[i] == ' ' {
		i++
	}
	if i != len(line) || ticks == 0 {
		return 0
	}
	return ticks
}
