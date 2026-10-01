package index_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/edalca/nodex/internal/index"
	"github.com/edalca/nodex/internal/syntax"
)

func cmt(text string, start, end int) syntax.Comment {
	return syntax.Comment{
		Text: text,
		Range: syntax.Range{
			Start: syntax.Position{Offset: start, Line: 1, Column: 1},
			End:   syntax.Position{Offset: end, Line: 1, Column: 1},
		},
	}
}

func goDoc(path string, comments ...syntax.Comment) *syntax.Document {
	return &syntax.Document{
		Path:     path,
		Language: syntax.Go,
		Comments: append([]syntax.Comment(nil), comments...),
	}
}

func mustBuild(t *testing.T, docs ...*syntax.Document) *index.Index {
	t.Helper()
	idx, err := index.Build(docs)
	if err != nil || idx == nil {
		t.Fatalf("Build: index=%v err=%v", idx, err)
	}
	return idx
}

func mustParse(t *testing.T, text string) index.ID {
	t.Helper()
	id, err := index.ParseID(text)
	if err != nil || !id.Valid() || id.String() != text {
		t.Fatalf("ParseID(%q) = %v, %v", text, id, err)
	}
	return id
}

func requireFailure(t *testing.T, docs []*syntax.Document) error {
	t.Helper()
	idx, err := index.Build(docs)
	if idx != nil {
		t.Fatal("failed build returned an index")
	}
	if err == nil {
		t.Fatal("expected an error")
	}
	return err
}

func TestBuildZeroDocuments(t *testing.T) {
	for _, docs := range [][]*syntax.Document{nil, {}} {
		idx, err := index.Build(docs)
		if err != nil || idx == nil {
			t.Fatalf("Build(%v) = %v, %v", docs, idx, err)
		}
		if idx.Len() != 0 {
			t.Fatalf("Len = %d, want 0", idx.Len())
		}
		if entries := idx.Entries(); len(entries) != 0 || entries == nil {
			t.Fatalf("Entries = %#v, want an empty slice", entries)
		}
		if _, ok := idx.Lookup(mustParse(t, "C000001")); ok {
			t.Fatal("empty index returned a comment")
		}
	}
}

func TestBuildDocumentWithoutComments(t *testing.T) {
	empty := &syntax.Document{Path: "a.go", Language: syntax.Go}
	idx := mustBuild(t, empty)
	if idx.Len() != 0 {
		t.Fatalf("Len = %d, want 0", idx.Len())
	}

	withComment := goDoc("b.go", cmt("only", 5, 6))
	idx = mustBuild(t, empty, withComment)
	entries := idx.Entries()
	if len(entries) != 1 || entries[0].ID.String() != "C000001" || entries[0].Path != "b.go" {
		t.Fatalf("entries = %+v, want only b.go as C000001", entries)
	}
}

func TestBuildAssignsSequentialIDs(t *testing.T) {
	idx := mustBuild(t, goDoc("a.go",
		cmt("one", 1, 2),
		cmt("two", 3, 4),
		cmt("three", 5, 6),
	))
	entries := idx.Entries()
	if len(entries) != 3 {
		t.Fatalf("Len = %d, want 3", len(entries))
	}
	want := []string{"C000001", "C000002", "C000003"}
	for i, id := range want {
		if entries[i].ID.String() != id || !entries[i].ID.Valid() {
			t.Fatalf("entry %d ID = %q, want %s", i, entries[i].ID.String(), id)
		}
		if entries[i].ID.String() == "C000000" {
			t.Fatal("assigned C000000")
		}
	}
}

func TestBuildOrdersPathThenOffset(t *testing.T) {
	// The supplied order is the reverse of the canonical order.
	a := goDoc("a.go", cmt("at-100", 100, 101), cmt("at-10", 10, 11))
	b := goDoc("b.go", cmt("at-5", 5, 6))
	idx := mustBuild(t, b, a)

	entries := idx.Entries()
	want := []struct {
		id     string
		path   string
		offset int
	}{
		{"C000001", "a.go", 10},
		{"C000002", "a.go", 100},
		{"C000003", "b.go", 5},
	}
	if len(entries) != len(want) {
		t.Fatalf("len = %d, want %d", len(entries), len(want))
	}
	for i, w := range want {
		got := entries[i]
		if got.ID.String() != w.id || got.Path != w.path || got.Range.Start.Offset != w.offset {
			t.Fatalf("entry %d = %s %s @ %d, want %s %s @ %d", i, got.ID, got.Path, got.Range.Start.Offset, w.id, w.path, w.offset)
		}
	}
}

func TestBuildOrdersPathsBytewise(t *testing.T) {
	docs := []*syntax.Document{
		goDoc("b.go", cmt("b", 1, 2)),
		goDoc("a/b.go", cmt("ab", 1, 2)),
		goDoc("A.go", cmt("A", 1, 2)),
		goDoc("a.go", cmt("a", 1, 2)),
		goDoc("\xff.go", cmt("high", 1, 2)),
	}
	idx := mustBuild(t, docs...)
	var got []string
	for _, entry := range idx.Entries() {
		got = append(got, entry.Path)
	}
	want := []string{"A.go", "a.go", "a/b.go", "b.go", "\xff.go"}
	if !slices.Equal(got, want) {
		t.Fatalf("paths = %#v, want %#v", got, want)
	}
	for _, entry := range idx.Entries() {
		if entry.Path != docsPath(docs, entry.Path) {
			t.Fatalf("path %q was rewritten", entry.Path)
		}
	}
}

func docsPath(docs []*syntax.Document, want string) string {
	for _, doc := range docs {
		if doc.Path == want {
			return doc.Path
		}
	}
	return ""
}

func TestBuildOrdersPhysicalPosition(t *testing.T) {
	sameStart := goDoc("a.go",
		syntax.Comment{
			Text: "later-end",
			Range: syntax.Range{
				Start: syntax.Position{Offset: 10, Line: 1, Column: 1},
				End:   syntax.Position{Offset: 30, Line: 1, Column: 21},
			},
		},
		syntax.Comment{
			Text: "earlier-end",
			Range: syntax.Range{
				Start: syntax.Position{Offset: 10, Line: 1, Column: 1},
				End:   syntax.Position{Offset: 20, Line: 1, Column: 11},
			},
		},
	)
	idx := mustBuild(t, sameStart)
	entries := idx.Entries()
	if entries[0].Text != "earlier-end" || entries[1].Text != "later-end" {
		t.Fatalf("end-offset order = %q, %q", entries[0].Text, entries[1].Text)
	}

	sameOffsets := goDoc("b.go",
		syntax.Comment{
			Text: "line-2",
			Range: syntax.Range{
				Start: syntax.Position{Offset: 10, Line: 2, Column: 1},
				End:   syntax.Position{Offset: 20, Line: 2, Column: 11},
			},
		},
		syntax.Comment{
			Text: "line-1",
			Range: syntax.Range{
				Start: syntax.Position{Offset: 10, Line: 1, Column: 4},
				End:   syntax.Position{Offset: 20, Line: 1, Column: 14},
			},
		},
	)
	idx = mustBuild(t, sameOffsets)
	entries = idx.Entries()
	if entries[0].Text != "line-1" || entries[1].Text != "line-2" {
		t.Fatalf("line order = %q, %q", entries[0].Text, entries[1].Text)
	}

	sameLine := goDoc("c.go",
		syntax.Comment{
			Text: "column-8",
			Range: syntax.Range{
				Start: syntax.Position{Offset: 10, Line: 1, Column: 8},
				End:   syntax.Position{Offset: 12, Line: 1, Column: 10},
			},
		},
		syntax.Comment{
			Text: "column-2",
			Range: syntax.Range{
				Start: syntax.Position{Offset: 10, Line: 1, Column: 2},
				End:   syntax.Position{Offset: 12, Line: 1, Column: 4},
			},
		},
	)
	idx = mustBuild(t, sameLine)
	entries = idx.Entries()
	if entries[0].Text != "column-2" || entries[1].Text != "column-8" {
		t.Fatalf("column order = %q, %q", entries[0].Text, entries[1].Text)
	}
}

