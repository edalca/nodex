package ecmascript

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"reflect"
	"strings"
	"sync"
	"testing"
	"unicode/utf8"

	"github.com/edalca/nodex/internal/syntax/contracts"
	"github.com/edalca/nodex/internal/syntax/types"
	"github.com/odvcencio/gotreesitter"
)

type fixture struct {
	Name         string
	Source       string
	Dialects     []string
	Comments     []string
	Declarations []expectedDeclaration
	Recovery     bool
}
type expectedDeclaration struct {
	Raw, Kind   string
	Names, Docs []string
}

func conformance(t *testing.T) []fixture {
	t.Helper()
	b, err := os.ReadFile("testdata/conformance.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixtures []fixture
	if err := json.Unmarshal(b, &fixtures); err != nil {
		t.Fatal(err)
	}
	return fixtures
}
func TestConformance(t *testing.T) {
	for _, f := range conformance(t) {
		for _, dialect := range f.Dialects {
			t.Run(dialect+"/"+f.Name, func(t *testing.T) {
				l := configured(dialect)
				source := []byte(f.Source)
				doc, err := l.Parse(source)
				if err != nil {
					t.Fatal(err)
				}
				if doc.Comments == nil || doc.Declarations == nil {
					t.Fatal("nil fact collection")
				}
				if len(doc.Comments) != len(f.Comments) {
					t.Fatalf("comments=%+v, want %q", doc.Comments, f.Comments)
				}
				for i, raw := range f.Comments {
					c := doc.Comments[i]
					r := expectedRange(t, source, raw)
					if c.Raw != raw || c.Start != r.Start || c.End != r.End {
						t.Fatalf("comment=%+v want %q %+v", c, raw, r)
					}
					if string(source[c.Start.Offset:c.End.Offset]) != c.Raw {
						t.Fatal("comment raw changed")
					}
					snippet, err := l.Context(source, r, contracts.Limits{Lines: 40, ExtraBytes: 8192})
					if err != nil {
						t.Fatal(err)
					}
					checkSnippet(t, source, snippet)
					if snippet.Start.Offset > c.Start.Offset || snippet.End.Offset < c.End.Offset {
						t.Fatal("small context lost comment")
					}
				}
				if len(doc.Declarations) != len(f.Declarations) {
					t.Fatalf("declarations=%+v want %+v", doc.Declarations, f.Declarations)
				}
				for i, d := range doc.Declarations {
					if i > 0 {
						p := doc.Declarations[i-1]
						if p.Start.Offset > d.Start.Offset || (p.Start.Offset == d.Start.Offset && (p.End.Offset > d.End.Offset || (p.End.Offset == d.End.Offset && p.Kind > d.Kind))) {
							t.Fatal("noncanonical declaration order")
						}
					}
					for _, r := range d.Docs {
						matches := 0
						for _, c := range doc.Comments {
							if c.Start == r.Start && c.End == r.End {
								matches++
							}
						}
						if matches != 1 {
							t.Fatal("documentation range does not resolve exactly")
						}
					}
				}
				for _, want := range f.Declarations {
					r := expectedRange(t, source, want.Raw)
					// Bare enum members must be located independently of their doc prose.
					if want.Kind == "enum-member" && want.Raw == "A" {
						start := strings.Index(f.Source, "\nA,") + 1
						r = types.Range{Start: independentPosition(source, start), End: independentPosition(source, start+1)}
					}
					var found *types.Declaration
					for i := range doc.Declarations {
						if doc.Declarations[i].Start == r.Start && doc.Declarations[i].End == r.End {
							found = &doc.Declarations[i]
							break
						}
					}
					if found == nil {
						t.Fatalf("missing exact declaration %q %+v; got %+v", want.Raw, r, doc.Declarations)
					}
					if found.Kind != want.Kind || !reflect.DeepEqual(found.Names, want.Names) {
						t.Fatalf("declaration=%+v want %+v", found, want)
					}
					docs := []types.Range{}
					for _, raw := range want.Docs {
						docs = append(docs, expectedRange(t, source, raw))
					}
					if !reflect.DeepEqual(found.Docs, docs) {
						t.Fatalf("Docs=%+v want %+v", found.Docs, docs)
					}
					snippet, err := l.DeclarationContext(source, r, contracts.Limits{Lines: 40, ExtraBytes: 8192})
					if err != nil {
						t.Fatal(err)
					}
					if snippet.Text != want.Raw {
						t.Fatalf("declaration context=%q want %q", snippet.Text, want.Raw)
					}
					checkSnippet(t, source, snippet)
				}
				repeated, err := l.Parse(source)
				if err != nil {
					t.Fatal(err)
				}
				if !reflect.DeepEqual(doc, repeated) {
					t.Fatal("parse not deterministic")
				}
			})
		}
	}
}
func configured(id string) contracts.Language {
	return New(map[string]Dialect{"JS": JavaScript, "JSX": JSX, "TS": TypeScript, "TSX": TSX}[id])
}

// Coordinates are computed independently of the adapter's line table.
func independentPosition(source []byte, offset int) types.Position {
	line, column := 1, 1
	for _, b := range source[:offset] {
		if b == '\n' {
			line++
			column = 1
		} else {
			column++
		}
	}
	return types.Position{Offset: offset, Line: line, Column: column}
}
func expectedRange(t *testing.T, source []byte, raw string) types.Range {
	t.Helper()
	start := bytes.Index(source, []byte(raw))
	if start < 0 {
		t.Fatalf("fixture fragment absent: %q", raw)
	}
	return types.Range{Start: independentPosition(source, start), End: independentPosition(source, start+len(raw))}
}
func checkSnippet(t *testing.T, source []byte, s types.Snippet) {
	t.Helper()
	if s.Start != independentPosition(source, s.Start.Offset) || s.End != independentPosition(source, s.End.Offset) || s.Text != string(source[s.Start.Offset:s.End.Offset]) {
		t.Fatalf("context changed physical source: %+v", s)
	}
}

func TestRecognitionAndPresets(t *testing.T) {
	for i, d := range []Dialect{JavaScript, JSX, TypeScript, TSX} {
		l := New(d)
		ext := []string{".js", ".jsx", ".ts", ".tsx"}[i]
		if !l.Recognize("a"+ext) || l.ID() != []string{"javascript", "jsx", "typescript", "tsx"}[i] {
			t.Fatal("capability configuration")
		}
		for _, path := range []string{"a" + strings.ToUpper(ext), "a.mjs", "a.cjs", "a.mts", "a.cts", "a.py", "a.php", "a.go"} {
			if l.Recognize(path) {
				t.Fatalf("unexpected recognition %q", path)
			}
		}
		if len(l.Presets().Concrete) != 0 || len(l.Presets().Aggregates) != 0 || l.PathExcluded("node_modules/a"+ext, []string{"go:all"}) || l.SourceExcluded("dist/a"+ext, []byte("generated"), []string{"go:generated"}) {
			t.Fatal("invented exclusion policy")
		}
		doc, err := l.Parse(nil)
		if err != nil || doc == nil || len(doc.Comments) != 0 || len(doc.Declarations) != 0 {
			t.Fatalf("empty parse=%+v %v", doc, err)
		}
	}
}

func TestRecoveryGuardPreventsSwallowedOwner(t *testing.T) {
	l := language{dialect: TypeScript}
	source := []byte("/** before-broken */\nconst broken = ;\nfunction later() {}\n")
	grammar, err := l.grammar()
	if err != nil {
		t.Fatal(err)
	}
	tree, err := gotreesitter.NewParser(grammar).Parse(source)
	if err != nil {
		t.Fatal(err)
	}
	defer tree.Release()
	a := adapter{source: source, grammar: grammar, lines: sourceLines(source), bounds: map[*gotreesitter.Node]span{}}
	a.observe(tree.RootNode())
	var ungated []types.Declaration
	a.slots(tree.RootNode(), &ungated)
	corrupt := false
	for _, d := range ungated {
		if len(d.Docs) > 0 && d.Kind == "const" && d.End.Offset > strings.Index(string(source), "function later") {
			corrupt = true
		}
	}
	if !corrupt || !a.unsafe {
		t.Fatalf("controlled corruption did not exercise safety guard: %+v", ungated)
	}
	doc, err := l.Parse(source)
	if err != nil || len(doc.Comments) != 1 || len(doc.Declarations) != 0 {
		t.Fatalf("unsafe declarations published: %+v %v", doc, err)
	}
}

func TestContextLimitsAndExactIdentity(t *testing.T) {
	for _, dialect := range []string{"JS", "JSX", "TS", "TSX"} {
		for _, family := range []string{"function f() {", "class C {", "interface I {", "type T = {", "enum E {", "namespace N {"} {
			if (family == "interface I {" || family == "type T = {" || family == "enum E {" || family == "namespace N {") && (dialect == "JS" || dialect == "JSX") {
				continue
			}
			t.Run(dialect+"/"+family, func(t *testing.T) {
				body := ""
				switch family {
				case "function f() {", "namespace N {":
					body = strings.Repeat("const 名称 = 1;\r\n", 60)
				case "class C {":
					body = strings.Repeat("m() {}\r\n", 60)
				case "enum E {":
					body = strings.Repeat("A,\r\n", 60)
				default:
					body = strings.Repeat("x: string;\r\n", 60)
				}
				source := []byte("/** A */\r\n" + family + "\r\n" + body + "}")
				l := configured(dialect)
				doc, err := l.Parse(source)
				if err != nil {
					t.Fatal(err)
				}
				if len(doc.Declarations) == 0 {
					t.Fatal("missing long declaration")
				}
				d := doc.Declarations[0]
				r := types.Range{Start: d.Start, End: d.End}
				for _, limits := range []contracts.Limits{{Lines: 40, ExtraBytes: 8192}, {Lines: 3, ExtraBytes: 32}, {Lines: 0, ExtraBytes: -1}} {
					s, err := l.DeclarationContext(source, r, limits)
					if err != nil {
						t.Fatal(err)
					}
					checkSnippet(t, source, s)
					firstEnd := min(lineEnd(source, d.Start.Offset), d.End.Offset)
					if s.Start != d.Start || s.End.Offset < firstEnd || lastLine(sourceLines(source), span{s.Start.Offset, s.End.Offset})-s.Start.Line+1 > max(1, limits.Lines) || s.End.Offset-firstEnd > max(0, limits.ExtraBytes) || !utf8.ValidString(s.Text) {
						t.Fatalf("limits violated: %+v %+v", limits, s)
					}
				}
				wrong := r
				wrong.Start.Column++
				if _, err := l.DeclarationContext(source, wrong, contracts.Limits{}); !errors.Is(err, contracts.ErrMalformedDeclaration) {
					t.Fatalf("coordinate mismatch=%v", err)
				}
				r.End = independentPosition(source, r.End.Offset-1)
				if _, err := l.DeclarationContext(source, r, contracts.Limits{}); !errors.Is(err, contracts.ErrDeclarationNotFound) {
					t.Fatalf("partial range accepted: %v", err)
				}
			})
		}
	}
	source := []byte("function f() {\n" + strings.Repeat("work();\n", 30) + "/** A */\n" + strings.Repeat("work();\n", 30) + "}")
	l := New(JavaScript)
	r := expectedRange(t, source, "/** A */")
	s, err := l.Context(source, r, contracts.Limits{Lines: 5, ExtraBytes: 20})
	if err != nil {
		t.Fatal(err)
	}
	checkSnippet(t, source, s)
	if s.Start.Offset > r.Start.Offset || s.End.Offset < r.End.Offset || s.End.Offset-s.Start.Offset-(r.End.Offset-r.Start.Offset) > 20 || lastLine(sourceLines(source), span{s.Start.Offset, s.End.Offset})-s.Start.Line+1 > 5 {
		t.Fatal("comment limits")
	}
	if _, err := l.Context(source, types.Range{}, contracts.Limits{}); !errors.Is(err, contracts.ErrMalformedRange) {
		t.Fatal(err)
	}
	missing := expectedRange(t, source, "work();")
	if _, err := l.Context(source, missing, contracts.Limits{}); !errors.Is(err, contracts.ErrCommentNotFound) {
		t.Fatal(err)
	}
}

func TestConcurrentCapabilities(t *testing.T) {
	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func(d Dialect) {
			defer wg.Done()
			l := New(d)
			for j := 0; j < 3; j++ {
				doc, err := l.Parse([]byte("/** A */\nfunction f() {}"))
				if err != nil || len(doc.Declarations) != 1 || len(doc.Declarations[0].Docs) != 1 {
					t.Errorf("concurrent parse=%+v %v", doc, err)
				}
			}
		}(Dialect(i % 4))
	}
	wg.Wait()
}

