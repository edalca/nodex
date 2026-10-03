package golang_test

import (
	"errors"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/edalca/nodex/internal/syntax/golang"

	"github.com/edalca/nodex/internal/syntax/contracts"
	"github.com/edalca/nodex/internal/syntax/types"
)

func TestOneGroupIsOneUnit(t *testing.T) {
	src := "package p\n\n// Service manages the graph.\n// It is safe for concurrent use.\nfunc F() {}\n"
	comments, err := golang.Parse([]byte(src))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	file := mustParse(t, src)
	if len(file.Comments) != 1 || len(file.Comments[0].List) != 2 {
		t.Fatalf("fixture groups = %d", len(file.Comments))
	}
	if len(comments) != 1 {
		t.Fatalf("units = %d, want 1", len(comments))
	}
	if comments[0].Text != "Service manages the graph.\nIt is safe for concurrent use." {
		t.Fatalf("text = %q", comments[0].Text)
	}
}

func TestEveryCommentGroupIsReturned(t *testing.T) {
	src := strings.Join([]string{
		"package sample",
		"",
		`var s = "// not a comment"`,
		"",
		"// real comment",
		"// same group",
		"func F() {",
		"\t// inside",
		"}",
		"",
		"type T struct {",
		"\t// field",
		"\tA int // trail",
		"}",
		"",
	}, "\n")
	comments, err := golang.Parse([]byte(src))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	fset, file := mustParseFile(t, src)
	tf := fset.File(file.FileStart)
	if len(comments) != len(file.Comments) {
		t.Fatalf("units = %d, groups = %d", len(comments), len(file.Comments))
	}
	markers := 0
	for i, group := range file.Comments {
		markers += len(group.List)
		phys := tf.PositionFor(group.Pos(), false)
		if comments[i].Start.Offset != phys.Offset || comments[i].Start.Line != phys.Line || comments[i].Start.Column != phys.Column {
			t.Fatalf("group %d start = %+v, physical %s", i, comments[i].Start, phys)
		}
	}
	if markers <= len(comments) {
		t.Fatal("fixture has no group of more than one comment and no single-comment group to compare")
	}
}

func TestNormalizedTextKeepsDirectivesTextDrops(t *testing.T) {
	src := "package p\n\n//go:generate stringer -type Op\nfunc F() {}\n"
	comments, err := golang.Parse([]byte(src))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(comments) != 1 || !strings.Contains(comments[0].Text, "go:generate") {
		t.Fatalf("comments = %#v", comments)
	}
	file := mustParse(t, src)
	if got := file.Comments[0].Text(); got != "" {
		t.Fatalf("CommentGroup.Text() = %q, want the directive removed by the standard library", got)
	}
}

func TestCRLFEndUsesSourceNotASTEnd(t *testing.T) {
	src := "package p\r\n\r\n/* a\r\nb */\r\nfunc F() {}\r\n"
	comments, err := golang.Parse([]byte(src))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	fset, file := mustParseFile(t, src)
	tf := fset.File(file.FileStart)
	astEnd := tf.Offset(file.Comments[0].End())
	if comments[0].End.Offset == astEnd {
		t.Fatal("comment end matched ast.Comment.End")
	}
	if comments[0].Raw != "/* a\r\nb */" {
		t.Fatalf("raw = %q", comments[0].Raw)
	}
	if src[comments[0].Start.Offset:comments[0].End.Offset] != comments[0].Raw {
		t.Fatal("raw is not the source slice")
	}
	if astEnd >= len(src) || src[comments[0].Start.Offset:astEnd] == comments[0].Raw {
		t.Fatal("fixture no longer shows carriage-return skew")
	}
}

