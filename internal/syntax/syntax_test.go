package syntax_test

import (
	"errors"
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

	"github.com/edalca/nodex/internal/syntax"
)

func TestRecognize(t *testing.T) {
	supported := []string{
		"foo.go",
		"Foo.go",
		"foo_test.go",
		"foo_linux.go",
		"foo_windows.go",
		"vendor/example.go",
		"vendor/foo_test.go",
		"a/b/c.go",
		".go",
	}
	for _, path := range supported {
		lang, ok := syntax.Recognize(path)
		if !ok || lang != syntax.Go {
			t.Errorf("Recognize(%q) = (%q, %v), want go", path, lang, ok)
		}
	}

	unsupported := []string{
		"foo.rs",
		"foo.py",
		"README.md",
		"foo.GO",
		"foo.Go",
		"foo.gO",
		"foo.go.bak",
		"foo.go.txt",
		"dir.go/readme",
		"",
		"go",
	}
	for _, path := range unsupported {
		lang, ok := syntax.Recognize(path)
		if ok || lang != "" {
			t.Errorf("Recognize(%q) = (%q, %v), want unsupported", path, lang, ok)
		}
	}
}

func TestGoNamesStaySupported(t *testing.T) {
	src := []byte("package p\n")
	paths := []string{
		"foo.go",
		"foo_test.go",
		"foo_linux.go",
		"foo_windows.go",
		"vendor/example.go",
		"vendor/foo_linux_test.go",
	}
	for _, path := range paths {
		doc, err := syntax.Parse(path, src)
		if err != nil || doc == nil {
			t.Fatalf("Parse(%q): %v", path, err)
		}
		if doc.Language != syntax.Go || doc.Path != path {
			t.Fatalf("Parse(%q) = language %q path %q", path, doc.Language, doc.Path)
		}
	}
}

func TestUnsupportedPathIgnoresContents(t *testing.T) {
	samples := []string{
		"package p\n",
		"this is not go",
		"",
	}
	for _, src := range samples {
		for _, path := range []string{"foo.rs", "foo.py", "README.md", "foo.GO"} {
			doc, err := syntax.Parse(path, []byte(src))
			if doc != nil {
				t.Fatalf("Parse(%q) returned a document", path)
			}
			var unsupported *syntax.UnsupportedError
			if !errors.As(err, &unsupported) || unsupported.Path != path {
				t.Fatalf("Parse(%q) error = %v, want UnsupportedError for that path", path, err)
			}
			var parseErr *syntax.ParseError
			if errors.As(err, &parseErr) {
				t.Fatalf("Parse(%q) error = %v, want the path rejected before parsing", path, err)
			}
		}
	}
}

func TestParseUsesCallerBytes(t *testing.T) {
	doc, err := syntax.Parse("no/such/dir/file.go", []byte("package p\n"))
	if err != nil || doc == nil {
		t.Fatalf("Parse of absent path with bytes: %v", err)
	}
	if doc.Path != "no/such/dir/file.go" || len(doc.Comments) != 0 {
		t.Fatalf("document = %+v", doc)
	}
}

func TestNilSource(t *testing.T) {
	doc, err := syntax.Parse("empty.go", nil)
	if doc != nil {
		t.Fatal("nil source returned a document")
	}
	var parseErr *syntax.ParseError
	if !errors.As(err, &parseErr) || parseErr.Path != "empty.go" {
		t.Fatalf("nil source error = %v", err)
	}

	doc, err = syntax.Parse("empty.rs", nil)
	if doc != nil {
		t.Fatal("unsupported nil source returned a document")
	}
	var unsupported *syntax.UnsupportedError
	if !errors.As(err, &unsupported) || unsupported.Path != "empty.rs" {
		t.Fatalf("unsupported nil source error = %v", err)
	}
}

func TestStringIsNotAComment(t *testing.T) {
	src := "package sample\n\nvar s = \"// not a comment\"\n\n// real comment\nfunc F() {}\n"
	doc, err := syntax.Parse("sample.go", []byte(src))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(doc.Comments) != 1 {
		t.Fatalf("comments = %d, want 1\n%#v", len(doc.Comments), doc.Comments)
	}
	got := doc.Comments[0]
	if got.Text != "real comment" || got.Raw != "// real comment" {
		t.Fatalf("comment = raw %q text %q", got.Raw, got.Text)
	}
	if strings.Contains(got.Raw, "not a comment") || strings.Contains(got.Text, "not a comment") {
		t.Fatalf("string text was returned as a comment: %+v", got)
	}
	assertComments(t, src, doc.Comments, []wantComment{{
		raw:  "// real comment",
		text: "real comment",
	}})
}

func TestNormalizedTextKeepsGenerateDirective(t *testing.T) {
	src := "package p\n\n//go:generate stringer -type Op\nfunc F() {}\n"
	doc, err := syntax.Parse("gen.go", []byte(src))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(doc.Comments) != 1 {
		t.Fatalf("comments = %#v", doc.Comments)
	}
	if doc.Comments[0].Text != "go:generate stringer -type Op" {
		t.Fatalf("text = %q", doc.Comments[0].Text)
	}
	if !strings.Contains(doc.Comments[0].Text, "go:generate") {
		t.Fatalf("directive text missing from %q", doc.Comments[0].Text)
	}
}

func TestLineDirectiveKeepsPhysicalPositions(t *testing.T) {
	src := "package p\n\n//line foo.go:10:9\n\n// after\nvar x int\n"
	doc, err := syntax.Parse("real.go", []byte(src))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if doc.Path != "real.go" {
		t.Fatalf("path = %q", doc.Path)
	}
	assertComments(t, src, doc.Comments, []wantComment{
		{raw: "//line foo.go:10:9", text: "line foo.go:10:9"},
		{raw: "// after", text: "after"},
	})
	start := doc.Comments[1].Range.Start
	if start.Line != 5 || start.Column != 1 {
		t.Fatalf("position = %d:%d, want 5:1", start.Line, start.Column)
	}
	if start.Line == 10 || start.Line == 11 || start.Column == 9 {
		t.Fatalf("position followed //line: %+v", start)
	}
}

func TestSyntaxErrorReturnsNoDocument(t *testing.T) {
	src := "package p\n\n// kept\nfunc (\n"
	doc, err := syntax.Parse("bad.go", []byte(src))
	if doc != nil {
		t.Fatalf("document = %+v", doc)
	}
	var parseErr *syntax.ParseError
	if !errors.As(err, &parseErr) || parseErr.Path != "bad.go" {
		t.Fatalf("error = %v", err)
	}
	if !strings.Contains(err.Error(), "bad.go") {
		t.Fatalf("error %q omits the logical path", err)
	}
}

func TestLineDirectiveSyntaxErrorStaysPhysical(t *testing.T) {
	src := "package p\n\n//line other.go:400:1\nfunc (\n"
	doc, err := syntax.Parse("bad.go", []byte(src))
	if doc != nil {
		t.Fatalf("document = %+v", doc)
	}
	var parseErr *syntax.ParseError
	if !errors.As(err, &parseErr) || parseErr.Path != "bad.go" {
		t.Fatalf("error = %v", err)
	}
	msg := err.Error()
	if strings.Contains(msg, "other.go") || strings.Contains(msg, "400") {
		t.Fatalf("error uses //line adjustment: %s", msg)
	}
	if !strings.Contains(msg, "bad.go:4:8:") {
		t.Fatalf("error = %s, want physical bad.go:4:8", msg)
	}
}