func TestEmbeddedGrammarIgnoresFilesystemOverride(t *testing.T) {
	t.Setenv("GOTREESITTER_GRAMMARGEN_BLOB_DIR", t.TempDir())
	for _, d := range []Dialect{JavaScript, TypeScript, TSX} {
		doc, err := New(d).Parse([]byte("/** A */\nfunction f() {}"))
		if err != nil || len(doc.Declarations) != 1 {
			t.Fatalf("embedded grammar=%+v %v", doc, err)
		}
	}
}

func TestCommentNormalizationAndStructuralContext(t *testing.T) {
	source := []byte("// @ts-check\r\n/** Café 😀\r\n * @param 名称\r\n */\r\nfunction résumé(名称) { return 名称?.x ?? 1; }\r\n")
	l := New(JavaScript)
	doc, err := l.Parse(source)
	if err != nil {
		t.Fatal(err)
	}
	if len(doc.Comments) != 2 || doc.Comments[0].Text != "@ts-check" || doc.Comments[1].Text != "* Café 😀\n * @param 名称\n " {
		t.Fatalf("normalized comments=%+v", doc.Comments)
	}
	if len(doc.Declarations) != 1 || len(doc.Declarations[0].Docs) != 1 {
		t.Fatalf("ordinary source operators suppressed declaration: %+v", doc.Declarations)
	}
	c := doc.Comments[1]
	snippet, err := l.Context(source, types.Range{Start: c.Start, End: c.End}, contracts.Limits{Lines: 40, ExtraBytes: 8192})
	if err != nil {
		t.Fatal(err)
	}
	want := string(source[c.Start.Offset:doc.Declarations[0].End.Offset])
	if snippet.Text != want {
		t.Fatalf("structural context=%q want %q", snippet.Text, want)
	}
	// Parser-clean syntax with duplicate bindings is still structural source.
	doc, err = l.Parse([]byte("const x = 1; const x = 2;"))
	if err != nil || len(doc.Declarations) != 2 {
		t.Fatalf("semantic gate introduced: %+v %v", doc, err)
	}
}

