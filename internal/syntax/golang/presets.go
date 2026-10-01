package golang

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path"
	"sort"
	"strings"
)

const (
	// PresetGenerated excludes Go source that the standard library classifies
	// as generated.
	PresetGenerated = "go:generated"

	// PresetTests excludes Go source whose base name ends in _test.go.
	PresetTests = "go:tests"

	// PresetVendor excludes Go source under a path element named vendor.
	PresetVendor = "go:vendor"

	// SelectorAll expands to every concrete Go preset known to this package.
	// It is not a concrete preset and is not a persisted identifier.
	SelectorAll = "go:all"
)

// ConcretePresets returns the concrete Go preset identifiers in lexical order.
// The result is a new slice and does not include SelectorAll.
func ConcretePresets() []string {
	ids := []string{PresetGenerated, PresetTests, PresetVendor}
	sort.Strings(ids)
	return ids
}

// Selectors returns the Go preset selectors a command line may name, in
// lexical order. The result includes SelectorAll and every concrete preset.
// It is a new slice.
func Selectors() []string {
	ids := append(ConcretePresets(), SelectorAll)
	sort.Strings(ids)
	return ids
}

// Expand returns the concrete presets named by selector.
//
// SelectorAll expands to ConcretePresets. A concrete preset selects itself.
// The result is a new lexical slice and does not contain SelectorAll. The
// second result is false when selector is not a Go selector.
func Expand(selector string) ([]string, bool) {
	if selector == SelectorAll {
		return ConcretePresets(), true
	}
	for _, id := range ConcretePresets() {
		if id == selector {
			return []string{id}, true
		}
	}
	return nil, false
}

// PathExcluded reports whether enabled Go presets exclude logicalPath without
// reading source.
//
// logicalPath must use slash separators. A path that is not Go source is not
// excluded. PresetGenerated is not applied here.
func PathExcluded(logicalPath string, enabled []string) bool {
	if !isGoPath(logicalPath) {
		return false
	}
	for _, id := range enabled {
		switch id {
		case PresetTests:
			if strings.HasSuffix(path.Base(logicalPath), "_test.go") {
				return true
			}
		case PresetVendor:
			if hasPathElement(logicalPath, "vendor") {
				return true
			}
		}
	}
	return false
}

// SourceExcluded reports whether enabled Go presets exclude logicalPath by
// inspecting source.
//
// source is parsed only when PresetGenerated is enabled. The parse asks the
// standard library whether the file matches the generated-source convention.
// It does not type-check or resolve imports. A path that is not Go source is
// not excluded. Path-only presets are not applied here.
func SourceExcluded(logicalPath string, source []byte, enabled []string) bool {
	if !isGoPath(logicalPath) || !containsPreset(enabled, PresetGenerated) {
		return false
	}
	return generatedFile(source)
}

func isGoPath(logicalPath string) bool {
	return strings.HasSuffix(logicalPath, ".go")
}

func hasPathElement(logicalPath, name string) bool {
	for _, elem := range strings.Split(logicalPath, "/") {
		if elem == name {
			return true
		}
	}
	return false
}

func containsPreset(ids []string, want string) bool {
	for _, id := range ids {
		if id == want {
			return true
		}
	}
	return false
}

// generatedFile reports whether source matches Go's generated-file convention.
//
// The classification uses ast.IsGenerated on a comment-preserving parse that
// stops at the package clause. A syntax error after that clause is outside
// the classification. A file that cannot be parsed far enough to expose its
// leading comments is not classified as generated.
func generatedFile(source []byte) bool {
	if source == nil {
		source = []byte{}
	}
	fset := token.NewFileSet()
	file, _ := parser.ParseFile(fset, "", source, parser.ParseComments|parser.PackageClauseOnly|parser.SkipObjectResolution)
	if file == nil {
		return false
	}
	return ast.IsGenerated(file)
}
