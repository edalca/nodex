package ignore

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"testing"
	"unicode/utf8"
)

func mustPolicy(t *testing.T, patterns ...string) Policy {
	t.Helper()
	if patterns == nil {
		patterns = []string{}
	}
	document, err := json.Marshal(struct {
		Schema  int      `json:"schema"`
		Presets []string `json:"presets"`
		Exclude []string `json:"exclude"`
	}{Schema: SchemaVersion, Presets: []string{}, Exclude: patterns})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	policy, err := Parse(document)
	if err != nil {
		t.Fatalf("Parse(%s): %v", document, err)
	}
	return policy
}

func assertExcluded(t *testing.T, policy Policy, path string, isDir, want bool) {
	t.Helper()
	if got := policy.Excluded(path, isDir); got != want {
		t.Fatalf("Excluded(%q, dir=%v) = %v, want %v", path, isDir, got, want)
	}
}

func requireRuleError(t *testing.T, document string, index int) *RuleError {
	t.Helper()
	policy, err := Parse([]byte(document))
	if err == nil {
		t.Fatalf("Parse(%s) succeeded", document)
	}
	if policy.RuleCount() != 0 || policy.Excluded("kept.log", false) {
		t.Fatalf("Parse error returned a usable policy: %+v", policy)
	}
	var ruleErr *RuleError
	if !errors.As(err, &ruleErr) {
		t.Fatalf("Parse(%s) error = %v, want RuleError", document, err)
	}
	if ruleErr.Index != index {
		t.Fatalf("Parse(%s) index = %d, want %d (%v)", document, ruleErr.Index, index, err)
	}
	if ruleErr.Err == nil || ruleErr.Err.Error() == "" {
		t.Fatalf("Parse(%s) has no cause", document)
	}
	if !strings.Contains(err.Error(), "exclude[") {
		t.Fatalf("error %q does not identify the exclude entry", err)
	}
	return ruleErr
}

func TestParseAcceptsSchemaOneAndEmptyExclude(t *testing.T) {
	policy, err := Parse([]byte("{\n  \"schema\": 1,\n  \"presets\": [],\n  \"exclude\": []\n}\n"))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if policy.RuleCount() != 0 {
		t.Fatalf("RuleCount = %d, want 0", policy.RuleCount())
	}
	assertExcluded(t, policy, "a.log", false, false)
	assertExcluded(t, policy, "vendor/lib.go", false, false)

	var zero Policy
	if policy.Identity() != zero.Identity() {
		t.Fatalf("empty document identity = %s, zero identity = %s", policy.Identity(), zero.Identity())
	}
	if DocumentPath != ".nodex/ignore.json" || SchemaVersion != 1 {
		t.Fatalf("document identity = %s schema %d", DocumentPath, SchemaVersion)
	}
}

func TestParseRejectsDocumentShape(t *testing.T) {
	cases := []struct {
		name     string
		document string
		target   error
	}{
		{name: "missing schema", document: `{"exclude":[]}`, target: ErrMissingSchema},
		{name: "missing presets", document: `{"schema":1,"exclude":[]}`, target: ErrMissingPresets},
		{name: "missing exclude", document: `{"schema":1,"presets":[]}`, target: ErrMissingExclude},
		{name: "null presets", document: `{"schema":1,"presets":null,"exclude":[]}`, target: ErrNullPresets},
		{name: "presets object", document: `{"schema":1,"presets":{},"exclude":[]}`, target: ErrPresetsType},
		{name: "presets string", document: `{"schema":1,"presets":"go:tests","exclude":[]}`, target: ErrPresetsType},
		{name: "null exclude", document: `{"schema":1,"presets":[],"exclude":null}`, target: ErrNullExclude},
		{name: "exclude object", document: `{"schema":1,"presets":[],"exclude":{}}`, target: ErrExcludeType},
		{name: "exclude string", document: `{"schema":1,"presets":[],"exclude":"vendor/"}`, target: ErrExcludeType},
		{name: "not an object", document: `[]`, target: ErrNotObject},
		{name: "json null", document: `null`, target: ErrNotObject},
		{name: "empty", document: ``, target: nil},
		{name: "whitespace", document: " \n\t", target: nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			policy, err := Parse([]byte(tc.document))
			if err == nil {
				t.Fatal("Parse succeeded")
			}
			if policy.RuleCount() != 0 {
				t.Fatalf("partial policy with %d rules", policy.RuleCount())
			}
			if tc.target != nil && !errors.Is(err, tc.target) {
				t.Fatalf("error = %v, want %v", err, tc.target)
			}
		})
	}
}