func TestColumnCountsBytes(t *testing.T) {
	src := "package p\n\nvar µ = 1 // c\n"
	doc, err := syntax.Parse("wide.go", []byte(src))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	assertComments(t, src, doc.Comments, []wantComment{{raw: "// c", text: "c"}})
	start := doc.Comments[0].Range.Start.Offset
	lineStart := strings.LastIndex(src[:start], "\n") + 1
	runeCol := 1
	for _, r := range src[lineStart:start] {
		_ = r
		runeCol++
	}
	byteCol := doc.Comments[0].Range.Start.Column
	if byteCol == runeCol {
		t.Fatalf("column %d does not distinguish bytes from runes", byteCol)
	}
	if byteCol != start-lineStart+1 {
		t.Fatalf("column = %d, want byte column %d", byteCol, start-lineStart+1)
	}
}

func TestParseComments(t *testing.T) {
	tests := []struct {
		name string
		src  string
		want []wantComment
	}{
		{
			name: "no comments",
			src:  "package p\n\nfunc F() int { return 1 }\n",
		},
		{
			name: "line comment",
			src:  "package p\n\n// only\n",
			want: []wantComment{{raw: "// only", text: "only"}},
		},
		{
			name: "block comment",
			src:  "package p\n\n/* only */\n",
			want: []wantComment{{raw: "/* only */", text: " only "}},
		},
		{
			name: "consecutive line comments",
			src:  "package p\n\n// Service manages the graph.\n// It is safe for concurrent use.\nfunc F() {}\n",
			want: []wantComment{{
				raw:  "// Service manages the graph.\n// It is safe for concurrent use.",
				text: "Service manages the graph.\nIt is safe for concurrent use.",
			}},
		},
		{
			name: "separated groups",
			src:  "package p\n\n// one\n\n// two\nfunc F() {}\n",
			want: []wantComment{
				{raw: "// one", text: "one"},
				{raw: "// two", text: "two"},
			},
		},
		{
			name: "package comment and godoc",
			src:  "// Package p is documented.\npackage p\n\n// F is documented.\nfunc F() {}\n",
			want: []wantComment{
				{raw: "// Package p is documented.", text: "Package p is documented."},
				{raw: "// F is documented.", text: "F is documented."},
			},
		},
		{
			name: "trailing comment",
			src:  "package p\n\nfunc F() {} // trailing\n",
			want: []wantComment{{raw: "// trailing", text: "trailing"}},
		},
		{
			name: "comment inside function",
			src:  "package p\n\nfunc F() {\n\t// inside\n\tx := 1\n\t_ = x\n}\n",
			want: []wantComment{{raw: "// inside", text: "inside"}},
		},
		{
			name: "struct field comments",
			src:  "package p\n\ntype T struct {\n\t// field doc\n\tA int // field line\n}\n",
			want: []wantComment{
				{raw: "// field doc", text: "field doc"},
				{raw: "// field line", text: "field line"},
			},
		},
		{
			name: "interface comments",
			src:  "package p\n\ntype I interface {\n\t// method doc\n\tM() // method line\n}\n",
			want: []wantComment{
				{raw: "// method doc", text: "method doc"},
				{raw: "// method line", text: "method line"},
			},
		},
		{
			name: "constant and variable comments",
			src:  "package p\n\n// K documents the constant.\nconst K = 1\n\n// V documents the variable.\nvar V = 1\n",
			want: []wantComment{
				{raw: "// K documents the constant.", text: "K documents the constant."},
				{raw: "// V documents the variable.", text: "V documents the variable."},
			},
		},
		{
			name: "strings are not comments",
			src:  "package p\n\nvar s = \"/* not */\"\nvar r = `// neither\n/* nor */`\n// real\nfunc F() {}\n",
			want: []wantComment{{raw: "// real", text: "real"}},
		},
		{
			name: "mixed line and block group",
			src:  "package p\n\n// line\n/* block */\nfunc F() {}\n",
			want: []wantComment{{
				raw:  "// line\n/* block */",
				text: "line\n block ",
			}},
		},
		{
			name: "same-line block and line",
			src:  "package p\n\n/* a */ // b\nfunc F() {}\n",
			want: []wantComment{{
				raw:  "/* a */ // b",
				text: " a \nb",
			}},
		},
		{
			name: "lexical order",
			src:  "package p\n\nfunc F() { // trail\n\t// inside\n}\n",
			want: []wantComment{
				{raw: "// trail", text: "trail"},
				{raw: "// inside", text: "inside"},
			},
		},
		{
			name: "directives",
			src:  "//go:build linux\n\npackage p\n\n//go:generate stringer -type Op\n//go:noinline\nfunc F() {}\n",
			want: []wantComment{
				{raw: "//go:build linux", text: "go:build linux"},
				{
					raw:  "//go:generate stringer -type Op\n//go:noinline",
					text: "go:generate stringer -type Op\ngo:noinline",
				},
			},
		},
		{
			name: "line directive content",
			src:  "package p\n\n//line foo.go:10\nfunc F() {}\n",
			want: []wantComment{{raw: "//line foo.go:10", text: "line foo.go:10"}},
		},
		{
			name: "generated marker stays",
			src:  "// Code generated by tool. DO NOT EDIT.\n\npackage p\nfunc F() {}\n",
			want: []wantComment{{
				raw:  "// Code generated by tool. DO NOT EDIT.",
				text: "Code generated by tool. DO NOT EDIT.",
			}},
		},
		{
			name: "build ignore stays",
			src:  "//go:build ignore\n\npackage p\n",
			want: []wantComment{{raw: "//go:build ignore", text: "go:build ignore"}},
		},
		{
			name: "windows build tag stays",
			src:  "//go:build windows\n\npackage p\n",
			want: []wantComment{{raw: "//go:build windows", text: "go:build windows"}},
		},
		{
			name: "blank line inside group",
			src:  "package p\n\n//\n// text\nfunc F() {}\n",
			want: []wantComment{{raw: "//\n// text", text: "\ntext"}},
		},
		{
			name: "trailing spaces",
			src:  "package p\n\n// foo  \nfunc F() {}\n",
			want: []wantComment{{raw: "// foo  ", text: "foo  "}},
		},
		{
			name: "block comment markers stay in the text",
			src:  "package p\n\n/*\n * Service\n * manages\n */\nfunc F() {}\n",
			want: []wantComment{{
				raw:  "/*\n * Service\n * manages\n */",
				text: "\n * Service\n * manages\n ",
			}},
		},
		{
			name: "lf line comment",
			src:  "package p\n\n// hi\n",
			want: []wantComment{{raw: "// hi", text: "hi"}},
		},
		{
			name: "crlf line comment",
			src:  "package p\r\n\r\n// hi\r\n",
			want: []wantComment{{raw: "// hi\r", text: "hi"}},
		},
		{
			name: "crlf block comment",
			src:  "package p\r\n\r\n/* a\r\nb */\r\n",
			want: []wantComment{{raw: "/* a\r\nb */", text: " a\nb "}},
		},
		{
			name: "crlf comment group",
			src:  "package p\r\n\r\n// a\r\n// b\r\n",
			want: []wantComment{{raw: "// a\r\n// b\r", text: "a\nb"}},
		},
		{
			name: "comment at eof",
			src:  "package p\n\n// eof",
			want: []wantComment{{raw: "// eof", text: "eof"}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doc, err := syntax.Parse("sample.go", []byte(tt.src))
			if err != nil {
				t.Fatalf("Parse: %v", err)
			}
			if doc.Path != "sample.go" || doc.Language != syntax.Go {
				t.Fatalf("document path %q language %q", doc.Path, doc.Language)
			}
			if doc.Comments == nil {
				t.Fatal("comments slice is nil")
			}
			assertComments(t, tt.src, doc.Comments, tt.want)
		})
	}
}