func TestBuildInputOrderDoesNotChangeEntries(t *testing.T) {
	comments := []syntax.Comment{cmt("at-10", 10, 11), cmt("at-100", 100, 101)}
	reversed := []syntax.Comment{comments[1], comments[0]}
	aOrders := [][]syntax.Comment{comments, reversed}
	b := goDoc("b.go", cmt("at-5", 5, 6))
	c := goDoc("c.go", cmt("at-1", 1, 2))

	var want []index.Entry
	for _, aComments := range aOrders {
		a := goDoc("a.go", aComments...)
		orders := [][]*syntax.Document{
			{a, b, c},
			{a, c, b},
			{b, a, c},
			{b, c, a},
			{c, a, b},
			{c, b, a},
		}
		for _, docs := range orders {
			before := append([]*syntax.Document(nil), docs...)
			idx := mustBuild(t, docs...)
			if !reflect.DeepEqual(docs, before) {
				t.Fatal("Build reordered the document slice")
			}
			got := idx.Entries()
			if want == nil {
				want = got
				continue
			}
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("entries differ:\n got %+v\nwant %+v", got, want)
			}
		}
	}
	if len(want) != 4 {
		t.Fatalf("canonical entries = %d, want 4", len(want))
	}
	if want[0].Path != "a.go" || want[0].Range.Start.Offset != 10 || want[0].Text != "at-10" {
		t.Fatalf("first = %+v", want[0])
	}
	if want[1].Path != "a.go" || want[1].Range.Start.Offset != 100 {
		t.Fatalf("second = %+v", want[1])
	}
	if want[2].Path != "b.go" || want[3].Path != "c.go" {
		t.Fatalf("paths = %s %s", want[2].Path, want[3].Path)
	}
}

func TestBuildIsRepeatable(t *testing.T) {
	docs := []*syntax.Document{
		goDoc("b.go", cmt("b", 5, 6)),
		goDoc("a.go", cmt("a2", 8, 9), cmt("a1", 2, 3)),
	}
	first := mustBuild(t, docs...)
	second := mustBuild(t, docs...)
	if !reflect.DeepEqual(first.Entries(), second.Entries()) {
		t.Fatal("repeated builds differ")
	}
}

func TestEarlierCommentRenumbersLaterIDs(t *testing.T) {
	// An ID belongs to one built index. Inserting a comment that sorts
	// earlier assigns a new ID to a comment that already had one. That
	// renumbering is snapshot-local identity.
	later := goDoc("b.go", cmt("later", 5, 6))
	first := mustBuild(t, later)
	got, ok := first.Lookup(mustParse(t, "C000001"))
	if !ok || got.Path != "b.go" || got.Text != "later" {
		t.Fatalf("first index = %+v, ok=%v", got, ok)
	}

	earlier := goDoc("a.go", cmt("earlier", 10, 11))
	second := mustBuild(t, later, earlier)
	entries := second.Entries()
	if len(entries) != 2 {
		t.Fatalf("len = %d", len(entries))
	}
	if entries[0].ID.String() != "C000001" || entries[0].Path != "a.go" || entries[0].Text != "earlier" {
		t.Fatalf("renumbered first = %+v", entries[0])
	}
	if entries[1].ID.String() != "C000002" || entries[1].Path != "b.go" || entries[1].Text != "later" {
		t.Fatalf("renumbered later = %+v, want C000002", entries[1])
	}
	if _, ok := second.Lookup(mustParse(t, "C000001")); !ok {
		t.Fatal("C000001 missing after renumbering")
	}
	found, ok := second.Lookup(mustParse(t, "C000002"))
	if !ok || found.Text != "later" {
		t.Fatalf("C000002 = %+v, ok=%v", found, ok)
	}
}

func TestBuildPreservesNormalizedText(t *testing.T) {
	texts := []string{
		"  keep  \n",
		"go:build linux",
		"\t// already normalized\r\n",
		"",
		"caf\u00e9",
	}
	comments := make([]syntax.Comment, len(texts))
	for i, text := range texts {
		comments[i] = cmt(text, i*10, i*10+1)
	}
	idx := mustBuild(t, goDoc("a.go", comments...))
	entries := idx.Entries()
	if len(entries) != len(texts) {
		t.Fatalf("len = %d", len(entries))
	}
	for i, text := range texts {
		if entries[i].Text != text {
			t.Fatalf("text %d = %q, want %q", i, entries[i].Text, text)
		}
	}
}

func TestBuildPassesLanguageThrough(t *testing.T) {
	goFile := goDoc("a.go", cmt("go", 0, 1))
	other := &syntax.Document{
		Path:     "notes.txt",
		Language: syntax.Language("text"),
		Comments: []syntax.Comment{cmt("text", 0, 1)},
	}
	idx := mustBuild(t, other, goFile)
	entries := idx.Entries()
	if len(entries) != 2 {
		t.Fatalf("len = %d", len(entries))
	}
	if entries[0].Path != "a.go" || entries[0].Language != syntax.Go {
		t.Fatalf("go entry = %+v", entries[0])
	}
	if entries[1].Path != "notes.txt" || entries[1].Language != syntax.Language("text") {
		t.Fatalf("text entry = %+v", entries[1])
	}
}

func TestBuildFromSyntaxDocument(t *testing.T) {
	src := []byte("//go:build linux\n\npackage p\n\n// keep me  \nfunc F() {}\n")
	doc, err := syntax.Parse("z.go", src)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	reversed := &syntax.Document{
		Path:     doc.Path,
		Language: doc.Language,
		Comments: append([]syntax.Comment(nil), doc.Comments...),
	}
	slices.Reverse(reversed.Comments)

	original := mustBuild(t, doc)
	flipped := mustBuild(t, reversed)
	if !reflect.DeepEqual(original.Entries(), flipped.Entries()) {
		t.Fatal("reversed syntax comments changed the index")
	}
	entries := original.Entries()
	if len(entries) != len(doc.Comments) {
		t.Fatalf("len = %d, want %d", len(entries), len(doc.Comments))
	}
	if doc.Comments[0].Text != "go:build linux" || doc.Comments[1].Text != "keep me  " {
		t.Fatalf("syntax texts = %q, %q", doc.Comments[0].Text, doc.Comments[1].Text)
	}
	for i, comment := range doc.Comments {
		got := entries[i]
		if got.Path != "z.go" || got.Language != syntax.Go || got.Text != comment.Text || got.Range != comment.Range {
			t.Fatalf("entry %d = %+v, comment %+v", i, got, comment)
		}
	}
	if doc.Comments[0].Range.Start.Offset >= doc.Comments[1].Range.Start.Offset {
		t.Fatal("syntax fixture is not in increasing offset order")
	}
}

