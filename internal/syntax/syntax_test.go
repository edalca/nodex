package syntax

import (
	"errors"
	"fmt"
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

	"github.com/edalca/nodex/internal/syntax/contracts"
	"github.com/edalca/nodex/internal/syntax/types"
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
		lang, ok := Recognize(path)
		if !ok || lang != Go {
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
		lang, ok := Recognize(path)
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
		doc, err := Parse(path, src)
		if err != nil || doc == nil {
			t.Fatalf("Parse(%q): %v", path, err)
		}
		if doc.Language != Go || doc.Path != path {
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
			doc, err := Parse(path, []byte(src))
			if doc != nil {
				t.Fatalf("Parse(%q) returned a document", path)
			}
			var unsupported *UnsupportedError
			if !errors.As(err, &unsupported) || unsupported.Path != path {
				t.Fatalf("Parse(%q) error = %v, want UnsupportedError for that path", path, err)
			}
			var parseErr *ParseError
			if errors.As(err, &parseErr) {
				t.Fatalf("Parse(%q) error = %v, want the path rejected before parsing", path, err)
			}
		}
	}
}

func TestParseUsesCallerBytes(t *testing.T) {
	doc, err := Parse("no/such/dir/file.go", []byte("package p\n"))
	if err != nil || doc == nil {
		t.Fatalf("Parse of absent path with bytes: %v", err)
	}
	if doc.Path != "no/such/dir/file.go" || len(doc.Comments) != 0 {
		t.Fatalf("document = %+v", doc)
	}
}

func TestNilSource(t *testing.T) {
	doc, err := Parse("empty.go", nil)
	if doc != nil {
		t.Fatal("nil source returned a document")
	}
	var parseErr *ParseError
	if !errors.As(err, &parseErr) || parseErr.Path != "empty.go" {
		t.Fatalf("nil source error = %v", err)
	}

	doc, err = Parse("empty.rs", nil)
	if doc != nil {
		t.Fatal("unsupported nil source returned a document")
	}
	var unsupported *UnsupportedError
	if !errors.As(err, &unsupported) || unsupported.Path != "empty.rs" {
		t.Fatalf("unsupported nil source error = %v", err)
	}
}

func TestStringIsNotAComment(t *testing.T) {
	src := "package sample\n\nvar s = \"// not a comment\"\n\n// real comment\nfunc F() {}\n"
	doc, err := Parse("sample.go", []byte(src))
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
	doc, err := Parse("gen.go", []byte(src))
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
	doc, err := Parse("real.go", []byte(src))
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
	doc, err := Parse("bad.go", []byte(src))
	if doc != nil {
		t.Fatalf("document = %+v", doc)
	}
	var parseErr *ParseError
	if !errors.As(err, &parseErr) || parseErr.Path != "bad.go" {
		t.Fatalf("error = %v", err)
	}
	if !strings.Contains(err.Error(), "bad.go") {
		t.Fatalf("error %q omits the logical path", err)
	}
}