func TestRepeatedParseIsDeterministic(t *testing.T) {
	src := []byte("package p\n\n// one\n// two\n\n/* three */\nfunc F() {}\n")
	first, err := syntax.Parse("sample.go", src)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	for i := 0; i < 8; i++ {
		again, err := syntax.Parse("sample.go", src)
		if err != nil {
			t.Fatalf("Parse %d: %v", i, err)
		}
		if !reflect.DeepEqual(first, again) {
			t.Fatalf("parse %d = %+v, want %+v", i, again, first)
		}
	}
}

func TestConcurrentParse(t *testing.T) {
	type job struct {
		path string
		src  string
		fail bool
	}
	jobs := []job{
		{path: "a.go", src: "package p\n\n// one\nfunc F() {}\n"},
		{path: "b.go", src: "package p\r\n\r\n// hi\r\n"},
		{path: "c.go", src: "package p\nfunc (\n", fail: true},
		{path: "d.rs", src: "package p\n", fail: true},
	}
	golden := make([]any, len(jobs))
	for i, job := range jobs {
		doc, err := syntax.Parse(job.path, []byte(job.src))
		golden[i] = doc
		if job.fail {
			golden[i] = err.Error()
			if doc != nil {
				t.Fatalf("seed %s returned a document", job.path)
			}
		} else if err != nil {
			t.Fatalf("seed %s: %v", job.path, err)
		}
	}

	var wg sync.WaitGroup
	errCh := make(chan error, 32)
	for n := 0; n < 32; n++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			job := jobs[n%len(jobs)]
			doc, err := syntax.Parse(job.path, []byte(job.src))
			if job.fail {
				if doc != nil || err == nil || err.Error() != golden[n%len(jobs)] {
					errCh <- errors.New(job.path)
				}
				return
			}
			if !reflect.DeepEqual(doc, golden[n%len(jobs)]) {
				errCh <- errors.New(job.path)
			}
		}(n)
	}
	wg.Wait()
	close(errCh)
	for err := range errCh {
		t.Errorf("concurrent parse mismatch: %v", err)
	}
}

func TestFacadeImportsStayInsideTheRoot(t *testing.T) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("locate syntax test")
	}
	dir := filepath.Dir(file)
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read dir: %v", err)
	}
	forbidden := []string{
		"go/ast",
		"go/token",
		"go/parser",
		"go/types",
		"go/packages",
		"golang.org/x/tools",
		"github.com/edalca/nodex/internal/project",
		"github.com/edalca/nodex/internal/ignore",
	}
	var sawGo bool
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
			path, err := strconv.Unquote(spec.Path.Value)
			if err != nil {
				t.Fatalf("import in %s: %v", name, err)
			}
			if path == "github.com/edalca/nodex/internal/syntax/golang" {
				sawGo = true
			}
			for _, bad := range forbidden {
				if path == bad || strings.HasPrefix(path, bad+"/") {
					t.Errorf("%s imports %s", name, path)
				}
			}
		}
	}
	if !sawGo {
		t.Fatal("syntax facade does not import its Go implementation")
	}
}

type wantComment struct {
	raw  string
	text string
}

func assertComments(t *testing.T, src string, got []syntax.Comment, want []wantComment) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("comments = %d, want %d\n%#v", len(got), len(want), got)
	}
	searchFrom := 0
	var previousEnd int
	for i, w := range want {
		c := got[i]
		if c.Raw != w.raw || c.Text != w.text {
			t.Fatalf("comment %d = raw %q text %q, want raw %q text %q", i, c.Raw, c.Text, w.raw, w.text)
		}
		at := strings.Index(src[searchFrom:], w.raw)
		if at < 0 {
			t.Fatalf("comment %d raw %q is not in the source", i, w.raw)
		}
		at += searchFrom
		start, end := c.Range.Start, c.Range.End
		if start.Offset != at || end.Offset != at+len(w.raw) {
			t.Fatalf("comment %d range [%d,%d), want [%d,%d)", i, start.Offset, end.Offset, at, at+len(w.raw))
		}
		if end.Offset-start.Offset != len(c.Raw) || src[start.Offset:end.Offset] != c.Raw {
			t.Fatalf("comment %d range does not slice to its raw text", i)
		}
		if i > 0 && start.Offset < previousEnd {
			t.Fatalf("comment %d starts at %d before previous end %d", i, start.Offset, previousEnd)
		}
		assertPosition(t, src, start)
		assertPosition(t, src, end)
		previousEnd = end.Offset
		searchFrom = end.Offset
	}
}

func assertPosition(t *testing.T, src string, pos syntax.Position) {
	t.Helper()
	if pos.Offset < 0 || pos.Offset > len(src) {
		t.Fatalf("offset %d outside source length %d", pos.Offset, len(src))
	}
	line, col := 1, 1
	for i := 0; i < pos.Offset; i++ {
		if src[i] == '\n' {
			line++
			col = 1
			continue
		}
		col++
	}
	if pos.Line != line || pos.Column != col {
		t.Fatalf("position %d:%d at offset %d, physical %d:%d", pos.Line, pos.Column, pos.Offset, line, col)
	}
}

