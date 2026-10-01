// Package architecture enforces the github.com/edalca/nodex structural laws that can be checked
// from package paths and imports, against this module and against synthetic
// package graphs.
package architecture

import (
	"fmt"
	"go/parser"
	"go/token"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"testing"
)

const (
	maxInternalSegments = 2

	ruleDepth      = "depth: "
	ruleForbidden  = "forbidden package: "
	ruleCrossRoot  = "cross-root subpackage import: "
	ruleCycle      = "root cycle: "
	ruleHomonymous = "homonymous root file: "
)

// forbiddenPackageNames are escape-hatch package names. Matching is exact on a
// path segment so names that merely contain these words stay legal.
var forbiddenPackageNames = map[string]struct{}{
	"utils":   {},
	"helpers": {},
	"common":  {},
	"shared":  {},
}

// pkg is one Go package in a module. relDir is the slash-separated path from
// the module root. goFiles lists non-test source files by base name.
type pkg struct {
	path    string
	relDir  string
	goFiles []string
	imports []string
}

// check reports structural violations for the given packages. Roots are derived
// from packages under internal/; an empty set is valid.
func check(modulePath string, pkgs []pkg) []string {
	var violations []string
	violations = append(violations, depthViolations(pkgs)...)
	violations = append(violations, forbiddenPackageViolations(pkgs)...)
	violations = append(violations, crossRootSubpackageViolations(modulePath, pkgs)...)
	violations = append(violations, rootCycleViolations(modulePath, pkgs)...)
	violations = append(violations, homonymousRootFileViolations(pkgs)...)
	sort.Strings(violations)
	return violations
}

func depthViolations(pkgs []pkg) []string {
	var out []string
	for _, p := range pkgs {
		segments, ok := internalSegments(p.relDir)
		if !ok || len(segments) <= maxInternalSegments {
			continue
		}
		out = append(out, ruleDepth+p.relDir+" exceeds internal/<root>/<subpackage>")
	}
	return out
}

func forbiddenPackageViolations(pkgs []pkg) []string {
	var out []string
	for _, p := range pkgs {
		segments, ok := internalSegments(p.relDir)
		if !ok {
			continue
		}
		for _, segment := range segments {
			if _, forbidden := forbiddenPackageNames[segment]; !forbidden {
				continue
			}
			out = append(out, ruleForbidden+p.relDir+" ("+segment+")")
			break
		}
	}
	return out
}

// crossRootSubpackageViolations allows a subpackage import only from inside
// the same bounded root. Another root, and any package outside internal/, must
// import that root's package itself.
func crossRootSubpackageViolations(modulePath string, pkgs []pkg) []string {
	var out []string
	for _, p := range pkgs {
		importerRoot, importerOK := internalRoot(p.relDir)
		for _, imp := range p.imports {
			targetRel, ok := relFromImport(modulePath, imp)
			if !ok {
				continue
			}
			segments, internal := internalSegments(targetRel)
			if !internal || len(segments) <= 1 {
				continue
			}
			if importerOK && importerRoot == segments[0] {
				continue
			}
			out = append(out, ruleCrossRoot+p.path+" imports "+imp)
		}
	}
	return out
}

func rootCycleViolations(modulePath string, pkgs []pkg) []string {
	deps := map[string]map[string]struct{}{}
	for _, p := range pkgs {
		from, ok := internalRoot(p.relDir)
		if !ok {
			continue
		}
		if deps[from] == nil {
			deps[from] = map[string]struct{}{}
		}
		for _, imp := range p.imports {
			targetRel, ok := relFromImport(modulePath, imp)
			if !ok {
				continue
			}
			to, ok := internalRoot(targetRel)
			if !ok || to == from {
				continue
			}
			deps[from][to] = struct{}{}
		}
	}
	cycle := findCycle(deps)
	if len(cycle) == 0 {
		return nil
	}
	return []string{ruleCycle + strings.Join(cycle, " -> ")}
}

// homonymousRootFileViolations requires <root>/<root>.go when the root package
// itself has more than one non-test Go file.
func homonymousRootFileViolations(pkgs []pkg) []string {
	var out []string
	for _, p := range pkgs {
		segments, ok := internalSegments(p.relDir)
		if !ok || len(segments) != 1 || len(p.goFiles) < 2 {
			continue
		}
		want := segments[0] + ".go"
		if contains(p.goFiles, want) {
			continue
		}
		out = append(out, ruleHomonymous+p.relDir+" missing "+want)
	}
	return out
}