func TestLineDirectiveUsesUnadjustedPositions(t *testing.T) {
	src := "package p\n\n//line foo.go:10:9\n\n// after\nvar x int\n"
	comments, err := golang.Parse([]byte(src))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	fset, file := mustParseFile(t, src)
	tf := fset.File(file.FileStart)
	if len(file.Comments) != 2 || len(comments) != 2 {
		t.Fatalf("groups = %d units = %d", len(file.Comments), len(comments))
	}
	group := file.Comments[1]
	adjusted := tf.Position(group.Pos())
	physical := tf.PositionFor(group.Pos(), false)
	if adjusted.Line == physical.Line && adjusted.Filename == physical.Filename {
		t.Fatalf("fixture did not adjust positions: %s", adjusted)
	}
	got := comments[1].Start
	if got.Line != physical.Line || got.Column != physical.Column || got.Offset != physical.Offset {
		t.Fatalf("start = %+v, physical %s", got, physical)
	}
	if got.Line == adjusted.Line {
		t.Fatalf("start line %d is the //line line %d", got.Line, adjusted.Line)
	}
	endAdjusted := tf.Position(group.End())
	endPhysical := tf.PositionFor(tf.Pos(comments[1].End.Offset), false)
	if comments[1].End.Line != endPhysical.Line || comments[1].End.Column != endPhysical.Column {
		t.Fatalf("end = %+v, physical %s", comments[1].End, endPhysical)
	}
	if endAdjusted.Filename == physical.Filename && endAdjusted.Line == endPhysical.Line {
		t.Fatalf("end fixture did not adjust: %s", endAdjusted)
	}
}

func TestSyntaxErrorDropsPartialComments(t *testing.T) {
	src := "package p\n\n// kept\nfunc (\n"
	comments, err := golang.Parse([]byte(src))
	if err == nil || comments != nil {
		t.Fatalf("Parse = (%v, %v), want nil comments and an error", comments, err)
	}
	var parseErr *golang.Error
	if !errors.As(err, &parseErr) || len(parseErr.Diagnostics) == 0 {
		t.Fatalf("error = %T %v", err, err)
	}
	file, astErr := parser.ParseFile(token.NewFileSet(), "bad.go", src, parser.ParseComments|parser.SkipObjectResolution)
	if astErr == nil || file == nil || len(file.Comments) == 0 {
		t.Fatal("fixture did not produce a partial comment group")
	}
}

func TestLineDirectiveFailureUsesPhysicalOffset(t *testing.T) {
	src := "package p\n\n//line other.go:400:1\nfunc (\n"
	_, err := golang.Parse([]byte(src))
	var parseErr *golang.Error
	if !errors.As(err, &parseErr) || len(parseErr.Diagnostics) == 0 {
		t.Fatalf("error = %v", err)
	}
	diag := parseErr.Diagnostics[0]
	if diag.Position.Line != 4 || diag.Position.Column != 8 || diag.Position.Offset != len(src) {
		t.Fatalf("diagnostic position = %+v, want line 4 column 8 at EOF", diag.Position)
	}
	if strings.Contains(err.Error(), "other.go") || strings.Contains(err.Error(), "400") {
		t.Fatalf("error leaks //line data: %v", err)
	}
}

func TestRepeatedParseIsDeterministic(t *testing.T) {
	src := []byte("package p\n\n// a\n// b\n\n/* c */\nfunc F() {}\n")
	first, err := golang.Parse(src)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	for i := 0; i < 8; i++ {
		again, err := golang.Parse(src)
		if err != nil {
			t.Fatalf("Parse %d: %v", i, err)
		}
		if !reflect.DeepEqual(first, again) {
			t.Fatalf("parse %d = %+v, want %+v", i, again, first)
		}
	}
}

func TestConcurrentParse(t *testing.T) {
	sources := [][]byte{
		[]byte("package p\n\n// one\nfunc F() {}\n"),
		[]byte("package p\r\n\r\n/* a\r\nb */\r\n"),
		[]byte("package p\nfunc (\n"),
	}
	golden := make([]any, len(sources))
	for i, src := range sources {
		comments, err := golang.Parse(src)
		if err != nil {
			golden[i] = err.Error()
			if comments != nil {
				t.Fatalf("source %d returned comments with %v", i, err)
			}
			continue
		}
		golden[i] = comments
	}

	var wg sync.WaitGroup
	errCh := make(chan error, 24)
	for n := 0; n < 24; n++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			i := n % len(sources)
			comments, err := golang.Parse(sources[i])
			switch want := golden[i].(type) {
			case string:
				if comments != nil || err == nil || err.Error() != want {
					errCh <- errors.New("failure mismatch")
				}
			case []types.Comment:
				if !reflect.DeepEqual(comments, want) {
					errCh <- errors.New("comment mismatch")
				}
			}
		}(n)
	}
	wg.Wait()
	close(errCh)
	for err := range errCh {
		t.Errorf("concurrent parse: %v", err)
	}
}