func TestContextStructuralSelection(t *testing.T) {
	cases := []struct {
		name    string
		src     string
		raw     string
		want    string
		absent  []string
		present []string
	}{
		{
			name:   "function godoc",
			src:    "package p\n\n// F adds numbers.\nfunc F(a, b int) int {\n\treturn a + b\n}\n",
			raw:    "// F adds numbers.",
			want:   "// F adds numbers.\nfunc F(a, b int) int {\n\treturn a + b\n}",
			absent: []string{"package p"},
		},
		{
			name:   "method godoc",
			src:    "package p\n\ntype T int\n\n// M doubles.\nfunc (t T) M() int {\n\treturn int(t)\n}\n",
			raw:    "// M doubles.",
			want:   "// M doubles.\nfunc (t T) M() int {\n\treturn int(t)\n}",
			absent: []string{"type T int"},
		},
		{
			name:   "type godoc",
			src:    "package p\n\n// T holds a value.\ntype T struct {\n\tA int\n}\n",
			raw:    "// T holds a value.",
			want:   "// T holds a value.\ntype T struct {\n\tA int\n}",
			absent: []string{"package p"},
		},
		{
			name: "const godoc",
			src:  "package p\n\n// N is a number.\nconst N = 3\n",
			raw:  "// N is a number.",
			want: "// N is a number.\nconst N = 3",
		},
		{
			name: "var godoc",
			src:  "package p\n\n// count is a count.\nvar count = 1\n",
			raw:  "// count is a count.",
			want: "// count is a count.\nvar count = 1",
		},
		{
			name:   "struct field",
			src:    "package p\n\ntype T struct {\n\t// A is a.\n\tA int\n}\n",
			raw:    "// A is a.",
			want:   "type T struct {\n\t// A is a.\n\tA int\n}",
			absent: []string{"package p"},
		},
		{
			name: "struct field trail",
			src:  "package p\n\ntype T struct {\n\tA int // trail\n}\n",
			raw:  "// trail",
			want: "type T struct {\n\tA int // trail\n}",
		},
		{
			name: "interface field",
			src:  "package p\n\ntype I interface {\n\t// M is m.\n\tM()\n}\n",
			raw:  "// M is m.",
			want: "type I interface {\n\t// M is m.\n\tM()\n}",
		},
		{
			name: "interface trail",
			src:  "package p\n\ntype I interface {\n\tM() // method line\n}\n",
			raw:  "// method line",
			want: "type I interface {\n\tM() // method line\n}",
		},
		{
			name:   "inside function",
			src:    "package p\n\nfunc F() {\n\t// inside\n\tx := 1\n\t_ = x\n}\n",
			raw:    "// inside",
			want:   "func F() {\n\t// inside\n\tx := 1\n\t_ = x\n}",
			absent: []string{"package p"},
		},
		{
			name:   "trailing function",
			src:    "package p\n\nfunc F() {} // trailing\n",
			raw:    "// trailing",
			want:   "func F() {} // trailing",
			absent: []string{"package p"},
		},
		{
			name:   "package comment",
			src:    "// Package p documents p.\npackage p\n\nfunc F() {}\n",
			raw:    "// Package p documents p.",
			want:   "// Package p documents p.\npackage p\n",
			absent: []string{"func F"},
		},
		{
			name:   "package trail",
			src:    "package p // pkg trail\n\nfunc F() {}\n",
			raw:    "// pkg trail",
			want:   "package p // pkg trail\n",
			absent: []string{"func F"},
		},
		{
			name:   "const inside function",
			src:    "package p\n\nfunc F() {\n\t// documented\n\tconst x = 1\n\t_ = x\n}\n",
			raw:    "// documented",
			want:   "// documented\n\tconst x = 1",
			absent: []string{"func F", "_ = x"},
		},
		{
			name:   "type inside function",
			src:    "package p\n\nfunc F() {\n\ttype T struct {\n\t\t// field\n\t\tA int\n\t}\n\t_ = T{}\n}\n",
			raw:    "// field",
			want:   "type T struct {\n\t\t// field\n\t\tA int\n\t}",
			absent: []string{"func F", "_ = T{}"},
		},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			buf := []byte(tt.src)
			before := append([]byte(nil), buf...)
			comment := commentByRaw(t, tt.src, "sample.go", tt.raw)
			snip := mustSnippet(t, "sample.go", buf, comment)
			if string(buf) != string(before) {
				t.Fatal("Context modified the caller bytes")
			}
			if snip.Text != tt.want {
				t.Fatalf("snippet = %q, want %q", snip.Text, tt.want)
			}
			for _, part := range tt.absent {
				if strings.Contains(snip.Text, part) {
					t.Fatalf("snippet contains %q: %q", part, snip.Text)
				}
			}
			for _, part := range tt.present {
				if !strings.Contains(snip.Text, part) {
					t.Fatalf("snippet lacks %q: %q", part, snip.Text)
				}
			}
			again := mustSnippet(t, "sample.go", buf, comment)
			if !reflect.DeepEqual(snip, again) {
				t.Fatalf("repeat = %+v, want %+v", again, snip)
			}
		})
	}
}

func TestContextUnassociatedNeighborhood(t *testing.T) {
	small := "package p\n\n// NOTE\n\nfunc F() {}\n"
	comment := commentByRaw(t, small, "sample.go", "// NOTE")
	snip := mustSnippet(t, "sample.go", []byte(small), comment)
	if snip.Text != small {
		t.Fatalf("small neighborhood = %q, want the whole file", snip.Text)
	}

	var b strings.Builder
	b.WriteString("package p\n\n// NOTE\n\nfunc F() {\n")
	for i := 0; i < 80; i++ {
		b.WriteString("\t_ = ")
		b.WriteString(strconv.Itoa(i))
		b.WriteByte('\n')
	}
	b.WriteString("}\n")
	src := b.String()
	comment = commentByRaw(t, src, "sample.go", "// NOTE")
	snip = mustSnippet(t, "sample.go", []byte(src), comment)
	if !strings.Contains(snip.Text, "package p") || !strings.Contains(snip.Text, "// NOTE") {
		t.Fatalf("neighborhood = %q", snip.Text)
	}
	if strings.Contains(snip.Text, "_ = 79") || snip.Text == src {
		t.Fatalf("neighborhood is not bounded: %q", snip.Text)
	}
	if n := snippetLines(snip.Text); n > syntax.MaxContextLines {
		t.Fatalf("neighborhood lines = %d", n)
	}
}

func TestContextBoundsLargeFunction(t *testing.T) {
	var b strings.Builder
	b.WriteString("package p\n\nfunc F() {\n")
	for i := 0; i < 80; i++ {
		if i == 40 {
			b.WriteString("\t// TARGET\n")
			continue
		}
		b.WriteString("\t_ = ")
		b.WriteString(strconv.Itoa(i))
		b.WriteByte('\n')
	}
	b.WriteString("}\n")
	src := b.String()
	comment := commentByRaw(t, src, "big.go", "// TARGET")
	buf := []byte(src)
	snip := mustSnippet(t, "big.go", buf, comment)
	if snip.Text == src || strings.Contains(snip.Text, "func F()") || strings.Contains(snip.Text, "_ = 79") || strings.Contains(snip.Text, "package p") {
		t.Fatalf("complete function returned:\n%s", snip.Text)
	}
	if !strings.Contains(snip.Text, "// TARGET") {
		t.Fatalf("snippet = %q", snip.Text)
	}
	if n := snippetLines(snip.Text); n != syntax.MaxContextLines {
		t.Fatalf("lines = %d, want %d\n%s", n, syntax.MaxContextLines, snip.Text)
	}
	if extraOutside(snip, comment) > syntax.MaxContextExtraBytes {
		t.Fatalf("extra bytes = %d", extraOutside(snip, comment))
	}
}