func TestBuildDoesNotRetainCallerState(t *testing.T) {
	doc := goDoc("a.go", cmt("orig", 0, 1))
	idx := mustBuild(t, doc)
	doc.Path = "moved.go"
	doc.Language = "changed"
	doc.Comments[0].Text = "changed"
	doc.Comments[0].Range.Start.Offset = 99
	got, ok := idx.Lookup(mustParse(t, "C000001"))
	if !ok || got.Path != "a.go" || got.Language != syntax.Go || got.Text != "orig" || got.Range.Start.Offset != 0 {
		t.Fatalf("lookup after caller mutation = %+v, ok=%v", got, ok)
	}
}

func TestBuildAllowsEmptyRange(t *testing.T) {
	idx := mustBuild(t, goDoc("a.go", cmt("empty-span", 0, 0)))
	got := idx.Entries()[0]
	if got.Range.Start.Offset != 0 || got.Range.End.Offset != 0 || got.Text != "empty-span" {
		t.Fatalf("entry = %+v", got)
	}
}

func TestParseID(t *testing.T) {
	valid := []string{"C000001", "C000042", "C099999", "C999999", "C1000000", "C1000001"}
	for _, text := range valid {
		id := mustParse(t, text)
		if id.String() != text {
			t.Fatalf("String = %q, want %q", id.String(), text)
		}
	}

	aboveInt64 := "C9223372036854775808"
	if got := mustParse(t, aboveInt64).String(); got != aboveInt64 {
		t.Fatalf("String = %q, want %s", got, aboveInt64)
	}
	maxUint := "C18446744073709551615"
	if got := mustParse(t, maxUint).String(); got != maxUint {
		t.Fatalf("String = %q, want %s", got, maxUint)
	}

	invalid := []string{
		"C000000",
		"c000001",
		"C1",
		"C00001",
		"C000001x",
		"X000001",
		"C-00001",
		"C",
		"",
		"C000001 ",
		" C000001",
		"C000_001",
		"C0000001",
		"C0999999",
		"C+000001",
	}
	for _, text := range invalid {
		id, err := index.ParseID(text)
		if id.Valid() || id.String() != "" {
			t.Fatalf("ParseID(%q) = %v, want the zero ID", text, id)
		}
		var idErr *index.IDError
		if !errors.As(err, &idErr) || !errors.Is(err, index.ErrInvalidID) || idErr.Text != text {
			t.Fatalf("ParseID(%q) error = %v", text, err)
		}
	}

	overflows := []string{
		"C18446744073709551616",
		"C" + strings.Repeat("9", 40),
	}
	for _, text := range overflows {
		id, err := index.ParseID(text)
		if id.Valid() {
			t.Fatalf("ParseID(%q) wrapped to %s", text, id)
		}
		var idErr *index.IDError
		if !errors.As(err, &idErr) || !errors.Is(err, index.ErrIDOverflow) || idErr.Text != text {
			t.Fatalf("ParseID(%q) error = %v", text, err)
		}
		if errors.Is(err, index.ErrInvalidID) {
			t.Fatalf("ParseID(%q) reported invalid syntax for overflow", text)
		}
	}

	if (index.ID{}).Valid() || (index.ID{}).String() != "" {
		t.Fatal("zero ID is valid")
	}
}

func TestLookup(t *testing.T) {
	idx := mustBuild(t,
		goDoc("b.go", cmt("b", 5, 6)),
		goDoc("a.go", cmt("a", 10, 11)),
	)
	found, ok := idx.Lookup(mustParse(t, "C000001"))
	if !ok || found.Path != "a.go" || found.Text != "a" || found.Range.Start.Offset != 10 {
		t.Fatalf("C000001 = %+v, ok=%v", found, ok)
	}
	found, ok = idx.Lookup(mustParse(t, "C000002"))
	if !ok || found.Path != "b.go" || found.Text != "b" {
		t.Fatalf("C000002 = %+v, ok=%v", found, ok)
	}
	for _, text := range []string{"C000003", "C000042", "C999999", "C1000000"} {
		if _, ok := idx.Lookup(mustParse(t, text)); ok {
			t.Fatalf("Lookup(%s) succeeded", text)
		}
	}
	if _, ok := idx.Lookup(index.ID{}); ok {
		t.Fatal("Lookup of the zero ID succeeded")
	}

	var none *index.Index
	if _, ok := none.Lookup(mustParse(t, "C000001")); ok || none.Len() != 0 || len(none.Entries()) != 0 {
		t.Fatal("nil index returned data")
	}
}

func TestBuildRejectsDuplicatePath(t *testing.T) {
	cases := []struct {
		name string
		docs []*syntax.Document
	}{
		{
			name: "both empty",
			docs: []*syntax.Document{
				{Path: "a.go", Language: syntax.Go},
				{Path: "a.go", Language: syntax.Go},
			},
		},
		{
			name: "one empty",
			docs: []*syntax.Document{
				goDoc("a.go", cmt("x", 1, 2)),
				{Path: "a.go", Language: syntax.Go},
			},
		},
		{
			name: "both commented",
			docs: []*syntax.Document{
				goDoc("a.go", cmt("x", 1, 2)),
				goDoc("dir/b.go", cmt("y", 1, 2)),
				goDoc("a.go", cmt("z", 3, 4)),
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := requireFailure(t, tc.docs)
			var dup *index.DuplicatePathError
			if !errors.As(err, &dup) || dup.Path != "a.go" {
				t.Fatalf("error = %v", err)
			}
			if !strings.Contains(err.Error(), "a.go") {
				t.Fatalf("error %q does not identify the path", err)
			}
		})
	}
}

func TestBuildRejectsNilDocument(t *testing.T) {
	err := requireFailure(t, []*syntax.Document{nil})
	var nilDoc *index.NilDocumentError
	if !errors.As(err, &nilDoc) || nilDoc.Index != 0 {
		t.Fatalf("error = %v", err)
	}

	err = requireFailure(t, []*syntax.Document{goDoc("a.go", cmt("x", 1, 2)), nil})
	nilDoc = nil
	if !errors.As(err, &nilDoc) || nilDoc.Index != 1 {
		t.Fatalf("error = %v", err)
	}
}