func TestSeparateIdenticalBlocksAndInitialTrivia(t *testing.T) {
	for _, dialect := range []Dialect{JavaScript, JSX, TypeScript, TSX} {
		for _, prefix := range []string{"", "   ", "\ufeff ", "\r\n\t"} {
			source := []byte(prefix + "/** same */\n/** same */\nfunction f() {}")
			doc, err := New(dialect).Parse(source)
			if err != nil || len(doc.Comments) != 2 || len(doc.Declarations) != 1 || len(doc.Declarations[0].Docs) != 2 {
				t.Fatalf("dialect=%v prefix=%q document=%+v error=%v", dialect, prefix, doc, err)
			}
			if doc.Comments[0].Raw != doc.Comments[1].Raw || doc.Comments[0].Start == doc.Comments[1].Start {
				t.Fatal("identical physical comments lost separate identities")
			}
			for i, r := range doc.Declarations[0].Docs {
				c := doc.Comments[i]
				if r.Start != c.Start || r.End != c.End || c.Start != independentPosition(source, c.Start.Offset) || c.End != independentPosition(source, c.End.Offset) {
					t.Fatal("separate ordered ranges or byte positions changed")
				}
			}
		}
	}
}

func TestLongCommentContextRetainsBeginning(t *testing.T) {
	source := []byte("/** first 😀\r\n" + strings.Repeat(" * 名称\r\n", 60) + " */\r\nfunction f() {}")
	for _, dialect := range []Dialect{JavaScript, JSX, TypeScript, TSX} {
		l := New(dialect)
		doc, err := l.Parse(source)
		if err != nil || len(doc.Comments) != 1 {
			t.Fatalf("long comment=%+v %v", doc, err)
		}
		c := doc.Comments[0]
		for _, limits := range []contracts.Limits{{Lines: 40, ExtraBytes: 8192}, {Lines: 3, ExtraBytes: 0}, {Lines: 0, ExtraBytes: -1}} {
			s, err := l.Context(source, types.Range{Start: c.Start, End: c.End}, limits)
			if err != nil {
				t.Fatal(err)
			}
			checkSnippet(t, source, s)
			if s.Start.Offset != 0 || s.Text != string(source[:s.End.Offset]) || bytes.Count([]byte(s.Text), []byte{'\n'}) != max(limits.Lines, 1) || !utf8.ValidString(s.Text) || !strings.HasSuffix(s.Text, "\r\n") {
				t.Fatalf("long comment bounds=%+v limits=%+v", s, limits)
			}
		}
	}
}