func TestContextBoundsLongLine(t *testing.T) {
	src := "package p\n\nfunc F() { /* TARGET */ s := \"" + strings.Repeat("a", 20000) + "\" }\n"
	comment := commentByRaw(t, src, "long.go", "/* TARGET */")
	snip := mustSnippet(t, "long.go", []byte(src), comment)
	if strings.Contains(snip.Text, "package p") {
		t.Fatalf("snippet left the function: %q", snip.Text[:80])
	}
	if !strings.Contains(snip.Text, "func F()") || !strings.Contains(snip.Text, "/* TARGET */") {
		t.Fatalf("snippet = %q", snip.Text)
	}
	if strings.HasSuffix(snip.Text, "}") || strings.Contains(snip.Text, strings.Repeat("a", 10000)) {
		t.Fatal("snippet kept the pathological line")
	}
	if extra := extraOutside(snip, comment); extra > syntax.MaxContextExtraBytes {
		t.Fatalf("extra bytes = %d", extra)
	}
	if snippetLines(snip.Text) != 1 {
		t.Fatalf("lines = %d", snippetLines(snip.Text))
	}
	again, err := syntax.Context("long.go", []byte(src), comment.Range)
	if err != nil || !reflect.DeepEqual(snip, again) {
		t.Fatalf("repeat = %+v err %v", again, err)
	}
}

func TestContextCapsCommentLongerThanTheLineLimit(t *testing.T) {
	var b strings.Builder
	b.WriteString("package p\n\n/*\n")
	for i := 0; i < 60; i++ {
		b.WriteString("line ")
		b.WriteString(strconv.Itoa(i))
		b.WriteByte('\n')
	}
	b.WriteString("*/\nfunc After() {}\n")
	src := b.String()
	doc, err := syntax.Parse("wide.go", []byte(src))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(doc.Comments) != 1 || !strings.HasPrefix(doc.Comments[0].Raw, "/*") {
		t.Fatalf("comments = %#v", doc.Comments)
	}
	comment := doc.Comments[0]
	snip, err := syntax.Context("wide.go", []byte(src), comment.Range)
	if err != nil {
		t.Fatalf("Context: %v", err)
	}
	if snip.Text != src[snip.Range.Start.Offset:snip.Range.End.Offset] {
		t.Fatal("snippet text is not the source slice")
	}
	if snip.Range.Start.Offset > comment.Range.Start.Offset || snip.Range.End.Offset <= comment.Range.Start.Offset {
		t.Fatalf("snippet [%d,%d) misses comment start %d", snip.Range.Start.Offset, snip.Range.End.Offset, comment.Range.Start.Offset)
	}
	if snip.Range.End.Offset >= comment.Range.End.Offset {
		t.Fatal("snippet kept a comment longer than the line limit")
	}
	if n := snippetLines(snip.Text); n > syntax.MaxContextLines {
		t.Fatalf("lines = %d", n)
	}
	if strings.Contains(snip.Text, "func After") || strings.Contains(snip.Text, "line 59") {
		t.Fatalf("snippet = %s", snip.Text)
	}
	if !strings.Contains(snip.Text, "line 0") {
		t.Fatalf("snippet = %s", snip.Text)
	}
}

func TestContextExactRangeMatching(t *testing.T) {
	src := "package p\n\n// only\nfunc F() {}\n"
	comment := commentByRaw(t, src, "sample.go", "// only")
	snip := mustSnippet(t, "no/such/dir/file.go", []byte(src), comment)
	if !strings.Contains(snip.Text, "// only") || !strings.Contains(snip.Text, "func F()") {
		t.Fatalf("snippet = %q", snip.Text)
	}

	shifted := comment.Range
	shifted.End = physicalPosition(src, comment.Range.End.Offset+1)
	_, err := syntax.Context("sample.go", []byte(src), shifted)
	if !errors.Is(err, syntax.ErrCommentNotFound) {
		t.Fatalf("shifted range error = %v", err)
	}
	var parseErr *syntax.ParseError
	if errors.As(err, &parseErr) {
		t.Fatalf("missing comment reported as a parse error: %v", err)
	}

	keyword := syntax.Range{
		Start: physicalPosition(src, 0),
		End:   physicalPosition(src, len("package")),
	}
	_, err = syntax.Context("sample.go", []byte(src), keyword)
	if !errors.Is(err, syntax.ErrCommentNotFound) {
		t.Fatalf("keyword range error = %v", err)
	}
}

func TestContextMalformedRange(t *testing.T) {
	src := "package p\n\n// only\nfunc F() {}\n"
	comment := commentByRaw(t, src, "sample.go", "// only")
	cases := []syntax.Range{
		{Start: syntax.Position{Offset: 4, Line: 1, Column: 5}, End: syntax.Position{Offset: 1, Line: 1, Column: 2}},
		{Start: syntax.Position{Offset: -1, Line: 1, Column: 1}, End: comment.Range.End},
		{Start: syntax.Position{Offset: 0, Line: 0, Column: 1}, End: syntax.Position{Offset: 1, Line: 1, Column: 2}},
		{Start: syntax.Position{Offset: 0, Line: 1, Column: 0}, End: syntax.Position{Offset: 1, Line: 1, Column: 2}},
		{Start: syntax.Position{Offset: 0, Line: 1, Column: 1}, End: syntax.Position{Offset: len(src) + 4, Line: 1, Column: 2}},
		{Start: syntax.Position{Offset: comment.Range.Start.Offset, Line: 99, Column: comment.Range.Start.Column}, End: comment.Range.End},
	}
	for _, r := range cases {
		snip, err := syntax.Context("sample.go", []byte(src), r)
		if snip.Text != "" || !errors.Is(err, syntax.ErrMalformedRange) {
			t.Fatalf("range %+v -> (%q, %v)", r, snip.Text, err)
		}
	}
}

func TestContextUnsupportedAndMalformedSource(t *testing.T) {
	src := []byte("package p\n\n// only\nfunc F() {}\n")
	comment := commentByRaw(t, string(src), "sample.go", "// only")
	for _, path := range []string{"notes.rs", "README.md", "foo.GO"} {
		snip, err := syntax.Context(path, src, comment.Range)
		if snip.Text != "" {
			t.Fatalf("Context(%q) returned %q", path, snip.Text)
		}
		var unsupported *syntax.UnsupportedError
		if !errors.As(err, &unsupported) || unsupported.Path != path {
			t.Fatalf("Context(%q) error = %v", path, err)
		}
		var parseErr *syntax.ParseError
		if errors.As(err, &parseErr) {
			t.Fatalf("Context(%q) parsed an unsupported path: %v", path, err)
		}
	}

	bad := []byte("package p\n\n// kept\nfunc (\n")
	r := syntax.Range{
		Start: physicalPosition(string(bad), 0),
		End:   physicalPosition(string(bad), len("package")),
	}
	snip, err := syntax.Context("bad.go", bad, r)
	if snip.Text != "" {
		t.Fatalf("malformed source returned %q", snip.Text)
	}
	var parseErr *syntax.ParseError
	if !errors.As(err, &parseErr) || parseErr.Path != "bad.go" {
		t.Fatalf("malformed source error = %v", err)
	}
	if errors.Is(err, syntax.ErrCommentNotFound) {
		t.Fatal("malformed source was treated as a missing comment")
	}
}

func TestContextLineDirectiveStaysPhysical(t *testing.T) {
	src := "package p\n\n//line foo.go:10:9\n\n// after\nvar x int\n"
	comment := commentByRaw(t, src, "real.go", "// after")
	if comment.Range.Start.Line != 5 {
		t.Fatalf("comment line = %d", comment.Range.Start.Line)
	}
	snip := mustSnippet(t, "real.go", []byte(src), comment)
	if snip.Range.Start.Line != 5 {
		t.Fatalf("context line = %d, want physical line 5; range %+v", snip.Range.Start.Line, snip.Range)
	}
	if strings.Contains(snip.Text, "//line") || strings.Contains(snip.Text, "package p") {
		t.Fatalf("var context = %q", snip.Text)
	}
	if !strings.Contains(snip.Text, "// after") || !strings.Contains(snip.Text, "var x int") {
		t.Fatalf("snippet = %q", snip.Text)
	}
}