func TestParseRejectsUnsupportedSchema(t *testing.T) {
	for _, schema := range []string{`2`, `0`, `-1`, `1.0`, `1e0`, `"1"`, `true`, `null`, `[]`, `{}`} {
		document := `{"schema":` + schema + `,"exclude":[]}`
		policy, err := Parse([]byte(document))
		if err == nil {
			t.Fatalf("schema %s was accepted", schema)
		}
		if !errors.Is(err, ErrUnsupportedSchema) {
			t.Fatalf("schema %s error = %v", schema, err)
		}
		if policy.RuleCount() != 0 {
			t.Fatalf("schema %s returned %d rules", schema, policy.RuleCount())
		}
	}
}

func TestParseRejectsUnknownFieldsAndTrailingData(t *testing.T) {
	policy, err := Parse([]byte(`{"zeta":1,"alpha":2}`))
	if err == nil {
		t.Fatal("unknown fields were accepted")
	}
	var unknown *UnknownFieldError
	if !errors.As(err, &unknown) || unknown.Name != "alpha" {
		t.Fatalf("error = %v", err)
	}
	if policy.RuleCount() != 0 {
		t.Fatal("unknown fields produced a policy")
	}

	_, err = Parse([]byte(`{"schema":1,"presets":[],"exclude":[],"rules":[]}`))
	if !errors.As(err, &unknown) || unknown.Name != "rules" {
		t.Fatalf("error = %v", err)
	}

	for _, document := range []string{
		`{"schema":1,"presets":[],"exclude":[]}{"schema":1,"presets":[],"exclude":[]}`,
		`{"schema":1,"presets":[],"exclude":[]} true`,
		`{"schema":1,"presets":[],"exclude":[]} junk`,
	} {
		_, err = Parse([]byte(document))
		if !errors.Is(err, ErrTrailingData) {
			t.Fatalf("Parse(%s) error = %v, want trailing data", document, err)
		}
	}

	if _, err = Parse([]byte("{\"schema\":1,\"presets\":[],\"exclude\":[]}  \n")); err != nil {
		t.Fatalf("trailing whitespace: %v", err)
	}
	if _, err = Parse([]byte(`{`)); err == nil {
		t.Fatal("truncated JSON was accepted")
	}
}

func TestParseRejectsInvalidUTF8Document(t *testing.T) {
	_, err := Parse([]byte{0xff, 0xfe})
	if !errors.Is(err, ErrInvalidUTF8) {
		t.Fatalf("error = %v", err)
	}
}

func TestInvalidEntryRejectsTheWholeDocument(t *testing.T) {
	ruleErr := requireRuleError(t, `{"schema":1,"presets":[],"exclude":["*.log","a//b","vendor/"]}`, 1)
	if ruleErr.Pattern != "a//b" {
		t.Fatalf("pattern = %q", ruleErr.Pattern)
	}
	if !strings.Contains(ruleErr.Err.Error(), "empty element") {
		t.Fatalf("cause = %v", ruleErr.Err)
	}

	ruleErr = requireRuleError(t, `{"schema":1,"presets":[],"exclude":["*.log",1,"vendor/"]}`, 1)
	if ruleErr.Pattern != "" || !strings.Contains(ruleErr.Err.Error(), "not a string") {
		t.Fatalf("error = %v", ruleErr)
	}
	ruleErr = requireRuleError(t, `{"schema":1,"presets":[],"exclude":[null]}`, 0)
	if !strings.Contains(ruleErr.Err.Error(), "not a string") {
		t.Fatalf("cause = %v", ruleErr.Err)
	}
	ruleErr = requireRuleError(t, `{"schema":1,"presets":[],"exclude":[""]}`, 0)
	if !strings.Contains(ruleErr.Err.Error(), "empty") {
		t.Fatalf("cause = %v", ruleErr.Err)
	}
	ruleErr = requireRuleError(t, `{"schema":1,"presets":[],"exclude":["vendor/","!"]}`, 1)
	if !strings.Contains(ruleErr.Err.Error(), "empty") {
		t.Fatalf("cause = %v", ruleErr.Err)
	}
}

