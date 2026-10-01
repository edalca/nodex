package main

import (
	"strings"

	"github.com/edalca/nodex/internal/index"
	"github.com/edalca/nodex/internal/project"
)

// declarations returns the compact Markdown form of the current declarations.
//
// The index is loaded through the index package. A missing index, a corrupt
// index, and a stale index are errors. Nothing is regenerated. An empty
// selection returns an empty string. Filters select already indexed facts.
// The text does not say whether documentation ought to exist.
func declarations(dirs project.Locations, filters discoveryFilters) (string, error) {
	idx, _, _, err := openCurrentIndex(dirs)
	if err != nil {
		return "", err
	}
	var selected []index.Declaration
	for _, decl := range idx.Declarations() {
		if filters.matchDeclaration(decl) {
			selected = append(selected, decl)
		}
	}
	return formatDeclarations(selected), nil
}

// formatDeclarations renders declarations as compact Markdown.
//
// Each block is the declaration ID, its logical file path, its kind, its names,
// and the comment ID of the directly associated documentation comment. A
// declaration with no names uses a names line that has no value. doc: none
// means the parser recorded no documentation comment on that node. No
// declarations produce an empty result.
func formatDeclarations(decls []index.Declaration) string {
	if len(decls) == 0 {
		return ""
	}
	var b strings.Builder
	for i, decl := range decls {
		if i > 0 {
			b.WriteByte('\n')
		}
		b.WriteString("## ")
		b.WriteString(decl.ID.String())
		b.WriteString("\n\n")
		b.WriteString("file: `")
		b.WriteString(decl.Path)
		b.WriteString("`\n")
		b.WriteString("kind: ")
		b.WriteString(string(decl.Kind))
		b.WriteByte('\n')
		b.WriteString(formatNamesLine(decl.Names))
		b.WriteByte('\n')
		b.WriteString("doc: ")
		b.WriteString(formatDoc(decl.Doc))
		b.WriteByte('\n')
	}
	return b.String()
}

// formatNamesLine renders the names field.
//
// Names stay in source order and are separated by a comma and a space.
// An empty list is "names:" with no value, so it is not a declared
// identifier and does not collide with a declaration named none.
func formatNamesLine(names []string) string {
	if len(names) == 0 {
		return "names:"
	}
	return "names: " + strings.Join(names, ", ")
}

// formatDoc renders the directly associated comment ID.
// The zero ID is the explicit value none.
func formatDoc(id index.ID) string {
	if !id.Valid() {
		return "none"
	}
	return id.String()
}