func TestBuildRejectsPath(t *testing.T) {
	cases := []struct {
		path string
		want error
	}{
		{path: "", want: index.ErrEmptyPath},
		{path: ".", want: index.ErrDotPath},
		{path: "/a.go", want: index.ErrAbsolutePath},
		{path: "/tmp/a.go", want: index.ErrAbsolutePath},
		{path: "a//b.go", want: index.ErrUncleanPath},
		{path: "a/./b.go", want: index.ErrUncleanPath},
		{path: "a/../b.go", want: index.ErrUncleanPath},
		{path: "a.go/", want: index.ErrUncleanPath},
		{path: "./a.go", want: index.ErrUncleanPath},
		{path: "../a.go", want: index.ErrEscapingPath},
		{path: "../../a.go", want: index.ErrEscapingPath},
		{path: "..", want: index.ErrEscapingPath},
	}
	for _, tc := range cases {
		t.Run(tc.path, func(t *testing.T) {
			err := requireFailure(t, []*syntax.Document{goDoc(tc.path, cmt("x", 1, 2))})
			var pathErr *index.PathError
			if !errors.As(err, &pathErr) || !errors.Is(err, tc.want) || pathErr.Path != tc.path || pathErr.Index != 0 {
				t.Fatalf("error = %v, want %v", err, tc.want)
			}
		})
	}

	kept := []string{
		"Foo.go",
		"a/b/c.go",
		".hidden/file.go",
		"foo/..bar.go",
		"my dir/a.go",
		`dir\file.go`,
		"caf\u00e9.go",
	}
	for _, path := range kept {
		idx := mustBuild(t, goDoc(path, cmt("x", 1, 2)))
		if got := idx.Entries()[0].Path; got != path {
			t.Fatalf("path = %q, want %q", got, path)
		}
	}
}

func TestBuildRejectsRange(t *testing.T) {
	valid := cmt("x", 5, 9).Range
	cases := []struct {
		name  string
		rng   syntax.Range
		want  error
		index int
	}{
		{
			name: "negative start",
			rng: syntax.Range{
				Start: syntax.Position{Offset: -1, Line: 1, Column: 1},
				End:   valid.End,
			},
			want: index.ErrNegativeOffset,
		},
		{
			name: "negative end",
			rng: syntax.Range{
				Start: valid.Start,
				End:   syntax.Position{Offset: -1, Line: 1, Column: 1},
			},
			want: index.ErrNegativeOffset,
		},
		{
			name: "end before start",
			rng: syntax.Range{
				Start: syntax.Position{Offset: 5, Line: 1, Column: 1},
				End:   syntax.Position{Offset: 4, Line: 1, Column: 1},
			},
			want: index.ErrEndBeforeStart,
		},
		{
			name: "start line",
			rng: syntax.Range{
				Start: syntax.Position{Offset: 5, Line: 0, Column: 1},
				End:   valid.End,
			},
			want: index.ErrPosition,
		},
		{
			name: "end line",
			rng: syntax.Range{
				Start: valid.Start,
				End:   syntax.Position{Offset: 9, Line: 0, Column: 1},
			},
			want: index.ErrPosition,
		},
		{
			name: "start column",
			rng: syntax.Range{
				Start: syntax.Position{Offset: 5, Line: 1, Column: 0},
				End:   valid.End,
			},
			want: index.ErrPosition,
		},
		{
			name: "end column",
			rng: syntax.Range{
				Start: valid.Start,
				End:   syntax.Position{Offset: 9, Line: 1, Column: 0},
			},
			want:  index.ErrPosition,
			index: 1,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			bad := syntax.Comment{Text: "bad", Range: tc.rng}
			docs := []*syntax.Document{goDoc("a.go", cmt("good", 1, 2), bad)}
			if tc.index == 0 {
				docs = []*syntax.Document{goDoc("a.go", bad)}
			}
			err := requireFailure(t, docs)
			var commentErr *index.CommentError
			if !errors.As(err, &commentErr) || !errors.Is(err, tc.want) {
				t.Fatalf("error = %v, want %v", err, tc.want)
			}
			if commentErr.Path != "a.go" || commentErr.Index != tc.index || commentErr.Range != tc.rng {
				t.Fatalf("error context = %+v", commentErr)
			}
			if !strings.Contains(err.Error(), "a.go") {
				t.Fatalf("error %q does not identify the path", err)
			}
		})
	}
}

func TestBuildRejectsDuplicatePosition(t *testing.T) {
	shared := cmt("one", 4, 8).Range
	docs := []*syntax.Document{goDoc("a.go",
		syntax.Comment{Text: "one", Range: shared},
		cmt("between", 1, 2),
		syntax.Comment{Text: "two", Range: shared},
	)}
	err := requireFailure(t, docs)
	var dup *index.DuplicatePositionError
	if !errors.As(err, &dup) {
		t.Fatalf("error = %v", err)
	}
	if dup.Path != "a.go" || dup.Index != 0 || dup.Other != 2 || dup.Range != shared {
		t.Fatalf("error = %+v", dup)
	}
}

func TestEntriesAreCopies(t *testing.T) {
	idx := mustBuild(t, goDoc("a.go", cmt("orig", 1, 2), cmt("next", 3, 4)))
	entries := idx.Entries()
	entries[0].Text = "changed"
	entries[0].Path = "other.go"
	entries[0].Language = "nope"
	entries[0].Range.Start.Offset = 50
	entries[0].ID = index.ID{}
	entries = append(entries, index.Entry{})

	if idx.Len() != 2 {
		t.Fatalf("Len = %d", idx.Len())
	}
	again := idx.Entries()
	if again[0].Text != "orig" || again[0].Path != "a.go" || again[0].Language != syntax.Go || again[0].Range.Start.Offset != 1 {
		t.Fatalf("internal entry changed: %+v", again[0])
	}
	if again[0].ID.String() != "C000001" {
		t.Fatalf("ID changed to %s", again[0].ID)
	}
	found, ok := idx.Lookup(mustParse(t, "C000001"))
	if !ok || found.Text != "orig" || found.Path != "a.go" {
		t.Fatalf("lookup changed: %+v", found)
	}
}

func TestConcurrentRead(t *testing.T) {
	idx := mustBuild(t,
		goDoc("b.go", cmt("b", 5, 6)),
		goDoc("a.go", cmt("second", 100, 101), cmt("first", 10, 11)),
	)
	golden := idx.Entries()
	first := mustParse(t, "C000001")
	unknown := mustParse(t, "C000099")

	var wg sync.WaitGroup
	errCh := make(chan error, 64)
	for n := 0; n < 32; n++ {
		wg.Add(3)
		go func() {
			defer wg.Done()
			if !reflect.DeepEqual(idx.Entries(), golden) || idx.Len() != len(golden) {
				errCh <- errors.New("entries")
			}
		}()
		go func() {
			defer wg.Done()
			got, ok := idx.Lookup(first)
			if !ok || got.Path != "a.go" || got.Text != "first" || got.Range.Start.Offset != 10 {
				errCh <- errors.New("lookup")
			}
		}()
		go func() {
			defer wg.Done()
			if _, ok := idx.Lookup(unknown); ok {
				errCh <- errors.New("unknown")
			}
		}()
	}
	wg.Wait()
	close(errCh)
	for err := range errCh {
		t.Fatal(err)
	}
}

const emptyDigest = "sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"

func TestDigestBytes(t *testing.T) {
	if index.DigestBytes(nil) != emptyDigest || index.DigestBytes([]byte{}) != emptyDigest {
		t.Fatalf("empty digest = %s", index.DigestBytes(nil))
	}
	if index.DigestBytes([]byte("a\r\n")) == index.DigestBytes([]byte("a\n")) {
		t.Fatal("CRLF and LF hashed as the same bytes")
	}
	if got := index.DigestBytes([]byte("a\r\n")); len(got) != len(emptyDigest) || got[:7] != "sha256:" {
		t.Fatalf("digest = %s", got)
	}
}