func internalSegments(relDir string) ([]string, bool) {
	relDir = path.Clean(relDir)
	if relDir == "internal" {
		return nil, true
	}
	if !strings.HasPrefix(relDir, "internal/") {
		return nil, false
	}
	rest := strings.TrimPrefix(relDir, "internal/")
	if rest == "" || rest == "." {
		return nil, true
	}
	return strings.Split(rest, "/"), true
}

func internalRoot(relDir string) (string, bool) {
	segments, ok := internalSegments(relDir)
	if !ok || len(segments) == 0 {
		return "", false
	}
	return segments[0], true
}

func relFromImport(modulePath, importPath string) (string, bool) {
	if importPath == modulePath {
		return ".", true
	}
	prefix := modulePath + "/"
	if !strings.HasPrefix(importPath, prefix) {
		return "", false
	}
	return strings.TrimPrefix(importPath, prefix), true
}

func findCycle(deps map[string]map[string]struct{}) []string {
	nodes := make([]string, 0, len(deps))
	neighbors := make(map[string][]string, len(deps))
	for from, toSet := range deps {
		nodes = append(nodes, from)
		tos := make([]string, 0, len(toSet))
		for to := range toSet {
			tos = append(tos, to)
		}
		sort.Strings(tos)
		neighbors[from] = tos
	}
	sort.Strings(nodes)

	const (
		white = 0
		gray  = 1
		black = 2
	)
	color := map[string]int{}
	var stack []string
	var cycle []string
	var visit func(string) bool
	visit = func(node string) bool {
		color[node] = gray
		stack = append(stack, node)
		for _, next := range neighbors[node] {
			switch color[next] {
			case gray:
				start := 0
				for i, item := range stack {
					if item == next {
						start = i
						break
					}
				}
				cycle = append([]string{}, stack[start:]...)
				cycle = append(cycle, next)
				return true
			case white:
				if visit(next) {
					return true
				}
			}
		}
		stack = stack[:len(stack)-1]
		color[node] = black
		return false
	}
	for _, node := range nodes {
		if color[node] == white && visit(node) {
			return cycle
		}
	}
	return nil
}

func contains(items []string, want string) bool {
	for _, item := range items {
		if item == want {
			return true
		}
	}
	return false
}

func loadModule(root string) (string, []pkg, error) {
	modulePath, err := readModulePath(filepath.Join(root, "go.mod"))
	if err != nil {
		return "", nil, err
	}
	byDir := map[string]*pkg{}
	err = filepath.WalkDir(root, func(filePath string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			if filePath != root {
				base := entry.Name()
				if strings.HasPrefix(base, ".") || base == "vendor" || base == "testdata" {
					return filepath.SkipDir
				}
			}
			return nil
		}
		if !strings.HasSuffix(entry.Name(), ".go") {
			return nil
		}
		relFile, err := filepath.Rel(root, filePath)
		if err != nil {
			return err
		}
		relFile = filepath.ToSlash(relFile)
		relDir := path.Dir(relFile)
		info := byDir[relDir]
		if info == nil {
			importPath := modulePath
			if relDir != "." {
				importPath = modulePath + "/" + relDir
			}
			info = &pkg{path: importPath, relDir: relDir}
			byDir[relDir] = info
		}
		if !strings.HasSuffix(entry.Name(), "_test.go") {
			info.goFiles = append(info.goFiles, entry.Name())
		}
		imports, err := fileImports(filePath)
		if err != nil {
			return err
		}
		info.imports = append(info.imports, imports...)
		return nil
	})
	if err != nil {
		return "", nil, err
	}
	out := make([]pkg, 0, len(byDir))
	for _, info := range byDir {
		sort.Strings(info.goFiles)
		sort.Strings(info.imports)
		out = append(out, *info)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].path < out[j].path })
	return modulePath, out, nil
}