func TestStaticConstructorClassification(t *testing.T) {
	for _, dialect := range []string{"JS", "JSX", "TS", "TSX"} {
		for _, member := range []struct {
			raw, kind string
			names     []string
		}{
			{"constructor() {}", "constructor", []string{}},
			{"static constructor() {}", "method", []string{"constructor"}},
			{"static ordinary() {}", "method", []string{"ordinary"}},
			{"static() {}", "method", []string{"static"}},
			{"\"quoted-name\"() {}", "method", []string{`"quoted-name"`}},
			{"static 42() {}", "method", []string{"42"}},
			{"static #private() {}", "method", []string{"#private"}},
			{"static [constructor]() {}", "method", []string{}},
			{"static get constructor() {}", "accessor", []string{"constructor"}},
			{"static set constructor(value) {}", "accessor", []string{"constructor"}},
			{"@dec static constructor() {}", "method", []string{"constructor"}},
		} {
			t.Run(dialect+"/"+member.raw, func(t *testing.T) {
				source := "class C {\n/** direct */\n" + member.raw + "\n}"
				checkRemediationFacts(t, dialect, fixture{
					Source: source, Comments: []string{"/** direct */"},
					Declarations: []expectedDeclaration{
						{source, "class", []string{"C"}, []string{}},
						{member.raw, member.kind, member.names, []string{"/** direct */"}},
					},
				})
			})
		}
	}
}