func TestPersistEmptyAndOneEntry(t *testing.T) {
	policy := index.DigestBytes([]byte("policy"))
	root := t.TempDir()
	snap, err := index.Persist(root, policy, nil, nil)
	if err != nil {
		t.Fatalf("Persist empty: %v", err)
	}
	comments := readPersisted(t, root, index.CommentsPath)
	snapshot := readPersisted(t, root, index.SnapshotPath)
	if len(comments) != 0 {
		t.Fatalf("comments = %q, want empty", comments)
	}
	if snap.CommentCount != 0 || snap.Schema != index.SchemaVersion || snap.CommentsDigest != emptyDigest {
		t.Fatalf("snapshot = %+v", snap)
	}
	wantSnap := fmt.Sprintf("{\"schema\":1,\"policy_identity\":%q,\"comments_digest\":%q,\"comment_count\":0,\"sources\":[]}\n", policy, emptyDigest)
	if string(snapshot) != wantSnap {
		t.Fatalf("snapshot bytes = %s\nwant %s", snapshot, wantSnap)
	}
	idx, loaded, err := index.Load(root)
	if err != nil || idx == nil || idx.Len() != 0 || !index.Current(loaded, policy, nil) {
		t.Fatalf("Load empty = %v %+v %v", idx, loaded, err)
	}
	if entries := idx.Entries(); entries == nil || len(entries) != 0 {
		t.Fatalf("entries = %#v", entries)
	}

	body := []byte("package a")
	source := index.Source{Path: "a.go", Language: syntax.Go, Digest: index.DigestBytes(body)}
	doc := goDoc("a.go", cmt("alpha", 10, 24))
	again := t.TempDir()
	idx = mustBuild(t, doc)
	snap, err = index.Persist(again, policy, []index.Source{source}, idx)
	if err != nil {
		t.Fatalf("Persist one: %v", err)
	}
	comments = readPersisted(t, again, index.CommentsPath)
	wantLine := "{\"id\":\"C000001\",\"path\":\"a.go\",\"language\":\"go\",\"range\":{\"start\":{\"offset\":10,\"line\":1,\"column\":1},\"end\":{\"offset\":24,\"line\":1,\"column\":1}},\"text\":\"alpha\"}\n"
	if string(comments) != wantLine {
		t.Fatalf("comments = %s", comments)
	}
	if snap.CommentCount != 1 || snap.CommentsDigest != index.DigestBytes(comments) {
		t.Fatalf("digest = %s count %d", snap.CommentsDigest, snap.CommentCount)
	}
	wantSnap = fmt.Sprintf("{\"schema\":1,\"policy_identity\":%q,\"comments_digest\":%q,\"comment_count\":1,\"sources\":[{\"path\":\"a.go\",\"language\":\"go\",\"digest\":%q}]}\n", policy, snap.CommentsDigest, source.Digest)
	if got := readPersisted(t, again, index.SnapshotPath); string(got) != wantSnap {
		t.Fatalf("snapshot = %s\nwant %s", got, wantSnap)
	}
	loadedIdx, loaded, err := index.Load(again)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	found, ok := loadedIdx.Lookup(mustParse(t, "C000001"))
	if !ok || found.Text != "alpha" || found.Path != "a.go" || found.Language != syntax.Go || found.Range.Start.Offset != 10 {
		t.Fatalf("lookup = %+v ok=%v", found, ok)
	}
	if !index.Current(loaded, policy, []index.Source{source}) {
		t.Fatal("one-entry snapshot is stale")
	}
	if bytes.Contains(readPersisted(t, again, index.SnapshotPath), []byte(again)) {
		t.Fatal("snapshot contains an absolute path")
	}
	if bytes.Contains(snapshot, []byte("timestamp")) || bytes.Contains(snapshot, []byte("hostname")) {
		t.Fatal("empty snapshot contains environment metadata")
	}
}

func TestPersistIsByteStable(t *testing.T) {
	policy := index.DigestBytes([]byte("policy"))
	a := index.Source{Path: "a.go", Language: syntax.Go, Digest: index.DigestBytes([]byte("a"))}
	b := index.Source{Path: "b.go", Language: syntax.Go, Digest: index.DigestBytes([]byte("b"))}
	text := "line\n<tag>\n"
	docs := []*syntax.Document{
		goDoc("b.go", cmt("b", 5, 6)),
		goDoc("a.go", cmt(text, 1, 2), cmt("second", 10, 16)),
	}
	reversedDocs := []*syntax.Document{docs[1], docs[0]}
	roots := []string{t.TempDir(), t.TempDir()}
	orders := [][]index.Source{{b, a}, {a, b}}
	docOrders := [][]*syntax.Document{docs, reversedDocs}
	var got [2][2][]byte
	for i := range roots {
		input := append([]index.Source(nil), orders[i]...)
		idx := mustBuild(t, docOrders[i]...)
		if _, err := index.Persist(roots[i], policy, input, idx); err != nil {
			t.Fatalf("Persist: %v", err)
		}
		if !slices.Equal(input, orders[i]) {
			t.Fatal("Persist reordered the caller sources")
		}
		got[i][0] = readPersisted(t, roots[i], index.SnapshotPath)
		got[i][1] = readPersisted(t, roots[i], index.CommentsPath)
		if _, err := index.Persist(roots[i], policy, input, idx); err != nil {
			t.Fatalf("Persist again: %v", err)
		}
		if !bytes.Equal(readPersisted(t, roots[i], index.SnapshotPath), got[i][0]) || !bytes.Equal(readPersisted(t, roots[i], index.CommentsPath), got[i][1]) {
			t.Fatal("repeating Persist changed bytes")
		}
	}
	if !bytes.Equal(got[0][0], got[1][0]) || !bytes.Equal(got[0][1], got[1][1]) {
		t.Fatalf("orders diverged\n%s\n%s", got[0][0], got[1][0])
	}
	if bytes.Count(got[0][1], []byte("\n")) != 3 {
		t.Fatalf("jsonl newlines = %d", bytes.Count(got[0][1], []byte("\n")))
	}
	if !bytes.Contains(got[0][1], []byte(`"text":"line\n<tag>\n"`)) {
		t.Fatal("multiline text was not one JSONL record")
	}
	idx, _, err := index.Load(roots[0])
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if idx.Entries()[0].Text != text {
		t.Fatalf("text = %q", idx.Entries()[0].Text)
	}
	names := indexNames(t, roots[0])
	if !slices.Equal(names, []string{"comments.jsonl", "snapshot.json"}) {
		t.Fatalf("index dir = %q", names)
	}
}