func readModulePath(goModPath string) (string, error) {
	data, err := os.ReadFile(goModPath)
	if err != nil {
		return "", err
	}
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(strings.TrimSpace(line))
		if len(fields) >= 2 && fields[0] == "module" {
			return strings.Trim(fields[1], `"`), nil
		}
	}
	return "", fmt.Errorf("%s has no module path", goModPath)
}

func fileImports(filePath string) ([]string, error) {
	file, err := parser.ParseFile(token.NewFileSet(), filePath, nil, parser.ImportsOnly)
	if err != nil {
		return nil, fmt.Errorf("parse %s: %w", filePath, err)
	}
	imports := make([]string, 0, len(file.Imports))
	for _, spec := range file.Imports {
		importPath, err := strconv.Unquote(spec.Path.Value)
		if err != nil {
			return nil, fmt.Errorf("parse import in %s: %w", filePath, err)
		}
		imports = append(imports, importPath)
	}
	return imports, nil
}

func moduleRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("locate architecture test file")
	}
	return filepath.Dir(filepath.Dir(filepath.Dir(file)))
}

func requireClean(t *testing.T, violations []string) {
	t.Helper()
	if len(violations) > 0 {
		t.Fatalf("unexpected violations:\n%s", strings.Join(violations, "\n"))
	}
}

func requireOnlyRule(t *testing.T, violations []string, rule string) {
	t.Helper()
	if len(violations) == 0 {
		t.Fatalf("expected %s violation", rule)
	}
	for _, violation := range violations {
		if !strings.HasPrefix(violation, rule) {
			t.Fatalf("violation %q is outside %s; all: %q", violation, rule, violations)
		}
	}
}

// TestRepositoryArchitecture checks this module. Bounded roots come from the
// tree that exists now, including a tree with no internal packages.
func TestRepositoryArchitecture(t *testing.T) {
	root := moduleRoot(t)
	modulePath, pkgs, err := loadModule(root)
	if err != nil {
		t.Fatalf("load module: %v", err)
	}
	if modulePath != "github.com/edalca/nodex" {
		t.Fatalf("module path = %q, want github.com/edalca/nodex", modulePath)
	}
	var foundCmd bool
	for _, p := range pkgs {
		if p.path == "github.com/edalca/nodex/cmd/nodex" && contains(p.goFiles, "main.go") {
			foundCmd = true
		}
	}
	if !foundCmd {
		t.Fatal("scanner did not find github.com/edalca/nodex/cmd/nodex with main.go")
	}
	requireClean(t, check(modulePath, pkgs))
}

// TestAcceptsNoBoundedRoots checks that an empty internal tree is valid.
func TestAcceptsNoBoundedRoots(t *testing.T) {
	requireClean(t, check("github.com/edalca/nodex", nil))
	requireClean(t, check("github.com/edalca/nodex", []pkg{{
		path:    "github.com/edalca/nodex/cmd/github.com/edalca/nodex",
		relDir:  "cmd/github.com/edalca/nodex",
		goFiles: []string{"main.go"},
	}}))
}

// TestAcceptsRootToRootAndShallowStructure checks a legal DAG: root-to-root
// imports, a same-root subpackage, depth of two, and a single-file root.
func TestAcceptsRootToRootAndShallowStructure(t *testing.T) {
	pkgs := []pkg{
		{
			path:    "github.com/edalca/nodex/internal/alpha",
			relDir:  "internal/alpha",
			goFiles: []string{"alpha.go", "extra.go"},
			imports: []string{"github.com/edalca/nodex/internal/beta", "github.com/edalca/nodex/internal/alpha/parse"},
		},
		{
			path:    "github.com/edalca/nodex/internal/alpha/parse",
			relDir:  "internal/alpha/parse",
			goFiles: []string{"parse.go"},
			imports: []string{"github.com/edalca/nodex/internal/alpha"},
		},
		{
			path:    "github.com/edalca/nodex/internal/beta",
			relDir:  "internal/beta",
			goFiles: []string{"scan.go"},
			imports: []string{"github.com/edalca/nodex/internal/gamma"},
		},
		{
			path:    "github.com/edalca/nodex/internal/gamma",
			relDir:  "internal/gamma",
			goFiles: []string{"gamma.go"},
		},
		{
			path:    "github.com/edalca/nodex/cmd/github.com/edalca/nodex",
			relDir:  "cmd/github.com/edalca/nodex",
			goFiles: []string{"main.go"},
			imports: []string{"github.com/edalca/nodex/internal/alpha"},
		},
		{
			path:    "github.com/edalca/nodex/internal/commons",
			relDir:  "internal/commons",
			goFiles: []string{"commons.go"},
		},
	}
	requireClean(t, check("github.com/edalca/nodex", pkgs))
}