func TestContextPreservesOriginalBytes(t *testing.T) {
	crlf := "package p\r\n\r\n// F docs.\r\nfunc F() {\r\n}\r\n"
	comment := commentByRaw(t, crlf, "crlf.go", "// F docs.\r")
	snip := mustSnippet(t, "crlf.go", []byte(crlf), comment)
	if !strings.Contains(snip.Text, "\r\n") || strings.Contains(strings.ReplaceAll(snip.Text, "\r\n", ""), "\r") {
		t.Fatalf("snippet normalized newlines: %q", snip.Text)
	}
	if snip.Text != "// F docs.\r\nfunc F() {\r\n}" {
		t.Fatalf("snippet = %q", snip.Text)
	}

	wide := "package p\n\n// naïve café\nfunc F() {\n\ts := \"café\"\n\t_ = s\n}\n"
	comment = commentByRaw(t, wide, "wide.go", "// naïve café")
	buf := []byte(wide)
	snip = mustSnippet(t, "wide.go", buf, comment)
	if !strings.Contains(snip.Text, "naïve café") || !strings.Contains(snip.Text, "s := \"café\"") {
		t.Fatalf("snippet = %q", snip.Text)
	}
	if snip.Text != wide[snip.Range.Start.Offset:snip.Range.End.Offset] {
		t.Fatal("UTF-8 snippet is not the original slice")
	}

	spaced := "package p\n\n// keep\nfunc  F( ){return}\n"
	comment = commentByRaw(t, spaced, "space.go", "// keep")
	snip = mustSnippet(t, "space.go", []byte(spaced), comment)
	if !strings.Contains(snip.Text, "func  F( ){return}") {
		t.Fatalf("snippet reformatted: %q", snip.Text)
	}
}

func commentByRaw(t *testing.T, src, path, raw string) syntax.Comment {
	t.Helper()
	doc, err := syntax.Parse(path, []byte(src))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	var found []syntax.Comment
	for _, c := range doc.Comments {
		if c.Raw == raw {
			found = append(found, c)
		}
	}
	if len(found) != 1 {
		t.Fatalf("raw %q matched %d comments", raw, len(found))
	}
	return found[0]
}

func mustSnippet(t *testing.T, path string, src []byte, comment syntax.Comment) syntax.Snippet {
	t.Helper()
	snip, err := syntax.Context(path, src, comment.Range)
	if err != nil {
		t.Fatalf("Context: %v", err)
	}
	if snip.Range.Start.Offset < 0 || snip.Range.End.Offset > len(src) || snip.Range.End.Offset < snip.Range.Start.Offset {
		t.Fatalf("snippet range %+v outside source", snip.Range)
	}
	if snip.Text != string(src[snip.Range.Start.Offset:snip.Range.End.Offset]) {
		t.Fatal("snippet text is not the source slice")
	}
	if snip.Range.Start.Offset > comment.Range.Start.Offset || snip.Range.End.Offset < comment.Range.End.Offset {
		t.Fatalf("snippet [%d,%d) does not contain comment [%d,%d)", snip.Range.Start.Offset, snip.Range.End.Offset, comment.Range.Start.Offset, comment.Range.End.Offset)
	}
	assertContextPosition(t, string(src), snip.Range.Start)
	assertContextPosition(t, string(src), snip.Range.End)
	return snip
}

// assertContextPosition checks a physical position. The position at the
// end of a newline-terminated file stays on the last line, one column past
// the newline, which is how the parser reports that offset.
func assertContextPosition(t *testing.T, src string, pos syntax.Position) {
	t.Helper()
	if pos.Offset == len(src) && len(src) > 0 && src[len(src)-1] == '\n' {
		prev := pos
		prev.Offset--
		prev.Column--
		assertPosition(t, src, prev)
		if pos.Line != prev.Line || pos.Column != prev.Column+1 {
			t.Fatalf("end position %+v, want one column past %+v", pos, prev)
		}
		return
	}
	assertPosition(t, src, pos)
}

func TestPresetCatalog(t *testing.T) {
	wantSelectors := []string{"go:all", "go:generated", "go:tests", "go:vendor"}
	wantConcrete := []string{"go:generated", "go:tests", "go:vendor"}
	if !reflect.DeepEqual(syntax.Selectors(), wantSelectors) {
		t.Fatalf("selectors = %q", syntax.Selectors())
	}
	if !reflect.DeepEqual(syntax.ConcretePresets(), wantConcrete) {
		t.Fatalf("concrete = %q", syntax.ConcretePresets())
	}
	expanded, err := syntax.ExpandSelector("go:all")
	if err != nil {
		t.Fatalf("expand: %v", err)
	}
	if !reflect.DeepEqual(expanded, syntax.ConcretePresets()) {
		t.Fatalf("go:all = %q, catalog %q", expanded, syntax.ConcretePresets())
	}
	for _, id := range expanded {
		if id == "go:all" {
			t.Fatal("aggregate selector was returned as a concrete preset")
		}
	}
	expanded[0] = "mutated"
	again, err := syntax.ExpandSelector("go:all")
	if err != nil || again[0] == "mutated" {
		t.Fatalf("expansion aliased the catalog: %q %v", again, err)
	}
	for _, id := range wantConcrete {
		got, err := syntax.ExpandSelector(id)
		if err != nil || !reflect.DeepEqual(got, []string{id}) {
			t.Fatalf("Expand(%s) = %q %v", id, got, err)
		}
	}
	for _, selector := range []string{"", "Go:tests", "go:all ", "not-a-preset"} {
		if _, err := syntax.ExpandSelector(selector); err == nil {
			t.Fatalf("Expand(%q) succeeded", selector)
		}
	}
	if err := syntax.ValidatePresets(nil); err != nil {
		t.Fatalf("empty validate: %v", err)
	}
	if err := syntax.ValidatePresets(append([]string{}, wantConcrete...)); err != nil {
		t.Fatalf("concrete validate: %v", err)
	}
	if err := syntax.ValidatePresets([]string{"go:all"}); err == nil {
		t.Fatal("persisted go:all was accepted")
	}
	if err := syntax.ValidatePresets([]string{"nope"}); err == nil {
		t.Fatal("unknown preset was accepted")
	}
}