func TestMemberTerminatorTrivia(t *testing.T) {
	for _, dialect := range []string{"JS", "JSX", "TS", "TSX"} {
		type memberFamily struct{ name, prefix, member, next, suffix, kind string }
		families := []memberFamily{
			{"class-field", "class C {\n", "x = 1", "y = 2;", "\n}", "property"},
		}
		if dialect == "TS" || dialect == "TSX" {
			families = append(families,
				memberFamily{"interface-property", "interface I {\n", "x: string", "y: number;", "\n}", "property"},
				memberFamily{"interface-method", "interface I {\n", "m(): void", "n(): void;", "\n}", "method"},
				memberFamily{"abstract-method", "abstract class C {\n", "abstract m(): void", "abstract n(): void;", "\n}", "method"},
				memberFamily{"class-signature", "declare class C {\n", "m(): void", "n(): void;", "\n}", "method"},
				memberFamily{"object-type-property", "type T = {\n", "x: string", "y: number;", "\n};", "property"},
				memberFamily{"object-type-method", "type T = {\n", "m(): void", "n(): void;", "\n};", "method"},
			)
		}
		for _, family := range families {
			for _, tail := range []struct {
				name, text string
				comments   []string
			}{
				{"touching", ";", nil},
				{"space", " \t ;", nil},
				{"newline", "\n;", nil},
				{"ordinary-comment", " /* note */ ;", []string{"/* note */"}},
				{"several-comments", " /* first */\n// second\n /* third */ ;", []string{"/* first */", "// second", "/* third */"}},
				{"interior-jsdoc", " /** interior */ ;", []string{"/** interior */"}},
				{"absent", "", nil},
				{"absent-with-comment", " /* trailing */", []string{"/* trailing */"}},
			} {
				for _, next := range []string{"", "\n" + family.next} {
					t.Run(dialect+"/"+family.name+"/"+tail.name+"/next="+next, func(t *testing.T) {
						source := []byte(family.prefix + family.member + tail.text + next + family.suffix)
						l := configured(dialect)
						doc, err := l.Parse(source)
						wantCount := 2
						if next != "" {
							wantCount++
						}
						if err != nil || len(doc.Declarations) != wantCount {
							t.Fatalf("member facts=%+v error=%v", doc, err)
						}
						want := family.member
						if strings.HasSuffix(tail.text, ";") {
							want += tail.text
						}
						d := doc.Declarations[1]
						r := expectedRange(t, source, want)
						if d.Start != r.Start || d.End != r.End || d.Kind != family.kind || len(d.Docs) != 0 {
							t.Fatalf("member=%+v want %q %+v", d, want, r)
						}
						if len(doc.Comments) != len(tail.comments) {
							t.Fatalf("physical comments=%+v want %q", doc.Comments, tail.comments)
						}
						for i, raw := range tail.comments {
							c, cr := doc.Comments[i], expectedRange(t, source, raw)
							if c.Raw != raw || c.Start != cr.Start || c.End != cr.End {
								t.Fatalf("physical comment changed: %+v", c)
							}
						}
						if next != "" && len(doc.Declarations[2].Docs) != 0 {
							t.Fatal("interior/trailing comment reassigned to next slot")
						}
						s, err := l.DeclarationContext(source, r, contracts.Limits{Lines: 40, ExtraBytes: 8192})
						if err != nil || s.Text != want || s.Start != r.Start || s.End != r.End {
							t.Fatalf("declaration context=%+v error=%v want %q", s, err, want)
						}
						checkSnippet(t, source, s)
					})
				}
			}
		}
		// Object pairs and enum members retain their separate comma policy.
		checkRemediationFacts(t, dialect, fixture{
			Source: "const o = { x: 1 /* pair */ , y: 2 };", Comments: []string{"/* pair */"},
			Declarations: []expectedDeclaration{
				{"const o = { x: 1 /* pair */ , y: 2 };", "const", []string{"o"}, []string{}},
				{"x: 1", "property", []string{"x"}, []string{}},
				{"y: 2", "property", []string{"y"}, []string{}},
			},
		})
		if dialect == "TS" || dialect == "TSX" {
			checkRemediationFacts(t, dialect, fixture{
				Source: "enum E { A = 1 /* enum */ , B = 2, }", Comments: []string{"/* enum */"},
				Declarations: []expectedDeclaration{
					{"enum E { A = 1 /* enum */ , B = 2, }", "enum", []string{"E"}, []string{}},
					{"A = 1", "enum-member", []string{"A"}, []string{}},
					{"B = 2", "enum-member", []string{"B"}, []string{}},
				},
			})
		}
	}
}