func TestImplementationImports(t *testing.T) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("locate golang test")
	}
	dir := filepath.Dir(file)
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read dir: %v", err)
	}
	forbidden := []string{
		"go/types",
		"go/packages",
		"golang.org/x/tools",
		"github.com/edalca/nodex/internal/project",
		"github.com/edalca/nodex/internal/ignore",
		"github.com/edalca/nodex/internal/syntax",
	}
	var sawParser bool
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		parsed, err := parser.ParseFile(token.NewFileSet(), filepath.Join(dir, name), nil, parser.ImportsOnly)
		if err != nil {
			t.Fatalf("parse %s: %v", name, err)
		}
		for _, spec := range parsed.Imports {
			path := strings.Trim(spec.Path.Value, `"`)
			if path == "go/parser" {
				sawParser = true
			}
			for _, bad := range forbidden {
				if path == bad || (bad != "github.com/edalca/nodex/internal/syntax" && strings.HasPrefix(path, bad+"/")) {
					t.Errorf("%s imports %s", name, path)
				}
			}
		}
	}
	if !sawParser {
		t.Fatal("Go implementation does not import go/parser")
	}
}

func TestContextUsesParsedDeclarations(t *testing.T) {
	src := "package p\n\n// F adds.\nfunc F() {\n\treturn\n}\n"
	comments := mustComments(t, src)
	snip := mustGoContext(t, src, comments[0])
	fset, file := mustParseFile(t, src)
	tf := fset.File(file.FileStart)
	fn := file.Decls[0].(*ast.FuncDecl)
	if fn.Doc == nil {
		t.Fatal("parser did not attach the function doc")
	}
	if snip.Start.Offset != tf.Offset(fn.Doc.Pos()) || snip.End.Offset != tf.Offset(fn.End()) {
		t.Fatalf("snippet [%d,%d), doc %d func end %d", snip.Start.Offset, snip.End.Offset, tf.Offset(fn.Doc.Pos()), tf.Offset(fn.End()))
	}
	if snip.Text != src[snip.Start.Offset:snip.End.Offset] {
		t.Fatal("text is not the original slice")
	}

	fieldSrc := "package p\n\ntype T struct {\n\t// A is a.\n\tA int\n}\n"
	fieldComments := mustComments(t, fieldSrc)
	fieldSnip := mustGoContext(t, fieldSrc, fieldComments[0])
	_, fieldFile := mustParseFile(t, fieldSrc)
	gen := fieldFile.Decls[0].(*ast.GenDecl)
	fset, _ = mustParseFile(t, fieldSrc)
	tf = fset.File(fieldFile.FileStart)
	if fieldSnip.Start.Offset != tf.Offset(gen.Pos()) || fieldSnip.End.Offset != tf.Offset(gen.End()) {
		t.Fatalf("field snippet [%d,%d), decl [%d,%d)", fieldSnip.Start.Offset, fieldSnip.End.Offset, tf.Offset(gen.Pos()), tf.Offset(gen.End()))
	}

	pkgSrc := "// Package p.\npackage p\n\nfunc F() {}\n"
	pkgComments := mustComments(t, pkgSrc)
	pkgSnip := mustGoContext(t, pkgSrc, pkgComments[0])
	if strings.Contains(pkgSnip.Text, "func F") || !strings.Contains(pkgSnip.Text, "package p") {
		t.Fatalf("package snippet = %q", pkgSnip.Text)
	}
}

func TestContextRejectsBadInput(t *testing.T) {
	src := "package p\n\n// only\nfunc F() {}\n"
	comments := mustComments(t, src)
	_, err := golang.Context([]byte("package p\nfunc (\n"), types.Position{Offset: 0, Line: 1, Column: 1}, types.Position{Offset: len("package"), Line: 1, Column: 8}, contracts.Limits{Lines: 40, ExtraBytes: 8192})
	var parseErr *golang.Error
	if !errors.As(err, &parseErr) {
		t.Fatalf("malformed source error = %v", err)
	}

	_, err = golang.Context([]byte(src), types.Position{Offset: 0, Line: 1, Column: 1}, types.Position{Offset: len("package"), Line: 1, Column: 8}, contracts.Limits{Lines: 40, ExtraBytes: 8192})
	if !errors.Is(err, contracts.ErrCommentNotFound) {
		t.Fatalf("keyword range error = %v", err)
	}

	bad := comments[0].Start
	bad.Line = 99
	_, err = golang.Context([]byte(src), bad, comments[0].End, contracts.Limits{Lines: 40, ExtraBytes: 8192})
	if !errors.Is(err, contracts.ErrMalformedRange) {
		t.Fatalf("inconsistent range error = %v", err)
	}

	fake := "package p\n\nvar s = \"// not a comment\"\n\n// real\nfunc F() {}\n"
	real := mustComments(t, fake)
	if len(real) != 1 || real[0].Text != "real" {
		t.Fatalf("comments = %#v", real)
	}
	snip := mustGoContext(t, fake, real[0])
	if strings.Contains(snip.Text, "not a comment") {
		t.Fatalf("string text entered the context: %q", snip.Text)
	}
}