func TestMalformedPatterns(t *testing.T) {
	patterns := []string{
		"!",
		"/",
		"a//b",
		"a/./b",
		"a/../b",
		".",
		"..",
		`bad\`,
		"[abc",
		"a[z-a]",
		"a[]",
		"a[!]",
		"a[bc",
		`a[b\`,
		"a[/]b",
		"//",
		"x[\xff]",
	}
	for _, pattern := range patterns {
		_, err := compileRule(pattern)
		if err == nil {
			t.Errorf("compileRule(%q) succeeded", pattern)
			continue
		}
		if pattern == "x[\xff]" && !strings.Contains(err.Error(), "UTF-8") {
			t.Errorf("compileRule(%q) cause = %v", pattern, err)
		}
		if pattern == "a[/]b" && !strings.Contains(err.Error(), "'/'") {
			t.Errorf("compileRule(%q) cause = %v", pattern, err)
		}
		if pattern == "a[z-a]" && !strings.Contains(err.Error(), "inverted") {
			t.Errorf("compileRule(%q) cause = %v", pattern, err)
		}
		document, err := json.Marshal(struct {
			Schema  int      `json:"schema"`
			Presets []string `json:"presets"`
			Exclude []string `json:"exclude"`
		}{Schema: 1, Presets: []string{}, Exclude: []string{"*.log", pattern}})
		if err != nil {
			t.Fatalf("marshal %q: %v", pattern, err)
		}
		if !utf8.ValidString(pattern) {
			continue
		}
		policy, err := Parse(document)
		if err == nil {
			t.Errorf("Parse accepted %q", pattern)
			continue
		}
		var ruleErr *RuleError
		if !errors.As(err, &ruleErr) || ruleErr.Index != 1 || policy.RuleCount() != 0 {
			t.Errorf("Parse(%q) = rules %d, err %v", pattern, policy.RuleCount(), err)
		}
	}
}

func TestStarQuestionAndDoubleStar(t *testing.T) {
	star := mustPolicy(t, "a*b")
	assertExcluded(t, star, "ab", false, true)
	assertExcluded(t, star, "axxb", false, true)
	assertExcluded(t, star, "a*b", false, true)
	assertExcluded(t, star, "a/b", false, false)

	collapsed := mustPolicy(t, "a**b")
	assertExcluded(t, collapsed, "axxb", false, true)
	assertExcluded(t, collapsed, "a/x/b", false, false)
	if star.Identity() != collapsed.Identity() || star.Identity() != mustPolicy(t, "a***b").Identity() {
		t.Fatal("consecutive stars inside one element did not collapse")
	}

	question := mustPolicy(t, "file?.txt")
	assertExcluded(t, question, "file1.txt", false, true)
	assertExcluded(t, question, "fileé.txt", false, true)
	assertExcluded(t, question, "file.txt", false, false)
	assertExcluded(t, question, "fileab.txt", false, false)
	assertExcluded(t, question, "fileéé.txt", false, false)

	recursive := mustPolicy(t, "internal/**/*.go")
	assertExcluded(t, recursive, "internal/a.go", false, true)
	assertExcluded(t, recursive, "internal/b/c.go", false, true)
	assertExcluded(t, recursive, "internal/b/c/d.go", false, true)
	assertExcluded(t, recursive, "internal/a.txt", false, false)
	assertExcluded(t, recursive, "internal", true, false)
	assertExcluded(t, recursive, "pkg/internal/a.go", false, false)

	inside := mustPolicy(t, "foo/**")
	assertExcluded(t, inside, "foo", true, false)
	assertExcluded(t, inside, "foo/a", false, true)
	assertExcluded(t, inside, "foo/a/b", false, true)
	assertExcluded(t, inside, "bar/foo/a", false, false)
	if inside.Identity() != mustPolicy(t, "foo/**/*").Identity() {
		t.Fatal("foo/** and foo/**/* compiled differently")
	}

	shallow := mustPolicy(t, "foo/*")
	if shallow.Match("foo/a/b", false, false) {
		t.Fatal("foo/* matched a nested path directly")
	}
	if !inside.Match("foo/a/b", false, false) {
		t.Fatal("foo/** did not match a nested path")
	}
	assertExcluded(t, shallow, "foo/a/b", false, true)
	if inside.Identity() == shallow.Identity() {
		t.Fatal("foo/** and foo/* share an identity")
	}
}

func TestAnchoringDirectoryOnlyAndDotfiles(t *testing.T) {
	anchored := mustPolicy(t, "/build")
	assertExcluded(t, anchored, "build", false, true)
	assertExcluded(t, anchored, "build", true, true)
	assertExcluded(t, anchored, "build/a", false, true)
	assertExcluded(t, anchored, "src/build", false, false)
	assertExcluded(t, anchored, "src/build/a", false, false)

	pair := mustPolicy(t, "a/b")
	assertExcluded(t, pair, "a/b", false, true)
	assertExcluded(t, pair, "x/a/b", false, false)
	if pair.Identity() != mustPolicy(t, "/a/b").Identity() {
		t.Fatal("/a/b and a/b compiled differently")
	}

	anyDepth := mustPolicy(t, "build")
	assertExcluded(t, anyDepth, "src/build", true, true)
	assertExcluded(t, anyDepth, "src/build/a", false, true)
	if anyDepth.Identity() != mustPolicy(t, "**/build").Identity() {
		t.Fatal("unanchored name and **/build compiled differently")
	}

	directories := mustPolicy(t, "build/")
	assertExcluded(t, directories, "build", false, false)
	assertExcluded(t, directories, "build", true, true)
	assertExcluded(t, directories, "src/build", true, true)
	assertExcluded(t, directories, "src/build/a.go", false, true)
	assertExcluded(t, directories, "src/builder", false, false)

	escapedName := mustPolicy(t, `foo\\/`)
	assertExcluded(t, escapedName, `foo\`, true, true)
	assertExcluded(t, escapedName, `foo\`, false, false)
	literalSlash := mustPolicy(t, `foo\/`)
	assertExcluded(t, literalSlash, "foo", false, false)
	assertExcluded(t, literalSlash, "foo/bar", false, false)

	rootDir := mustPolicy(t, "/build/")
	assertExcluded(t, rootDir, "build", true, true)
	assertExcluded(t, rootDir, "build", false, false)
	assertExcluded(t, rootDir, "src/build", true, false)
	assertExcluded(t, rootDir, "build/a", false, true)

	any := mustPolicy(t, "*")
	assertExcluded(t, any, ".hidden", false, true)
	assertExcluded(t, any, "visible", false, true)
	assertExcluded(t, any, ".git/config", false, true)
	if any.Identity() != mustPolicy(t, "/**").Identity() {
		t.Fatal("* and /** compiled differently")
	}
	dot := mustPolicy(t, ".*")
	assertExcluded(t, dot, ".env", false, true)
	assertExcluded(t, dot, "env", false, false)
}

func TestNegationLastMatchAndAncestorSealing(t *testing.T) {
	logs := mustPolicy(t, "*.log", "!important.log")
	assertExcluded(t, logs, "foo.log", false, true)
	assertExcluded(t, logs, "important.log", false, false)
	assertExcluded(t, logs, "dir/foo.log", false, true)
	assertExcluded(t, logs, "dir/important.log", false, false)

	reversed := mustPolicy(t, "!important.log", "*.log")
	assertExcluded(t, reversed, "important.log", false, true)
	restored := mustPolicy(t, "*.log", "!important.log", "*.log")
	assertExcluded(t, restored, "important.log", false, true)
	if logs.Identity() == reversed.Identity() || logs.Identity() == restored.Identity() {
		t.Fatal("rule order did not change policy identity")
	}

	sealed := mustPolicy(t, "vendor/", "!vendor/patched.go")
	assertExcluded(t, sealed, "vendor", true, true)
	assertExcluded(t, sealed, "vendor/patched.go", false, true)
	assertExcluded(t, sealed, "vendor/lib.go", false, true)
	if !sealed.Match("vendor/patched.go", false, true) {
		t.Fatal("a sealed parent did not exclude its child")
	}
	if sealed.Match("vendor/patched.go", false, false) {
		t.Fatal("Match re-included nothing when the parent was not sealed")
	}

	open := mustPolicy(t, "vendor/*", "!vendor/patched.go")
	assertExcluded(t, open, "vendor", true, false)
	assertExcluded(t, open, "vendor/patched.go", false, false)
	assertExcluded(t, open, "vendor/lib.go", false, true)
	if sealed.Identity() == open.Identity() {
		t.Fatal("vendor/ and vendor/* share an identity")
	}

	reopened := mustPolicy(t, "foo/*", "!foo/bar/", "!foo/bar/baz.txt")
	assertExcluded(t, reopened, "foo", true, false)
	assertExcluded(t, reopened, "foo/bar", true, false)
	assertExcluded(t, reopened, "foo/bar/baz.txt", false, false)
	assertExcluded(t, reopened, "foo/bar/skip.txt", false, false)
	assertExcluded(t, reopened, "foo/other.txt", false, true)
}

func TestCharacterClassesEscapesAndExactMatch(t *testing.T) {
	digits := mustPolicy(t, "file[0-9].tmp")
	assertExcluded(t, digits, "file7.tmp", false, true)
	assertExcluded(t, digits, "file0.tmp", false, true)
	assertExcluded(t, digits, "file9.tmp", false, true)
	assertExcluded(t, digits, "filex.tmp", false, false)

	outside := mustPolicy(t, "x[!0-9].tmp")
	assertExcluded(t, outside, "xa.tmp", false, true)
	assertExcluded(t, outside, "x1.tmp", false, false)

	members := mustPolicy(t, "x[abc]")
	assertExcluded(t, members, "xa", false, true)
	assertExcluded(t, members, "xb", false, true)
	assertExcluded(t, members, "xc", false, true)
	assertExcluded(t, members, "xd", false, false)
	negated := mustPolicy(t, "x[!abc]")
	assertExcluded(t, negated, "xd", false, true)
	assertExcluded(t, negated, "xa", false, false)

	caret := mustPolicy(t, "[^a].tmp")
	assertExcluded(t, caret, "^.tmp", false, true)
	assertExcluded(t, caret, "a.tmp", false, true)
	assertExcluded(t, caret, "b.tmp", false, false)

	dash := mustPolicy(t, "x[a-]")
	assertExcluded(t, dash, "xa", false, true)
	assertExcluded(t, dash, "x-", false, true)
	assertExcluded(t, dash, "xb", false, false)
	escapedDash := mustPolicy(t, `x[a\-c]`)
	assertExcluded(t, escapedDash, "xa", false, true)
	assertExcluded(t, escapedDash, "x-", false, true)
	assertExcluded(t, escapedDash, "xc", false, true)
	assertExcluded(t, escapedDash, "xb", false, false)

	bracket := mustPolicy(t, `x[\]]`)
	assertExcluded(t, bracket, "x]", false, true)
	assertExcluded(t, mustPolicy(t, "a]b"), "a]b", false, true)

	greek := mustPolicy(t, "x[α-ω]")
	assertExcluded(t, greek, "xβ", false, true)
	assertExcluded(t, greek, "xa", false, false)

	literalStar := mustPolicy(t, `a\*b`)
	assertExcluded(t, literalStar, "a*b", false, true)
	assertExcluded(t, literalStar, "axb", false, false)
	literalMark := mustPolicy(t, `a\?b`)
	assertExcluded(t, literalMark, "a?b", false, true)
	assertExcluded(t, literalMark, "axb", false, false)
	literalBang := mustPolicy(t, `\!foo`)
	assertExcluded(t, literalBang, "!foo", false, true)
	assertExcluded(t, literalBang, "foo", false, false)
	assertExcluded(t, mustPolicy(t, "/!foo"), "!foo", false, true)
	assertExcluded(t, mustPolicy(t, "!/foo"), "foo", false, false)

	logs := mustPolicy(t, "*.LOG")
	assertExcluded(t, logs, "a.LOG", false, true)
	assertExcluded(t, logs, "a.log", false, false)
	assertExcluded(t, mustPolicy(t, "Build/"), "Build", true, true)
	assertExcluded(t, mustPolicy(t, "Build/"), "build", true, false)

	composed := "caf\u00e9"
	decomposed := "cafe\u0301"
	name := mustPolicy(t, composed)
	assertExcluded(t, name, composed, false, true)
	assertExcluded(t, name, decomposed, false, false)
}

func TestRuleOrderAndDuplicates(t *testing.T) {
	one := mustPolicy(t, "*.log")
	two := mustPolicy(t, "*.log", "*.log")
	if one.RuleCount() != 1 || two.RuleCount() != 2 {
		t.Fatalf("counts = %d and %d", one.RuleCount(), two.RuleCount())
	}
	assertExcluded(t, one, "a.log", false, true)
	assertExcluded(t, two, "a.log", false, true)
	if one.Identity() == two.Identity() {
		t.Fatal("a duplicate rule did not change identity")
	}

	forward := mustPolicy(t, "*.log", "!keep.log")
	backward := mustPolicy(t, "!keep.log", "*.log")
	if forward.Identity() == backward.Identity() {
		t.Fatal("swapped rules did not change identity")
	}
	assertExcluded(t, forward, "keep.log", false, false)
	assertExcluded(t, backward, "keep.log", false, true)
}

func TestZeroPolicyAndInputIndependence(t *testing.T) {
	var zero Policy
	assertExcluded(t, zero, "a/b", false, false)
	assertExcluded(t, zero, "a", true, false)
	if zero.Match("a/b", false, false) {
		t.Fatal("zero policy excluded a path")
	}
	if !zero.Match("a/b", false, true) {
		t.Fatal("zero policy did not keep a caller-supplied seal")
	}
	if zero.RuleCount() != 0 {
		t.Fatalf("RuleCount = %d", zero.RuleCount())
	}

	buf := []byte(`{"schema":1,"presets":[],"exclude":["*.log"]}`)
	policy, err := Parse(buf)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	for i := range buf {
		buf[i] = ' '
	}
	assertExcluded(t, policy, "a.log", false, true)
	first := policy.Identity()
	if policy.Identity() != first {
		t.Fatal("Identity changed between calls")
	}
}

func TestIdentityIsDomainSeparatedAndFormattingIndependent(t *testing.T) {
	if identityHeader != "nodex.ignore.policy\n" {
		t.Fatalf("header = %q", identityHeader)
	}
	if strings.Contains(strings.ToLower(identityHeader), "agentary") {
		t.Fatalf("header %q is not Nodex-specific", identityHeader)
	}
	body, ok := (Policy{}).canonicalBody()
	if !ok || hex.EncodeToString(body) != "0000000000000000" {
		t.Fatalf("empty body = %x, ok %v", body, ok)
	}

	compact := []byte(`{"schema":1,"presets":[],"exclude":["*.log","!important.log"]}`)
	pretty := []byte("{\n  \"exclude\": [\n    \"*.log\",\n    \"!important.log\"\n  ],\n  \"presets\": [],\n  \"schema\": 1\n}\n")
	reordered := []byte(`{"exclude":["*.log","!important.log"],"presets":[],"schema":1}`)
	parsed := make([]Policy, 0, 3)
	for _, document := range [][]byte{compact, pretty, reordered} {
		policy, err := Parse(document)
		if err != nil {
			t.Fatalf("Parse(%s): %v", document, err)
		}
		parsed = append(parsed, policy)
	}
	if parsed[0].Identity() != parsed[1].Identity() || parsed[0].Identity() != parsed[2].Identity() {
		t.Fatalf("formatting changed identity: %s %s %s", parsed[0].Identity(), parsed[1].Identity(), parsed[2].Identity())
	}

	identity := parsed[0].Identity()
	if !regexp.MustCompile(`^sha256:[0-9a-f]{64}$`).MatchString(identity) {
		t.Fatalf("identity = %s", identity)
	}
	encoded, ok := parsed[0].canonicalBody()
	if !ok {
		t.Fatal("canonical body failed")
	}
	sum := sha256.New()
	_, _ = sum.Write([]byte(identityHeader))
	_, _ = sum.Write(encoded)
	if identity != "sha256:"+hex.EncodeToString(sum.Sum(nil)) {
		t.Fatal("Identity is not the domain-separated SHA-256")
	}
	documentSum := sha256.Sum256(compact)
	if identity == "sha256:"+hex.EncodeToString(documentSum[:]) {
		t.Fatal("Identity hashes the JSON bytes")
	}

	if mustPolicy(t, "x[ab]").Identity() == mustPolicy(t, "x[ba]").Identity() {
		t.Fatal("character-class order was normalized")
	}
	if mustPolicy(t, "x[a-c]").Identity() == mustPolicy(t, "x[abc]").Identity() {
		t.Fatal("a range and a member list were normalized")
	}
	if mustPolicy(t, "*").Identity() == mustPolicy(t, "**").Identity() {
		t.Fatal("* and ** share an identity")
	}
	if mustPolicy(t, "build/").Identity() == mustPolicy(t, "build").Identity() {
		t.Fatal("directory-only and plain rules share an identity")
	}
}

func TestConcurrentMatchAndIdentity(t *testing.T) {
	policy := mustPolicy(t, "*.log", "!important.log", "vendor/")
	identity := policy.Identity()
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 200; j++ {
				if !policy.Excluded("dir/a.log", false) || policy.Excluded("important.log", false) || !policy.Excluded("vendor/x.go", false) {
					t.Error("concurrent decision changed")
					return
				}
				if policy.Identity() != identity {
					t.Error("concurrent identity changed")
					return
				}
			}
		}()
	}
	wg.Wait()
}

func TestExplicitOutputsExclusion(t *testing.T) {
	data := []byte(`{"schema":1,"presets":[],"exclude":[".outputs/"]}`)
	var document struct {
		Schema  int      `json:"schema"`
		Presets []string `json:"presets"`
		Exclude []string `json:"exclude"`
	}
	if err := json.Unmarshal(data, &document); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if document.Schema != 1 || document.Presets == nil || len(document.Presets) != 0 || len(document.Exclude) != 1 || document.Exclude[0] != ".outputs/" {
		t.Fatalf("document = %+v", document)
	}
	policy, err := Parse(data)
	if err != nil {
		t.Fatalf("Parse explicit exclusion document: %v", err)
	}
	if policy.Identity() != mustPolicy(t, ".outputs/").Identity() || policy.RuleCount() != 1 {
		t.Fatalf("explicit policy = %d rules, identity %s", policy.RuleCount(), policy.Identity())
	}
	assertExcluded(t, policy, ".outputs", true, true)
	assertExcluded(t, policy, ".outputs", false, false)
	assertExcluded(t, policy, ".outputs/report.md", false, true)
	assertExcluded(t, policy, "pkg/.outputs/note.txt", false, true)
	assertExcluded(t, policy, "main.go", false, false)
	assertExcluded(t, policy, "vendor/lib.go", false, false)
}

func TestOutputsIsNotABuiltInPolicy(t *testing.T) {
	var zero Policy
	assertExcluded(t, zero, ".outputs/report.md", false, false)
	assertExcluded(t, mustPolicy(t, "vendor/"), ".outputs/report.md", false, false)

	root := moduleRoot(t)
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			switch entry.Name() {
			case ".git", ".outputs", ".nodex", "testdata", "vendor":
				if path != root {
					return filepath.SkipDir
				}
			}
			return nil
		}
		if !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if bytes.Contains(data, []byte(".outputs")) {
			rel, _ := filepath.Rel(root, path)
			t.Errorf("%s encodes .outputs as product text", filepath.ToSlash(rel))
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk: %v", err)
	}
}

// Production project files must not import ignore.
// A project test may import it to prove the exclusion port.
// cmd/nodex may import ignore. That command is the composition point that
// loads the ignore document and passes the compiled policy to discovery.
func TestProjectDoesNotImportIgnore(t *testing.T) {
	root := moduleRoot(t)
	for _, rel := range []string{"internal/project"} {
		err := filepath.WalkDir(filepath.Join(root, rel), func(path string, entry os.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
				return nil
			}
			file, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
			if err != nil {
				return err
			}
			for _, spec := range file.Imports {
				if strings.Contains(spec.Path.Value, "ignore") {
					relFile, relErr := filepath.Rel(root, path)
					if relErr != nil {
						relFile = path
					}
					t.Errorf("%s imports %s", filepath.ToSlash(relFile), spec.Path.Value)
				}
			}
			return nil
		})
		if err != nil {
			t.Fatalf("walk %s: %v", rel, err)
		}
	}
}

func TestPresetDocument(t *testing.T) {
	empty, err := Parse([]byte("{\n  \"schema\": 1,\n  \"presets\": [],\n  \"exclude\": []\n}\n"))
	if err != nil {
		t.Fatalf("empty presets: %v", err)
	}
	if empty.RuleCount() != 0 || len(empty.Presets()) != 0 || len(empty.Excludes()) != 0 {
		t.Fatalf("empty = rules %d presets %v excludes %v", empty.RuleCount(), empty.Presets(), empty.Excludes())
	}
	var zero Policy
	if empty.Identity() != zero.Identity() {
		t.Fatal("empty presets changed the zero identity")
	}

	one, err := Parse([]byte(`{"schema":1,"presets":["go:tests"],"exclude":["zzz/","aaa/","zzz/"]}`))
	if err != nil {
		t.Fatalf("one preset: %v", err)
	}
	if !slicesEqual(one.Presets(), []string{"go:tests"}) || !slicesEqual(one.Excludes(), []string{"zzz/", "aaa/", "zzz/"}) {
		t.Fatalf("one = presets %v excludes %v", one.Presets(), one.Excludes())
	}

	many, err := Parse([]byte(`{"schema":1,"presets":["go:vendor","go:tests","go:generated"],"exclude":[]}`))
	if err != nil {
		t.Fatalf("many: %v", err)
	}
	if !slicesEqual(many.Presets(), []string{"go:generated", "go:tests", "go:vendor"}) {
		t.Fatalf("presets = %v", many.Presets())
	}

	reordered, err := Parse([]byte(`{"presets":["go:vendor","go:generated","go:tests"],"exclude":[],"schema":1}`))
	if err != nil {
		t.Fatalf("reordered: %v", err)
	}
	if many.Identity() != reordered.Identity() {
		t.Fatal("preset order changed identity")
	}
	other, err := Parse([]byte(`{"schema":1,"presets":["go:tests"],"exclude":[]}`))
	if err != nil {
		t.Fatalf("other: %v", err)
	}
	if many.Identity() == other.Identity() || many.Identity() == zero.Identity() {
		t.Fatal("preset set did not change identity")
	}

	pretty := []byte("{\n  \"exclude\": [],\n  \"presets\": [\n    \"go:vendor\",\n    \"go:tests\"\n  ],\n  \"schema\": 1\n}\n")
	compact := []byte(`{"schema":1,"presets":["go:tests","go:vendor"],"exclude":[]}`)
	left, err := Parse(pretty)
	if err != nil {
		t.Fatalf("pretty: %v", err)
	}
	right, err := Parse(compact)
	if err != nil {
		t.Fatalf("compact: %v", err)
	}
	if left.Identity() != right.Identity() {
		t.Fatal("formatting changed preset identity")
	}

	if _, err := Parse([]byte(`{"schema":1,"presets":["go:tests","go:tests"],"exclude":[]}`)); !errors.Is(err, ErrDuplicatePreset) {
		t.Fatalf("duplicate = %v", err)
	}
	if _, err := Parse([]byte(`{"schema":1,"presets":[""],"exclude":[]}`)); err == nil || !strings.Contains(err.Error(), "empty") {
		t.Fatalf("empty preset = %v", err)
	}
	if _, err := Parse([]byte(`{"schema":1,"presets":["go:tests",1],"exclude":[]}`)); err == nil || !strings.Contains(err.Error(), "presets[1]") {
		t.Fatalf("non-string = %v", err)
	}

	// Preset strings are opaque here. Meaning is not decided by this package.
	opaque, err := Parse([]byte(`{"schema":1,"presets":["go:all","zzz"],"exclude":["b","a"]}`))
	if err != nil {
		t.Fatalf("opaque presets: %v", err)
	}
	if !slicesEqual(opaque.Presets(), []string{"go:all", "zzz"}) || !slicesEqual(opaque.Excludes(), []string{"b", "a"}) {
		t.Fatalf("opaque = presets %v excludes %v", opaque.Presets(), opaque.Excludes())
	}
	forward := mustPolicy(t, "keep.log", "!keep.log")
	backward := mustPolicy(t, "!keep.log", "keep.log")
	if forward.Identity() == backward.Identity() {
		t.Fatal("manual rule order stopped affecting identity")
	}
}

func TestPresetAPICopiesAndEncode(t *testing.T) {
	callerPresets := []string{"go:vendor", "go:tests"}
	callerExcludes := []string{"zzz/", "aaa/", "zzz/"}
	policy, err := New(callerPresets, callerExcludes)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	callerPresets[0] = "changed"
	callerExcludes[0] = "changed"
	if !slicesEqual(policy.Presets(), []string{"go:tests", "go:vendor"}) || !slicesEqual(policy.Excludes(), []string{"zzz/", "aaa/", "zzz/"}) {
		t.Fatalf("New retained caller slices: %v %v", policy.Presets(), policy.Excludes())
	}
	gotPresets := policy.Presets()
	gotExcludes := policy.Excludes()
	gotPresets[0] = "changed"
	gotExcludes[0] = "changed"
	if policy.Presets()[0] == "changed" || policy.Excludes()[0] == "changed" {
		t.Fatal("Presets or Excludes exposed internal storage")
	}
	if _, err := New([]string{"go:tests", "go:tests"}, nil); !errors.Is(err, ErrDuplicatePreset) {
		t.Fatalf("duplicate New = %v", err)
	}
	if _, err := New([]string{""}, nil); err == nil {
		t.Fatal("empty preset was accepted")
	}
	failed, err := New([]string{"go:tests"}, []string{"["})
	if err == nil || failed.Identity() != (Policy{}).Identity() || failed.RuleCount() != 0 || len(failed.Presets()) != 0 {
		t.Fatalf("failed New = %v policy %+v", err, failed)
	}

	encoded, err := policy.Encode()
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	if !bytes.HasSuffix(encoded, []byte("\n")) || bytes.HasSuffix(encoded, []byte("\n\n")) {
		t.Fatalf("encoded = %q", encoded)
	}
	again, err := policy.Encode()
	if err != nil || !bytes.Equal(encoded, again) {
		t.Fatal("Encode was not deterministic")
	}
	round, err := Parse(encoded)
	if err != nil {
		t.Fatalf("Parse(Encode): %v", err)
	}
	if round.Identity() != policy.Identity() || !slicesEqual(round.Presets(), policy.Presets()) || !slicesEqual(round.Excludes(), policy.Excludes()) {
		t.Fatalf("round trip = presets %v excludes %v", round.Presets(), round.Excludes())
	}
	want := "{\n  \"schema\": 1,\n  \"presets\": [\n    \"go:tests\",\n    \"go:vendor\"\n  ],\n  \"exclude\": [\n    \"zzz/\",\n    \"aaa/\",\n    \"zzz/\"\n  ]\n}\n"
	if string(encoded) != want {
		t.Fatalf("encoded = %q", encoded)
	}
	var zero Policy
	empty, err := zero.Encode()
	if err != nil {
		t.Fatalf("zero Encode: %v", err)
	}
	if string(empty) != "{\n  \"schema\": 1,\n  \"presets\": [],\n  \"exclude\": []\n}\n" {
		t.Fatalf("zero encoded = %q", empty)
	}
}

func slicesEqual(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}

func moduleRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("locate ignore test")
	}
	return filepath.Dir(filepath.Dir(filepath.Dir(file)))
}