func TestJSDocECMAScriptLineTerminators(t *testing.T) {
	for _, dialect := range []string{"JS", "JSX", "TS", "TSX"} {
		for _, sep := range []struct{ name, text string }{
			{"CR", "\r"}, {"LF", "\n"}, {"CRLF", "\r\n"}, {"LS", "\u2028"}, {"PS", "\u2029"},
		} {
			for _, placement := range []struct {
				name, before, after, comment string
				attached                     bool
			}{
				{"before", sep.text, " ", "/** doc */", true},
				{"after-only", " ", sep.text, "/** doc */", false},
				{"both", sep.text, sep.text, "/** doc */", true},
				{"inside-trailing", " ", sep.text, "/** doc" + sep.text + "inside */", false},
			} {
				t.Run(dialect+"/"+sep.name+"/"+placement.name, func(t *testing.T) {
					source := "work();" + placement.before + placement.comment + placement.after + "function f() {}"
					docs := []string{}
					if placement.attached {
						docs = append(docs, placement.comment)
					}
					checkRemediationFacts(t, dialect, fixture{
						Source: source, Comments: []string{placement.comment},
						Declarations: []expectedDeclaration{{"function f() {}", "function", []string{"f"}, docs}},
					})
					if (sep.name == "LS" || sep.name == "PS") && placement.name == "both" {
						doc, err := configured(dialect).Parse([]byte(source))
						if err != nil {
							t.Fatal(err)
						}
						c, d := doc.Comments[0], doc.Declarations[0]
						if c.Start != (types.Position{Offset: 10, Line: 1, Column: 11}) || c.End != (types.Position{Offset: 20, Line: 1, Column: 21}) || d.Start != (types.Position{Offset: 23, Line: 1, Column: 24}) || d.End != (types.Position{Offset: 38, Line: 1, Column: 39}) {
							t.Fatalf("LS/PS relocated physical coordinates: %+v %+v", c, d)
						}
						s, err := configured(dialect).Context([]byte(source), types.Range{Start: c.Start, End: c.End}, contracts.Limits{Lines: 40, ExtraBytes: 8192})
						if err != nil || s.Text != placement.comment+sep.text+"function f() {}" {
							t.Fatalf("LS/PS context changed: %+v %v", s, err)
						}
						checkSnippet(t, []byte(source), s)
					}
				})
			}
			for _, boundary := range []struct {
				prefix   string
				comments []string
				docs     []string
			}{
				{"/** initial */" + sep.text, []string{"/** initial */"}, []string{"/** initial */"}},
				{"work(); /** trailing */" + sep.text + "/** leading */ ", []string{"/** trailing */", "/** leading */"}, []string{"/** leading */"}},
				{"work(); /* ordinary" + sep.text + "inside */ /** same-line */" + sep.text, []string{"/* ordinary" + sep.text + "inside */", "/** same-line */"}, []string{}},
				{"work(\"é😀\");" + sep.text + "/** first */ /* ordinary */ /** second */" + sep.text, []string{"/** first */", "/* ordinary */", "/** second */"}, []string{"/** first */", "/** second */"}},
			} {
				t.Run(dialect+"/"+sep.name+"/"+boundary.prefix, func(t *testing.T) {
					checkRemediationFacts(t, dialect, fixture{
						Source: boundary.prefix + "function f() {}", Comments: boundary.comments,
						Declarations: []expectedDeclaration{{"function f() {}", "function", []string{"f"}, boundary.docs}},
					})
				})
			}
		}
	}
}