func TestContextPhysicalPositionsAndOriginalBytes(t *testing.T) {
	src := "package p\n\n//line foo.go:10:9\n\n// after\nvar x int\n"
	comments := mustComments(t, src)
	var after types.Comment
	for _, c := range comments {
		if c.Text == "after" {
			after = c
		}
	}
	snip := mustGoContext(t, src, after)
	if snip.Start.Line != 5 || strings.Contains(snip.Text, "package p") {
		t.Fatalf("snippet line %d text %q", snip.Start.Line, snip.Text)
	}

	crlf := "package p\r\n\r\n// F docs.\r\nfunc F() {\r\n}\r\n"
	crlfComments := mustComments(t, crlf)
	crlfSnip := mustGoContext(t, crlf, crlfComments[0])
	if !strings.Contains(crlfSnip.Text, "\r\n") || crlfSnip.Text != crlf[crlfSnip.Start.Offset:crlfSnip.End.Offset] {
		t.Fatalf("crlf snippet = %q", crlfSnip.Text)
	}

	wide := "package p\n\n// naïve\nfunc F() {}\n"
	wideComments := mustComments(t, wide)
	wideSnip := mustGoContext(t, wide, wideComments[0])
	if !strings.Contains(wideSnip.Text, "naïve") || wideSnip.Text != wide[wideSnip.Start.Offset:wideSnip.End.Offset] {
		t.Fatalf("utf-8 snippet = %q", wideSnip.Text)
	}
}

func TestContextHonorsCallerLimits(t *testing.T) {
	var b strings.Builder
	b.WriteString("package p\n\nfunc F() {\n")
	for i := 0; i < 30; i++ {
		if i == 15 {
			b.WriteString("\t// TARGET\n")
			continue
		}
		b.WriteString("\t_ = ")
		b.WriteString(strconv.Itoa(i))
		b.WriteByte('\n')
	}
	b.WriteString("}\n")
	src := b.String()
	comments := mustComments(t, src)
	var target types.Comment
	for _, c := range comments {
		if c.Text == "TARGET" {
			target = c
		}
	}
	limited := mustGoContextLimits(t, src, target, contracts.Limits{Lines: 5, ExtraBytes: 8192})
	full := mustGoContextLimits(t, src, target, contracts.Limits{Lines: 40, ExtraBytes: 8192})
	if strings.Contains(limited.Text, "func F()") || snippetLineCount(limited.Text) > 5 {
		t.Fatalf("limited snippet = %q", limited.Text)
	}
	if !strings.Contains(limited.Text, "// TARGET") {
		t.Fatalf("limited snippet = %q", limited.Text)
	}
	if len(full.Text) <= len(limited.Text) {
		t.Fatalf("full context is not larger than the tight limit")
	}
	if !strings.Contains(full.Text, "func F()") {
		t.Fatalf("40-line limit dropped a 30-line function: %q", full.Text)
	}

	long := "package p\n\nfunc F() { /* TARGET */ s := \"" + strings.Repeat("a", 400) + "\" }\n"
	longComments := mustComments(t, long)
	tight := mustGoContextLimits(t, long, longComments[0], contracts.Limits{Lines: 40, ExtraBytes: 32})
	extra := 0
	if longComments[0].Start.Offset > tight.Start.Offset {
		extra += longComments[0].Start.Offset - tight.Start.Offset
	}
	if tight.End.Offset > longComments[0].End.Offset {
		extra += tight.End.Offset - longComments[0].End.Offset
	}
	if extra > 32 || !strings.Contains(tight.Text, "/* TARGET */") || strings.Contains(tight.Text, strings.Repeat("a", 200)) {
		t.Fatalf("extra %d text %q", extra, tight.Text)
	}
}

func mustComments(t *testing.T, src string) []types.Comment {
	t.Helper()
	comments, err := golang.Parse([]byte(src))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	return comments
}

func mustGoContext(t *testing.T, src string, comment types.Comment) types.Snippet {
	t.Helper()
	return mustGoContextLimits(t, src, comment, contracts.Limits{Lines: 40, ExtraBytes: 8192})
}