func TestPersistLargeComment(t *testing.T) {
	text := strings.Repeat("x", 40*1024) + "\n" + strings.Repeat("y", 40*1024)
	if len(text) <= 64*1024 {
		t.Fatal("fixture is not larger than 64 KiB")
	}
	root := t.TempDir()
	policy := index.DigestBytes([]byte("policy"))
	source := index.Source{Path: "a.go", Language: syntax.Go, Digest: index.DigestBytes([]byte("package a"))}
	idx := mustBuild(t, goDoc("a.go", cmt(text, 0, 1)))
	if _, err := index.Persist(root, policy, []index.Source{source}, idx); err != nil {
		t.Fatalf("Persist: %v", err)
	}
	loaded, _, err := index.Load(root)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if loaded.Len() != 1 || loaded.Entries()[0].Text != text {
		t.Fatalf("len=%d text len=%d", loaded.Len(), len(loaded.Entries()[0].Text))
	}
	entries := loaded.Entries()
	entries[0].Text = "changed"
	if again := loaded.Entries(); again[0].Text != text {
		t.Fatal("Entries returned shared state")
	}
	found, ok := loaded.Lookup(mustParse(t, "C000001"))
	if !ok || found.Text != text {
		t.Fatal("lookup saw the mutated copy")
	}
}

func TestPersistRejectsInvalidInputBeforeWrite(t *testing.T) {
	root := t.TempDir()
	policy := index.DigestBytes([]byte("policy"))
	if _, err := index.Persist(root, "sha256:nope", nil, nil); !errors.Is(err, index.ErrInvalidDigest) {
		t.Fatalf("bad policy = %v", err)
	}
	source := index.Source{Path: "a.go", Language: syntax.Go, Digest: policy}
	dup := []index.Source{source, source}
	idx := mustBuild(t, goDoc("a.go", cmt("a", 0, 1)))
	if _, err := index.Persist(root, policy, dup, idx); !errors.Is(err, index.ErrDuplicateSource) {
		t.Fatalf("duplicate = %v", err)
	}
	bad := source
	bad.Path = "a/../a.go"
	if _, err := index.Persist(root, policy, []index.Source{bad}, idx); err == nil {
		t.Fatal("unclean source path was accepted")
	}
	if _, err := os.Stat(filepath.Join(root, ".nodex")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf(".nodex stat = %v", err)
	}
	if _, err := index.Persist("", policy, nil, nil); err == nil {
		t.Fatal("empty root was accepted")
	}
}

func TestLoadRejectsCorruptState(t *testing.T) {
	policy := index.DigestBytes([]byte("policy"))
	source := testSource{Path: "a.go", Language: "go", Digest: index.DigestBytes([]byte("package a"))}
	other := testSource{Path: "b.go", Language: "go", Digest: index.DigestBytes([]byte("package b"))}
	good := []string{entryLine(t, "C000001", "a.go", "alpha", 1, 2)}
	comments := lines(good...)

	t.Run("missing snapshot", func(t *testing.T) {
		root := t.TempDir()
		if _, _, err := index.Load(root); !errors.Is(err, index.ErrAbsent) || errors.Is(err, index.ErrCorrupt) {
			t.Fatalf("missing = %v", err)
		}
		writeRaw(t, root, index.CommentsPath, comments)
		if _, _, err := index.Load(root); !errors.Is(err, index.ErrAbsent) {
			t.Fatalf("comments without snapshot = %v", err)
		}
	})
	t.Run("missing comments", func(t *testing.T) {
		root := t.TempDir()
		snap := matchingSnapshot(policy, 1, []testSource{source}, comments)
		writeRaw(t, root, index.SnapshotPath, mustJSON(t, snap))
		_, _, err := index.Load(root)
		if !errors.Is(err, index.ErrCorrupt) || errors.Is(err, index.ErrAbsent) {
			t.Fatalf("missing comments = %v", err)
		}
	})
	t.Run("malformed snapshot", func(t *testing.T) {
		root := t.TempDir()
		writePair(t, root, []byte("{"), comments)
		if _, _, err := index.Load(root); !errors.Is(err, index.ErrCorrupt) {
			t.Fatalf("malformed = %v", err)
		}
	})
	t.Run("unknown field", func(t *testing.T) {
		root := t.TempDir()
		raw := bytes.Replace(mustJSON(t, matchingSnapshot(policy, 1, []testSource{source}, comments)), []byte("{"), []byte("{\"generated_at\":1,"), 1)
		writePair(t, root, raw, comments)
		_, _, err := index.Load(root)
		var unknown *index.UnknownFieldError
		if !errors.Is(err, index.ErrCorrupt) || !errors.As(err, &unknown) || unknown.Name != "generated_at" {
			t.Fatalf("unknown = %v", err)
		}
	})
	t.Run("trailing snapshot", func(t *testing.T) {
		root := t.TempDir()
		raw := append(mustJSON(t, matchingSnapshot(policy, 1, []testSource{source}, comments)), []byte("{\"extra\":true}\n")...)
		writePair(t, root, raw, comments)
		if _, _, err := index.Load(root); !errors.Is(err, index.ErrCorrupt) || !errors.Is(err, index.ErrTrailingData) {
			t.Fatalf("trailing = %v", err)
		}
	})
	t.Run("schema", func(t *testing.T) {
		for _, schema := range []string{"2", "\"1\"", "1.0", "true"} {
			root := t.TempDir()
			raw := bytes.Replace(mustJSON(t, matchingSnapshot(policy, 1, []testSource{source}, comments)), []byte(`"schema":1`), []byte(`"schema":`+schema), 1)
			writePair(t, root, raw, comments)
			if _, _, err := index.Load(root); !errors.Is(err, index.ErrCorrupt) || !errors.Is(err, index.ErrUnsupportedSchema) {
				t.Fatalf("schema %s = %v", schema, err)
			}
		}
	})
	t.Run("digest", func(t *testing.T) {
		root := t.TempDir()
		snap := matchingSnapshot(policy, 1, []testSource{source}, comments)
		snap.PolicyIdentity = "sha256:ABCD"
		writePair(t, root, mustJSON(t, snap), comments)
		if _, _, err := index.Load(root); !errors.Is(err, index.ErrCorrupt) || !errors.Is(err, index.ErrInvalidDigest) {
			t.Fatalf("digest = %v", err)
		}
	})
	t.Run("count", func(t *testing.T) {
		root := t.TempDir()
		snap := matchingSnapshot(policy, 1, []testSource{source}, comments)
		snap.CommentCount = 2
		writePair(t, root, mustJSON(t, snap), comments)
		if _, _, err := index.Load(root); !errors.Is(err, index.ErrCorrupt) || !errors.Is(err, index.ErrCountMismatch) {
			t.Fatalf("count = %v", err)
		}
	})
	t.Run("malformed record", func(t *testing.T) {
		root := t.TempDir()
		bad := []byte("{not json}\n")
		writePair(t, root, mustJSON(t, matchingSnapshot(policy, 1, []testSource{source}, bad)), bad)
		if _, _, err := index.Load(root); !errors.Is(err, index.ErrCorrupt) {
			t.Fatalf("record = %v", err)
		}
	})
	t.Run("blank line and missing newline", func(t *testing.T) {
		root := t.TempDir()
		blank := append(append([]byte{}, comments...), '\n')
		writePair(t, root, mustJSON(t, matchingSnapshot(policy, 2, []testSource{source}, blank)), blank)
		if _, _, err := index.Load(root); !errors.Is(err, index.ErrCorrupt) {
			t.Fatalf("blank = %v", err)
		}
		root = t.TempDir()
		trimmed := bytes.TrimSuffix(comments, []byte("\n"))
		writePair(t, root, mustJSON(t, matchingSnapshot(policy, 1, []testSource{source}, trimmed)), trimmed)
		if _, _, err := index.Load(root); !errors.Is(err, index.ErrCorrupt) {
			t.Fatalf("newline = %v", err)
		}
	})
	t.Run("id", func(t *testing.T) {
		for name, id := range map[string]string{
			"noncanonical": "C0000001",
			"short":        "C1",
			"skipped":      "C000003",
			"duplicate":    "C000001",
		} {
			t.Run(name, func(t *testing.T) {
				root := t.TempDir()
				var body []byte
				if name == "skipped" || name == "duplicate" {
					body = lines(good[0], entryLine(t, id, "b.go", "beta", 1, 2))
				} else {
					body = lines(entryLine(t, id, "a.go", "alpha", 1, 2))
				}
				count := bytes.Count(body, []byte("\n"))
				sources := []testSource{source}
				if count == 2 {
					sources = []testSource{source, other}
				}
				writePair(t, root, mustJSON(t, matchingSnapshot(policy, count, sources, body)), body)
				if _, _, err := index.Load(root); !errors.Is(err, index.ErrCorrupt) {
					t.Fatalf("Load = %v", err)
				}
			})
		}
	})
	t.Run("out of order", func(t *testing.T) {
		root := t.TempDir()
		body := lines(
			entryLine(t, "C000001", "b.go", "beta", 1, 2),
			entryLine(t, "C000002", "a.go", "alpha", 1, 2),
		)
		writePair(t, root, mustJSON(t, matchingSnapshot(policy, 2, []testSource{source, other}, body)), body)
		if _, _, err := index.Load(root); !errors.Is(err, index.ErrCorrupt) || !errors.Is(err, index.ErrOutOfOrder) {
			t.Fatalf("order = %v", err)
		}
	})
	t.Run("range", func(t *testing.T) {
		root := t.TempDir()
		body := lines(entryLineAt(t, "C000001", "a.go", "alpha", 0, 1, 0, 1))
		writePair(t, root, mustJSON(t, matchingSnapshot(policy, 1, []testSource{source}, body)), body)
		if _, _, err := index.Load(root); !errors.Is(err, index.ErrCorrupt) || !errors.Is(err, index.ErrPosition) {
			t.Fatalf("range = %v", err)
		}
	})
	t.Run("unknown comment field", func(t *testing.T) {
		root := t.TempDir()
		body := lines(strings.Replace(good[0], "{", "{\"raw\":\"x\",", 1))
		writePair(t, root, mustJSON(t, matchingSnapshot(policy, 1, []testSource{source}, body)), body)
		var unknown *index.UnknownFieldError
		_, _, err := index.Load(root)
		if !errors.Is(err, index.ErrCorrupt) || !errors.As(err, &unknown) || unknown.Name != "raw" {
			t.Fatalf("unknown comment field = %v", err)
		}
	})
	t.Run("duplicate source", func(t *testing.T) {
		root := t.TempDir()
		snap := matchingSnapshot(policy, 1, []testSource{source, source}, comments)
		writePair(t, root, mustJSON(t, snap), comments)
		if _, _, err := index.Load(root); !errors.Is(err, index.ErrCorrupt) || !errors.Is(err, index.ErrDuplicateSource) {
			t.Fatalf("duplicate source = %v", err)
		}
	})
	t.Run("duplicate position", func(t *testing.T) {
		root := t.TempDir()
		body := lines(
			entryLine(t, "C000001", "a.go", "one", 1, 2),
			entryLine(t, "C000002", "a.go", "two", 1, 2),
		)
		writePair(t, root, mustJSON(t, matchingSnapshot(policy, 2, []testSource{source}, body)), body)
		_, _, err := index.Load(root)
		var dup *index.DuplicatePositionError
		if !errors.Is(err, index.ErrCorrupt) || !errors.As(err, &dup) {
			t.Fatalf("position = %v", err)
		}
	})
	t.Run("unsorted sources", func(t *testing.T) {
		root := t.TempDir()
		body := lines(
			entryLine(t, "C000001", "a.go", "alpha", 1, 2),
			entryLine(t, "C000002", "b.go", "beta", 3, 4),
		)
		snap := matchingSnapshot(policy, 2, []testSource{other, source}, body)
		writePair(t, root, mustJSON(t, snap), body)
		if _, _, err := index.Load(root); !errors.Is(err, index.ErrCorrupt) || !errors.Is(err, index.ErrUnsortedSources) {
			t.Fatalf("unsorted = %v", err)
		}
	})
}