// Expectations are authored source fragments; positions are independently
// computed in LF-delimited physical lines and UTF-8 byte columns.
func checkRemediationFacts(t *testing.T, dialect string, f fixture) {
	t.Helper()
	source := []byte(f.Source)
	l := configured(dialect)
	doc, err := l.Parse(source)
	if err != nil {
		t.Fatal(err)
	}
	if len(doc.Comments) != len(f.Comments) || len(doc.Declarations) != len(f.Declarations) {
		t.Fatalf("facts=%+v want comments=%q declarations=%+v", doc, f.Comments, f.Declarations)
	}
	for i, raw := range f.Comments {
		c, r := doc.Comments[i], expectedRange(t, source, raw)
		if c.Raw != raw || c.Start != r.Start || c.End != r.End {
			t.Fatalf("comment=%+v want %q %+v", c, raw, r)
		}
	}
	for i, want := range f.Declarations {
		d, r := doc.Declarations[i], expectedRange(t, source, want.Raw)
		docs := []types.Range{}
		for _, raw := range want.Docs {
			docs = append(docs, expectedRange(t, source, raw))
		}
		if d.Kind != want.Kind || !reflect.DeepEqual(d.Names, want.Names) || d.Start != r.Start || d.End != r.End || !reflect.DeepEqual(d.Docs, docs) {
			t.Fatalf("declaration=%+v want %+v range=%+v docs=%+v", d, want, r, docs)
		}
		s, err := l.DeclarationContext(source, r, contracts.Limits{Lines: 40, ExtraBytes: 8192})
		if err != nil || s.Text != want.Raw {
			t.Fatalf("declaration context=%+v error=%v want %q", s, err, want.Raw)
		}
		checkSnippet(t, source, s)
	}
}