func mustGoContextLimits(t *testing.T, src string, comment types.Comment, limits contracts.Limits) types.Snippet {
	t.Helper()
	snip, err := golang.Context([]byte(src), comment.Start, comment.End, limits)
	if err != nil {
		t.Fatalf("Context: %v", err)
	}
	if snip.Text != src[snip.Start.Offset:snip.End.Offset] {
		t.Fatal("snippet text is not the source slice")
	}
	return snip
}

func snippetLineCount(text string) int {
	if text == "" {
		return 0
	}
	n := strings.Count(text, "\n")
	if strings.HasSuffix(text, "\n") {
		return n
	}
	return n + 1
}

func TestGoPresetSemantics(t *testing.T) {
	if !reflect.DeepEqual(golang.ConcretePresets(), []string{"go:generated", "go:tests", "go:vendor"}) {
		t.Fatalf("concrete = %q", golang.ConcretePresets())
	}
	if !reflect.DeepEqual(golang.Selectors(), []string{"go:all", "go:generated", "go:tests", "go:vendor"}) {
		t.Fatalf("selectors = %q", golang.Selectors())
	}
	expanded, ok := golang.Expand(golang.SelectorAll)
	if !ok || !reflect.DeepEqual(expanded, golang.ConcretePresets()) {
		t.Fatalf("Expand(go:all) = %q %v", expanded, ok)
	}
	for _, id := range expanded {
		if id == golang.SelectorAll {
			t.Fatal("go:all was concrete")
		}
	}
	if _, ok := golang.Expand("go:all"); !ok {
		t.Fatal("missing aggregate")
	}
	one, ok := golang.Expand(golang.PresetTests)
	if !ok || !reflect.DeepEqual(one, []string{golang.PresetTests}) {
		t.Fatalf("Expand(tests) = %q %v", one, ok)
	}
	if _, ok := golang.Expand("nope"); ok {
		t.Fatal("unknown selector expanded")
	}

	tests := []string{golang.PresetTests}
	if !golang.PathExcluded("a_test.go", tests) || !golang.PathExcluded("dir/a_test.go", tests) {
		t.Fatal("tests preset missed _test.go")
	}
	if golang.PathExcluded("a.go", tests) || golang.PathExcluded("test.go", tests) || golang.PathExcluded("a_TEST.go", tests) || golang.PathExcluded("notes_test.md", tests) {
		t.Fatal("tests preset matched the wrong path")
	}
	vendor := []string{golang.PresetVendor}
	if !golang.PathExcluded("vendor/a.go", vendor) || !golang.PathExcluded("pkg/vendor/a.go", vendor) {
		t.Fatal("vendor preset missed a vendor element")
	}
	if golang.PathExcluded("vendorized/a.go", vendor) || golang.PathExcluded("pkg/myvendor/a.go", vendor) || golang.PathExcluded("vendor/readme.md", vendor) {
		t.Fatal("vendor preset matched the wrong path")
	}

	generated := []byte("// Code generated by nodex-test. DO NOT EDIT.\n\npackage p\n")
	words := []byte("package p\n\nvar s = \"Code generated by x. DO NOT EDIT.\"\n// generated\n// DO NOT EDIT\n")
	gen := []string{golang.PresetGenerated}
	if golang.SourceExcluded("gen.go", generated, nil) || golang.SourceExcluded("gen.go", generated, tests) {
		t.Fatal("generated classification ran without its preset")
	}
	if !golang.SourceExcluded("plain.go", generated, gen) {
		t.Fatal("canonical generated source was included")
	}
	if golang.SourceExcluded("zz_generated.go", words, gen) || golang.SourceExcluded("notes.md", generated, gen) {
		t.Fatal("generated classification used a name or unrelated words")
	}
	if golang.PathExcluded("zz_generated.go", gen) {
		t.Fatal("generated preset used a path heuristic")
	}
}

func mustParse(t *testing.T, src string) *ast.File {
	t.Helper()
	_, file := mustParseFile(t, src)
	return file
}

func mustParseFile(t *testing.T, src string) (*token.FileSet, *ast.File) {
	t.Helper()
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "sample.go", src, parser.ParseComments|parser.SkipObjectResolution)
	if err != nil {
		t.Fatalf("parser: %v", err)
	}
	return fset, file
}