func TestPresetExclusion(t *testing.T) {
	generated := []byte("// Code generated by nodex-test. DO NOT EDIT.\n\npackage p\n")
	ordinary := []byte("package p\n\nvar s = \"generated DO NOT EDIT\"\n// generated\n// DO NOT EDIT\n")
	tests := []string{"go:tests"}
	vendor := []string{"go:vendor"}
	gen := []string{"go:generated"}
	none := []string{}

	if syntax.PathExcluded("a_test.go", none) || syntax.PathExcluded("vendor/a.go", none) || syntax.InspectExcluded("gen.go", generated, none) {
		t.Fatal("disabled presets excluded a file")
	}
	if !syntax.PathExcluded("a_test.go", tests) || !syntax.PathExcluded("dir/a_test.go", tests) {
		t.Fatal("go:tests did not exclude _test.go")
	}
	if syntax.PathExcluded("a.go", tests) || syntax.PathExcluded("test.go", tests) || syntax.PathExcluded("a_TEST.go", tests) {
		t.Fatal("go:tests excluded a file that is not _test.go")
	}
	if !syntax.PathExcluded("vendor/a.go", vendor) || !syntax.PathExcluded("pkg/vendor/a.go", vendor) {
		t.Fatal("go:vendor did not exclude a vendor element")
	}
	if syntax.PathExcluded("vendorized/a.go", vendor) || syntax.PathExcluded("pkg/myvendor/a.go", vendor) || syntax.PathExcluded("Vendor/a.go", vendor) {
		t.Fatal("go:vendor matched a non-exact element")
	}
	if syntax.PathExcluded("vendor/readme.md", vendor) || syntax.InspectExcluded("vendor/readme.md", []byte("generated"), vendor) {
		t.Fatal("a preset excluded an unsupported path")
	}
	if syntax.InspectExcluded("gen.go", generated, tests) {
		t.Fatal("go:tests excluded a generated file by contents")
	}
	if !syntax.InspectExcluded("gen.go", generated, gen) {
		t.Fatal("canonical generated Go source was included")
	}
	if syntax.InspectExcluded("gen.go", generated, none) || syntax.InspectExcluded("noted.go", ordinary, gen) {
		t.Fatal("unrelated generated words excluded a file")
	}
	if syntax.InspectExcluded("zz_generated.go", ordinary, gen) {
		t.Fatal("a generated file name was treated as generated source")
	}
	broken := []byte("// Code generated by nodex-test. DO NOT EDIT.\n\npackage p\n\nfunc (\n")
	if !syntax.InspectExcluded("gen.go", broken, gen) {
		t.Fatal("generated classification required a complete program")
	}
}

func physicalPosition(src string, offset int) syntax.Position {
	line, col := 1, 1
	for i := 0; i < offset && i < len(src); i++ {
		if src[i] == '\n' {
			line++
			col = 1
			continue
		}
		col++
	}
	return syntax.Position{Offset: offset, Line: line, Column: col}
}

func snippetLines(text string) int {
	if text == "" {
		return 0
	}
	n := strings.Count(text, "\n")
	if strings.HasSuffix(text, "\n") {
		return n
	}
	return n + 1
}

func TestGoDeclarations(t *testing.T) {
	const sample = "" +
		"// Package sample documents sample.\n" +
		"package sample\n" +
		"\n" +
		"// F documents F.\n" +
		"func F() {}\n" +
		"\n" +
		"func g() {}\n" +
		"\n" +
		"const (\n" +
		"\t// A documents A.\n" +
		"\tA = 1\n" +
		"\tB = 2\n" +
		")\n" +
		"\n" +
		"// TGroup documents the type group.\n" +
		"type (\n" +
		"\tT struct {\n" +
		"\t\t// Name documents Name.\n" +
		"\t\tName string\n" +
		"\t\tHidden string\n" +
		"\t}\n" +
		")\n"
	doc := mustParseDoc(t, "sample.go", sample)
	if len(doc.Comments) != 5 {
		t.Fatalf("comments = %d", len(doc.Comments))
	}
	want := []struct {
		kind  syntax.Kind
		names []string
		doc   string
	}{
		{syntax.KindPackage, []string{"sample"}, "Package sample documents sample."},
		{syntax.KindFunction, []string{"F"}, "F documents F."},
		{syntax.KindFunction, []string{"g"}, ""},
		{syntax.KindConstGroup, []string{"A", "B"}, ""},
		{syntax.KindConst, []string{"A"}, "A documents A."},
		{syntax.KindConst, []string{"B"}, ""},
		{syntax.KindTypeGroup, []string{"T"}, "TGroup documents the type group."},
		{syntax.KindType, []string{"T"}, ""},
		{syntax.KindField, []string{"Name"}, "Name documents Name."},
		{syntax.KindField, []string{"Hidden"}, ""},
	}
	assertDecls(t, doc, want)

	doc = mustParseDoc(t, "one.go", "package one\n\n// X documents X.\nconst X = 1\n")
	assertDecls(t, doc, []struct {
		kind  syntax.Kind
		names []string
		doc   string
	}{
		{syntax.KindPackage, []string{"one"}, ""},
		{syntax.KindConst, []string{"X"}, "X documents X."},
	})

	doc = mustParseDoc(t, "vars.go", "package vars\n\nvar A, B int\n")
	assertDecls(t, doc, []struct {
		kind  syntax.Kind
		names []string
		doc   string
	}{
		{syntax.KindPackage, []string{"vars"}, ""},
		{syntax.KindVar, []string{"A", "B"}, ""},
	})

	doc = mustParseDoc(t, "group.go", "package group\n\nvar (\n\tA, B int\n\t// C documents C.\n\tC = 1\n)\n")
	assertDecls(t, doc, []struct {
		kind  syntax.Kind
		names []string
		doc   string
	}{
		{syntax.KindPackage, []string{"group"}, ""},
		{syntax.KindVarGroup, []string{"A", "B", "C"}, ""},
		{syntax.KindVar, []string{"A", "B"}, ""},
		{syntax.KindVar, []string{"C"}, "C documents C."},
	})

	doc = mustParseDoc(t, "method.go", "package method\n\ntype T struct{}\n\nfunc (T) M(a int) int { return a }\n")
	assertDecls(t, doc, []struct {
		kind  syntax.Kind
		names []string
		doc   string
	}{
		{syntax.KindPackage, []string{"method"}, ""},
		{syntax.KindType, []string{"T"}, ""},
		{syntax.KindMethod, []string{"M"}, ""},
	})

	doc = mustParseDoc(t, "iface.go", "package iface\n\ntype I interface {\n\tM(x int)\n\tU\n}\n")
	assertDecls(t, doc, []struct {
		kind  syntax.Kind
		names []string
		doc   string
	}{
		{syntax.KindPackage, []string{"iface"}, ""},
		{syntax.KindType, []string{"I"}, ""},
		{syntax.KindField, []string{"M"}, ""},
		{syntax.KindField, nil, ""},
	})

	doc = mustParseDoc(t, "embed.go", "package embed\n\ntype E struct {\n\tU\n\t*T\n}\n")
	assertDecls(t, doc, []struct {
		kind  syntax.Kind
		names []string
		doc   string
	}{
		{syntax.KindPackage, []string{"embed"}, ""},
		{syntax.KindType, []string{"E"}, ""},
		{syntax.KindField, nil, ""},
		{syntax.KindField, nil, ""},
	})
	for _, decl := range doc.Declarations {
		if decl.Names == nil {
			t.Fatalf("names slice is nil: %+v", decl)
		}
	}

	doc = mustParseDoc(t, "local.go", "package local\n\nfunc F(a int) (b int) {\n\tconst local = 1\n\ttype localT struct{ E int }\n\treturn a + b\n}\n")
	assertDecls(t, doc, []struct {
		kind  syntax.Kind
		names []string
		doc   string
	}{
		{syntax.KindPackage, []string{"local"}, ""},
		{syntax.KindFunction, []string{"F"}, ""},
		{syntax.KindConst, []string{"local"}, ""},
		{syntax.KindType, []string{"localT"}, ""},
		{syntax.KindField, []string{"E"}, ""},
	})

	doc = mustParseDoc(t, "trail.go", "package trail\n\ntype T struct {\n\t// Name documents Name.\n\tName string // trailing\n\tHidden int // also trailing\n}\n")
	if len(doc.Comments) != 3 {
		t.Fatalf("trailing comments = %+v", doc.Comments)
	}
	assertDecls(t, doc, []struct {
		kind  syntax.Kind
		names []string
		doc   string
	}{
		{syntax.KindPackage, []string{"trail"}, ""},
		{syntax.KindType, []string{"T"}, ""},
		{syntax.KindField, []string{"Name"}, "Name documents Name."},
		{syntax.KindField, []string{"Hidden"}, ""},
	})

	doc = mustParseDoc(t, "imports.go", "package imports\n\nimport \"fmt\"\n\nimport (\n\t\"io\"\n\talias \"os\"\n)\n\nvar _ = fmt.Append\n")
	for _, decl := range doc.Declarations {
		if strings.Contains(strings.Join(decl.Names, " "), "fmt") || decl.Kind == "import" {
			t.Fatalf("import was indexed: %+v", decl)
		}
	}
	if len(doc.Declarations) != 2 || doc.Declarations[0].Kind != syntax.KindPackage || doc.Declarations[1].Kind != syntax.KindVar {
		t.Fatalf("imports.go declarations = %+v", doc.Declarations)
	}

	src := []byte(sample)
	pkg := docByKind(t, mustParseDoc(t, "sample.go", sample), syntax.KindPackage, "sample")
	snip, err := syntax.DeclarationContext("sample.go", src, pkg.Range)
	if err != nil {
		t.Fatalf("package context: %v", err)
	}
	if snip.Text != "package sample" || strings.Contains(snip.Text, "documents") || strings.Contains(snip.Text, "...") {
		t.Fatalf("package context = %q", snip.Text)
	}
	if string(src[snip.Range.Start.Offset:snip.Range.End.Offset]) != snip.Text {
		t.Fatal("package context is not the original bytes")
	}

	var body strings.Builder
	body.WriteString("package p\n\nfunc F() {\n")
	for i := 0; i < 80; i++ {
		body.WriteString("\t_ = ")
		body.WriteString(strconv.Itoa(i))
		body.WriteByte('\n')
	}
	body.WriteString("}\n")
	longSrc := []byte(body.String())
	longDoc := mustParseDoc(t, "long.go", body.String())
	fn := docByKind(t, longDoc, syntax.KindFunction, "F")
	snip, err = syntax.DeclarationContext("long.go", longSrc, fn.Range)
	if err != nil {
		t.Fatalf("function context: %v", err)
	}
	if !strings.HasPrefix(snip.Text, "func F() {\n") || strings.Contains(snip.Text, "...") || strings.Contains(snip.Text, "_ = 79") {
		t.Fatalf("function context = %q", snip.Text)
	}
	if snippetLines(snip.Text) > syntax.MaxContextLines {
		t.Fatalf("function context lines = %d", snippetLines(snip.Text))
	}
	if string(longSrc[snip.Range.Start.Offset:snip.Range.End.Offset]) != snip.Text {
		t.Fatal("function context is not the original bytes")
	}

	if _, err := syntax.DeclarationContext("sample.go", src, syntax.Range{}); !errors.Is(err, syntax.ErrMalformedDeclaration) {
		t.Fatalf("malformed = %v", err)
	}
	missing := syntax.Range{
		Start: syntax.Position{Offset: 0, Line: 1, Column: 1},
		End:   syntax.Position{Offset: 1, Line: 1, Column: 2},
	}
	if _, err := syntax.DeclarationContext("sample.go", src, missing); !errors.Is(err, syntax.ErrDeclarationNotFound) {
		t.Fatalf("missing = %v", err)
	}
	if _, err := syntax.Parse("bad.go", []byte("package {\n")); err == nil {
		t.Fatal("malformed source returned a document")
	}
}