func TestCommentsTamperAndMixedGeneration(t *testing.T) {
	policy := index.DigestBytes([]byte("policy"))
	source := func(path, body string) index.Source {
		return index.Source{Path: path, Language: syntax.Go, Digest: index.DigestBytes([]byte(body))}
	}
	root := t.TempDir()
	a := mustBuild(t, goDoc("a.go", cmt("alpha", 0, 1)))
	if _, err := index.Persist(root, policy, []index.Source{source("a.go", "A")}, a); err != nil {
		t.Fatalf("Persist A: %v", err)
	}
	snapshotA := readPersisted(t, root, index.SnapshotPath)
	commentsA := readPersisted(t, root, index.CommentsPath)
	commentsA[len(commentsA)-2] = 'Z'
	if err := os.WriteFile(filepath.Join(root, filepath.FromSlash(index.CommentsPath)), commentsA, 0o644); err != nil {
		t.Fatalf("tamper: %v", err)
	}
	if _, _, err := index.Load(root); !errors.Is(err, index.ErrCorrupt) || !errors.Is(err, index.ErrDigestMismatch) {
		t.Fatalf("tamper = %v", err)
	}
	if !bytes.Equal(readPersisted(t, root, index.SnapshotPath), snapshotA) {
		t.Fatal("tamper rewrote the snapshot")
	}

	mixed := t.TempDir()
	if _, err := index.Persist(mixed, policy, []index.Source{source("a.go", "A")}, a); err != nil {
		t.Fatalf("Persist A: %v", err)
	}
	snapshotA = append([]byte(nil), readPersisted(t, mixed, index.SnapshotPath)...)
	b := mustBuild(t, goDoc("a.go", cmt("beta", 0, 1)))
	if _, err := index.Persist(mixed, policy, []index.Source{source("a.go", "B")}, b); err != nil {
		t.Fatalf("Persist B: %v", err)
	}
	if err := os.WriteFile(filepath.Join(mixed, filepath.FromSlash(index.SnapshotPath)), snapshotA, 0o644); err != nil {
		t.Fatalf("restore snapshot A: %v", err)
	}
	if _, _, err := index.Load(mixed); !errors.Is(err, index.ErrCorrupt) || !errors.Is(err, index.ErrDigestMismatch) {
		t.Fatalf("mixed = %v", err)
	}
}