func TestLineDirectiveSyntaxErrorStaysPhysical(t *testing.T) {
	src := "package p\n\n//line other.go:400:1\nfunc (\n"
	doc, err := Parse("bad.go", []byte(src))
	if doc != nil {
		t.Fatalf("document = %+v", doc)
	}
	var parseErr *ParseError
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
	doc, err := Parse("wide.go", []byte(src))
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
			doc, err := Parse("sample.go", []byte(tt.src))
			if err != nil {
				t.Fatalf("Parse: %v", err)
			}
			if doc.Path != "sample.go" || doc.Language != Go {
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
	first, err := Parse("sample.go", src)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	for i := 0; i < 8; i++ {
		again, err := Parse("sample.go", src)
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
		doc, err := Parse(job.path, []byte(job.src))
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
			doc, err := Parse(job.path, []byte(job.src))
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

func assertComments(t *testing.T, src string, got []Comment, want []wantComment) {
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

func assertPosition(t *testing.T, src string, pos Position) {
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
	if n := snippetLines(snip.Text); n > MaxContextLines {
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
	if n := snippetLines(snip.Text); n != MaxContextLines {
		t.Fatalf("lines = %d, want %d\n%s", n, MaxContextLines, snip.Text)
	}
	if extraOutside(snip, comment) > MaxContextExtraBytes {
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
	if extra := extraOutside(snip, comment); extra > MaxContextExtraBytes {
		t.Fatalf("extra bytes = %d", extra)
	}
	if snippetLines(snip.Text) != 1 {
		t.Fatalf("lines = %d", snippetLines(snip.Text))
	}
	again, err := Context("long.go", []byte(src), comment.Range)
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
	doc, err := Parse("wide.go", []byte(src))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(doc.Comments) != 1 || !strings.HasPrefix(doc.Comments[0].Raw, "/*") {
		t.Fatalf("comments = %#v", doc.Comments)
	}
	comment := doc.Comments[0]
	snip, err := Context("wide.go", []byte(src), comment.Range)
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
	if n := snippetLines(snip.Text); n > MaxContextLines {
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
	_, err := Context("sample.go", []byte(src), shifted)
	if !errors.Is(err, ErrCommentNotFound) {
		t.Fatalf("shifted range error = %v", err)
	}
	var parseErr *ParseError
	if errors.As(err, &parseErr) {
		t.Fatalf("missing comment reported as a parse error: %v", err)
	}

	keyword := Range{
		Start: physicalPosition(src, 0),
		End:   physicalPosition(src, len("package")),
	}
	_, err = Context("sample.go", []byte(src), keyword)
	if !errors.Is(err, ErrCommentNotFound) {
		t.Fatalf("keyword range error = %v", err)
	}
}

func TestContextMalformedRange(t *testing.T) {
	src := "package p\n\n// only\nfunc F() {}\n"
	comment := commentByRaw(t, src, "sample.go", "// only")
	cases := []Range{
		{Start: Position{Offset: 4, Line: 1, Column: 5}, End: Position{Offset: 1, Line: 1, Column: 2}},
		{Start: Position{Offset: -1, Line: 1, Column: 1}, End: comment.Range.End},
		{Start: Position{Offset: 0, Line: 0, Column: 1}, End: Position{Offset: 1, Line: 1, Column: 2}},
		{Start: Position{Offset: 0, Line: 1, Column: 0}, End: Position{Offset: 1, Line: 1, Column: 2}},
		{Start: Position{Offset: 0, Line: 1, Column: 1}, End: Position{Offset: len(src) + 4, Line: 1, Column: 2}},
		{Start: Position{Offset: comment.Range.Start.Offset, Line: 99, Column: comment.Range.Start.Column}, End: comment.Range.End},
	}
	for _, r := range cases {
		snip, err := Context("sample.go", []byte(src), r)
		if snip.Text != "" || !errors.Is(err, ErrMalformedRange) {
			t.Fatalf("range %+v -> (%q, %v)", r, snip.Text, err)
		}
	}
}

func TestContextUnsupportedAndMalformedSource(t *testing.T) {
	src := []byte("package p\n\n// only\nfunc F() {}\n")
	comment := commentByRaw(t, string(src), "sample.go", "// only")
	for _, path := range []string{"notes.rs", "README.md", "foo.GO"} {
		snip, err := Context(path, src, comment.Range)
		if snip.Text != "" {
			t.Fatalf("Context(%q) returned %q", path, snip.Text)
		}
		var unsupported *UnsupportedError
		if !errors.As(err, &unsupported) || unsupported.Path != path {
			t.Fatalf("Context(%q) error = %v", path, err)
		}
		var parseErr *ParseError
		if errors.As(err, &parseErr) {
			t.Fatalf("Context(%q) parsed an unsupported path: %v", path, err)
		}
	}

	bad := []byte("package p\n\n// kept\nfunc (\n")
	r := Range{
		Start: physicalPosition(string(bad), 0),
		End:   physicalPosition(string(bad), len("package")),
	}
	snip, err := Context("bad.go", bad, r)
	if snip.Text != "" {
		t.Fatalf("malformed source returned %q", snip.Text)
	}
	var parseErr *ParseError
	if !errors.As(err, &parseErr) || parseErr.Path != "bad.go" {
		t.Fatalf("malformed source error = %v", err)
	}
	if errors.Is(err, ErrCommentNotFound) {
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

func commentByRaw(t *testing.T, src, path, raw string) Comment {
	t.Helper()
	doc, err := Parse(path, []byte(src))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	var found []Comment
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

func mustSnippet(t *testing.T, path string, src []byte, comment Comment) Snippet {
	t.Helper()
	snip, err := Context(path, src, comment.Range)
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
func assertContextPosition(t *testing.T, src string, pos Position) {
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
	if !reflect.DeepEqual(Selectors(), wantSelectors) {
		t.Fatalf("selectors = %q", Selectors())
	}
	if !reflect.DeepEqual(ConcretePresets(), wantConcrete) {
		t.Fatalf("concrete = %q", ConcretePresets())
	}
	expanded, err := ExpandSelector("go:all")
	if err != nil {
		t.Fatalf("expand: %v", err)
	}
	if !reflect.DeepEqual(expanded, ConcretePresets()) {
		t.Fatalf("go:all = %q, catalog %q", expanded, ConcretePresets())
	}
	for _, id := range expanded {
		if id == "go:all" {
			t.Fatal("aggregate selector was returned as a concrete preset")
		}
	}
	expanded[0] = "mutated"
	again, err := ExpandSelector("go:all")
	if err != nil || again[0] == "mutated" {
		t.Fatalf("expansion aliased the catalog: %q %v", again, err)
	}
	for _, id := range wantConcrete {
		got, err := ExpandSelector(id)
		if err != nil || !reflect.DeepEqual(got, []string{id}) {
			t.Fatalf("Expand(%s) = %q %v", id, got, err)
		}
	}
	for _, selector := range []string{"", "Go:tests", "go:all ", "not-a-preset"} {
		if _, err := ExpandSelector(selector); err == nil {
			t.Fatalf("Expand(%q) succeeded", selector)
		}
	}
	if err := ValidatePresets(nil); err != nil {
		t.Fatalf("empty validate: %v", err)
	}
	if err := ValidatePresets(append([]string{}, wantConcrete...)); err != nil {
		t.Fatalf("concrete validate: %v", err)
	}
	if err := ValidatePresets([]string{"go:all"}); err == nil {
		t.Fatal("persisted go:all was accepted")
	}
	if err := ValidatePresets([]string{"nope"}); err == nil {
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

	if PathExcluded("a_test.go", none) || PathExcluded("vendor/a.go", none) || InspectExcluded("gen.go", generated, none) {
		t.Fatal("disabled presets excluded a file")
	}
	if !PathExcluded("a_test.go", tests) || !PathExcluded("dir/a_test.go", tests) {
		t.Fatal("go:tests did not exclude _test.go")
	}
	if PathExcluded("a.go", tests) || PathExcluded("test.go", tests) || PathExcluded("a_TEST.go", tests) {
		t.Fatal("go:tests excluded a file that is not _test.go")
	}
	if !PathExcluded("vendor/a.go", vendor) || !PathExcluded("pkg/vendor/a.go", vendor) {
		t.Fatal("go:vendor did not exclude a vendor element")
	}
	if PathExcluded("vendorized/a.go", vendor) || PathExcluded("pkg/myvendor/a.go", vendor) || PathExcluded("Vendor/a.go", vendor) {
		t.Fatal("go:vendor matched a non-exact element")
	}
	if PathExcluded("vendor/readme.md", vendor) || InspectExcluded("vendor/readme.md", []byte("generated"), vendor) {
		t.Fatal("a preset excluded an unsupported path")
	}
	if InspectExcluded("gen.go", generated, tests) {
		t.Fatal("go:tests excluded a generated file by contents")
	}
	if !InspectExcluded("gen.go", generated, gen) {
		t.Fatal("canonical generated Go source was included")
	}
	if InspectExcluded("gen.go", generated, none) || InspectExcluded("noted.go", ordinary, gen) {
		t.Fatal("unrelated generated words excluded a file")
	}
	if InspectExcluded("zz_generated.go", ordinary, gen) {
		t.Fatal("a generated file name was treated as generated source")
	}
	broken := []byte("// Code generated by nodex-test. DO NOT EDIT.\n\npackage p\n\nfunc (\n")
	if !InspectExcluded("gen.go", broken, gen) {
		t.Fatal("generated classification required a complete program")
	}
}

func physicalPosition(src string, offset int) Position {
	line, col := 1, 1
	for i := 0; i < offset && i < len(src); i++ {
		if src[i] == '\n' {
			line++
			col = 1
			continue
		}
		col++
	}
	return Position{Offset: offset, Line: line, Column: col}
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
		kind  Kind
		names []string
		doc   string
	}{
		{KindPackage, []string{"sample"}, "Package sample documents sample."},
		{KindFunction, []string{"F"}, "F documents F."},
		{KindFunction, []string{"g"}, ""},
		{KindConstGroup, []string{"A", "B"}, ""},
		{KindConst, []string{"A"}, "A documents A."},
		{KindConst, []string{"B"}, ""},
		{KindTypeGroup, []string{"T"}, "TGroup documents the type group."},
		{KindType, []string{"T"}, ""},
		{KindField, []string{"Name"}, "Name documents Name."},
		{KindField, []string{"Hidden"}, ""},
	}
	assertDecls(t, doc, want)

	doc = mustParseDoc(t, "one.go", "package one\n\n// X documents X.\nconst X = 1\n")
	assertDecls(t, doc, []struct {
		kind  Kind
		names []string
		doc   string
	}{
		{KindPackage, []string{"one"}, ""},
		{KindConst, []string{"X"}, "X documents X."},
	})

	doc = mustParseDoc(t, "vars.go", "package vars\n\nvar A, B int\n")
	assertDecls(t, doc, []struct {
		kind  Kind
		names []string
		doc   string
	}{
		{KindPackage, []string{"vars"}, ""},
		{KindVar, []string{"A", "B"}, ""},
	})

	doc = mustParseDoc(t, "group.go", "package group\n\nvar (\n\tA, B int\n\t// C documents C.\n\tC = 1\n)\n")
	assertDecls(t, doc, []struct {
		kind  Kind
		names []string
		doc   string
	}{
		{KindPackage, []string{"group"}, ""},
		{KindVarGroup, []string{"A", "B", "C"}, ""},
		{KindVar, []string{"A", "B"}, ""},
		{KindVar, []string{"C"}, "C documents C."},
	})

	doc = mustParseDoc(t, "method.go", "package method\n\ntype T struct{}\n\nfunc (T) M(a int) int { return a }\n")
	assertDecls(t, doc, []struct {
		kind  Kind
		names []string
		doc   string
	}{
		{KindPackage, []string{"method"}, ""},
		{KindType, []string{"T"}, ""},
		{KindMethod, []string{"M"}, ""},
	})

	doc = mustParseDoc(t, "iface.go", "package iface\n\ntype I interface {\n\tM(x int)\n\tU\n}\n")
	assertDecls(t, doc, []struct {
		kind  Kind
		names []string
		doc   string
	}{
		{KindPackage, []string{"iface"}, ""},
		{KindType, []string{"I"}, ""},
		{KindField, []string{"M"}, ""},
		{KindField, nil, ""},
	})

	doc = mustParseDoc(t, "embed.go", "package embed\n\ntype E struct {\n\tU\n\t*T\n}\n")
	assertDecls(t, doc, []struct {
		kind  Kind
		names []string
		doc   string
	}{
		{KindPackage, []string{"embed"}, ""},
		{KindType, []string{"E"}, ""},
		{KindField, nil, ""},
		{KindField, nil, ""},
	})
	for _, decl := range doc.Declarations {
		if decl.Names == nil {
			t.Fatalf("names slice is nil: %+v", decl)
		}
	}

	doc = mustParseDoc(t, "local.go", "package local\n\nfunc F(a int) (b int) {\n\tconst local = 1\n\ttype localT struct{ E int }\n\treturn a + b\n}\n")
	assertDecls(t, doc, []struct {
		kind  Kind
		names []string
		doc   string
	}{
		{KindPackage, []string{"local"}, ""},
		{KindFunction, []string{"F"}, ""},
		{KindConst, []string{"local"}, ""},
		{KindType, []string{"localT"}, ""},
		{KindField, []string{"E"}, ""},
	})

	doc = mustParseDoc(t, "trail.go", "package trail\n\ntype T struct {\n\t// Name documents Name.\n\tName string // trailing\n\tHidden int // also trailing\n}\n")
	if len(doc.Comments) != 3 {
		t.Fatalf("trailing comments = %+v", doc.Comments)
	}
	assertDecls(t, doc, []struct {
		kind  Kind
		names []string
		doc   string
	}{
		{KindPackage, []string{"trail"}, ""},
		{KindType, []string{"T"}, ""},
		{KindField, []string{"Name"}, "Name documents Name."},
		{KindField, []string{"Hidden"}, ""},
	})

	doc = mustParseDoc(t, "imports.go", "package imports\n\nimport \"fmt\"\n\nimport (\n\t\"io\"\n\talias \"os\"\n)\n\nvar _ = fmt.Append\n")
	for _, decl := range doc.Declarations {
		if strings.Contains(strings.Join(decl.Names, " "), "fmt") || decl.Kind == "import" {
			t.Fatalf("import was indexed: %+v", decl)
		}
	}
	if len(doc.Declarations) != 2 || doc.Declarations[0].Kind != KindPackage || doc.Declarations[1].Kind != KindVar {
		t.Fatalf("imports.go declarations = %+v", doc.Declarations)
	}

	src := []byte(sample)
	pkg := docByKind(t, mustParseDoc(t, "sample.go", sample), KindPackage, "sample")
	snip, err := DeclarationContext("sample.go", src, pkg.Range)
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
	fn := docByKind(t, longDoc, KindFunction, "F")
	snip, err = DeclarationContext("long.go", longSrc, fn.Range)
	if err != nil {
		t.Fatalf("function context: %v", err)
	}
	if !strings.HasPrefix(snip.Text, "func F() {\n") || strings.Contains(snip.Text, "...") || strings.Contains(snip.Text, "_ = 79") {
		t.Fatalf("function context = %q", snip.Text)
	}
	if snippetLines(snip.Text) > MaxContextLines {
		t.Fatalf("function context lines = %d", snippetLines(snip.Text))
	}
	if string(longSrc[snip.Range.Start.Offset:snip.Range.End.Offset]) != snip.Text {
		t.Fatal("function context is not the original bytes")
	}

	if _, err := DeclarationContext("sample.go", src, Range{}); !errors.Is(err, ErrMalformedDeclaration) {
		t.Fatalf("malformed = %v", err)
	}
	missing := Range{
		Start: Position{Offset: 0, Line: 1, Column: 1},
		End:   Position{Offset: 1, Line: 1, Column: 2},
	}
	if _, err := DeclarationContext("sample.go", src, missing); !errors.Is(err, ErrDeclarationNotFound) {
		t.Fatalf("missing = %v", err)
	}
	if _, err := Parse("bad.go", []byte("package {\n")); err == nil {
		t.Fatal("malformed source returned a document")
	}
}

func TestGoDirectDocsCardinality(t *testing.T) {
	cases := []struct {
		kind                       Kind
		before, declaration, after string
	}{
		{KindPackage, "", "package p\n", ""},
		{KindFunction, "package p\n", "func F() {}\n", ""},
		{KindMethod, "package p\ntype T struct{}\n", "func (T) M() {}\n", ""},
		{KindConstGroup, "package p\n", "const (\nA = 1\n)\n", ""},
		{KindVarGroup, "package p\n", "var (\nA int\n)\n", ""},
		{KindTypeGroup, "package p\n", "type (\nT int\n)\n", ""},
		{KindConst, "package p\n", "const A = 1\n", ""},
		{KindVar, "package p\n", "var A int\n", ""},
		{KindType, "package p\n", "type T int\n", ""},
		{KindConst, "package p\nconst (\n", "A = 1\n", ")\n"},
		{KindVar, "package p\nvar (\n", "A int\n", ")\n"},
		{KindType, "package p\ntype (\n", "T int\n", ")\n"},
		{KindField, "package p\ntype T struct {\n", "F int // trailing\n", "}\n"},
		{KindField, "package p\ntype T interface {\n", "M() // trailing\n", "}\n"},
	}
	for i, tc := range cases {
		for _, documented := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/%d/%v", tc.kind, i, documented), func(t *testing.T) {
				comment := ""
				if documented {
					comment = "// direct\n"
				}
				doc := mustParseDoc(t, "a.go", tc.before+comment+tc.declaration+tc.after)
				start := len(tc.before) + len(comment)
				var target *Declaration
				for j := range doc.Declarations {
					decl := &doc.Declarations[j]
					if decl.Kind == tc.kind && decl.Range.Start.Offset == start {
						target = decl
					}
					if decl.Docs == nil || len(decl.Docs) > 1 {
						t.Fatalf("Go docs must be a non-nil 0..1 collection: %+v", decl)
					}
				}
				if target == nil {
					t.Fatal("declaration missing")
				}
				if !documented {
					if len(target.Docs) != 0 {
						t.Fatalf("undocumented: %+v", target)
					}
					return
				}
				if len(target.Docs) != 1 || target.Docs[0].Start.Offset != len(tc.before) || target.Docs[0].End.Offset != len(tc.before)+len("// direct") {
					t.Fatalf("direct docs: %+v", target)
				}
				for _, other := range doc.Declarations {
					if other.Range != target.Range && len(other.Docs) != 0 {
						t.Fatalf("documentation propagated: %+v", other)
					}
				}
			})
		}
	}
	separated := mustParseDoc(t, "a.go", "package p\n// nearby\n\nfunc F() {}\n")
	if len(docByKind(t, separated, KindFunction, "F").Docs) != 0 {
		t.Fatal("proximity created a direct documentation relationship")
	}
}

func TestFacadePluralDocs(t *testing.T) {
	for _, count := range []int{0, 1, 2, 4} {
		t.Run(strconv.Itoa(count), func(t *testing.T) {
			ranges := make([]types.Range, count)
			comments := make([]types.Comment, count)
			want := make([]Range, count)
			for i := range ranges {
				start := types.Position{Offset: i * 6, Line: 1, Column: i*6 + 1}
				end := types.Position{Offset: i*6 + 4, Line: 1, Column: i*6 + 5}
				ranges[i] = types.Range{Start: start, End: end}
				comments[i] = types.Comment{Raw: "same", Text: "same", Start: start, End: end}
				want[i] = facadeRange(start, end)
			}
			f := &fixtureLanguage{id: "fixture", suffix: ".fixture", parsed: &contracts.Document{
				Comments:     comments,
				Declarations: []types.Declaration{{Kind: "custom-form", Docs: ranges}},
			}}
			r, err := newRegistry([]contracts.Language{f})
			if err != nil {
				t.Fatal(err)
			}
			doc, err := r.parse("a.fixture", nil)
			if err != nil || len(doc.Comments) != count || !reflect.DeepEqual(doc.Declarations[0].Docs, want) {
				t.Fatalf("facade docs = %+v, %v", doc, err)
			}
			if count > 0 {
				doc.Declarations[0].Docs[0] = Range{}
				if ranges[0].End.Offset != 4 {
					t.Fatal("facade docs alias private parser output")
				}
			}
		})
	}
}

func mustParseDoc(t *testing.T, path, src string) *Document {
	t.Helper()
	doc, err := Parse(path, []byte(src))
	if err != nil || doc == nil {
		t.Fatalf("Parse %s: doc=%v err=%v", path, doc, err)
	}
	return doc
}

func assertDecls(t *testing.T, doc *Document, want []struct {
	kind  Kind
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
		if decl.Docs == nil {
			t.Fatalf("decl %d docs is nil", i)
		}
		if w.doc == "" {
			if len(decl.Docs) != 0 {
				t.Fatalf("decl %d %s has unexpected documentation\n%s", i, decl.Kind, describeDecls(doc))
			}
			continue
		}
		if len(decl.Docs) != 1 {
			t.Fatalf("decl %d %s has no documentation, want %q", i, decl.Kind, w.doc)
		}
		var matched *Comment
		for j := range doc.Comments {
			comment := &doc.Comments[j]
			if comment.Range == decl.Docs[0] && comment.Text == w.doc {
				matched = comment
				break
			}
		}
		if matched == nil {
			t.Fatalf("decl %d documentation range does not match %q", i, w.doc)
		}
	}
}

func docByKind(t *testing.T, doc *Document, kind Kind, name string) Declaration {
	t.Helper()
	for _, decl := range doc.Declarations {
		if decl.Kind == kind && len(decl.Names) == 1 && decl.Names[0] == name {
			return decl
		}
	}
	t.Fatalf("no %s %s in %s", kind, name, describeDecls(doc))
	return Declaration{}
}

func describeDecls(doc *Document) string {
	var b strings.Builder
	for i, decl := range doc.Declarations {
		fmtDecl := decl.Kind
		b.WriteString(string(fmtDecl))
		b.WriteString(" ")
		b.WriteString(strings.Join(decl.Names, ","))
		if len(decl.Docs) != 0 {
			b.WriteString(" doc")
		}
		if i+1 < len(doc.Declarations) {
			b.WriteString("; ")
		}
	}
	return b.String()
}

func extraOutside(snip Snippet, comment Comment) int {
	extra := 0
	if comment.Range.Start.Offset > snip.Range.Start.Offset {
		extra += comment.Range.Start.Offset - snip.Range.Start.Offset
	}
	if snip.Range.End.Offset > comment.Range.End.Offset {
		extra += snip.Range.End.Offset - comment.Range.End.Offset
	}
	return extra
}

// fixtureLanguage exposes an arbitrary capability object to a test-local
// registry. No global registry is modified by these tests.
type fixtureLanguage struct {
	id      string
	suffix  string
	catalog contracts.PresetCatalog
	parsed  *contracts.Document
	err     error
	calls   []string
}

var _ contracts.Language = (*fixtureLanguage)(nil)

func (f *fixtureLanguage) ID() string { return f.id }
func (f *fixtureLanguage) Recognize(path string) bool {
	return f.suffix != "" && strings.HasSuffix(path, f.suffix)
}
func (f *fixtureLanguage) Presets() contracts.PresetCatalog { return f.catalog }
func (f *fixtureLanguage) Parse(source []byte) (*contracts.Document, error) {
	f.calls = append(f.calls, "parse:"+string(source))
	return f.parsed, f.err
}
func (f *fixtureLanguage) Context(source []byte, target types.Range, limits contracts.Limits) (types.Snippet, error) {
	f.calls = append(f.calls, "comment")
	return f.snippet(source, target, limits)
}
func (f *fixtureLanguage) DeclarationContext(source []byte, target types.Range, limits contracts.Limits) (types.Snippet, error) {
	f.calls = append(f.calls, "declaration")
	return f.snippet(source, target, limits)
}
func (f *fixtureLanguage) snippet(source []byte, target types.Range, limits contracts.Limits) (types.Snippet, error) {
	if limits != (contracts.Limits{Lines: MaxContextLines, ExtraBytes: MaxContextExtraBytes}) {
		return types.Snippet{}, errors.New("unexpected context limits")
	}
	if f.err != nil {
		return types.Snippet{}, f.err
	}
	return types.Snippet{Start: target.Start, End: target.End, Text: string(source[target.Start.Offset:target.End.Offset])}, nil
}
func (f *fixtureLanguage) PathExcluded(path string, enabled []string) bool {
	f.calls = append(f.calls, "path:"+path)
	return len(enabled) > 0 && enabled[0] == "fixture:path"
}
func (f *fixtureLanguage) SourceExcluded(path string, source []byte, enabled []string) bool {
	f.calls = append(f.calls, "source:"+path+":"+string(source))
	return len(enabled) > 0 && enabled[0] == "fixture:source"
}

func TestRegistryDispatchesCompleteLanguage(t *testing.T) {
	start := types.Position{Offset: 0, Line: 1, Column: 1}
	end := types.Position{Offset: 3, Line: 1, Column: 4}
	f := &fixtureLanguage{
		id: "fixture", suffix: ".fixture",
		catalog: contracts.PresetCatalog{
			Concrete:   []string{"fixture:source", "fixture:path"},
			Aggregates: []contracts.Aggregate{{ID: "fixture:all", Presets: []string{"fixture:source", "fixture:path"}}},
		},
		parsed: &contracts.Document{
			Comments: []types.Comment{{Raw: "abc", Text: "normalized", Start: start, End: end}},
			Declarations: []types.Declaration{
				{Kind: "custom-form", Names: []string{"A", "B"}, Start: start, End: end, Docs: []types.Range{{Start: start, End: end}}},
				{Kind: "unnamed", Start: start, End: end, Docs: []types.Range{}},
			},
		},
	}
	other := &fixtureLanguage{id: "other", suffix: ".other"}
	r, err := newRegistry([]contracts.Language{other, f})
	if err != nil {
		t.Fatal(err)
	}
	if r.resolve("x.fixture") != f || r.resolve("x.other") != other || r.resolve("x.unknown") != nil {
		t.Fatal("recognition did not resolve the claiming object")
	}
	doc, err := r.parse("x.fixture", []byte("abc"))
	if err != nil || doc == nil {
		t.Fatalf("parse: %v", err)
	}
	rng := facadeRange(start, end)
	want := &Document{
		Path: "x.fixture", Language: "fixture",
		Comments: []Comment{{Raw: "abc", Text: "normalized", Range: rng}},
		Declarations: []Declaration{
			{Kind: "custom-form", Names: []string{"A", "B"}, Range: rng, Docs: []Range{rng}},
			{Kind: "unnamed", Names: []string{}, Range: rng, Docs: []Range{}},
		},
	}
	if !reflect.DeepEqual(doc, want) {
		t.Fatalf("facade document = %+v, want %+v", doc, want)
	}
	doc.Declarations[0].Names[0] = "changed"
	if f.parsed.Declarations[0].Names[0] != "A" {
		t.Fatal("facade names alias the implementation result")
	}
	for _, declaration := range []bool{false, true} {
		snippet, err := r.context("x.fixture", []byte("abc"), rng, declaration)
		if err != nil || snippet != (Snippet{Range: rng, Text: "abc"}) {
			t.Fatalf("context %v = %+v, %v", declaration, snippet, err)
		}
	}
	if !reflect.DeepEqual(r.concrete, []string{"fixture:path", "fixture:source"}) ||
		!reflect.DeepEqual(r.selectors, []string{"fixture:all", "fixture:path", "fixture:source"}) {
		t.Fatalf("language catalog = %q / %q", r.concrete, r.selectors)
	}
	expansion, err := r.expandSelector("fixture:all")
	if err != nil || !reflect.DeepEqual(expansion, r.concrete) {
		t.Fatalf("aggregate expansion = %q, %v", expansion, err)
	}
	expansion[0] = "changed"
	f.catalog.Aggregates[0].Presets[0] = "changed"
	if next, _ := r.expandSelector("fixture:all"); !reflect.DeepEqual(next, r.concrete) {
		t.Fatal("registry expansion aliases returned or provider catalog values")
	}
	if !r.pathExcluded("x.fixture", []string{"fixture:path"}) ||
		!r.inspectExcluded("x.fixture", []byte("abc"), []string{"fixture:source"}) {
		t.Fatal("exclusions did not use the same capability object")
	}
	wantCalls := []string{"parse:abc", "comment", "declaration", "path:x.fixture", "path:x.fixture", "source:x.fixture:abc"}
	if !reflect.DeepEqual(f.calls, wantCalls) || len(other.calls) != 0 {
		t.Fatalf("dispatch = %q, other = %q", f.calls, other.calls)
	}
	f.calls = nil
	if !r.inspectExcluded("x.fixture", nil, []string{"fixture:path"}) || len(f.calls) != 1 {
		t.Fatal("path exclusion did not short-circuit structural inspection")
	}
	f.calls = nil
	if r.inspectExcluded("x.fixture", nil, nil) || r.pathExcluded("x.fixture", nil) ||
		r.inspectExcluded("x.unknown", nil, []string{"fixture:source"}) || len(f.calls) != 0 {
		t.Fatal("disabled or unsupported exclusion invoked a language capability")
	}
	if doc, err := r.parse("x.unknown", nil); doc != nil || err == nil {
		t.Fatalf("unsupported parse = %v, %v", doc, err)
	}
	for _, declaration := range []bool{false, true} {
		if snippet, err := r.context("x.unknown", nil, Range{}, declaration); snippet != (Snippet{}) || err == nil {
			t.Fatalf("unsupported context = %v, %v", snippet, err)
		}
	}
}

func TestRegistryRejectsInvalidCapabilities(t *testing.T) {
	concrete := func(id string, presets ...string) *fixtureLanguage {
		return &fixtureLanguage{id: id, catalog: contracts.PresetCatalog{Concrete: presets}}
	}
	aggregate := func(id, selector string, members ...string) *fixtureLanguage {
		return &fixtureLanguage{id: id, catalog: contracts.PresetCatalog{
			Concrete:   []string{id + ":preset"},
			Aggregates: []contracts.Aggregate{{ID: selector, Presets: members}},
		}}
	}
	cases := []struct {
		name            string
		implementations []contracts.Language
		message         string
	}{
		{"nil language", []contracts.Language{nil}, "missing language identity"},
		{"empty identity", []contracts.Language{concrete("")}, "missing language identity"},
		{"duplicate language", []contracts.Language{concrete("a"), concrete("a")}, "duplicate language"},
		{"duplicate concrete", []contracts.Language{concrete("a", "same"), concrete("b", "same")}, "duplicate selector"},
		{"duplicate own concrete", []contracts.Language{concrete("a", "same", "same")}, "duplicate selector"},
		{"empty concrete", []contracts.Language{concrete("a", "")}, "empty preset"},
		{"duplicate aggregate", []contracts.Language{aggregate("a", "all", "a:preset"), aggregate("b", "all", "b:preset")}, "duplicate selector"},
		{"aggregate concrete collision", []contracts.Language{concrete("a", "all"), aggregate("b", "all", "b:preset")}, "duplicate selector"},
		{"concrete aggregate collision", []contracts.Language{aggregate("b", "all", "b:preset"), concrete("a", "all")}, "duplicate selector"},
		{"empty aggregate identity", []contracts.Language{aggregate("a", "", "a:preset")}, "empty aggregate"},
		{"empty aggregate", []contracts.Language{aggregate("a", "all")}, "empty aggregate"},
		{"unknown member", []contracts.Language{aggregate("a", "all", "unknown")}, "invalid member"},
		{"foreign member", []contracts.Language{concrete("b", "b:preset"), aggregate("a", "all", "b:preset")}, "invalid member"},
		{"aggregate member", []contracts.Language{aggregate("a", "all", "all")}, "invalid member"},
		{"duplicate member", []contracts.Language{aggregate("a", "all", "a:preset", "a:preset")}, "invalid member"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := newRegistry(tc.implementations); err == nil || !strings.Contains(err.Error(), tc.message) {
				t.Fatalf("registry error = %v, want %q", err, tc.message)
			}
		})
	}
	if r, err := newRegistry(nil); err != nil || r.resolve("any") != nil {
		t.Fatalf("empty registry = %+v, %v", r, err)
	}
}

func TestRegistryOverlapIsIndependentOfOrder(t *testing.T) {
	a := &fixtureLanguage{id: "a", suffix: ".fixture"}
	b := &fixtureLanguage{id: "b", suffix: ".fixture"}
	for _, implementations := range [][]contracts.Language{{a, b}, {b, a}} {
		r, err := newRegistry(implementations)
		if err != nil {
			t.Fatal(err)
		}
		for _, operation := range []func(){
			func() { r.resolve("x.fixture") },
			func() { r.parse("x.fixture", nil) },
			func() { r.context("x.fixture", nil, Range{}, false) },
			func() { r.context("x.fixture", nil, Range{}, true) },
			func() { r.pathExcluded("x.fixture", []string{"enabled"}) },
			func() { r.inspectExcluded("x.fixture", nil, []string{"enabled"}) },
		} {
			func() {
				defer func() {
					if got := recover(); got != `syntax registry: ambiguous path "x.fixture" claimed by a, b` {
						t.Fatalf("overlap panic = %v", got)
					}
				}()
				operation()
			}()
		}
	}
	if len(a.calls)+len(b.calls) != 0 {
		t.Fatal("ambiguous recognition invoked a capability")
	}
}

func TestRegistryAdaptsNeutralFailures(t *testing.T) {
	f := &fixtureLanguage{id: "fixture", suffix: ".fixture", parsed: &contracts.Document{}}
	r := mustRegistry([]contracts.Language{f})
	f.err = &contracts.ParseError{Diagnostics: []types.Diagnostic{
		{Position: types.Position{Line: 2, Column: 3}, Msg: "bad token"},
		{Msg: "invalid source"},
	}}
	if doc, err := r.parse("x.fixture", nil); doc != nil || err == nil || err.Error() != "x.fixture:2:3: bad token\nx.fixture: invalid source" {
		t.Fatalf("failed parse = %v, %v", doc, err)
	}
	for _, pair := range []struct{ internal, external error }{
		{contracts.ErrMalformedRange, ErrMalformedRange},
		{contracts.ErrCommentNotFound, ErrCommentNotFound},
		{contracts.ErrAmbiguousComment, ErrAmbiguousComment},
		{contracts.ErrMalformedDeclaration, ErrMalformedDeclaration},
		{contracts.ErrDeclarationNotFound, ErrDeclarationNotFound},
		{contracts.ErrAmbiguousDeclaration, ErrAmbiguousDeclaration},
	} {
		f.err = pair.internal
		for _, declaration := range []bool{false, true} {
			if snippet, err := r.context("x.fixture", nil, Range{}, declaration); snippet != (Snippet{}) || !errors.Is(err, pair.external) || err.Error() != "x.fixture: "+pair.external.Error() {
				t.Fatalf("context failure = %+v, %v", snippet, err)
			}
		}
	}
}