func mustParseDoc(t *testing.T, path, src string) *syntax.Document {
	t.Helper()
	doc, err := syntax.Parse(path, []byte(src))
	if err != nil || doc == nil {
		t.Fatalf("Parse %s: doc=%v err=%v", path, doc, err)
	}
	return doc
}

func assertDecls(t *testing.T, doc *syntax.Document, want []struct {
	kind  syntax.Kind
	names []string
	doc   string
}) {
	t.Helper()
	if len(doc.Declarations) != len(want) {
		t.Fatalf("declarations = %s", describeDecls(doc))
	}
	for i, w := range want {
		decl := doc.Declarations[i]
		names := w.names
		if names == nil {
			names = []string{}
		}
		if decl.Kind != w.kind || !reflect.DeepEqual(decl.Names, names) {
			t.Fatalf("decl %d = %s %q, want %s %q\n%s", i, decl.Kind, decl.Names, w.kind, names, describeDecls(doc))
		}
		if w.doc == "" {
			if decl.HasDoc {
				t.Fatalf("decl %d %s has unexpected documentation\n%s", i, decl.Kind, describeDecls(doc))
			}
			continue
		}
		if !decl.HasDoc {
			t.Fatalf("decl %d %s has no documentation, want %q", i, decl.Kind, w.doc)
		}
		var matched *syntax.Comment
		for j := range doc.Comments {
			comment := &doc.Comments[j]
			if comment.Range == decl.Doc && comment.Text == w.doc {
				matched = comment
				break
			}
		}
		if matched == nil {
			t.Fatalf("decl %d documentation range does not match %q", i, w.doc)
		}
	}
}

func docByKind(t *testing.T, doc *syntax.Document, kind syntax.Kind, name string) syntax.Declaration {
	t.Helper()
	for _, decl := range doc.Declarations {
		if decl.Kind == kind && len(decl.Names) == 1 && decl.Names[0] == name {
			return decl
		}
	}
	t.Fatalf("no %s %s in %s", kind, name, describeDecls(doc))
	return syntax.Declaration{}
}

func describeDecls(doc *syntax.Document) string {
	var b strings.Builder
	for i, decl := range doc.Declarations {
		fmtDecl := decl.Kind
		b.WriteString(string(fmtDecl))
		b.WriteString(" ")
		b.WriteString(strings.Join(decl.Names, ","))
		if decl.HasDoc {
			b.WriteString(" doc")
		}
		if i+1 < len(doc.Declarations) {
			b.WriteString("; ")
		}
	}
	return b.String()
}

func extraOutside(snip syntax.Snippet, comment syntax.Comment) int {
	extra := 0
	if comment.Range.Start.Offset > snip.Range.Start.Offset {
		extra += comment.Range.Start.Offset - snip.Range.Start.Offset
	}
	if snip.Range.End.Offset > comment.Range.End.Offset {
		extra += snip.Range.End.Offset - comment.Range.End.Offset
	}
	return extra
}