func TestPublishFailureCleansTemps(t *testing.T) {
	policy := index.DigestBytes([]byte("policy"))
	source := index.Source{Path: "a.go", Language: syntax.Go, Digest: index.DigestBytes([]byte("A"))}
	root := t.TempDir()
	first := mustBuild(t, goDoc("a.go", cmt("alpha", 0, 1)))
	if _, err := index.Persist(root, policy, []index.Source{source}, first); err != nil {
		t.Fatalf("Persist: %v", err)
	}
	before := readPersisted(t, root, index.SnapshotPath)
	indexDir := filepath.Join(root, filepath.FromSlash(index.IndexDir))
	t.Cleanup(func() { _ = os.Chmod(indexDir, 0o755) })
	if err := os.Chmod(indexDir, 0o555); err != nil {
		t.Fatalf("chmod: %v", err)
	}
	second := mustBuild(t, goDoc("a.go", cmt("beta", 0, 1)))
	if _, err := index.Persist(root, policy, []index.Source{source}, second); err == nil {
		t.Fatal("Persist into a read-only directory succeeded")
	}
	if err := os.Chmod(indexDir, 0o755); err != nil {
		t.Fatalf("restore: %v", err)
	}
	if !bytes.Equal(readPersisted(t, root, index.SnapshotPath), before) {
		t.Fatal("failed Persist replaced the snapshot")
	}
	if names := indexNames(t, root); !slices.Equal(names, []string{"comments.jsonl", "snapshot.json"}) {
		t.Fatalf("index dir = %q", names)
	}

	snapPath := filepath.Join(root, filepath.FromSlash(index.SnapshotPath))
	if err := os.Remove(snapPath); err != nil {
		t.Fatalf("remove snapshot: %v", err)
	}
	if err := os.Mkdir(snapPath, 0o755); err != nil {
		t.Fatalf("mkdir snapshot: %v", err)
	}
	if _, err := index.Persist(root, policy, []index.Source{source}, second); err == nil {
		t.Fatal("Persist over a snapshot directory succeeded")
	}
	for _, name := range indexNames(t, root) {
		if strings.HasPrefix(name, ".") {
			t.Fatalf("temporary file remained: %s", name)
		}
	}
	if _, _, err := index.Load(root); !errors.Is(err, index.ErrCorrupt) {
		t.Fatalf("partial publish = %v", err)
	}
}

func TestCurrent(t *testing.T) {
	policy := index.DigestBytes([]byte("policy"))
	other := index.DigestBytes([]byte("other"))
	a := index.Source{Path: "a.go", Language: syntax.Go, Digest: index.DigestBytes([]byte("package a"))}
	b := index.Source{Path: "b.go", Language: syntax.Go, Digest: index.DigestBytes([]byte("package b"))}
	changed := a
	changed.Digest = index.DigestBytes([]byte("package a\n"))
	renamed := a
	renamed.Path = "c.go"
	added := index.Source{Path: "d.go", Language: syntax.Go, Digest: index.DigestBytes([]byte("package d"))}
	lang := a
	lang.Language = syntax.Language("text")
	base := []index.Source{a, b}
	snap := index.Snapshot{PolicyIdentity: policy, Sources: []index.Source{a, b}, CommentCount: 9, CommentsDigest: "ignored"}
	cases := []struct {
		name   string
		policy string
		src    []index.Source
		want   bool
	}{
		{name: "same", policy: policy, src: base, want: true},
		{name: "reordered", policy: policy, src: []index.Source{b, a}, want: true},
		{name: "content", policy: policy, src: []index.Source{changed, b}, want: false},
		{name: "added", policy: policy, src: []index.Source{a, b, added}, want: false},
		{name: "removed", policy: policy, src: []index.Source{a}, want: false},
		{name: "renamed", policy: policy, src: []index.Source{renamed, b}, want: false},
		{name: "language", policy: policy, src: []index.Source{lang, b}, want: false},
		{name: "policy", policy: other, src: base, want: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := index.Current(snap, tc.policy, tc.src); got != tc.want {
				t.Fatalf("Current = %v, want %v", got, tc.want)
			}
		})
	}
	if !index.Current(index.Snapshot{PolicyIdentity: policy}, policy, nil) {
		t.Fatal("empty generation is stale")
	}
}

func readPersisted(t *testing.T, root, logical string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(logical)))
	if err != nil {
		t.Fatalf("read %s: %v", logical, err)
	}
	return data
}

func indexNames(t *testing.T, root string) []string {
	t.Helper()
	entries, err := os.ReadDir(filepath.Join(root, filepath.FromSlash(index.IndexDir)))
	if err != nil {
		t.Fatalf("readdir: %v", err)
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		names = append(names, entry.Name())
	}
	slices.Sort(names)
	return names
}

type testSnapshot struct {
	Schema         int          `json:"schema"`
	PolicyIdentity string       `json:"policy_identity"`
	CommentsDigest string       `json:"comments_digest"`
	CommentCount   int          `json:"comment_count"`
	Sources        []testSource `json:"sources"`
}

type testSource struct {
	Path     string `json:"path"`
	Language string `json:"language"`
	Digest   string `json:"digest"`
}

type testEntry struct {
	ID       string    `json:"id"`
	Path     string    `json:"path"`
	Language string    `json:"language"`
	Range    testRange `json:"range"`
	Text     string    `json:"text"`
}

type testRange struct {
	Start testPosition `json:"start"`
	End   testPosition `json:"end"`
}

type testPosition struct {
	Offset int `json:"offset"`
	Line   int `json:"line"`
	Column int `json:"column"`
}

func matchingSnapshot(policy string, count int, sources []testSource, comments []byte) testSnapshot {
	if sources == nil {
		sources = []testSource{}
	}
	return testSnapshot{
		Schema:         1,
		PolicyIdentity: policy,
		CommentsDigest: index.DigestBytes(comments),
		CommentCount:   count,
		Sources:        sources,
	}
}

func entryLine(t *testing.T, id, path, text string, start, end int) string {
	t.Helper()
	return entryLineAt(t, id, path, text, start, end, 1, 1)
}

func entryLineAt(t *testing.T, id, path, text string, start, end, line, column int) string {
	t.Helper()
	raw, err := json.Marshal(testEntry{
		ID:       id,
		Path:     path,
		Language: "go",
		Text:     text,
		Range: testRange{
			Start: testPosition{Offset: start, Line: line, Column: column},
			End:   testPosition{Offset: end, Line: line, Column: column},
		},
	})
	if err != nil {
		t.Fatalf("marshal entry: %v", err)
	}
	return string(raw)
}

func lines(parts ...string) []byte {
	var buf bytes.Buffer
	for _, part := range parts {
		buf.WriteString(part)
		buf.WriteByte('\n')
	}
	return buf.Bytes()
}

func mustJSON(t *testing.T, snap testSnapshot) []byte {
	t.Helper()
	raw, err := json.Marshal(snap)
	if err != nil {
		t.Fatalf("marshal snapshot: %v", err)
	}
	return append(raw, '\n')
}

func writePair(t *testing.T, root string, snapshot, comments []byte) {
	t.Helper()
	writeRaw(t, root, index.SnapshotPath, snapshot)
	writeRaw(t, root, index.CommentsPath, comments)
}

func writeRaw(t *testing.T, root, logical string, data []byte) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(logical))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write %s: %v", logical, err)
	}
}