// TestRejectsCrossRootSubpackageImport checks that another root cannot import
// a subpackage, and that a package outside internal/ cannot either.
func TestRejectsCrossRootSubpackageImport(t *testing.T) {
	rootToSub := []pkg{
		{
			path:    "github.com/edalca/nodex/internal/alpha",
			relDir:  "internal/alpha",
			goFiles: []string{"alpha.go"},
			imports: []string{"github.com/edalca/nodex/internal/beta/contracts"},
		},
		{
			path:    "github.com/edalca/nodex/internal/beta/contracts",
			relDir:  "internal/beta/contracts",
			goFiles: []string{"contracts.go"},
		},
	}
	requireOnlyRule(t, check("github.com/edalca/nodex", rootToSub), ruleCrossRoot)

	outsideToSub := []pkg{
		{
			path:    "github.com/edalca/nodex/cmd/github.com/edalca/nodex",
			relDir:  "cmd/github.com/edalca/nodex",
			goFiles: []string{"main.go"},
			imports: []string{"github.com/edalca/nodex/internal/beta/parser"},
		},
		{
			path:    "github.com/edalca/nodex/internal/beta/parser",
			relDir:  "internal/beta/parser",
			goFiles: []string{"parser.go"},
		},
	}
	requireOnlyRule(t, check("github.com/edalca/nodex", outsideToSub), ruleCrossRoot)
}

// TestRejectsExcessiveDepth checks that internal/a/b/c is rejected.
func TestRejectsExcessiveDepth(t *testing.T) {
	pkgs := []pkg{{
		path:    "github.com/edalca/nodex/internal/alpha/parse/token",
		relDir:  "internal/alpha/parse/token",
		goFiles: []string{"token.go"},
	}}
	requireOnlyRule(t, check("github.com/edalca/nodex", pkgs), ruleDepth)
}

// TestRejectsForbiddenGenericPackage checks utils, helpers, common, and shared.
func TestRejectsForbiddenGenericPackage(t *testing.T) {
	for _, name := range []string{"utils", "helpers", "common", "shared"} {
		t.Run(name, func(t *testing.T) {
			pkgs := []pkg{{
				path:    "github.com/edalca/nodex/internal/" + name,
				relDir:  "internal/" + name,
				goFiles: []string{name + ".go"},
			}}
			requireOnlyRule(t, check("github.com/edalca/nodex", pkgs), ruleForbidden)
		})
	}
}

// TestRejectsRootDependencyCycle checks a two-root import cycle.
func TestRejectsRootDependencyCycle(t *testing.T) {
	pkgs := []pkg{
		{
			path:    "github.com/edalca/nodex/internal/alpha",
			relDir:  "internal/alpha",
			goFiles: []string{"alpha.go"},
			imports: []string{"github.com/edalca/nodex/internal/beta"},
		},
		{
			path:    "github.com/edalca/nodex/internal/beta",
			relDir:  "internal/beta",
			goFiles: []string{"beta.go"},
			imports: []string{"github.com/edalca/nodex/internal/alpha"},
		},
	}
	violations := check("github.com/edalca/nodex", pkgs)
	requireOnlyRule(t, violations, ruleCycle)
	if violations[0] != "root cycle: alpha -> beta -> alpha" {
		t.Fatalf("cycle = %q", violations[0])
	}
}

// TestRejectsMissingHomonymousRootFile checks a multi-file root without <root>.go.
func TestRejectsMissingHomonymousRootFile(t *testing.T) {
	pkgs := []pkg{{
		path:    "github.com/edalca/nodex/internal/index",
		relDir:  "internal/index",
		goFiles: []string{"a.go", "b.go"},
	}}
	violations := check("github.com/edalca/nodex", pkgs)
	requireOnlyRule(t, violations, ruleHomonymous)
	if violations[0] != "homonymous root file: internal/index missing index.go" {
		t.Fatalf("violation = %q", violations[0])
	}
}