func TestNewSuppliesCompleteLanguageContract(t *testing.T) {
	var implementation contracts.Language = golang.New()
	if implementation.ID() != "go" || !implementation.Recognize("dir/a.go") || implementation.Recognize("a.GO") || implementation.Recognize("a.txt") {
		t.Fatal("Go identity or recognition changed")
	}
	sources := []string{
		"package p\n",
		"// Package p.\r\npackage p\r\n\r\n// F docs.\r\nfunc  F( ){return}\r\n",
		"package p\n\n//line other.go:400:1\n//go:generate tool\nfunc F() {} // trail\n",
		"package p\n\n// Group\nconst (\n// A\nA = 1\nB = 2\n)\n\ntype T struct {\n// Named\nNamed int\nU\n}\n",
		"package p\n\nfunc F() {\n// inside\n" + strings.Repeat("_ = 1\n", 100) + "}\n",
	}
	limits := contracts.Limits{Lines: 40, ExtraBytes: 8192}
	for _, source := range sources {
		body := []byte(source)
		comments, declarations, err := golang.ParseFile(body)
		if err != nil {
			t.Fatal(err)
		}
		doc, err := implementation.Parse(body)
		if err != nil || doc == nil || !reflect.DeepEqual(doc.Comments, comments) || !reflect.DeepEqual(doc.Declarations, declarations) {
			t.Fatalf("contract parse differs from parser facts: %+v, %v", doc, err)
		}
		for _, comment := range comments {
			want, err := golang.Context(body, comment.Start, comment.End, limits)
			got, gotErr := implementation.Context(body, types.Range{Start: comment.Start, End: comment.End}, limits)
			if err != nil || gotErr != nil || got != want {
				t.Fatalf("contract comment context = %+v, %v; parser = %+v, %v", got, gotErr, want, err)
			}
		}
		for _, declaration := range declarations {
			if declaration.Docs == nil || len(declaration.Docs) > 1 {
				t.Fatalf("Go direct docs must be a non-nil 0..1 collection: %+v", declaration)
			}
			want, err := golang.DeclarationContext(body, declaration.Start, declaration.End, limits)
			got, gotErr := implementation.DeclarationContext(body, types.Range{Start: declaration.Start, End: declaration.End}, limits)
			if err != nil || gotErr != nil || got != want {
				t.Fatalf("contract declaration context = %+v, %v; parser = %+v, %v", got, gotErr, want, err)
			}
		}
	}
	for _, source := range [][]byte{nil, []byte("package p\n//line other.go:400:1\nfunc (\n")} {
		doc, err := implementation.Parse(source)
		var failure *contracts.ParseError
		if doc != nil || !errors.As(err, &failure) || len(failure.Diagnostics) == 0 {
			t.Fatalf("contract malformed parse = %v, %v", doc, err)
		}
		_, parserErr := golang.Parse(source)
		var parserFailure *golang.Error
		if !errors.As(parserErr, &parserFailure) || !reflect.DeepEqual(failure.Diagnostics, parserFailure.Diagnostics) {
			t.Fatalf("contract diagnostics differ: %v / %v", failure, parserErr)
		}
	}
	catalog := implementation.Presets()
	if !reflect.DeepEqual(catalog.Concrete, golang.ConcretePresets()) ||
		!reflect.DeepEqual(catalog.Aggregates, []contracts.Aggregate{{ID: "go:all", Presets: golang.ConcretePresets()}}) {
		t.Fatalf("contract preset catalog = %+v", catalog)
	}
	catalog.Concrete[0] = "changed"
	catalog.Aggregates[0].Presets[0] = "changed"
	if next := implementation.Presets(); !reflect.DeepEqual(next.Concrete, golang.ConcretePresets()) || !reflect.DeepEqual(next.Aggregates[0].Presets, next.Concrete) {
		t.Fatal("contract catalog is not a fresh result")
	}
	generated := []byte("// Code generated by tool. DO NOT EDIT.\n\npackage p\n")
	for _, path := range []string{"a.go", "a_test.go", "vendor/a.go", "pkg/vendor/a.go", "vendorized/a.go", "a.GO", "a.txt"} {
		for _, enabled := range [][]string{nil, {"go:tests"}, {"go:vendor"}, {"go:generated"}, golang.ConcretePresets()} {
			if implementation.PathExcluded(path, enabled) != golang.PathExcluded(path, enabled) || implementation.SourceExcluded(path, generated, enabled) != golang.SourceExcluded(path, generated, enabled) {
				t.Fatalf("contract preset behavior differs for %s, %q", path, enabled)
			}
		}
	}
}
