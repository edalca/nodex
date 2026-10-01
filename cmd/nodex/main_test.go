package main

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/edalca/nodex/internal/ignore"
	"github.com/edalca/nodex/internal/index"
	"github.com/edalca/nodex/internal/project"
	"github.com/edalca/nodex/internal/skill"
	"github.com/edalca/nodex/internal/syntax"
)

// version stays a source constant. This declaration does not compile if
// version becomes a variable.
const _ = version

// defaultVersionOutput is the stdout of a normal build, including its
// single terminating newline.
const defaultVersionOutput = "nodex 0.1.0-beta.1\ncommit unknown\nbuilt unknown\n"

func TestMain(m *testing.M) {
	userHomeDir = func() (string, error) {
		return "", errors.New("user home is not available")
	}
	os.Exit(m.Run())
}

func TestGenerateUsage(t *testing.T) {
	root := t.TempDir()
	stdout, stderr, err := runAt(root)
	if err == nil || stdout != "" || stderr != commandUsage+"\n" {
		t.Fatalf("no args = %q %q %v", stdout, stderr, err)
	}
	for _, command := range []string{"generate", "comments", "status", "show"} {
		stdout, stderr, err = runAt(root, command)
		if err == nil || stdout != "" || stderr != fmt.Sprintf("unknown command %q\n", command) {
			t.Fatalf("legacy %s = %q %q %v", command, stdout, stderr, err)
		}
	}
	stdout, stderr, err = runAt(root, "index")
	if err == nil || stdout != "" || stderr != indexUsage+"\n" {
		t.Fatalf("index = %q %q %v", stdout, stderr, err)
	}
	stdout, stderr, err = runAt(root, "index", "generate", "extra")
	if err == nil || stdout != "" || stderr != "index generate takes no arguments\n" {
		t.Fatalf("extra = %q %q %v", stdout, stderr, err)
	}
	stdout, stderr, err = runAt(root, "index", "generate", "--root", root)
	if err == nil || stdout != "" || stderr != "index generate takes no arguments\n" {
		t.Fatalf("--root = %q %q %v", stdout, stderr, err)
	}
	stdout, stderr, err = runAt(root, "index", "comments", "--root", root)
	if err == nil || stdout != "" || stderr != "index comments: unsupported filter \"--root\"\n" {
		t.Fatalf("comments --root = %q %q %v", stdout, stderr, err)
	}
	stdout, stderr, err = runAt(root, "index", "status", "extra")
	if err == nil || stdout != "" || stderr != "index status takes no arguments\n" {
		t.Fatalf("status extra = %q %q %v", stdout, stderr, err)
	}
	stdout, stderr, err = runAt(root, "index", "declarations", "extra")
	if err == nil || stdout != "" || stderr != "index declarations: unexpected argument \"extra\"\n" {
		t.Fatalf("declarations extra = %q %q %v", stdout, stderr, err)
	}
	stdout, stderr, err = runAt(root, "index", "show")
	if err == nil || stdout != "" || stderr != "index show requires an ID\n" {
		t.Fatalf("show without ID = %q %q %v", stdout, stderr, err)
	}
	stdout, stderr, err = runAt(root, "index", "nope")
	if err == nil || stdout != "" || stderr != "unknown index command \"nope\"\n" {
		t.Fatalf("unknown index = %q %q %v", stdout, stderr, err)
	}
	stdout, stderr, err = runAt(root, "version")
	if err != nil || stdout != defaultVersionOutput || stderr != "" {
		t.Fatalf("version = %q %q %v", stdout, stderr, err)
	}
	stdout, stderr, err = runAt(root, "ignore")
	if err == nil || stdout != "" || stderr != "usage: nodex ignore presets|list|enable|disable\n" {
		t.Fatalf("ignore = %q %q %v", stdout, stderr, err)
	}
	stdout, stderr, err = runAt(root, "nope")
	if err == nil || stdout != "" || stderr != "unknown command \"nope\"\n" {
		t.Fatalf("unknown = %q %q %v", stdout, stderr, err)
	}
	var out, errOut bytes.Buffer
	err = run([]string{"index", "generate"}, func() (string, error) {
		return "", errors.New("no working directory")
	}, &out, &errOut)
	if err == nil || out.Len() != 0 || !strings.Contains(errOut.String(), "no working directory") {
		t.Fatalf("getwd = %q %q %v", out.String(), errOut.String(), err)
	}
}

func TestGenerateReportsCounts(t *testing.T) {
	t.Run("empty", func(t *testing.T) {
		root := t.TempDir()
		if got := generateOK(t, root); got != "generated 0 comments and 0 declarations from 0 source files\n" {
			t.Fatalf("stdout = %q", got)
		}
	})
	t.Run("one", func(t *testing.T) {
		root := t.TempDir()
		writeProjectFile(t, root, "a.go", "// alpha\n\npackage a\n")
		if got := generateOK(t, root); got != "generated 1 comment and 1 declaration from 1 source file\n" {
			t.Fatalf("stdout = %q", got)
		}
	})
	t.Run("many comments one file", func(t *testing.T) {
		root := t.TempDir()
		writeProjectFile(t, root, "a.go", "// beta\n\n// gamma\npackage a\n")
		if got := generateOK(t, root); got != "generated 2 comments and 1 declaration from 1 source file\n" {
			t.Fatalf("stdout = %q", got)
		}
	})
	t.Run("source without comments", func(t *testing.T) {
		root := t.TempDir()
		writeProjectFile(t, root, "a.go", "package a\n")
		if got := generateOK(t, root); got != "generated 0 comments and 1 declaration from 1 source file\n" {
			t.Fatalf("stdout = %q", got)
		}
	})
	t.Run("supported files only", func(t *testing.T) {
		root := t.TempDir()
		writeProjectFile(t, root, "a.go", "// alpha\n\npackage a\n")
		writeProjectFile(t, root, "b.go", "// beta\n\npackage b\n")
		writeProjectFile(t, root, "README.md", "package { not go\n")
		if got := generateOK(t, root); got != "generated 2 comments and 2 declarations from 2 source files\n" {
			t.Fatalf("stdout = %q", got)
		}
		_, snap := loadIndex(t, root)
		if pathsOf(snap) != "a.go,b.go" {
			t.Fatalf("sources = %s", pathsOf(snap))
		}
		if bytes.Contains(readGen(t, root, index.SnapshotPath), []byte(resolvedRoot(t, root))) {
			t.Fatal("snapshot contains the project root")
		}
	})
}

func TestGenerateSelectsSupportedSource(t *testing.T) {
	root := t.TempDir()
	writeProjectFile(t, root, "a.go", "// alpha\n\npackage a\n")
	writeProjectFile(t, root, "b.go", "// beta\n\npackage b\n")
	writeProjectFile(t, root, "README.md", "notes\n")
	writeProjectFile(t, root, "a_test.go", "// test comment\npackage a\n")
	writeProjectFile(t, root, "file_linux.go", "// linux\npackage p\n")
	writeProjectFile(t, root, "file_windows.go", "// windows\npackage p\n")
	writeProjectFile(t, root, "zz_generated.go", "// Code generated by test. DO NOT EDIT.\n\npackage p\n")
	writeProjectFile(t, root, "vendor/lib.go", "// vendored\npackage vendor\n")
	generateOK(t, root)
	_, snap := loadIndex(t, root)
	want := []string{
		"a.go",
		"a_test.go",
		"b.go",
		"file_linux.go",
		"file_windows.go",
		"vendor/lib.go",
		"zz_generated.go",
	}
	if !slices.Equal(sourcePaths(snap), want) {
		t.Fatalf("sources = %q, want %q", sourcePaths(snap), want)
	}
	idx, _ := loadIndex(t, root)
	var texts []string
	for _, entry := range idx.Entries() {
		texts = append(texts, entry.Path+":"+entry.Text)
	}
	if !slices.Contains(texts, "zz_generated.go:Code generated by test. DO NOT EDIT.") {
		t.Fatalf("generated comment missing: %q", texts)
	}
	if !slices.Contains(texts, "a_test.go:test comment") {
		t.Fatalf("test comment missing: %q", texts)
	}
	if tree := nodexTree(t, root); !slices.Equal(tree, []string{"index", "index/comments.jsonl", "index/declarations.jsonl", "index/snapshot.json"}) {
		t.Fatalf(".nodex = %q", tree)
	}
	if _, err := os.Stat(filepath.Join(resolvedRoot(t, root), filepath.FromSlash(ignore.DocumentPath))); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("ignore.json stat = %v, want missing", err)
	}
}

func TestGenerateAppliesIgnorePolicy(t *testing.T) {
	root := t.TempDir()
	ignorePath := filepath.Join(root, filepath.FromSlash(ignore.DocumentPath))
	document := "{\n  \"schema\": 1,\n  \"presets\": [],\n  \"exclude\": [\".outputs/\"]\n}\n"
	writeProjectFile(t, root, ignore.DocumentPath, document)
	writeProjectFile(t, root, "main.go", "// main\n\npackage main\n")
	writeProjectFile(t, root, ".outputs/generated.go", "// hidden\npackage p\n")
	before := readFile(t, ignorePath)
	generateOK(t, root)
	if !bytes.Equal(readFile(t, ignorePath), before) {
		t.Fatal("generate modified ignore.json")
	}
	_, snap := loadIndex(t, root)
	if pathsOf(snap) != "main.go" {
		t.Fatalf("sources = %s", pathsOf(snap))
	}
	if snap.PolicyIdentity != mustPolicy(t, document).Identity() {
		t.Fatalf("policy = %s", snap.PolicyIdentity)
	}
	if tree := nodexTree(t, root); !slices.Equal(tree, []string{
		"ignore.json",
		"index",
		"index/comments.jsonl",
		"index/declarations.jsonl",
		"index/snapshot.json",
	}) {
		t.Fatalf(".nodex = %q", tree)
	}

	if err := os.Remove(ignorePath); err != nil {
		t.Fatalf("remove ignore: %v", err)
	}
	generateOK(t, root)
	_, snap = loadIndex(t, root)
	if pathsOf(snap) != ".outputs/generated.go,main.go" {
		t.Fatalf("sources without exclusion = %s", pathsOf(snap))
	}
	if snap.PolicyIdentity != (ignore.Policy{}).Identity() {
		t.Fatalf("zero policy = %s, snap %s", ignore.Policy{}.Identity(), snap.PolicyIdentity)
	}
}

func TestGenerateMissingIgnoreUsesZeroPolicy(t *testing.T) {
	root := t.TempDir()
	writeProjectFile(t, root, "main.go", "package main\n")
	generateOK(t, root)
	_, snap := loadIndex(t, root)
	if snap.PolicyIdentity != (ignore.Policy{}).Identity() {
		t.Fatalf("policy = %s, want zero policy %s", snap.PolicyIdentity, ignore.Policy{}.Identity())
	}
}

func TestGenerateRejectsInvalidIgnore(t *testing.T) {
	root := t.TempDir()
	writeProjectFile(t, root, "main.go", "// main\n\npackage main\n")
	writeProjectFile(t, root, ignore.DocumentPath, "{")
	stderr := generateFail(t, root)
	if strings.Contains(stderr, "generated") {
		t.Fatalf("stderr = %q", stderr)
	}
	if _, err := os.Stat(filepath.Join(resolvedRoot(t, root), filepath.FromSlash(index.IndexDir))); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("index dir stat = %v", err)
	}
}

func TestGenerateRejectsUnreadableIgnore(t *testing.T) {
	root := t.TempDir()
	writeProjectFile(t, root, "main.go", "package main\n")
	path := filepath.Join(root, filepath.FromSlash(ignore.DocumentPath))
	writeProjectFile(t, root, ignore.DocumentPath, "{\n  \"schema\": 1,\n  \"presets\": [],\n  \"exclude\": []\n}\n")
	restoreMode(t, path, 0o644)
	if err := os.Chmod(path, 0); err != nil {
		t.Fatalf("chmod: %v", err)
	}
	generateFail(t, root)
}

func TestGenerateIgnoresMalformedExcludedAndUnsupported(t *testing.T) {
	root := t.TempDir()
	writeProjectFile(t, root, ignore.DocumentPath, "{\n  \"schema\": 1,\n  \"presets\": [],\n  \"exclude\": [\".outputs/\"]\n}\n")
	writeProjectFile(t, root, "main.go", "// main\n\npackage main\n")
	writeProjectFile(t, root, ".outputs/bad.go", "package {\n")
	writeProjectFile(t, root, "README.md", "package {\n")
	restoreMode(t, filepath.Join(root, "README.md"), 0o644)
	if err := os.Chmod(filepath.Join(root, "README.md"), 0); err != nil {
		t.Fatalf("chmod: %v", err)
	}
	if got := generateOK(t, root); got != "generated 1 comment and 1 declaration from 1 source file\n" {
		t.Fatalf("stdout = %q", got)
	}
	_, snap := loadIndex(t, root)
	if pathsOf(snap) != "main.go" {
		t.Fatalf("sources = %s", pathsOf(snap))
	}
}

func TestGenerateRejectsMalformedSource(t *testing.T) {
	root := t.TempDir()
	writeProjectFile(t, root, "main.go", "package {\n")
	generateFail(t, root)
	if _, err := os.Stat(filepath.Join(resolvedRoot(t, root), ".nodex")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf(".nodex stat = %v, want missing", err)
	}
}

func TestGenerateWithoutMarkerUsesStartDirectory(t *testing.T) {
	parent := t.TempDir()
	writeProjectFile(t, parent, "root.go", "// root\n\npackage root\n")
	sub := filepath.Join(parent, "sub")
	writeProjectFile(t, sub, "main.go", "// main\n\npackage main\n")
	generateOK(t, sub)
	_, snap := loadIndex(t, sub)
	if pathsOf(snap) != "main.go" {
		t.Fatalf("sources = %s", pathsOf(snap))
	}
	if _, err := os.Stat(filepath.Join(parent, ".nodex")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("parent gained .nodex")
	}
}

func TestGenerateDoesNotFollowNodexSymlink(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	writeProjectFile(t, root, "main.go", "package main\n")
	if err := os.Symlink(outside, filepath.Join(root, ".nodex")); err != nil {
		t.Fatalf("symlink: %v", err)
	}
	generateFail(t, root)
	if _, err := os.Stat(filepath.Join(outside, "index")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("wrote through .nodex symlink: %v", err)
	}
}

func TestGenerateRepeatedBytesAndRegeneration(t *testing.T) {
	root := t.TempDir()
	writeProjectFile(t, root, "a.go", "// alpha\n\npackage a\n")
	writeProjectFile(t, root, "README.md", "notes\n")
	generateOK(t, root)
	firstSnap := readGen(t, root, index.SnapshotPath)
	firstComments := readGen(t, root, index.CommentsPath)
	generateOK(t, root)
	if !bytes.Equal(readGen(t, root, index.SnapshotPath), firstSnap) || !bytes.Equal(readGen(t, root, index.CommentsPath), firstComments) {
		t.Fatal("repeated generation changed bytes")
	}

	writeProjectFile(t, root, "README.md", "notes changed\n")
	generateOK(t, root)
	if !bytes.Equal(readGen(t, root, index.SnapshotPath), firstSnap) || !bytes.Equal(readGen(t, root, index.CommentsPath), firstComments) {
		t.Fatal("unsupported file change affected the index")
	}
	policy, sources := generationInputs(t, root)
	_, snap := loadIndex(t, root)
	if !index.Current(snap, policy.Identity(), sources) {
		t.Fatal("unsupported change reported stale")
	}

	writeProjectFile(t, root, "a.go", "// changed\n\npackage a\n")
	if index.Current(snap, policy.Identity(), fingerprints(t, root, policy)) {
		t.Fatal("source change reported current")
	}
	generateOK(t, root)
	if bytes.Equal(readGen(t, root, index.SnapshotPath), firstSnap) {
		t.Fatal("regeneration kept the previous snapshot")
	}
	idx, snap := loadIndex(t, root)
	if idx.Len() != 1 || idx.Entries()[0].Text != "changed" {
		t.Fatalf("entries = %+v", idx.Entries())
	}
	if !index.Current(snap, policy.Identity(), fingerprints(t, root, policy)) {
		t.Fatal("regenerated snapshot is stale")
	}
}

func TestGenerateStaleSourceSet(t *testing.T) {
	root := t.TempDir()
	writeProjectFile(t, root, "a.go", "// alpha\n\npackage a\n")
	writeProjectFile(t, root, "b.go", "// beta\n\npackage b\n")
	generateOK(t, root)
	policy, _ := generationInputs(t, root)
	_, before := loadIndex(t, root)

	writeProjectFile(t, root, "c.go", "// gamma\n\npackage c\n")
	if index.Current(before, policy.Identity(), fingerprints(t, root, policy)) {
		t.Fatal("added source reported current")
	}
	if err := os.Remove(filepath.Join(root, "c.go")); err != nil {
		t.Fatalf("remove c.go: %v", err)
	}
	if err := os.Rename(filepath.Join(root, "b.go"), filepath.Join(root, "d.go")); err != nil {
		t.Fatalf("rename: %v", err)
	}
	if index.Current(before, policy.Identity(), fingerprints(t, root, policy)) {
		t.Fatal("renamed source reported current")
	}
	if err := os.Remove(filepath.Join(root, "d.go")); err != nil {
		t.Fatalf("remove d.go: %v", err)
	}
	if index.Current(before, policy.Identity(), fingerprints(t, root, policy)) {
		t.Fatal("removed source reported current")
	}
}

func TestGenerateIgnoredFileDoesNotStale(t *testing.T) {
	root := t.TempDir()
	compact := "{\"schema\":1,\"presets\":[],\"exclude\":[\".outputs/\"]}\n"
	pretty := "{\n  \"exclude\": [\".outputs/\"],\n  \"presets\": [],\n  \"schema\": 1\n}\n"
	if mustPolicy(t, compact).Identity() != mustPolicy(t, pretty).Identity() {
		t.Fatal("equivalent documents compiled to different policies")
	}
	writeProjectFile(t, root, ignore.DocumentPath, compact)
	writeProjectFile(t, root, "main.go", "// main\n\npackage main\n")
	writeProjectFile(t, root, ".outputs/generated.go", "// hidden\npackage p\n")
	generateOK(t, root)
	snapBytes := readGen(t, root, index.SnapshotPath)
	commentBytes := readGen(t, root, index.CommentsPath)
	_, snap := loadIndex(t, root)

	writeProjectFile(t, root, ".outputs/generated.go", "package {\ncompletely different\n")
	policy := mustPolicy(t, compact)
	if !index.Current(snap, policy.Identity(), fingerprints(t, root, policy)) {
		t.Fatal("ignored file change reported stale")
	}
	generateOK(t, root)
	if !bytes.Equal(readGen(t, root, index.SnapshotPath), snapBytes) || !bytes.Equal(readGen(t, root, index.CommentsPath), commentBytes) {
		t.Fatal("ignored file change changed persisted bytes")
	}

	writeProjectFile(t, root, ignore.DocumentPath, pretty)
	policy = mustPolicy(t, pretty)
	if !index.Current(snap, policy.Identity(), fingerprints(t, root, policy)) {
		t.Fatal("equivalent ignore document reported stale")
	}
	generateOK(t, root)
	if !bytes.Equal(readGen(t, root, index.SnapshotPath), snapBytes) || !bytes.Equal(readGen(t, root, index.CommentsPath), commentBytes) {
		t.Fatal("reformatting ignore.json changed persisted bytes")
	}
}

func TestFailedGenerateKeepsExistingIndex(t *testing.T) {
	t.Run("syntax", func(t *testing.T) {
		root := committedProject(t)
		before := pairBytes(t, root)
		writeProjectFile(t, root, "main.go", "package {\n")
		generateFail(t, root)
		if !bytes.Equal(pairBytes(t, root), before) {
			t.Fatal("syntax failure changed the index")
		}
	})
	t.Run("read", func(t *testing.T) {
		root := committedProject(t)
		before := pairBytes(t, root)
		path := filepath.Join(root, "main.go")
		restoreMode(t, path, 0o644)
		if err := os.Chmod(path, 0); err != nil {
			t.Fatalf("chmod: %v", err)
		}
		generateFail(t, root)
		if err := os.Chmod(path, 0o644); err != nil {
			t.Fatalf("restore: %v", err)
		}
		if !bytes.Equal(pairBytes(t, root), before) {
			t.Fatal("read failure changed the index")
		}
	})
	t.Run("discovery", func(t *testing.T) {
		root := committedProject(t)
		writeProjectFile(t, root, "hidden/a.go", "// hidden\npackage p\n")
		generateOK(t, root)
		before := pairBytes(t, root)
		dir := filepath.Join(root, "hidden")
		restoreMode(t, dir, 0o755)
		if err := os.Chmod(dir, 0); err != nil {
			t.Fatalf("chmod: %v", err)
		}
		generateFail(t, root)
		if err := os.Chmod(dir, 0o755); err != nil {
			t.Fatalf("restore: %v", err)
		}
		if !bytes.Equal(pairBytes(t, root), before) {
			t.Fatal("discovery failure changed the index")
		}
	})
	t.Run("ignore", func(t *testing.T) {
		root := committedProject(t)
		before := pairBytes(t, root)
		writeProjectFile(t, root, ignore.DocumentPath, "{")
		generateFail(t, root)
		if !bytes.Equal(pairBytes(t, root), before) {
			t.Fatal("ignore failure changed the index")
		}
	})
}

func runAt(root string, args ...string) (string, string, error) {
	var stdout, stderr bytes.Buffer
	err := run(args, func() (string, error) { return root, nil }, &stdout, &stderr)
	return stdout.String(), stderr.String(), err
}

func generateOK(t *testing.T, root string) string {
	t.Helper()
	stdout, stderr, err := runAt(root, "index", "generate")
	if err != nil {
		t.Fatalf("generate: %v\n%s", err, stderr)
	}
	if stderr != "" {
		t.Fatalf("stderr = %q", stderr)
	}
	if !strings.HasPrefix(stdout, "generated ") {
		t.Fatalf("stdout = %q", stdout)
	}
	return stdout
}

func generateFail(t *testing.T, root string) string {
	t.Helper()
	stdout, stderr, err := runAt(root, "index", "generate")
	if err == nil {
		t.Fatal("generate succeeded")
	}
	if stdout != "" {
		t.Fatalf("stdout = %q", stdout)
	}
	if !strings.Contains(stderr, "nodex: ") {
		t.Fatalf("stderr = %q", stderr)
	}
	if strings.Contains(stdout, "generated") || strings.Contains(stderr, "generated ") {
		t.Fatalf("failure printed a success line: stdout %q stderr %q", stdout, stderr)
	}
	return stderr
}

func committedProject(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	writeProjectFile(t, root, "main.go", "// main\n\npackage main\n")
	generateOK(t, root)
	return root
}

func writeProjectFile(t *testing.T, root, logical, data string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(logical))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", path, err)
	}
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func readFile(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return data
}

func resolvedRoot(t *testing.T, dir string) string {
	t.Helper()
	opened, err := project.Open(dir)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	return opened.Root()
}

func readGen(t *testing.T, dir, logical string) []byte {
	t.Helper()
	return readFile(t, filepath.Join(resolvedRoot(t, dir), filepath.FromSlash(logical)))
}

func pairBytes(t *testing.T, dir string) []byte {
	t.Helper()
	return bytes.Join([][]byte{
		readGen(t, dir, index.SnapshotPath),
		readGen(t, dir, index.CommentsPath),
		readGen(t, dir, index.DeclarationsPath),
	}, []byte{0})
}

func loadIndex(t *testing.T, dir string) (*index.Index, index.Snapshot) {
	t.Helper()
	idx, snap, err := index.Load(resolvedRoot(t, dir))
	if err != nil || idx == nil {
		t.Fatalf("Load: index=%v err=%v", idx, err)
	}
	return idx, snap
}

func sourcePaths(snap index.Snapshot) []string {
	out := make([]string, len(snap.Sources))
	for i, src := range snap.Sources {
		out[i] = src.Path
	}
	return out
}

func pathsOf(snap index.Snapshot) string {
	return strings.Join(sourcePaths(snap), ",")
}

func nodexTree(t *testing.T, dir string) []string {
	t.Helper()
	base := filepath.Join(resolvedRoot(t, dir), ".nodex")
	var out []string
	err := filepath.WalkDir(base, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if path == base {
			return nil
		}
		rel, err := filepath.Rel(base, path)
		if err != nil {
			return err
		}
		out = append(out, filepath.ToSlash(rel))
		return nil
	})
	if err != nil {
		t.Fatalf("walk .nodex: %v", err)
	}
	slices.Sort(out)
	return out
}

func mustPolicy(t *testing.T, document string) ignore.Policy {
	t.Helper()
	policy, err := ignore.Parse([]byte(document))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	return policy
}

func generationInputs(t *testing.T, dir string) (ignore.Policy, []index.Source) {
	t.Helper()
	root := resolvedRoot(t, dir)
	policy, err := loadPolicy(root)
	if err != nil {
		t.Fatalf("loadPolicy: %v", err)
	}
	return policy, fingerprints(t, dir, policy)
}

func fingerprints(t *testing.T, dir string, policy ignore.Policy) []index.Source {
	t.Helper()
	opened, err := project.Open(dir)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	files, err := selectedSources(opened, policy)
	if err != nil {
		t.Fatalf("selectedSources: %v", err)
	}
	var sources []index.Source
	for _, file := range files {
		sources = append(sources, file.source)
	}
	return sources
}

func restoreMode(t *testing.T, path string, mode os.FileMode) {
	t.Helper()
	t.Cleanup(func() {
		_ = os.Chmod(path, mode)
	})
}

func TestCommentsOutput(t *testing.T) {
	t.Run("missing", func(t *testing.T) {
		root := t.TempDir()
		writeProjectFile(t, root, "a.go", "// hello\n\npackage a\n")
		stdout, stderr, err := runAt(root, "index", "comments")
		if err == nil || stdout != "" || stderr != "nodex: no generated index; run nodex index generate\n" {
			t.Fatalf("missing = %q %q %v", stdout, stderr, err)
		}
		if _, statErr := os.Stat(filepath.Join(resolvedRoot(t, root), ".nodex")); !errors.Is(statErr, os.ErrNotExist) {
			t.Fatalf(".nodex stat = %v", statErr)
		}
	})
	t.Run("empty", func(t *testing.T) {
		root := t.TempDir()
		writeProjectFile(t, root, "a.go", "package a\n")
		generateOK(t, root)
		if got := commandOK(t, root, "comments"); got != "" {
			t.Fatalf("stdout = %q", got)
		}
		if got := commandOK(t, root, "status"); got != statusCurrent(1, 0) {
			t.Fatalf("status = %q", got)
		}
	})
	t.Run("no sources", func(t *testing.T) {
		root := t.TempDir()
		generateOK(t, root)
		if got := commandOK(t, root, "comments"); got != "" {
			t.Fatalf("stdout = %q", got)
		}
		if got := commandOK(t, root, "status"); got != statusCurrent(0, 0) {
			t.Fatalf("status = %q", got)
		}
	})
	t.Run("one", func(t *testing.T) {
		root := t.TempDir()
		writeProjectFile(t, root, "pkg/main.go", "// hello\n\npackage main\n")
		generateOK(t, root)
		got := commandOK(t, root, "comments")
		want := "## C000001\n\nfile: `pkg/main.go`\n\nhello\n"
		if got != want {
			t.Fatalf("stdout = %q, want %q", got, want)
		}
		assertNoDetailedCommentMetadata(t, got)
	})
	t.Run("trailing space", func(t *testing.T) {
		root := t.TempDir()
		writeProjectFile(t, root, "a.go", "// hello \n\npackage a\n")
		generateOK(t, root)
		if got := commandOK(t, root, "comments"); got != "## C000001\n\nfile: `a.go`\n\nhello \n" {
			t.Fatalf("stdout = %q", got)
		}
	})
	t.Run("many", func(t *testing.T) {
		root := t.TempDir()
		writeProjectFile(t, root, "a.go", "// first\n\npackage a\n")
		writeProjectFile(t, root, "b.go", "// second\n\npackage b\n")
		generateOK(t, root)
		got := commandOK(t, root, "comments")
		want := "## C000001\n\nfile: `a.go`\n\nfirst\n\n## C000002\n\nfile: `b.go`\n\nsecond\n"
		if got != want {
			t.Fatalf("stdout = %q, want %q", got, want)
		}
		assertNoDetailedCommentMetadata(t, got)
		if strings.Contains(got, root) {
			t.Fatalf("stdout contains an absolute path: %q", got)
		}
	})
	t.Run("multiline", func(t *testing.T) {
		root := t.TempDir()
		writeProjectFile(t, root, "a.go", "// alpha\n//\n// beta\npackage a\n")
		generateOK(t, root)
		got := commandOK(t, root, "comments")
		want := "## C000001\n\nfile: `a.go`\n\nalpha\n\nbeta\n"
		if got != want {
			t.Fatalf("stdout = %q, want %q", got, want)
		}
	})
	t.Run("markdown", func(t *testing.T) {
		root := t.TempDir()
		writeProjectFile(t, root, "a.go", "// # Title\n// **bold** and `code`\n// a < b & c\n// ```\npackage a\n")
		generateOK(t, root)
		got := commandOK(t, root, "comments")
		want := "## C000001\n\nfile: `a.go`\n\n# Title\n**bold** and `code`\na < b & c\n```\n"
		if got != want {
			t.Fatalf("stdout = %q, want %q", got, want)
		}
		if strings.Count(got, "```") != 1 {
			t.Fatalf("code fence count = %d in %q", strings.Count(got, "```"), got)
		}
		assertNoDetailedCommentMetadata(t, got)
		idx, _ := loadIndex(t, root)
		if got != formatComments(idx.Entries()) {
			t.Fatal("stdout does not match the stored comment text")
		}
	})
	t.Run("block text", func(t *testing.T) {
		root := t.TempDir()
		writeProjectFile(t, root, "a.go", "package a\n\n/*\nalpha\nbeta\n*/\n")
		generateOK(t, root)
		idx, _ := loadIndex(t, root)
		got := commandOK(t, root, "comments")
		if got != formatComments(idx.Entries()) {
			t.Fatalf("stdout = %q", got)
		}
		if len(idx.Entries()) != 1 || idx.Entries()[0].Text != "\nalpha\nbeta\n" {
			t.Fatalf("stored text = %q", idx.Entries()[0].Text)
		}
		if got != "## C000001\n\nfile: `a.go`\n\n\nalpha\nbeta\n" {
			t.Fatalf("stdout = %q", got)
		}
	})
}

func TestCommentsRejectsCorrupt(t *testing.T) {
	root := t.TempDir()
	writeProjectFile(t, root, "a.go", "// hello\n\npackage a\n")
	generateOK(t, root)
	commentsPath := filepath.Join(resolvedRoot(t, root), filepath.FromSlash(index.CommentsPath))
	broken := readFile(t, commentsPath)
	broken[len(broken)-2] ^= 0x1
	if err := os.WriteFile(commentsPath, broken, 0o644); err != nil {
		t.Fatalf("write comments: %v", err)
	}
	before := readFile(t, commentsPath)
	stdout, stderr, err := runAt(root, "index", "comments")
	if err == nil || stdout != "" || !strings.Contains(stderr, "corrupt") {
		t.Fatalf("corrupt = %q %q %v", stdout, stderr, err)
	}
	if strings.Contains(stdout, "## ") {
		t.Fatalf("stdout = %q", stdout)
	}
	if !bytes.Equal(readFile(t, commentsPath), before) {
		t.Fatal("comments rewrote a corrupt index")
	}
}

func TestSnapshotCurrentness(t *testing.T) {
	t.Run("unchanged", func(t *testing.T) {
		root := oneCommentProject(t)
		if got := commandOK(t, root, "comments"); got != "## C000001\n\nfile: `a.go`\n\nalpha\n" {
			t.Fatalf("comments = %q", got)
		}
		if got := commandOK(t, root, "status"); got != statusCurrent(1, 1) {
			t.Fatalf("status = %q", got)
		}
	})
	t.Run("content", func(t *testing.T) {
		root := oneCommentProject(t)
		before := pairBytes(t, root)
		writeProjectFile(t, root, "a.go", "// changed\n\npackage a\n")
		assertStalePair(t, root, statusStale(1, 1, 1))
		requireSameIndex(t, root, before)
	})
	t.Run("added", func(t *testing.T) {
		root := oneCommentProject(t)
		before := pairBytes(t, root)
		writeProjectFile(t, root, "b.go", "// beta\n\npackage b\n")
		assertStalePair(t, root, statusStale(1, 2, 1))
		requireSameIndex(t, root, before)
	})
	t.Run("removed", func(t *testing.T) {
		root := oneCommentProject(t)
		before := pairBytes(t, root)
		if err := os.Remove(filepath.Join(root, "a.go")); err != nil {
			t.Fatalf("remove: %v", err)
		}
		assertStalePair(t, root, statusStale(1, 0, 1))
		requireSameIndex(t, root, before)
	})
	t.Run("renamed", func(t *testing.T) {
		root := oneCommentProject(t)
		before := pairBytes(t, root)
		if err := os.Rename(filepath.Join(root, "a.go"), filepath.Join(root, "b.go")); err != nil {
			t.Fatalf("rename: %v", err)
		}
		assertStalePair(t, root, statusStale(1, 1, 1))
		requireSameIndex(t, root, before)
	})
	t.Run("policy", func(t *testing.T) {
		root := oneCommentProject(t)
		before := pairBytes(t, root)
		writeProjectFile(t, root, ignore.DocumentPath, "{\n  \"schema\": 1,\n  \"presets\": [],\n  \"exclude\": [\".outputs/\"]\n}\n")
		assertStalePair(t, root, statusStale(1, 1, 1))
		requireSameIndex(t, root, before)
	})
	t.Run("malformed", func(t *testing.T) {
		root := oneCommentProject(t)
		before := pairBytes(t, root)
		writeProjectFile(t, root, "a.go", "package {\n")
		assertStalePair(t, root, statusStale(1, 1, 1))
		requireSameIndex(t, root, before)
		stdout, stderr, err := runAt(root, "index", "comments")
		if err == nil || stdout != "" || stderr != "nodex: index is stale; run nodex index generate\n" {
			t.Fatalf("malformed comments = %q %q %v", stdout, stderr, err)
		}
		if strings.Contains(stderr, "expected") || strings.Contains(stderr, "syntax") {
			t.Fatalf("stderr = %q", stderr)
		}
	})
}

func TestStatusCanonicalCounts(t *testing.T) {
	root := t.TempDir()
	writeProjectFile(t, root, "a.go", "// beta\n\n// gamma\npackage a\n")
	generateOK(t, root)
	if got := commandOK(t, root, "status"); got != "index: current\nsources: 1\ncomments: 2\n" {
		t.Fatalf("current = %q", got)
	}
	before := pairBytes(t, root)
	writeProjectFile(t, root, "b.go", "package b\n")
	if got := commandOK(t, root, "status"); got != "index: stale\nindexed sources: 1\ncurrent sources: 2\ncomments: 2\n" {
		t.Fatalf("stale = %q", got)
	}
	assertStaleComments(t, root)
	requireSameIndex(t, root, before)
}

func TestStatusReportsMissingAndCorrupt(t *testing.T) {
	t.Run("missing", func(t *testing.T) {
		root := t.TempDir()
		writeProjectFile(t, root, "a.go", "package {\n")
		if got := commandOK(t, root, "status"); got != "index: missing\n" {
			t.Fatalf("status = %q", got)
		}
		if _, err := os.Stat(filepath.Join(resolvedRoot(t, root), ".nodex")); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf(".nodex stat = %v", err)
		}
	})
	t.Run("missing keeps ignore document", func(t *testing.T) {
		root := t.TempDir()
		writeProjectFile(t, root, ignore.DocumentPath, "{\n  \"schema\": 1,\n  \"presets\": [],\n  \"exclude\": []\n}\n")
		writeProjectFile(t, root, "a.go", "package a\n")
		if got := commandOK(t, root, "status"); got != "index: missing\n" {
			t.Fatalf("status = %q", got)
		}
		if tree := nodexTree(t, root); !slices.Equal(tree, []string{"ignore.json"}) {
			t.Fatalf(".nodex = %q", tree)
		}
		stdout, stderr, err := runAt(root, "index", "comments")
		if err == nil || stdout != "" {
			t.Fatalf("comments = %q %q %v", stdout, stderr, err)
		}
		if tree := nodexTree(t, root); !slices.Equal(tree, []string{"ignore.json"}) {
			t.Fatalf("comments created index state: %q", tree)
		}
	})
	t.Run("corrupt", func(t *testing.T) {
		root := t.TempDir()
		writeProjectFile(t, root, "a.go", "package a\n")
		writeProjectFile(t, root, index.SnapshotPath, "{}\n")
		before := readGen(t, root, index.SnapshotPath)
		tree := nodexTree(t, root)
		if got := commandOK(t, root, "status"); got != "index: corrupt\n" {
			t.Fatalf("status = %q", got)
		}
		if !bytes.Equal(readGen(t, root, index.SnapshotPath), before) {
			t.Fatal("status rewrote snapshot.json")
		}
		if !slices.Equal(nodexTree(t, root), tree) {
			t.Fatal("status changed .nodex")
		}
	})
}

func TestFreshnessIgnoresNonInputs(t *testing.T) {
	compact := "{\"schema\":1,\"presets\":[],\"exclude\":[\".outputs/\"]}\n"
	pretty := "{\n  \"exclude\": [\".outputs/\"],\n  \"presets\": [],\n  \"schema\": 1\n}\n"
	if mustPolicy(t, compact).Identity() != mustPolicy(t, pretty).Identity() {
		t.Fatal("equivalent documents compiled to different policies")
	}
	root := t.TempDir()
	writeProjectFile(t, root, ignore.DocumentPath, compact)
	writeProjectFile(t, root, "a.go", "// alpha\n\npackage a\n")
	writeProjectFile(t, root, "README.md", "notes\n")
	writeProjectFile(t, root, ".outputs/hidden.go", "// hidden\npackage hidden\n")
	generateOK(t, root)
	before := pairBytes(t, root)
	wantComments := "## C000001\n\nfile: `a.go`\n\nalpha\n"
	wantStatus := statusCurrent(1, 1)

	writeProjectFile(t, root, "README.md", "package {\nnot go\n")
	readme := filepath.Join(root, "README.md")
	restoreMode(t, readme, 0o644)
	if err := os.Chmod(readme, 0); err != nil {
		t.Fatalf("chmod readme: %v", err)
	}
	if got := commandOK(t, root, "comments"); got != wantComments {
		t.Fatalf("comments after unsupported change = %q", got)
	}
	if got := commandOK(t, root, "status"); got != wantStatus {
		t.Fatalf("status after unsupported change = %q", got)
	}

	writeProjectFile(t, root, ".outputs/hidden.go", "package {\n")
	if got := commandOK(t, root, "comments"); got != wantComments {
		t.Fatalf("comments after ignored change = %q", got)
	}
	if got := commandOK(t, root, "status"); got != wantStatus {
		t.Fatalf("status after ignored change = %q", got)
	}

	writeProjectFile(t, root, ignore.DocumentPath, pretty)
	if got := commandOK(t, root, "comments"); got != wantComments {
		t.Fatalf("comments after ignore reformat = %q", got)
	}
	if got := commandOK(t, root, "status"); got != wantStatus {
		t.Fatalf("status after ignore reformat = %q", got)
	}
	requireSameIndex(t, root, before)
}

func TestStatusOperationalFailures(t *testing.T) {
	t.Run("invalid ignore", func(t *testing.T) {
		root := t.TempDir()
		writeProjectFile(t, root, "a.go", "// alpha\n\npackage a\n")
		writeProjectFile(t, root, ignore.DocumentPath, "{")
		stdout, stderr, err := runAt(root, "index", "status")
		if err == nil || stdout != "" || !strings.Contains(stderr, "nodex: ") {
			t.Fatalf("status = %q %q %v", stdout, stderr, err)
		}
		if strings.Contains(stdout, "index:") || strings.Contains(stderr, "index: ") {
			t.Fatalf("invalid ignore reported a status: %q %q", stdout, stderr)
		}
		if _, statErr := os.Stat(filepath.Join(resolvedRoot(t, root), filepath.FromSlash(index.IndexDir))); !errors.Is(statErr, os.ErrNotExist) {
			t.Fatalf("index dir stat = %v", statErr)
		}
		stdout, stderr, err = runAt(root, "index", "comments")
		if err == nil || stdout != "" {
			t.Fatalf("comments = %q %q %v", stdout, stderr, err)
		}
	})
	t.Run("unreadable ignore", func(t *testing.T) {
		root := oneCommentProject(t)
		writeProjectFile(t, root, ignore.DocumentPath, "{\n  \"schema\": 1,\n  \"presets\": [],\n  \"exclude\": []\n}\n")
		// The new document already makes the index stale. An unreadable
		// document must fail before that comparison.
		path := filepath.Join(root, filepath.FromSlash(ignore.DocumentPath))
		restoreMode(t, path, 0o644)
		if err := os.Chmod(path, 0); err != nil {
			t.Fatalf("chmod: %v", err)
		}
		before := pairBytes(t, root)
		stdout, stderr, err := runAt(root, "index", "status")
		if err == nil || stdout != "" || !strings.Contains(stderr, "nodex: ") {
			t.Fatalf("status = %q %q %v", stdout, stderr, err)
		}
		requireSameIndex(t, root, before)
	})
	t.Run("unreadable source", func(t *testing.T) {
		root := oneCommentProject(t)
		path := filepath.Join(root, "a.go")
		restoreMode(t, path, 0o644)
		if err := os.Chmod(path, 0); err != nil {
			t.Fatalf("chmod: %v", err)
		}
		before := pairBytes(t, root)
		for _, command := range []string{"status", "comments"} {
			stdout, stderr, err := runAt(root, "index", command)
			if err == nil || stdout != "" || !strings.Contains(stderr, "nodex: ") {
				t.Fatalf("%s = %q %q %v", command, stdout, stderr, err)
			}
			if strings.Contains(stderr, "stale") || strings.Contains(stderr, "index: ") {
				t.Fatalf("%s stderr = %q", command, stderr)
			}
		}
		if err := os.Chmod(path, 0o644); err != nil {
			t.Fatalf("restore: %v", err)
		}
		requireSameIndex(t, root, before)
	})
	t.Run("unreadable snapshot", func(t *testing.T) {
		root := oneCommentProject(t)
		path := filepath.Join(resolvedRoot(t, root), filepath.FromSlash(index.SnapshotPath))
		restoreMode(t, path, 0o644)
		if err := os.Chmod(path, 0); err != nil {
			t.Fatalf("chmod: %v", err)
		}
		stdout, stderr, err := runAt(root, "index", "status")
		if err == nil || stdout != "" {
			t.Fatalf("status = %q %q %v", stdout, stderr, err)
		}
		if strings.Contains(stdout, "corrupt") || strings.Contains(stderr, "corrupt") {
			t.Fatalf("permission failure reported corrupt: %q %q", stdout, stderr)
		}
	})
}

func TestCommandsDoNotFollowNodexSymlink(t *testing.T) {
	for _, command := range []string{"comments", "status"} {
		t.Run(command, func(t *testing.T) {
			root := t.TempDir()
			outside := t.TempDir()
			writeProjectFile(t, root, "main.go", "package main\n")
			if err := os.Symlink(outside, filepath.Join(root, ".nodex")); err != nil {
				t.Fatalf("symlink: %v", err)
			}
			stdout, stderr, err := runAt(root, "index", command)
			if err == nil || stdout != "" || !strings.Contains(stderr, "nodex: ") {
				t.Fatalf("%s = %q %q %v", command, stdout, stderr, err)
			}
			if _, statErr := os.Stat(filepath.Join(outside, "index")); !errors.Is(statErr, os.ErrNotExist) {
				t.Fatalf("wrote through .nodex symlink: %v", statErr)
			}
		})
	}
}

func commandOK(t *testing.T, root, command string) string {
	t.Helper()
	stdout, stderr, err := runAt(root, "index", command)
	if err != nil {
		t.Fatalf("%s: %v\n%s", command, err, stderr)
	}
	if stderr != "" {
		t.Fatalf("%s stderr = %q", command, stderr)
	}
	return stdout
}

func oneCommentProject(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	writeProjectFile(t, root, "a.go", "// alpha\n\npackage a\n")
	generateOK(t, root)
	return root
}

func assertStaleComments(t *testing.T, root string) {
	t.Helper()
	stdout, stderr, err := runAt(root, "index", "comments")
	if err == nil || stdout != "" || stderr != "nodex: index is stale; run nodex index generate\n" {
		t.Fatalf("comments = %q %q %v", stdout, stderr, err)
	}
}

func assertStalePair(t *testing.T, root, wantStatus string) {
	t.Helper()
	if got := commandOK(t, root, "status"); got != wantStatus {
		t.Fatalf("status = %q, want %q", got, wantStatus)
	}
	assertStaleComments(t, root)
}

func requireSameIndex(t *testing.T, root string, before []byte) {
	t.Helper()
	if !bytes.Equal(pairBytes(t, root), before) {
		t.Fatal("index bytes changed")
	}
}

func statusCurrent(sources, comments int) string {
	return fmt.Sprintf("index: current\nsources: %d\ncomments: %d\n", sources, comments)
}

func statusStale(indexed, current, comments int) string {
	return fmt.Sprintf("index: stale\nindexed sources: %d\ncurrent sources: %d\ncomments: %d\n", indexed, current, comments)
}

func assertNoDetailedCommentMetadata(t *testing.T, got string) {
	t.Helper()
	for _, forbidden := range []string{"language:", "offsets:", "columns:", "lines:", "range:", "### Context", "source digest:"} {
		if strings.Contains(got, forbidden) {
			t.Fatalf("stdout contains %q: %q", forbidden, got)
		}
	}
}

func TestShow(t *testing.T) {
	t.Run("missing", func(t *testing.T) {
		root := t.TempDir()
		writeProjectFile(t, root, "a.go", "package a\n\n// hello\nfunc F() {}\n")
		stdout, stderr, err := runAt(root, "index", "show", "C000001")
		if err == nil || stdout != "" || stderr != "nodex: no generated index; run nodex index generate\n" {
			t.Fatalf("missing = %q %q %v", stdout, stderr, err)
		}
		if _, statErr := os.Stat(filepath.Join(resolvedRoot(t, root), ".nodex")); !errors.Is(statErr, os.ErrNotExist) {
			t.Fatalf(".nodex stat = %v", statErr)
		}
	})

	t.Run("corrupt", func(t *testing.T) {
		root := t.TempDir()
		writeProjectFile(t, root, "a.go", "package a\n\n// hello\nfunc F() {}\n")
		generateOK(t, root)
		commentsPath := filepath.Join(resolvedRoot(t, root), filepath.FromSlash(index.CommentsPath))
		broken := readFile(t, commentsPath)
		broken[len(broken)-2] ^= 0x1
		if err := os.WriteFile(commentsPath, broken, 0o644); err != nil {
			t.Fatalf("write comments: %v", err)
		}
		before := readFile(t, commentsPath)
		stdout, stderr, err := runAt(root, "index", "show", "C000001")
		if err == nil || stdout != "" || !strings.Contains(stderr, "corrupt") {
			t.Fatalf("corrupt = %q %q %v", stdout, stderr, err)
		}
		if strings.Contains(stdout, "## ") || strings.Contains(stderr, "## ") {
			t.Fatalf("corrupt produced show output: %q %q", stdout, stderr)
		}
		if !bytes.Equal(readFile(t, commentsPath), before) {
			t.Fatal("show rewrote a corrupt index")
		}
	})

	t.Run("stale", func(t *testing.T) {
		root := t.TempDir()
		writeProjectFile(t, root, "a.go", "package a\n\n// hello\nfunc F() {}\n")
		generateOK(t, root)
		before := pairBytes(t, root)
		writeProjectFile(t, root, "a.go", "package a\n\n// changed\nfunc F() {}\n")
		stdout, stderr, err := runAt(root, "index", "show", "C000001")
		if err == nil || stdout != "" || stderr != "nodex: index is stale; run nodex index generate\n" {
			t.Fatalf("stale = %q %q %v", stdout, stderr, err)
		}
		if strings.Contains(stdout, "changed") || strings.Contains(stdout, "hello") {
			t.Fatalf("stdout = %q", stdout)
		}
		requireSameIndex(t, root, before)
	})

	t.Run("malformed and unknown IDs", func(t *testing.T) {
		root := t.TempDir()
		writeProjectFile(t, root, "a.go", "package a\n\n// hello\nfunc F() {}\n")
		generateOK(t, root)
		before := pairBytes(t, root)
		for _, args := range [][]string{
			{"index", "show", "nope"},
			{"index", "show", "c000001"},
			{"index", "show", "C000000"},
			{"index", "show", "C000001", "nope"},
			{"index", "show", "nope", "C000001"},
			{"index", "show", "C000001", "C000099"},
			{"index", "show", "C000099", "C000001"},
		} {
			stdout, stderr, err := runAt(root, args...)
			if err == nil || stdout != "" {
				t.Fatalf("%v = %q %q %v", args, stdout, stderr, err)
			}
			if strings.Contains(stdout, "## ") || strings.Contains(stdout, "hello") {
				t.Fatalf("%v leaked output %q", args, stdout)
			}
		}
		requireSameIndex(t, root, before)
	})

	t.Run("one ID", func(t *testing.T) {
		root := t.TempDir()
		src := "package example\n\n// Example adds nothing.\nfunc Example() {\n}\n"
		writeProjectFile(t, root, "internal/example.go", src)
		generateOK(t, root)
		before := pairBytes(t, root)
		got := showOK(t, root, "C000001")
		want := "## C000001\n\nfile: `internal/example.go`\nlines: 3\n\n### Context\n\n```go\n// Example adds nothing.\nfunc Example() {\n}\n```\n"
		if got != want {
			t.Fatalf("stdout = %q\nwant %q", got, want)
		}
		if strings.Contains(got, "offset") || strings.Contains(got, "column") || strings.Contains(got, "sha256") || strings.Contains(got, "FuncDecl") {
			t.Fatalf("stdout includes hidden metadata: %q", got)
		}
		requireSameIndex(t, root, before)
	})

	t.Run("order and repetition", func(t *testing.T) {
		root := t.TempDir()
		writeProjectFile(t, root, "b.go", "package b\n\n// beta\nfunc B() {}\n")
		writeProjectFile(t, root, "a.go", "package a\n\n// alpha\nfunc A() {}\n")
		generateOK(t, root)
		idx, _ := loadIndex(t, root)
		if len(idx.Entries()) != 2 || idx.Entries()[0].Path != "a.go" || idx.Entries()[1].Path != "b.go" {
			t.Fatalf("entries = %+v", idx.Entries())
		}
		got := showOK(t, root, "C000002", "C000001")
		first := strings.Index(got, "## C000002")
		second := strings.Index(got, "## C000001")
		if first < 0 || second < first {
			t.Fatalf("order = %q", got)
		}
		if !strings.Contains(got, "file: `b.go`") || !strings.Contains(got, "file: `a.go`") {
			t.Fatalf("files = %q", got)
		}
		if strings.Index(got, "file: `b.go`") > strings.Index(got, "file: `a.go`") {
			t.Fatalf("files follow index order: %q", got)
		}
		beta := strings.Index(got, "// beta\n")
		alpha := strings.Index(got, "// alpha\n")
		if beta < 0 || alpha < beta {
			t.Fatalf("comment order = %q", got)
		}
		if strings.Contains(got, "### Comment") {
			t.Fatalf("show repeated the comment section: %q", got)
		}
		repeated := showOK(t, root, "C000001", "C000001", "C000001")
		if strings.Count(repeated, "## C000001\n") != 3 {
			t.Fatalf("repetition = %q", repeated)
		}
		if strings.HasSuffix(repeated, "\n\n") {
			t.Fatalf("trailing blank line: %q", repeated)
		}
	})

	t.Run("same file and two files", func(t *testing.T) {
		root := t.TempDir()
		writeProjectFile(t, root, "a.go", "package a\n\n// alpha\nfunc A() {}\n\n// beta\nfunc B() {}\n")
		generateOK(t, root)
		var reads []string
		showReadFile = func(opened *project.Project, logical string) ([]byte, error) {
			reads = append(reads, logical)
			return opened.ReadFile(logical)
		}
		t.Cleanup(func() { showReadFile = nil })
		got := showOK(t, root, "C000002", "C000001")
		if strings.Count(got, "file: `a.go`") != 2 || !strings.Contains(got, "beta") || !strings.Contains(got, "alpha") {
			t.Fatalf("same file = %q", got)
		}
		if len(reads) != 1 || reads[0] != "a.go" {
			t.Fatalf("reads = %v", reads)
		}

		root = t.TempDir()
		writeProjectFile(t, root, "a.go", "package a\n\n// alpha\nfunc A() {}\n")
		writeProjectFile(t, root, "b.go", "package b\n\n// beta\nfunc B() {}\n")
		generateOK(t, root)
		reads = nil
		got = showOK(t, root, "C000001", "C000002")
		if !strings.Contains(got, "file: `a.go`") || !strings.Contains(got, "file: `b.go`") {
			t.Fatalf("files = %q", got)
		}
		if len(reads) != 2 || reads[0] != "a.go" || reads[1] != "b.go" {
			t.Fatalf("reads = %v", reads)
		}
	})

	t.Run("comment shapes", func(t *testing.T) {
		root := t.TempDir()
		writeProjectFile(t, root, "a.go", "package a\n\n// alpha\n// beta\nfunc F() {}\n")
		generateOK(t, root)
		got := showOK(t, root, "C000001")
		if strings.Contains(got, "### Comment") || strings.Contains(got, "alpha\nbeta") {
			t.Fatalf("multiline repeated normalized text: %q", got)
		}
		body := contextBody(t, got)
		if !strings.Contains(body, "// alpha\n// beta\n") || !strings.Contains(body, "func F()") {
			t.Fatalf("multiline = %q", got)
		}

		root = t.TempDir()
		writeProjectFile(t, root, "a.go", "package a\n\n//\nfunc F() {}\n")
		generateOK(t, root)
		idx, _ := loadIndex(t, root)
		if idx.Entries()[0].Text != "" {
			t.Fatalf("stored text = %q", idx.Entries()[0].Text)
		}
		got = showOK(t, root, "C000001")
		if strings.Contains(got, "### Comment") {
			t.Fatalf("empty comment was repeated: %q", got)
		}
		if !strings.Contains(contextBody(t, got), "//\n") {
			t.Fatalf("empty = %q", got)
		}

		root = t.TempDir()
		writeProjectFile(t, root, "a.go", "package a\n\n// # Title\n// **bold** and `code`\nfunc F() {}\n")
		generateOK(t, root)
		idx, _ = loadIndex(t, root)
		text := idx.Entries()[0].Text
		got = showOK(t, root, "C000001")
		if strings.Contains(got, "### Comment") || strings.Contains(got, text) {
			t.Fatalf("markdown comment was repeated outside the source: %q", got)
		}
		body = contextBody(t, got)
		if !strings.Contains(body, "// # Title") || !strings.Contains(body, "// **bold** and `code`") {
			t.Fatalf("markdown comment = %q", got)
		}
	})

	t.Run("backtick fence", func(t *testing.T) {
		root := t.TempDir()
		src := "package p\n\nfunc F() {\n\t/*\n```\n*/\n\n\t// note\n\tx := 1\n\t_ = x\n}\n"
		writeProjectFile(t, root, "a.go", src)
		generateOK(t, root)
		idx, _ := loadIndex(t, root)
		var id string
		for _, entry := range idx.Entries() {
			if entry.Text == "note" {
				id = entry.ID.String()
			}
		}
		if id == "" {
			t.Fatalf("entries = %+v", idx.Entries())
		}
		got := showOK(t, root, id)
		if !strings.Contains(got, "````go\n") {
			t.Fatalf("fence = %q", got)
		}
		body := contextBody(t, got)
		if !strings.Contains(body, "```") || !strings.Contains(body, "// note") || !strings.Contains(body, "func F()") {
			t.Fatalf("body = %q", body)
		}
	})

	t.Run("bounded function and locations", func(t *testing.T) {
		root := t.TempDir()
		var b strings.Builder
		b.WriteString("package p\n\nfunc F() {\n")
		for i := 0; i < 80; i++ {
			if i == 40 {
				b.WriteString("\t// TARGET\n")
				continue
			}
			fmt.Fprintf(&b, "\t_ = %d\n", i)
		}
		b.WriteString("}\n")
		writeProjectFile(t, root, "a.go", b.String())
		generateOK(t, root)
		got := showOK(t, root, "C000001")
		body := contextBody(t, got)
		if strings.Contains(body, "func F()") || strings.Contains(body, "_ = 79") || snippetLineCount(body) > 40 {
			t.Fatalf("unbounded context:\n%s", body)
		}
		if !strings.Contains(body, "// TARGET") || strings.Contains(got, "### Comment") || strings.Contains(got, "\nTARGET\n") {
			t.Fatalf("show = %q", got)
		}

		root = t.TempDir()
		writeProjectFile(t, root, "a.go", "package p\n\nfunc F() {\n\t// inside\n\tx := 1\n\t_ = x\n}\n")
		generateOK(t, root)
		got = showOK(t, root, "C000001")
		body = contextBody(t, got)
		if !strings.Contains(body, "func F()") || !strings.Contains(body, "// inside") || strings.Contains(body, "package p") {
			t.Fatalf("inside = %q", body)
		}
		if !strings.Contains(got, "lines: ") {
			t.Fatalf("lines missing: %q", got)
		}

		root = t.TempDir()
		writeProjectFile(t, root, "a.go", "package p\n\n// F documents it.\nfunc F() {}\n\nfunc Other() {}\n")
		generateOK(t, root)
		got = showOK(t, root, "C000001")
		body = contextBody(t, got)
		if !strings.Contains(body, "// F documents it.") || !strings.Contains(body, "func F()") || strings.Contains(body, "func Other") {
			t.Fatalf("godoc = %q", body)
		}

		root = t.TempDir()
		writeProjectFile(t, root, "a.go", "// Package p documents p.\npackage p\n\nfunc Later() {}\n")
		generateOK(t, root)
		got = showOK(t, root, "C000001")
		body = contextBody(t, got)
		if !strings.Contains(body, "package p") || !strings.Contains(body, "Package p documents p.") || strings.Contains(body, "Later") {
			t.Fatalf("package = %q", body)
		}
		if strings.Contains(got, "lines: 1-") {
			t.Fatalf("package comment lines span the file: %q", got)
		}
	})

	t.Run("large normalized comment", func(t *testing.T) {
		root := t.TempDir()
		var b strings.Builder
		b.WriteString("package p\n\n")
		for i := 0; i < 100; i++ {
			fmt.Fprintf(&b, "// line %d\n", i)
		}
		b.WriteString("func F() {}\n")
		writeProjectFile(t, root, "a.go", b.String())
		generateOK(t, root)
		idx, _ := loadIndex(t, root)
		if len(idx.Entries()) != 1 || !strings.Contains(idx.Entries()[0].Text, "line 99") {
			t.Fatalf("stored = %q", idx.Entries()[0].Text)
		}
		got := showOK(t, root, "C000001")
		if strings.Contains(got, "### Comment") || strings.Contains(got, idx.Entries()[0].Text) {
			t.Fatal("show repeated the normalized comment outside the source context")
		}
		body := contextBody(t, got)
		if snippetLineCount(body) > 40 {
			t.Fatalf("context lines = %d\n%s", snippetLineCount(body), body)
		}
		if !strings.Contains(body, "// line 0\n") {
			t.Fatal("context dropped the source comment")
		}
		if strings.Contains(body, "line 99") {
			t.Fatal("context kept the whole oversized comment group")
		}
	})

	t.Run("source changed after currentness", func(t *testing.T) {
		root := t.TempDir()
		writeProjectFile(t, root, "a.go", "package p\n\n// original\nfunc F() {}\n")
		generateOK(t, root)
		before := pairBytes(t, root)
		showReadFile = func(opened *project.Project, logical string) ([]byte, error) {
			if logical == "a.go" {
				return []byte("package p\n\n// CHANGED_SOURCE\nfunc F() {}\n"), nil
			}
			return opened.ReadFile(logical)
		}
		t.Cleanup(func() { showReadFile = nil })
		stdout, stderr, err := runAt(root, "index", "show", "C000001")
		if err == nil || stdout != "" || stderr != "nodex: index is stale; run nodex index generate\n" {
			t.Fatalf("race = %q %q %v", stdout, stderr, err)
		}
		if strings.Contains(stdout, "CHANGED_SOURCE") || strings.Contains(stdout, "original") {
			t.Fatalf("stdout = %q", stdout)
		}
		requireSameIndex(t, root, before)
	})
}

func showOK(t *testing.T, root string, ids ...string) string {
	t.Helper()
	args := append([]string{"index", "show"}, ids...)
	stdout, stderr, err := runAt(root, args...)
	if err != nil || stderr != "" {
		t.Fatalf("show %v: %v\nstdout %q\nstderr %q", ids, err, stdout, stderr)
	}
	return stdout
}

func contextBody(t *testing.T, section string) string {
	t.Helper()
	const marker = "### Context\n\n"
	i := strings.Index(section, marker)
	if i < 0 {
		t.Fatalf("no context section: %q", section)
	}
	rest := section[i+len(marker):]
	nl := strings.IndexByte(rest, '\n')
	if nl < 0 {
		t.Fatalf("context fence has no body: %q", section)
	}
	open := rest[:nl]
	n := 0
	for n < len(open) && open[n] == '`' {
		n++
	}
	if n < 3 {
		t.Fatalf("opening fence = %q", open)
	}
	closer := strings.Repeat("`", n)
	rest = rest[nl+1:]
	end := strings.LastIndex(rest, "\n"+closer+"\n")
	if end < 0 {
		if strings.HasSuffix(rest, closer+"\n") && !strings.Contains(rest[:len(rest)-len(closer)-1], closer) {
			return strings.TrimSuffix(rest, closer+"\n")
		}
		t.Fatalf("unclosed fence in %q", section)
	}
	return rest[:end+1]
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

func TestVersion(t *testing.T) {
	root := t.TempDir()
	if version != "0.1.0-beta.1" || commit != "unknown" || buildDate != "unknown" {
		t.Fatalf("defaults = %q %q %q", version, commit, buildDate)
	}
	if got := runOK(t, root, "version"); got != defaultVersionOutput {
		t.Fatalf("stdout = %q", got)
	}
	if _, err := os.Stat(filepath.Join(root, ".nodex")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf(".nodex stat = %v", err)
	}

	project := committedProject(t)
	before := pairBytes(t, project)
	if got := runOK(t, project, "version"); got != defaultVersionOutput {
		t.Fatalf("project stdout = %q", got)
	}
	if !bytes.Equal(pairBytes(t, project), before) {
		t.Fatal("version changed the index")
	}

	noProject := func() (string, error) {
		return "", errors.New("no working directory")
	}
	var out, errOut bytes.Buffer
	err := run([]string{"version"}, noProject, &out, &errOut)
	if err != nil || out.String() != defaultVersionOutput || errOut.Len() != 0 {
		t.Fatalf("version without a project = %q %q %v", out.String(), errOut.String(), err)
	}
	missing := filepath.Join(root, "missing")
	for _, args := range [][]string{
		{"--root", missing, "version"},
		{"--root=" + missing, "version"},
	} {
		out.Reset()
		errOut.Reset()
		err = run(args, noProject, &out, &errOut)
		if err != nil || out.String() != defaultVersionOutput || errOut.Len() != 0 {
			t.Fatalf("%q = %q %q %v", args, out.String(), errOut.String(), err)
		}
	}
	if _, err := os.Stat(missing); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("missing root stat = %v", err)
	}

	stdout, stderr, err := runAt(root, "version", "1.2.3")
	if err == nil || stdout != "" || stderr != "version takes no arguments\n" {
		t.Fatalf("extra = %q %q %v", stdout, stderr, err)
	}
	stdout, stderr, err = runAt(root, "version", "--root", root)
	if err == nil || stdout != "" || stderr != "version takes no arguments\n" {
		t.Fatalf("trailing --root = %q %q %v", stdout, stderr, err)
	}

	out.Reset()
	errOut.Reset()
	err = run([]string{"index", "generate"}, noProject, &out, &errOut)
	if err == nil || out.Len() != 0 || !strings.Contains(errOut.String(), "no working directory") {
		t.Fatalf("generate dispatch = %q %q %v", out.String(), errOut.String(), err)
	}
	stdout, stderr, err = runAt(root, "nope")
	if err == nil || stdout != "" || stderr != "unknown command \"nope\"\n" {
		t.Fatalf("unknown dispatch = %q %q %v", stdout, stderr, err)
	}

	savedCommit, savedDate := commit, buildDate
	t.Cleanup(func() {
		commit, buildDate = savedCommit, savedDate
	})
	cases := []struct {
		commit, buildDate, want string
	}{
		{"0123456789abcdef", "unknown", "nodex 0.1.0-beta.1\ncommit 0123456789abcdef\nbuilt unknown\n"},
		{"unknown", "2026-10-01T08:00:00Z", "nodex 0.1.0-beta.1\ncommit unknown\nbuilt 2026-10-01T08:00:00Z\n"},
		{"0123456789abcdef", "2026-10-01T08:00:00Z", "nodex 0.1.0-beta.1\ncommit 0123456789abcdef\nbuilt 2026-10-01T08:00:00Z\n"},
	}
	for _, tc := range cases {
		commit, buildDate = tc.commit, tc.buildDate
		if version != "0.1.0-beta.1" {
			t.Fatalf("version changed to %q", version)
		}
		if versionText() != tc.want {
			t.Fatalf("helper %q %q = %q", tc.commit, tc.buildDate, versionText())
		}
		if got := runOK(t, root, "version"); got != tc.want {
			t.Fatalf("overridden %q %q = %q", tc.commit, tc.buildDate, got)
		}
	}
	commit, buildDate = savedCommit, savedDate
	if version != "0.1.0-beta.1" || commit != "unknown" || buildDate != "unknown" {
		t.Fatalf("restored values = %q %q %q", version, commit, buildDate)
	}
	if versionText() != defaultVersionOutput {
		t.Fatalf("restored helper = %q", versionText())
	}
	if got := runOK(t, root, "version"); got != defaultVersionOutput {
		t.Fatalf("restored stdout = %q", got)
	}
}

func TestIgnoreCommands(t *testing.T) {
	t.Run("presets", func(t *testing.T) {
		root := t.TempDir()
		want := "go:all\ngo:generated\ngo:tests\ngo:vendor\n"
		if got := runOK(t, root, "ignore", "presets"); got != want {
			t.Fatalf("stdout = %q", got)
		}
		if _, err := os.Stat(filepath.Join(root, ".nodex")); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf(".nodex stat = %v", err)
		}
		var out, errOut bytes.Buffer
		err := run([]string{"ignore", "presets"}, func() (string, error) {
			return "", errors.New("no working directory")
		}, &out, &errOut)
		if err != nil || out.String() != want || errOut.Len() != 0 {
			t.Fatalf("presets without a project = %q %q %v", out.String(), errOut.String(), err)
		}
		stdout, stderr, err := runAt(root, "ignore", "presets", "go:tests")
		if err == nil || stdout != "" || stderr != "ignore presets takes no arguments\n" {
			t.Fatalf("extra = %q %q %v", stdout, stderr, err)
		}
	})
	t.Run("list missing", func(t *testing.T) {
		root := t.TempDir()
		if got := runOK(t, root, "ignore", "list"); got != "{\n  \"schema\": 1,\n  \"presets\": [],\n  \"exclude\": []\n}\n" {
			t.Fatalf("stdout = %q", got)
		}
		if _, err := os.Stat(filepath.Join(root, ".nodex")); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf(".nodex stat = %v", err)
		}
	})
	t.Run("enable and disable", func(t *testing.T) {
		root := t.TempDir()
		writeProjectFile(t, root, ignore.DocumentPath, "{\n  \"schema\": 1,\n  \"presets\": [],\n  \"exclude\": [\n    \"zzz/\",\n    \"aaa/\",\n    \"zzz/\"\n  ]\n}\n")
		excludes := []string{"zzz/", "aaa/", "zzz/"}
		runOK(t, root, "ignore", "enable", "go:tests")
		assertIgnore(t, root, []string{"go:tests"}, excludes)
		before := readGen(t, root, ignore.DocumentPath)
		runOK(t, root, "ignore", "enable", "go:tests")
		if !bytes.Equal(readGen(t, root, ignore.DocumentPath), before) {
			t.Fatal("idempotent enable rewrote ignore.json")
		}
		runOK(t, root, "ignore", "enable", "go:all")
		assertIgnore(t, root, syntax.ConcretePresets(), excludes)
		if bytes.Contains(readGen(t, root, ignore.DocumentPath), []byte("go:all")) {
			t.Fatal("go:all was persisted")
		}
		if tree := nodexTree(t, root); !slices.Equal(tree, []string{"ignore.json"}) {
			t.Fatalf(".nodex = %q", tree)
		}
		runOK(t, root, "ignore", "disable", "go:tests")
		var kept []string
		for _, id := range syntax.ConcretePresets() {
			if id != "go:tests" {
				kept = append(kept, id)
			}
		}
		assertIgnore(t, root, kept, excludes)
		runOK(t, root, "ignore", "disable", "go:all")
		assertIgnore(t, root, nil, excludes)
		before = readGen(t, root, ignore.DocumentPath)
		runOK(t, root, "ignore", "disable", "go:vendor")
		if !bytes.Equal(readGen(t, root, ignore.DocumentPath), before) {
			t.Fatal("disabling an absent preset rewrote ignore.json")
		}
		listed := runOK(t, root, "ignore", "list")
		if listed != string(before) {
			t.Fatalf("list = %q, file %q", listed, before)
		}
	})
	t.Run("enable creates a missing document", func(t *testing.T) {
		root := t.TempDir()
		runOK(t, root, "ignore", "enable", "go:vendor")
		assertIgnore(t, root, []string{"go:vendor"}, nil)
	})
	t.Run("disable missing creates nothing", func(t *testing.T) {
		root := t.TempDir()
		if got := runOK(t, root, "ignore", "disable", "go:tests"); got != "" {
			t.Fatalf("stdout = %q", got)
		}
		if _, err := os.Stat(filepath.Join(root, ".nodex")); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf(".nodex stat = %v", err)
		}
		stdout, stderr, err := runAt(root, "ignore", "disable", "nope")
		if err == nil || stdout != "" || !strings.Contains(stderr, "unsupported preset") {
			t.Fatalf("unknown = %q %q %v", stdout, stderr, err)
		}
		if _, err := os.Stat(filepath.Join(root, ".nodex")); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("unknown disable created .nodex: %v", err)
		}
	})
	t.Run("unknown selector does not mutate", func(t *testing.T) {
		root := t.TempDir()
		writeProjectFile(t, root, ignore.DocumentPath, "{\n  \"schema\": 1,\n  \"presets\": [\n    \"go:tests\"\n  ],\n  \"exclude\": [\n    \"zzz/\"\n  ]\n}\n")
		before := readGen(t, root, ignore.DocumentPath)
		for _, args := range [][]string{
			{"ignore", "enable", "go:all", "go:tests"},
			{"ignore", "enable", "nope"},
			{"ignore", "disable"},
			{"ignore", "nope"},
		} {
			stdout, stderr, err := runAt(root, args...)
			if err == nil || stdout != "" {
				t.Fatalf("%v = %q %q %v", args, stdout, stderr, err)
			}
			if !bytes.Equal(readGen(t, root, ignore.DocumentPath), before) {
				t.Fatalf("%v mutated ignore.json", args)
			}
		}
	})
	t.Run("rejects aggregate and unknown persisted ids", func(t *testing.T) {
		for _, presets := range []string{`["go:all"]`, `["nope"]`} {
			root := t.TempDir()
			document := "{\n  \"schema\": 1,\n  \"presets\": " + presets + ",\n  \"exclude\": []\n}\n"
			writeProjectFile(t, root, ignore.DocumentPath, document)
			before := readGen(t, root, ignore.DocumentPath)
			for _, args := range [][]string{{"ignore", "list"}, {"ignore", "enable", "go:tests"}, {"index", "generate"}, {"index", "status"}} {
				stdout, stderr, err := runAt(root, args...)
				if err == nil || stdout != "" || !strings.Contains(stderr, "unsupported preset") {
					t.Fatalf("%v presets %s = %q %q %v", args, presets, stdout, stderr, err)
				}
			}
			if !bytes.Equal(readGen(t, root, ignore.DocumentPath), before) {
				t.Fatal("rejected preset mutated ignore.json")
			}
		}
	})
	t.Run("symlinks are not followed", func(t *testing.T) {
		root := t.TempDir()
		outside := t.TempDir()
		if err := os.WriteFile(filepath.Join(outside, "secret"), []byte("keep"), 0o644); err != nil {
			t.Fatalf("write: %v", err)
		}
		if err := os.Symlink(outside, filepath.Join(root, ".nodex")); err != nil {
			t.Fatalf("symlink: %v", err)
		}
		stdout, stderr, err := runAt(root, "ignore", "enable", "go:tests")
		if err == nil || stdout != "" || !strings.Contains(stderr, "symbolic link") {
			t.Fatalf("nodex symlink = %q %q %v", stdout, stderr, err)
		}
		if _, statErr := os.Stat(filepath.Join(outside, "ignore.json")); !errors.Is(statErr, os.ErrNotExist) {
			t.Fatalf("wrote through .nodex symlink: %v", statErr)
		}

		root = t.TempDir()
		if err := os.Mkdir(filepath.Join(root, ".nodex"), 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		target := filepath.Join(outside, "ignore.json")
		if err := os.WriteFile(target, []byte("original"), 0o644); err != nil {
			t.Fatalf("write: %v", err)
		}
		if err := os.Symlink(target, filepath.Join(root, ".nodex", "ignore.json")); err != nil {
			t.Fatalf("symlink: %v", err)
		}
		stdout, stderr, err = runAt(root, "ignore", "enable", "go:tests")
		if err == nil || stdout != "" || !strings.Contains(stderr, "symbolic link") {
			t.Fatalf("ignore symlink = %q %q %v", stdout, stderr, err)
		}
		if string(readFile(t, target)) != "original" {
			t.Fatal("enable followed ignore.json")
		}
	})
	t.Run("failed write leaves the document unpublished", func(t *testing.T) {
		root := t.TempDir()
		writeProjectFile(t, root, ignore.DocumentPath, "{\n  \"schema\": 1,\n  \"presets\": [],\n  \"exclude\": [\n    \"zzz/\"\n  ]\n}\n")
		before := readGen(t, root, ignore.DocumentPath)
		dir := filepath.Join(root, ".nodex")
		restoreMode(t, dir, 0o755)
		if err := os.Chmod(dir, 0o555); err != nil {
			t.Fatalf("chmod: %v", err)
		}
		stdout, stderr, err := runAt(root, "ignore", "enable", "go:tests")
		if err == nil || stdout != "" {
			t.Fatalf("read-only enable = %q %q %v", stdout, stderr, err)
		}
		if err := os.Chmod(dir, 0o755); err != nil {
			t.Fatalf("restore: %v", err)
		}
		if !bytes.Equal(readGen(t, root, ignore.DocumentPath), before) {
			t.Fatal("failed enable published a document")
		}
		entries, err := os.ReadDir(dir)
		if err != nil {
			t.Fatalf("read dir: %v", err)
		}
		for _, entry := range entries {
			if strings.HasPrefix(entry.Name(), ".") {
				t.Fatalf("temporary file remained: %s", entry.Name())
			}
		}
	})
}

func TestPresetSourceSelection(t *testing.T) {
	const (
		generatedSrc = "// Code generated by nodex-test. DO NOT EDIT.\n\npackage p\n"
		notedSrc     = "package p\n\n// generated\n// DO NOT EDIT\nvar s = \"Code generated by x. DO NOT EDIT.\"\n"
	)
	newProject := func(t *testing.T) string {
		t.Helper()
		root := t.TempDir()
		files := map[string]string{
			"keep.go":           "// keep\n\npackage p\n",
			"a_test.go":         "// test comment\n\npackage p\n",
			"dir/b_test.go":     "// nested test\n\npackage p\n",
			"dir/keep.go":       "// dir keep\n\npackage p\n",
			"test.go":           "// not a test file\n\npackage p\n",
			"a_TEST.go":         "// case\n\npackage p\n",
			"vendor/a.go":       "// vendored\n\npackage vendor\n",
			"pkg/vendor/a.go":   "// nested vendor\n\npackage vendor\n",
			"vendorized/a.go":   "// vendorized\n\npackage p\n",
			"pkg/myvendor/a.go": "// myvendor\n\npackage p\n",
			"generated.go":      generatedSrc,
			"noted.go":          notedSrc,
		}
		for path, body := range files {
			writeProjectFile(t, root, path, body)
		}
		return root
	}
	all := []string{
		"a_TEST.go", "a_test.go", "dir/b_test.go", "dir/keep.go", "generated.go", "keep.go",
		"noted.go", "pkg/myvendor/a.go", "pkg/vendor/a.go", "test.go", "vendor/a.go", "vendorized/a.go",
	}
	without := func(drop ...string) []string {
		skip := map[string]bool{}
		for _, path := range drop {
			skip[path] = true
		}
		var out []string
		for _, path := range all {
			if !skip[path] {
				out = append(out, path)
			}
		}
		return out
	}
	t.Run("empty presets include every class", func(t *testing.T) {
		root := newProject(t)
		writeIgnore(t, root, nil, nil)
		generateOK(t, root)
		_, snap := loadIndex(t, root)
		if !slices.Equal(sourcePaths(snap), all) {
			t.Fatalf("sources = %q", sourcePaths(snap))
		}
	})
	t.Run("go:tests", func(t *testing.T) {
		root := newProject(t)
		runOK(t, root, "ignore", "enable", "go:tests")
		generateOK(t, root)
		_, snap := loadIndex(t, root)
		if !slices.Equal(sourcePaths(snap), without("a_test.go", "dir/b_test.go")) {
			t.Fatalf("sources = %q", sourcePaths(snap))
		}
		runOK(t, root, "ignore", "disable", "go:tests")
		generateOK(t, root)
		_, snap = loadIndex(t, root)
		if !slices.Equal(sourcePaths(snap), all) {
			t.Fatalf("sources after disable = %q", sourcePaths(snap))
		}
	})
	t.Run("go:vendor", func(t *testing.T) {
		root := newProject(t)
		runOK(t, root, "ignore", "enable", "go:vendor")
		generateOK(t, root)
		_, snap := loadIndex(t, root)
		if !slices.Equal(sourcePaths(snap), without("pkg/vendor/a.go", "vendor/a.go")) {
			t.Fatalf("sources = %q", sourcePaths(snap))
		}
	})
	t.Run("go:generated", func(t *testing.T) {
		root := newProject(t)
		runOK(t, root, "ignore", "enable", "go:generated")
		generateOK(t, root)
		_, snap := loadIndex(t, root)
		if !slices.Equal(sourcePaths(snap), without("generated.go")) {
			t.Fatalf("sources = %q", sourcePaths(snap))
		}
		idx, _ := loadIndex(t, root)
		for _, entry := range idx.Entries() {
			if entry.Path == "generated.go" || strings.Contains(entry.Text, "Code generated by nodex-test") {
				t.Fatalf("generated comment indexed: %+v", entry)
			}
		}
	})
	t.Run("all three", func(t *testing.T) {
		root := newProject(t)
		runOK(t, root, "ignore", "enable", "go:all")
		generateOK(t, root)
		_, snap := loadIndex(t, root)
		want := without("a_test.go", "dir/b_test.go", "generated.go", "pkg/vendor/a.go", "vendor/a.go")
		if !slices.Equal(sourcePaths(snap), want) {
			t.Fatalf("sources = %q", sourcePaths(snap))
		}
	})
	t.Run("manual exclude composes with presets", func(t *testing.T) {
		root := newProject(t)
		writeIgnore(t, root, []string{"go:tests"}, []string{"dir/"})
		generateOK(t, root)
		_, snap := loadIndex(t, root)
		want := without("a_test.go", "dir/b_test.go", "dir/keep.go")
		if !slices.Equal(sourcePaths(snap), want) {
			t.Fatalf("sources = %q", sourcePaths(snap))
		}
	})
	t.Run("currentness follows the preset set", func(t *testing.T) {
		root := newProject(t)
		generateOK(t, root)
		before := pairBytes(t, root)
		runOK(t, root, "ignore", "enable", "go:tests")
		requireSameIndex(t, root, before)
		if got := commandOK(t, root, "status"); got != statusStale(len(all), len(all)-2, len(all)) {
			t.Fatalf("status = %q", got)
		}
		assertStaleComments(t, root)
		stdout, stderr, err := runAt(root, "index", "show", "C000001")
		if err == nil || stdout != "" || stderr != "nodex: index is stale; run nodex index generate\n" {
			t.Fatalf("show = %q %q %v", stdout, stderr, err)
		}
		generateOK(t, root)
		if got := commandOK(t, root, "status"); got != statusCurrent(len(all)-2, len(all)-2) {
			t.Fatalf("status = %q", got)
		}
		_, snap := loadIndex(t, root)
		if slices.Contains(sourcePaths(snap), "a_test.go") {
			t.Fatal("test file remained indexed")
		}
		kept := pairBytes(t, root)
		writeProjectFile(t, root, "a_test.go", "// changed test\n\npackage p\n")
		writeProjectFile(t, root, "dir/b_test.go", "// changed nested test\n\npackage p\n")
		if got := commandOK(t, root, "status"); got != statusCurrent(len(all)-2, len(all)-2) {
			t.Fatalf("excluded edit status = %q", got)
		}
		if !bytes.Equal(pairBytes(t, root), kept) {
			t.Fatal("excluded edit changed the index")
		}
		writeProjectFile(t, root, "keep.go", "// changed\n\npackage p\n")
		if got := commandOK(t, root, "status"); got != statusStale(len(all)-2, len(all)-2, len(all)-2) {
			t.Fatalf("included edit status = %q", got)
		}
	})
	t.Run("preset identity stales even when the source set does not", func(t *testing.T) {
		root := t.TempDir()
		writeProjectFile(t, root, "keep.go", "// keep\n\npackage p\n")
		generateOK(t, root)
		before := pairBytes(t, root)
		runOK(t, root, "ignore", "enable", "go:vendor")
		requireSameIndex(t, root, before)
		if got := commandOK(t, root, "status"); got != statusStale(1, 1, 1) {
			t.Fatalf("enable status = %q", got)
		}
		generateOK(t, root)
		before = pairBytes(t, root)
		runOK(t, root, "ignore", "disable", "go:vendor")
		requireSameIndex(t, root, before)
		if got := commandOK(t, root, "status"); got != statusStale(1, 1, 1) {
			t.Fatalf("disable status = %q", got)
		}
	})
	t.Run("preset order and formatting do not stale", func(t *testing.T) {
		root := t.TempDir()
		writeProjectFile(t, root, "keep.go", "// keep\n\npackage p\n")
		writeIgnore(t, root, []string{"go:tests", "go:vendor"}, []string{"zzz/"})
		generateOK(t, root)
		before := pairBytes(t, root)
		writeProjectFile(t, root, ignore.DocumentPath, "{\n  \"exclude\": [\"zzz/\"],\n  \"presets\": [\"go:vendor\", \"go:tests\"],\n  \"schema\": 1\n}\n")
		if got := commandOK(t, root, "comments"); got != "## C000001\n\nfile: `keep.go`\n\nkeep\n" {
			t.Fatalf("comments = %q", got)
		}
		if got := commandOK(t, root, "status"); got != statusCurrent(1, 1) {
			t.Fatalf("status = %q", got)
		}
		requireSameIndex(t, root, before)
	})
	t.Run("broken excluded test file is not parsed", func(t *testing.T) {
		root := t.TempDir()
		writeProjectFile(t, root, "keep.go", "// keep\n\npackage p\n")
		writeProjectFile(t, root, "a_test.go", "package {\n")
		runOK(t, root, "ignore", "enable", "go:tests")
		if got := generateOK(t, root); got != "generated 1 comment and 1 declaration from 1 source file\n" {
			t.Fatalf("stdout = %q", got)
		}
	})
	t.Run("unreadable vendor file is not an input", func(t *testing.T) {
		root := t.TempDir()
		writeProjectFile(t, root, "keep.go", "package p\n")
		writeProjectFile(t, root, "vendor/a.go", "package vendor\n")
		path := filepath.Join(root, "vendor", "a.go")
		restoreMode(t, path, 0o644)
		if err := os.Chmod(path, 0); err != nil {
			t.Fatalf("chmod: %v", err)
		}
		runOK(t, root, "ignore", "enable", "go:vendor")
		generateOK(t, root)
		_, snap := loadIndex(t, root)
		if pathsOf(snap) != "keep.go" {
			t.Fatalf("sources = %s", pathsOf(snap))
		}
	})
}

func TestRootFlagAndNestedResolution(t *testing.T) {
	parent := t.TempDir()
	if err := os.Mkdir(filepath.Join(parent, ".git"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	writeProjectFile(t, parent, "main.go", "// root\n\npackage main\n")
	nested := filepath.Join(parent, "nested", "pkg")
	writeProjectFile(t, nested, "leaf.go", "// leaf\n\npackage leaf\n")
	if got := runOK(t, nested, "generate"); got != "generated 2 comments and 2 declarations from 2 source files\n" {
		t.Fatalf("generate = %q", got)
	}
	if _, err := os.Stat(filepath.Join(nested, ".nodex")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("nested directory gained .nodex")
	}
	_, snap := loadIndex(t, parent)
	if pathsOf(snap) != "main.go,nested/pkg/leaf.go" {
		t.Fatalf("sources = %s", pathsOf(snap))
	}
	if got := runOK(t, nested, "status"); got != statusCurrent(2, 2) {
		t.Fatalf("status = %q", got)
	}
	if got := runOK(t, nested, "comments"); got != "## C000001\n\nfile: `main.go`\n\nroot\n\n## C000002\n\nfile: `nested/pkg/leaf.go`\n\nleaf\n" {
		t.Fatalf("comments = %q", got)
	}
	shown := runOK(t, nested, "show", "C000002")
	if !strings.Contains(shown, "file: `nested/pkg/leaf.go`") || !strings.Contains(shown, "leaf") {
		t.Fatalf("show = %q", shown)
	}

	other := t.TempDir()
	writeProjectFile(t, other, "other.go", "// other\n\npackage other\n")
	before := pairBytes(t, parent)
	if got := runOK(t, nested, "--root", other, "--out-dir", other, "generate"); got != "generated 1 comment and 1 declaration from 1 source file\n" {
		t.Fatalf("explicit generate = %q", got)
	}
	requireSameIndex(t, parent, before)
	_, snap = loadIndex(t, other)
	if pathsOf(snap) != "other.go" {
		t.Fatalf("explicit sources = %s", pathsOf(snap))
	}
	if got := runOK(t, parent, "--root", other, "--out-dir", other, "comments"); got != "## C000001\n\nfile: `other.go`\n\nother\n" {
		t.Fatalf("explicit comments = %q", got)
	}
	if got := runOK(t, parent, "--root", other, "--out-dir", other, "status"); got != statusCurrent(1, 1) {
		t.Fatalf("explicit status = %q", got)
	}
	if got := runOK(t, parent, "--root", other, "--out-dir", other, "show", "C000001"); !strings.Contains(got, "file: `other.go`") {
		t.Fatalf("explicit show = %q", got)
	}
	runOK(t, parent, "--root", other, "--out-dir", other, "ignore", "list")
	runOK(t, parent, "--root", other, "--out-dir", other, "ignore", "enable", "go:tests")
	assertIgnore(t, other, []string{"go:tests"}, nil)
	if _, err := os.Stat(filepath.Join(parent, ".nodex", "ignore.json")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("explicit enable wrote the discovered project")
	}
	runOK(t, nested, "--root", other, "--out-dir", other, "ignore", "disable", "go:tests")
	assertIgnore(t, other, nil, nil)

	if got := runOK(t, parent, "--", "status"); got != statusCurrent(2, 2) {
		t.Fatalf("end of options = %q", got)
	}
	if got := runOK(t, parent, "--root", other, "--out-dir", other, "--", "status"); got != statusCurrent(1, 1) {
		t.Fatalf("root then end of options = %q", got)
	}
	if got := runOK(t, parent, "--root="+other, "--out-dir="+other, "status"); got != statusCurrent(1, 1) {
		t.Fatalf("equals form = %q", got)
	}
	stdout, stderr, err := runAt(parent, "--", "--root", other, "--out-dir", other, "status")
	if err == nil || stdout != "" || stderr != "unknown command \"--root\"\n" {
		t.Fatalf("option after end = %q %q %v", stdout, stderr, err)
	}
	stdout, stderr, err = runAt(parent, "--root")
	if err == nil || stdout != "" || stderr != "--root requires a path\n" {
		t.Fatalf("missing value = %q %q %v", stdout, stderr, err)
	}
	stdout, stderr, err = runAt(parent, "--root=", "status")
	if err == nil || stdout != "" || stderr != "--root requires a path\n" {
		t.Fatalf("empty equals = %q %q %v", stdout, stderr, err)
	}
	stdout, stderr, err = runAt(parent, "--root=-missing", "status")
	if err == nil || stdout != "" || stderr != "--root requires a path\n" {
		t.Fatalf("dashed value = %q %q %v", stdout, stderr, err)
	}
	stdout, stderr, err = runAt(parent, "--root", "--", "generate")
	if err == nil || stdout != "" || stderr != "--root requires a path\n" {
		t.Fatalf("flag value = %q %q %v", stdout, stderr, err)
	}
	stdout, stderr, err = runAt(parent, "--root", other, "--root", other, "status")
	if err == nil || stdout != "" || stderr != "--root was provided more than once\n" {
		t.Fatalf("duplicate = %q %q %v", stdout, stderr, err)
	}
	stdout, stderr, err = runAt(parent, "--foo", "status")
	if err == nil || stdout != "" || stderr != "unknown option \"--foo\"\n" {
		t.Fatalf("unknown = %q %q %v", stdout, stderr, err)
	}
	stdout, stderr, err = runAt(parent, "-root", other, "status")
	if err == nil || stdout != "" || stderr != "unknown option \"-root\"\n" {
		t.Fatalf("short = %q %q %v", stdout, stderr, err)
	}

	explicit := filepath.Join(parent, "explicit")
	writeProjectFile(t, explicit, "main.go", "// explicit\n\npackage main\n")
	if got := runOK(t, parent, "--root", "explicit", "--out-dir", "explicit", "generate"); got != "generated 1 comment and 1 declaration from 1 source file\n" {
		t.Fatalf("relative = %q", got)
	}
	_, snap = loadIndex(t, explicit)
	if pathsOf(snap) != "main.go" {
		t.Fatalf("relative sources = %s", pathsOf(snap))
	}
}

func TestFinalCLILifecycle(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, ".git"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	writeProjectFile(t, root, "main.go", "// hello\n\npackage main\n")
	writeProjectFile(t, root, "main_test.go", "// test note\n\npackage main\n")
	writeIgnore(t, root, nil, []string{"zzz/", "aaa/"})
	nested := filepath.Join(root, "nested")
	if err := os.Mkdir(nested, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	if got := runOK(t, nested, "generate"); got != "generated 2 comments and 2 declarations from 2 source files\n" {
		t.Fatalf("generate = %q", got)
	}
	if got := runOK(t, nested, "status"); got != statusCurrent(2, 2) {
		t.Fatalf("status = %q", got)
	}
	if got := runOK(t, nested, "comments"); got != "## C000001\n\nfile: `main.go`\n\nhello\n\n## C000002\n\nfile: `main_test.go`\n\ntest note\n" {
		t.Fatalf("comments = %q", got)
	}
	shown := runOK(t, nested, "show", "C000001")
	if !strings.Contains(shown, "## C000001\n") || !strings.Contains(shown, "file: `main.go`") || !strings.Contains(shown, "hello") {
		t.Fatalf("show = %q", shown)
	}

	before := pairBytes(t, root)
	runOK(t, nested, "ignore", "enable", "go:tests")
	assertIgnore(t, root, []string{"go:tests"}, []string{"zzz/", "aaa/"})
	requireSameIndex(t, root, before)
	if got := runOK(t, nested, "status"); got != statusStale(2, 1, 2) {
		t.Fatalf("stale status = %q", got)
	}
	assertStaleComments(t, root)
	stdout, stderr, err := runAt(nested, "index", "show", "C000001")
	if err == nil || stdout != "" || stderr != "nodex: index is stale; run nodex index generate\n" {
		t.Fatalf("stale show = %q %q %v", stdout, stderr, err)
	}

	if got := runOK(t, nested, "generate"); got != "generated 1 comment and 1 declaration from 1 source file\n" {
		t.Fatalf("regenerate = %q", got)
	}
	if got := runOK(t, nested, "status"); got != statusCurrent(1, 1) {
		t.Fatalf("restored status = %q", got)
	}
	if got := runOK(t, nested, "comments"); got != "## C000001\n\nfile: `main.go`\n\nhello\n" {
		t.Fatalf("restored comments = %q", got)
	}
	shown = runOK(t, nested, "show", "C000001")
	if !strings.Contains(shown, "file: `main.go`") || strings.Contains(shown, "main_test.go") {
		t.Fatalf("restored show = %q", shown)
	}
	_, snap := loadIndex(t, root)
	if pathsOf(snap) != "main.go" {
		t.Fatalf("sources = %s", pathsOf(snap))
	}

	runOK(t, nested, "ignore", "disable", "go:tests")
	assertIgnore(t, root, nil, []string{"zzz/", "aaa/"})
	if got := runOK(t, nested, "status"); got != statusStale(1, 2, 1) {
		t.Fatalf("disable status = %q", got)
	}

	var out, errOut bytes.Buffer
	err = run([]string{"version"}, func() (string, error) {
		return "", errors.New("no working directory")
	}, &out, &errOut)
	if err != nil || out.String() != defaultVersionOutput || errOut.Len() != 0 {
		t.Fatalf("version = %q %q %v", out.String(), errOut.String(), err)
	}

	other := t.TempDir()
	writeProjectFile(t, other, "solo.go", "// solo\n\npackage solo\n")
	if got := runOK(t, nested, "--root", other, "--out-dir", other, "generate"); got != "generated 1 comment and 1 declaration from 1 source file\n" {
		t.Fatalf("root generate = %q", got)
	}
	if got := runOK(t, root, "--root", other, "--out-dir", other, "status"); got != statusCurrent(1, 1) {
		t.Fatalf("root status = %q", got)
	}
	_, snap = loadIndex(t, other)
	if pathsOf(snap) != "solo.go" {
		t.Fatalf("explicit sources = %s", pathsOf(snap))
	}
}

func runOK(t *testing.T, root string, args ...string) string {
	t.Helper()
	args = indexArgs(args)
	stdout, stderr, err := runAt(root, args...)
	if err != nil || stderr != "" {
		t.Fatalf("%v: %v\nstdout %q\nstderr %q", args, err, stdout, stderr)
	}
	return stdout
}

// indexArgs inserts the index group before a snapshot subcommand.
// Flags and the end-of-options marker stay in front of the group.
func indexArgs(args []string) []string {
	out := make([]string, 0, len(args)+1)
	i := 0
	for i < len(args) {
		arg := args[i]
		if arg == "--" {
			out = append(out, arg)
			i++
			break
		}
		if strings.HasPrefix(arg, "--") {
			name, _, hasValue := splitFlag(arg)
			out = append(out, arg)
			i++
			if (name == "root" || name == "out-dir") && !hasValue && i < len(args) {
				out = append(out, args[i])
				i++
			}
			continue
		}
		if strings.HasPrefix(arg, "-") {
			out = append(out, arg)
			i++
			continue
		}
		break
	}
	if i < len(args) {
		switch args[i] {
		case "generate", "comments", "declarations", "status", "show":
			out = append(out, "index")
		}
	}
	return append(out, args[i:]...)
}

func writeIgnore(t *testing.T, root string, presets, excludes []string) {
	t.Helper()
	policy, err := ignore.New(presets, excludes)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	data, err := policy.Encode()
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	writeProjectFile(t, root, ignore.DocumentPath, string(data))
}

func assertIgnore(t *testing.T, root string, presets, excludes []string) {
	t.Helper()
	policy := mustPolicy(t, string(readGen(t, root, ignore.DocumentPath)))
	if !slices.Equal(policy.Presets(), normalizeIDs(presets)) || !slices.Equal(policy.Excludes(), normalizeIDs(excludes)) {
		t.Fatalf("presets %v excludes %v", policy.Presets(), policy.Excludes())
	}
	want, err := ignore.New(presets, excludes)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	encoded, err := want.Encode()
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	if !bytes.Equal(readGen(t, root, ignore.DocumentPath), encoded) {
		t.Fatalf("file = %q\nwant %q", readGen(t, root, ignore.DocumentPath), encoded)
	}
}

func normalizeIDs(ids []string) []string {
	if ids == nil {
		return []string{}
	}
	return ids
}

func TestSkillCommands(t *testing.T) {
	doc := string(skill.Document())
	if doc == "" || !strings.HasSuffix(doc, "\n") {
		t.Fatal("canonical skill is empty or has no final newline")
	}

	t.Run("targets and show need no project", func(t *testing.T) {
		const targets = "agents\nclaude\ncodex\ngemini\ngrok\n"
		called := false
		getwd := func() (string, error) {
			called = true
			return "", errors.New("no working directory")
		}
		stdout, stderr, err := runWith(getwd, "skill", "targets")
		if err != nil || stdout != targets || stderr != "" || called {
			t.Fatalf("targets = %q %q %v called=%v", stdout, stderr, err, called)
		}
		stdout, stderr, err = runWith(getwd, "--root", filepath.Join(t.TempDir(), "missing"), "skill", "show")
		if err != nil || stdout != doc || stderr != "" || called {
			t.Fatalf("show = %d bytes %q %v called=%v", len(stdout), stderr, err, called)
		}
		if stdout != string(skill.Document()) {
			t.Fatal("show differs from the embedded skill")
		}
	})

	t.Run("usage", func(t *testing.T) {
		root := t.TempDir()
		cases := []struct {
			args []string
			want string
		}{
			{[]string{"skill"}, "usage: nodex skill targets|show|install|uninstall\n"},
			{[]string{"skill", "nope"}, "unknown skill command \"nope\"\n"},
			{[]string{"skill", "targets", "extra"}, "skill targets takes no arguments\n"},
			{[]string{"skill", "targets", "--global"}, "unknown option \"--global\"\n"},
			{[]string{"skill", "show", "extra"}, "skill show takes no arguments\n"},
			{[]string{"skill", "show", "--global"}, "unknown option \"--global\"\n"},
			{[]string{"skill", "install"}, "skill install requires one target\n"},
			{[]string{"skill", "install", "codex", "extra"}, "skill install requires one target\n"},
			{[]string{"skill", "uninstall"}, "skill uninstall requires one target\n"},
			{[]string{"skill", "uninstall", "codex", "grok"}, "skill uninstall requires one target\n"},
			{[]string{"skill", "install", "codex", "--force"}, "unknown option \"--force\"\n"},
			{[]string{"skill", "install", "codex", "--global", "--global"}, "--global was provided more than once\n"},
			{[]string{"skill", "uninstall", "--global=true", "codex"}, "unknown option \"--global=true\"\n"},
			{[]string{"skill", "install", "codex", "--root", root}, "unknown option \"--root\"\n"},
			{[]string{"--global", "skill", "install", "codex"}, "unknown option \"--global\"\n"},
			{[]string{"skill", "install", "Agents"}, "nodex: unknown skill target \"Agents\"\n"},
			{[]string{"skill", "install", "CODEX"}, "nodex: unknown skill target \"CODEX\"\n"},
		}
		for _, tc := range cases {
			stdout, stderr, err := runAt(root, tc.args...)
			if err == nil || stdout != "" || stderr != tc.want {
				t.Fatalf("%v = %q %q %v", tc.args, stdout, stderr, err)
			}
		}
		if _, err := os.Stat(filepath.Join(root, ".codex")); !errors.Is(err, os.ErrNotExist) {
			t.Fatal("usage failure created a skill directory")
		}
	})

	t.Run("project install follows root resolution", func(t *testing.T) {
		parent := t.TempDir()
		if err := os.Mkdir(filepath.Join(parent, ".git"), 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		deep := filepath.Join(parent, "internal", "package")
		if err := os.MkdirAll(deep, 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		homeCalled := false
		setHomeFunc(t, func() (string, error) {
			homeCalled = true
			return "", errors.New("home called")
		})
		if stdout, stderr, err := runAt(deep, "skill", "install", "codex"); err != nil || stdout != "" || stderr != "" || homeCalled {
			t.Fatalf("install = %q %q %v home=%v", stdout, stderr, err, homeCalled)
		}
		dest := filepath.Join(resolvedRoot(t, parent), ".codex", "skills", "nodex", "SKILL.md")
		if !bytes.Equal(readFile(t, dest), skill.Document()) {
			t.Fatal("installed skill differs from skill show")
		}
		if _, err := os.Stat(filepath.Join(deep, ".codex")); !errors.Is(err, os.ErrNotExist) {
			t.Fatal("install wrote inside the nested working directory")
		}
		if _, err := os.Stat(filepath.Join(resolvedRoot(t, parent), ".nodex")); !errors.Is(err, os.ErrNotExist) {
			t.Fatal("install created .nodex")
		}
		if stdout, stderr, err := runAt(deep, "skill", "install", "codex"); err != nil || stdout != "" || stderr != "" {
			t.Fatalf("second install = %q %q %v", stdout, stderr, err)
		}
		if !bytes.Equal(readFile(t, dest), skill.Document()) {
			t.Fatal("second install changed the skill")
		}
	})

	t.Run("nearer marker wins", func(t *testing.T) {
		parent := t.TempDir()
		if err := os.Mkdir(filepath.Join(parent, ".git"), 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		nested := filepath.Join(parent, "nested")
		if err := os.MkdirAll(filepath.Join(nested, ".nodex"), 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		cwd := filepath.Join(nested, "pkg")
		if err := os.MkdirAll(cwd, 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		mustSkill(t, cwd, "skill", "install", "grok")
		if _, err := os.Stat(filepath.Join(resolvedRoot(t, nested), ".grok", "skills", "nodex", "SKILL.md")); err != nil {
			t.Fatalf("nested skill: %v", err)
		}
		if _, err := os.Stat(filepath.Join(parent, ".grok")); !errors.Is(err, os.ErrNotExist) {
			t.Fatal("install used the parent marker")
		}
		entries, err := os.ReadDir(filepath.Join(nested, ".nodex"))
		if err != nil || len(entries) != 0 {
			t.Fatalf(".nodex entries = %v %v", entries, err)
		}
	})

	t.Run("explicit root bypasses ancestors", func(t *testing.T) {
		parent := t.TempDir()
		if err := os.Mkdir(filepath.Join(parent, ".git"), 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		explicit := filepath.Join(parent, "explicit")
		if err := os.Mkdir(explicit, 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		called := false
		getwd := func() (string, error) {
			called = true
			return "", errors.New("no working directory")
		}
		stdout, stderr, err := runWith(getwd, "--root", explicit, "skill", "install", "claude")
		if err != nil || stdout != "" || stderr != "" || called {
			t.Fatalf("explicit = %q %q %v called=%v", stdout, stderr, err, called)
		}
		if _, statErr := os.Stat(filepath.Join(resolvedRoot(t, explicit), ".claude", "skills", "nodex", "SKILL.md")); statErr != nil {
			t.Fatalf("explicit skill: %v", statErr)
		}
		if _, statErr := os.Stat(filepath.Join(parent, ".claude")); !errors.Is(statErr, os.ErrNotExist) {
			t.Fatal("explicit install wrote the ancestor")
		}
	})

	t.Run("explicit root must exist and be a directory", func(t *testing.T) {
		parent := t.TempDir()
		missing := filepath.Join(parent, "missing")
		stdout, stderr, err := runAt(parent, "--root", missing, "skill", "install", "codex")
		if err == nil || stdout != "" || stderr == "" {
			t.Fatalf("missing = %q %q %v", stdout, stderr, err)
		}
		if _, statErr := os.Stat(missing); !errors.Is(statErr, os.ErrNotExist) {
			t.Fatal("missing root was created")
		}
		file := filepath.Join(parent, "file")
		if writeErr := os.WriteFile(file, []byte("keep"), 0o644); writeErr != nil {
			t.Fatalf("write: %v", writeErr)
		}
		stdout, stderr, err = runAt(parent, "--root", file, "skill", "uninstall", "codex")
		if err == nil || stdout != "" || !strings.Contains(stderr, "not a directory") {
			t.Fatalf("file root = %q %q %v", stdout, stderr, err)
		}
		if string(readFile(t, file)) != "keep" {
			t.Fatal("file root was modified")
		}
	})

	t.Run("no marker uses the working directory", func(t *testing.T) {
		parent := t.TempDir()
		child := filepath.Join(parent, "child")
		if err := os.Mkdir(child, 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		mustSkill(t, child, "skill", "install", "grok")
		if _, err := os.Stat(filepath.Join(resolvedRoot(t, child), ".grok", "skills", "nodex", "SKILL.md")); err != nil {
			t.Fatalf("child skill: %v", err)
		}
		if _, err := os.Stat(filepath.Join(parent, ".grok")); !errors.Is(err, os.ErrNotExist) {
			t.Fatal("install walked to the parent")
		}
		if _, err := os.Stat(filepath.Join(resolvedRoot(t, child), ".nodex")); !errors.Is(err, os.ErrNotExist) {
			t.Fatal("fallback created .nodex")
		}
	})

	t.Run("shared project path is idempotent", func(t *testing.T) {
		root := t.TempDir()
		mustSkill(t, root, "skill", "install", "agents")
		mustSkill(t, root, "skill", "install", "gemini")
		matches := 0
		err := filepath.WalkDir(resolvedRoot(t, root), func(path string, entry os.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if entry.Name() == "SKILL.md" && entry.Type().IsRegular() {
				matches++
				if !bytes.Equal(readFile(t, path), skill.Document()) {
					t.Fatal("shared skill bytes differ")
				}
			}
			return nil
		})
		if err != nil {
			t.Fatalf("walk: %v", err)
		}
		if matches != 1 {
			t.Fatalf("SKILL.md count = %d", matches)
		}
	})

	t.Run("install does not modify the index", func(t *testing.T) {
		root := t.TempDir()
		if err := os.Mkdir(filepath.Join(root, ".git"), 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		writeProjectFile(t, root, "main.go", "// hello\n\npackage main\n")
		if got := runOK(t, root, "generate"); got != "generated 1 comment and 1 declaration from 1 source file\n" {
			t.Fatalf("generate = %q", got)
		}
		before := pairBytes(t, root)
		mustSkill(t, root, "skill", "install", "codex")
		if !bytes.Equal(pairBytes(t, root), before) {
			t.Fatal("skill install changed the index")
		}
		_, snap := loadIndex(t, root)
		if snap.Schema != index.SchemaVersion || index.SchemaVersion != 1 {
			t.Fatalf("schema = %d", snap.Schema)
		}
		if _, err := os.Stat(filepath.Join(resolvedRoot(t, root), ".nodex", "ignore.json")); !errors.Is(err, os.ErrNotExist) {
			t.Fatal("skill install created ignore.json")
		}
	})

	t.Run("global install", func(t *testing.T) {
		home := t.TempDir()
		setHome(t, home, nil)
		paths := map[string]string{
			"agents": ".agents/skills/nodex/SKILL.md",
			"claude": ".claude/skills/nodex/SKILL.md",
			"codex":  ".codex/skills/nodex/SKILL.md",
			"gemini": ".gemini/config/skills/nodex/SKILL.md",
			"grok":   ".grok/skills/nodex/SKILL.md",
		}
		called := false
		getwd := func() (string, error) {
			called = true
			return "", errors.New("no working directory")
		}
		for name, rel := range paths {
			stdout, stderr, err := runWith(getwd, "skill", "install", "--global", name)
			if err != nil || stdout != "" || stderr != "" || called {
				t.Fatalf("install %s = %q %q %v called=%v", name, stdout, stderr, err, called)
			}
			got := readFile(t, filepath.Join(home, filepath.FromSlash(rel)))
			if !bytes.Equal(got, skill.Document()) {
				t.Fatalf("%s bytes differ", name)
			}
		}
		if _, err := os.Stat(filepath.Join(home, ".nodex")); !errors.Is(err, os.ErrNotExist) {
			t.Fatal("global install created .nodex")
		}
		if _, err := os.Stat(filepath.Join(home, ".agents", "skills", "nodex", "SKILL.md")); err != nil {
			t.Fatal("agents global path missing")
		}
		if _, err := os.Stat(filepath.Join(home, ".gemini", "skills")); !errors.Is(err, os.ErrNotExist) {
			t.Fatal("gemini used the project-local path")
		}
		stdout, stderr, err := runWith(getwd, "skill", "uninstall", "gemini", "--global")
		if err != nil || stdout != "" || stderr != "" || called {
			t.Fatalf("uninstall gemini = %q %q %v called=%v", stdout, stderr, err, called)
		}
		if _, statErr := os.Stat(filepath.Join(home, ".gemini", "config", "skills", "nodex")); !errors.Is(statErr, os.ErrNotExist) {
			t.Fatal("global uninstall left the nodex directory")
		}
		if _, statErr := os.Stat(filepath.Join(home, ".gemini", "config", "skills")); statErr != nil {
			t.Fatal("global uninstall removed the skills directory")
		}
		if _, statErr := os.Stat(filepath.Join(home, ".agents", "skills", "nodex", "SKILL.md")); statErr != nil {
			t.Fatal("global uninstall removed a different target")
		}
	})

	t.Run("global home failure", func(t *testing.T) {
		setHome(t, "", errors.New("home unavailable"))
		root := t.TempDir()
		stdout, stderr, err := runAt(root, "skill", "install", "codex", "--global")
		if err == nil || stdout != "" || stderr != "nodex: home unavailable\n" {
			t.Fatalf("home = %q %q %v", stdout, stderr, err)
		}
		if _, statErr := os.Stat(filepath.Join(root, ".codex")); !errors.Is(statErr, os.ErrNotExist) {
			t.Fatal("home failure wrote the working directory")
		}
	})

	t.Run("root and global are mutually exclusive", func(t *testing.T) {
		root := t.TempDir()
		home := t.TempDir()
		called := false
		homeCalled := false
		setHomeFunc(t, func() (string, error) {
			homeCalled = true
			return home, nil
		})
		getwd := func() (string, error) {
			called = true
			return root, nil
		}
		missing := filepath.Join(root, "missing")
		stdout, stderr, err := runWith(getwd, "--root", missing, "skill", "install", "codex", "--global")
		if err == nil || stdout != "" || stderr != "--root and --global cannot be combined\n" || called || homeCalled {
			t.Fatalf("conflict = %q %q %v wd=%v home=%v", stdout, stderr, err, called, homeCalled)
		}
		if _, statErr := os.Stat(missing); !errors.Is(statErr, os.ErrNotExist) {
			t.Fatal("conflict created the root")
		}
		if _, statErr := os.Stat(filepath.Join(home, ".codex")); !errors.Is(statErr, os.ErrNotExist) {
			t.Fatal("conflict wrote the home directory")
		}
		stdout, stderr, err = runWith(getwd, "--root", root, "skill", "uninstall", "--global", "grok")
		if err == nil || stdout != "" || stderr != "--root and --global cannot be combined\n" {
			t.Fatalf("uninstall conflict = %q %q %v", stdout, stderr, err)
		}
	})

	t.Run("uninstall", func(t *testing.T) {
		root := t.TempDir()
		mustSkill(t, root, "skill", "install", "codex")
		mustSkill(t, root, "skill", "uninstall", "codex")
		nodexDir := filepath.Join(resolvedRoot(t, root), ".codex", "skills", "nodex")
		if _, err := os.Stat(nodexDir); !errors.Is(err, os.ErrNotExist) {
			t.Fatal("empty nodex directory remained")
		}
		if _, err := os.Stat(filepath.Join(resolvedRoot(t, root), ".codex", "skills")); err != nil {
			t.Fatal("skills directory was removed")
		}
		if _, err := os.Stat(filepath.Join(resolvedRoot(t, root), ".codex")); err != nil {
			t.Fatal(".codex was removed")
		}
		mustSkill(t, root, "skill", "uninstall", "codex")

		mustSkill(t, root, "skill", "install", "codex")
		notes := filepath.Join(nodexDir, "notes.txt")
		if err := os.WriteFile(notes, []byte("keep"), 0o644); err != nil {
			t.Fatalf("write: %v", err)
		}
		mustSkill(t, root, "skill", "uninstall", "codex")
		if string(readFile(t, notes)) != "keep" {
			t.Fatal("uninstall removed a sibling file")
		}
		if _, err := os.Stat(filepath.Join(nodexDir, "SKILL.md")); !errors.Is(err, os.ErrNotExist) {
			t.Fatal("SKILL.md remained")
		}
		if _, err := os.Stat(nodexDir); err != nil {
			t.Fatal("nodex directory with a sibling was removed")
		}

		unmanaged := filepath.Join(resolvedRoot(t, root), ".claude", "skills", "nodex", "SKILL.md")
		if err := os.MkdirAll(filepath.Dir(unmanaged), 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		if err := os.WriteFile(unmanaged, []byte("# mine\n"), 0o644); err != nil {
			t.Fatalf("write: %v", err)
		}
		stdout, stderr, err := runAt(root, "skill", "uninstall", "claude")
		if err == nil || stdout != "" || !strings.Contains(stderr, "not a Nodex-managed skill") {
			t.Fatalf("unmanaged = %q %q %v", stdout, stderr, err)
		}
		if string(readFile(t, unmanaged)) != "# mine\n" {
			t.Fatal("unmanaged skill was removed")
		}
	})
}

func runWith(getwd func() (string, error), args ...string) (string, string, error) {
	var stdout, stderr bytes.Buffer
	err := run(args, getwd, &stdout, &stderr)
	return stdout.String(), stderr.String(), err
}

func mustSkill(t *testing.T, dir string, args ...string) {
	t.Helper()
	stdout, stderr, err := runAt(dir, args...)
	if err != nil || stdout != "" || stderr != "" {
		t.Fatalf("%v: %v\nstdout %q\nstderr %q", args, err, stdout, stderr)
	}
}

func setHome(t *testing.T, home string, homeErr error) {
	t.Helper()
	setHomeFunc(t, func() (string, error) { return home, homeErr })
}

func setHomeFunc(t *testing.T, fn func() (string, error)) {
	t.Helper()
	previous := userHomeDir
	userHomeDir = fn
	t.Cleanup(func() { userHomeDir = previous })
}

func TestIndexDeclarations(t *testing.T) {
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
	const wantDecls = "" +
		"## D000001\n\nfile: `sample.go`\nkind: package\nnames: sample\ndoc: C000001\n\n" +
		"## D000002\n\nfile: `sample.go`\nkind: function\nnames: F\ndoc: C000002\n\n" +
		"## D000003\n\nfile: `sample.go`\nkind: function\nnames: g\ndoc: none\n\n" +
		"## D000004\n\nfile: `sample.go`\nkind: const-group\nnames: A, B\ndoc: none\n\n" +
		"## D000005\n\nfile: `sample.go`\nkind: const\nnames: A\ndoc: C000003\n\n" +
		"## D000006\n\nfile: `sample.go`\nkind: const\nnames: B\ndoc: none\n\n" +
		"## D000007\n\nfile: `sample.go`\nkind: type-group\nnames: T\ndoc: C000004\n\n" +
		"## D000008\n\nfile: `sample.go`\nkind: type\nnames: T\ndoc: none\n\n" +
		"## D000009\n\nfile: `sample.go`\nkind: field\nnames: Name\ndoc: C000005\n\n" +
		"## D000010\n\nfile: `sample.go`\nkind: field\nnames: Hidden\ndoc: none\n"
	const wantComments = "" +
		"## C000001\n\nfile: `sample.go`\n\nPackage sample documents sample.\n\n" +
		"## C000002\n\nfile: `sample.go`\n\nF documents F.\n\n" +
		"## C000003\n\nfile: `sample.go`\n\nA documents A.\n\n" +
		"## C000004\n\nfile: `sample.go`\n\nTGroup documents the type group.\n\n" +
		"## C000005\n\nfile: `sample.go`\n\nName documents Name.\n"

	t.Run("empty project", func(t *testing.T) {
		root := t.TempDir()
		if got := generateOK(t, root); got != "generated 0 comments and 0 declarations from 0 source files\n" {
			t.Fatalf("generate = %q", got)
		}
		if got := commandOK(t, root, "declarations"); got != "" {
			t.Fatalf("declarations = %q", got)
		}
		if got := commandOK(t, root, "comments"); got != "" {
			t.Fatalf("comments = %q", got)
		}
	})

	t.Run("package only", func(t *testing.T) {
		root := t.TempDir()
		writeProjectFile(t, root, "a.go", "package a\n")
		generateOK(t, root)
		if got := commandOK(t, root, "comments"); got != "" {
			t.Fatalf("comments = %q", got)
		}
		got := commandOK(t, root, "declarations")
		want := "## D000001\n\nfile: `a.go`\nkind: package\nnames: a\ndoc: none\n"
		if got != want {
			t.Fatalf("declarations = %q\nwant %q", got, want)
		}
		shown := showOK(t, root, "D000001")
		wantShow := "## D000001\n\nfile: `a.go`\nlines: 1\nkind: package\nnames: a\ndoc: none\n\n### Context\n\n```go\npackage a\n```\n"
		if shown != wantShow {
			t.Fatalf("show = %q\nwant %q", shown, wantShow)
		}
	})

	t.Run("empty names", func(t *testing.T) {
		root := t.TempDir()
		writeProjectFile(t, root, "a.go", "package p\n\ntype T struct {\n\tnone int\n\tU\n}\n\ntype U struct{}\n")
		generateOK(t, root)
		got := commandOK(t, root, "declarations")
		want := "" +
			"## D000001\n\nfile: `a.go`\nkind: package\nnames: p\ndoc: none\n\n" +
			"## D000002\n\nfile: `a.go`\nkind: type\nnames: T\ndoc: none\n\n" +
			"## D000003\n\nfile: `a.go`\nkind: field\nnames: none\ndoc: none\n\n" +
			"## D000004\n\nfile: `a.go`\nkind: field\nnames:\ndoc: none\n\n" +
			"## D000005\n\nfile: `a.go`\nkind: type\nnames: U\ndoc: none\n"
		if got != want {
			t.Fatalf("declarations = %q\nwant %q", got, want)
		}
		shown := showOK(t, root, "D000003", "D000004")
		if !strings.Contains(shown, "kind: field\nnames: none\ndoc: none\n") || !strings.Contains(shown, "kind: field\nnames:\ndoc: none\n") {
			t.Fatalf("show = %q", shown)
		}
	})

	root := t.TempDir()
	writeProjectFile(t, root, "sample.go", sample)
	if got := generateOK(t, root); got != "generated 5 comments and 10 declarations from 1 source file\n" {
		t.Fatalf("generate = %q", got)
	}
	if got := commandOK(t, root, "comments"); got != wantComments {
		t.Fatalf("comments = %q", got)
	}
	if got := commandOK(t, root, "declarations"); got != wantDecls {
		t.Fatalf("declarations = %q\nwant %q", got, wantDecls)
	}
	idx, snap := loadIndex(t, root)
	if snap.CommentCount != 5 || snap.DeclarationCount != 10 || snap.CommentsDigest != index.DigestBytes(readGen(t, root, index.CommentsPath)) || snap.DeclarationsDigest != index.DigestBytes(readGen(t, root, index.DeclarationsPath)) {
		t.Fatalf("snapshot = %+v", snap)
	}
	if idx.Declarations()[1].Doc.String() != "C000002" || idx.Declarations()[2].Doc.Valid() || idx.Declarations()[6].Doc.String() != "C000004" || idx.Declarations()[7].Doc.Valid() {
		t.Fatalf("relationships = %+v", idx.Declarations())
	}

	commentShow := showOK(t, root, "C000002")
	if strings.Contains(commentShow, "### Comment") {
		t.Fatalf("comment show repeated a comment section: %q", commentShow)
	}
	before := strings.Split(commentShow, "### Context\n")[0]
	if strings.Contains(before, "F documents F.") {
		t.Fatalf("normalized comment precedes context: %q", commentShow)
	}
	if !strings.Contains(contextBody(t, commentShow), "// F documents F.\n") {
		t.Fatalf("context dropped the source comment: %q", commentShow)
	}

	declShow := showOK(t, root, "D000002")
	wantDeclShow := "## D000002\n\nfile: `sample.go`\nlines: 5\nkind: function\nnames: F\ndoc: C000002\n\n### Context\n\n```go\nfunc F() {}\n```\n"
	if declShow != wantDeclShow {
		t.Fatalf("declaration show = %q\nwant %q", declShow, wantDeclShow)
	}

	groupShow := showOK(t, root, "D000004")
	groupHead := strings.Split(groupShow, "### Context\n")[0]
	if strings.Contains(groupHead, "A documents A.") || !strings.Contains(groupHead, "doc: none\n") {
		t.Fatalf("group metadata = %q", groupHead)
	}

	mixed := showOK(t, root, "C000002", "D000005", "C000002")
	first := strings.Index(mixed, "## C000002\n")
	second := strings.Index(mixed, "## D000005\n")
	third := strings.Index(mixed[first+1:], "## C000002\n")
	if first < 0 || second < first || third < 0 {
		t.Fatalf("order = %q", mixed)
	}
	if strings.Count(mixed, "## C000002\n") != 2 || strings.Count(mixed, "## D000005\n") != 1 {
		t.Fatalf("duplicates = %q", mixed)
	}
	if strings.Contains(mixed, "### Comment") {
		t.Fatalf("mixed show repeated a comment section: %q", mixed)
	}

	beforeBytes := pairBytes(t, root)
	for _, args := range [][]string{
		{"index", "show", "C000002", "D000099"},
		{"index", "show", "D000099", "C000002"},
		{"index", "show", "D000001", "nope"},
		{"index", "show", "nope", "D000001"},
		{"index", "show", "D000000"},
		{"index", "show", "d000001"},
		{"index", "show", "C000001", "D000001", "C000099"},
	} {
		stdout, stderr, err := runAt(root, args...)
		if err == nil || stdout != "" {
			t.Fatalf("%v = %q %q %v", args, stdout, stderr, err)
		}
		if strings.Contains(stdout, "## ") || strings.Contains(stdout, "func F") {
			t.Fatalf("%v leaked %q", args, stdout)
		}
	}
	requireSameIndex(t, root, beforeBytes)

	writeProjectFile(t, root, "sample.go", sample+"\nfunc extra() {}\n")
	stdout, stderr, err := runAt(root, "index", "declarations")
	if err == nil || stdout != "" || stderr != "nodex: index is stale; run nodex index generate\n" {
		t.Fatalf("stale declarations = %q %q %v", stdout, stderr, err)
	}
	requireSameIndex(t, root, beforeBytes)
	writeProjectFile(t, root, "sample.go", sample)

	showReadFile = func(opened *project.Project, logical string) ([]byte, error) {
		if logical == "sample.go" {
			return []byte("package sample\n"), nil
		}
		return opened.ReadFile(logical)
	}
	t.Cleanup(func() { showReadFile = nil })
	stdout, stderr, err = runAt(root, "index", "show", "D000002")
	if err == nil || stdout != "" || stderr != "nodex: index is stale; run nodex index generate\n" {
		t.Fatalf("race = %q %q %v", stdout, stderr, err)
	}
	if strings.Contains(stdout, "func F") || strings.Contains(stdout, "package sample") {
		t.Fatalf("race stdout = %q", stdout)
	}
	requireSameIndex(t, root, beforeBytes)
	showReadFile = nil

	declPath := filepath.Join(resolvedRoot(t, root), filepath.FromSlash(index.DeclarationsPath))
	broken := readFile(t, declPath)
	broken[len(broken)-2] ^= 0x1
	if err := os.WriteFile(declPath, broken, 0o644); err != nil {
		t.Fatalf("tamper: %v", err)
	}
	if got := commandOK(t, root, "status"); got != "index: corrupt\n" {
		t.Fatalf("status = %q", got)
	}
	stdout, stderr, err = runAt(root, "index", "declarations")
	if err == nil || stdout != "" || !strings.Contains(stderr, "corrupt") {
		t.Fatalf("corrupt declarations = %q %q %v", stdout, stderr, err)
	}
	if err := os.Remove(declPath); err != nil {
		t.Fatalf("remove: %v", err)
	}
	if got := commandOK(t, root, "status"); got != "index: corrupt\n" {
		t.Fatalf("missing declarations status = %q", got)
	}
}

func TestDiscoveryLogicalPaths(t *testing.T) {
	for _, mode := range []string{"local", "detached", "out-dir"} {
		t.Run(mode, func(t *testing.T) {
			source, workspace, cwd := t.TempDir(), t.TempDir(), t.TempDir()
			writeProjectFile(t, source, "z/example.go", "package example\n\nfunc Example() {}\n")
			writeProjectFile(t, source, "a/b/example.go", "package example\n\n// Example comment.\nfunc Example() {}\n")
			var options []string
			switch mode {
			case "local":
				cwd, workspace = source, source
			case "detached":
				cwd = workspace
				options = []string{"--root", source}
			case "out-dir":
				options = []string{"--root", source, "--out-dir", workspace}
			}
			runCommand := func(command string) string {
				t.Helper()
				return runOK(t, cwd, append(options, "index", command)...)
			}
			runCommand("generate")
			before := pairBytes(t, workspace)
			comments := runCommand("comments")
			const wantComments = "## C000001\n\nfile: `a/b/example.go`\n\nExample comment.\n"
			if comments != wantComments {
				t.Fatalf("comments = %q, want %q", comments, wantComments)
			}
			declarations := runCommand("declarations")
			const wantDeclarations = "" +
				"## D000001\n\nfile: `a/b/example.go`\nkind: package\nnames: example\ndoc: none\n\n" +
				"## D000002\n\nfile: `a/b/example.go`\nkind: function\nnames: Example\ndoc: C000001\n\n" +
				"## D000003\n\nfile: `z/example.go`\nkind: package\nnames: example\ndoc: none\n\n" +
				"## D000004\n\nfile: `z/example.go`\nkind: function\nnames: Example\ndoc: none\n"
			if declarations != wantDeclarations {
				t.Fatalf("declarations = %q, want %q", declarations, wantDeclarations)
			}
			for _, dir := range []string{source, workspace, cwd} {
				if strings.Contains(comments, dir) || strings.Contains(declarations, dir) {
					t.Fatalf("discovery contains absolute directory %q", dir)
				}
			}
			requireSameIndex(t, workspace, before)
		})
	}
}

func TestDetachedWorkspace(t *testing.T) {
	source, workspace := t.TempDir(), t.TempDir()
	writeProjectFile(t, source, "a.go", "// alpha\npackage a\n")
	writeProjectFile(t, source, "a_test.go", "// test\npackage a\n")
	writeProjectFile(t, workspace, "wrong.go", "package {\n")
	detached := func(args ...string) string {
		t.Helper()
		return runOK(t, workspace, append([]string{"--root", source}, args...)...)
	}
	if got := detached("status"); got != "index: missing\n" {
		t.Fatalf("missing = %q", got)
	}
	detached("ignore", "list")
	for _, command := range []string{"comments", "declarations", "show"} {
		args := []string{"--root", source, "index", command}
		if command == "show" {
			args = append(args, "C000001")
		}
		stdout, _, err := runAt(workspace, args...)
		if err == nil || stdout != "" {
			t.Fatalf("missing %s = %q %v", command, stdout, err)
		}
	}
	for _, dir := range []string{source, workspace} {
		if _, err := os.Lstat(filepath.Join(dir, ".nodex")); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("read created state in %s", dir)
		}
	}
	if got := detached("generate"); got != "generated 2 comments and 2 declarations from 2 source files\n" {
		t.Fatalf("generate = %q", got)
	}
	if got := detached("status"); got != statusCurrent(2, 2) {
		t.Fatalf("current = %q", got)
	}
	if got := detached("comments"); got != "## C000001\n\nfile: `a.go`\n\nalpha\n\n## C000002\n\nfile: `a_test.go`\n\ntest\n" {
		t.Fatalf("comments = %q", got)
	}
	const wantDeclarations = "" +
		"## D000001\n\nfile: `a.go`\nkind: package\nnames: a\ndoc: C000001\n\n" +
		"## D000002\n\nfile: `a_test.go`\nkind: package\nnames: a\ndoc: C000002\n"
	if got := detached("declarations"); got != wantDeclarations {
		t.Fatalf("declarations = %q", got)
	}
	for _, id := range []string{"C000001", "D000001"} {
		if got := detached("show", id); !strings.Contains(got, "file: `a.go`") || !strings.Contains(got, "package a") {
			t.Fatalf("show = %q", got)
		}
	}
	_, snap := loadIndex(t, workspace)
	if pathsOf(snap) != "a.go,a_test.go" || snap.Sources[0].Digest != index.DigestBytes([]byte("// alpha\npackage a\n")) {
		t.Fatalf("source fingerprints = %+v", snap.Sources)
	}
	for _, path := range []string{index.SnapshotPath, index.CommentsPath, index.DeclarationsPath} {
		data := readGen(t, workspace, path)
		for _, dir := range []string{source, workspace} {
			if bytes.Contains(data, []byte(dir)) {
				t.Fatalf("%s persisted absolute path", path)
			}
		}
	}
	if _, err := os.Lstat(filepath.Join(source, ".nodex")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("generation wrote source control state")
	}
	// Equal source bytes and policy remain current across source boundaries.
	clone := t.TempDir()
	writeProjectFile(t, clone, "a.go", "// alpha\npackage a\n")
	writeProjectFile(t, clone, "a_test.go", "// test\npackage a\n")
	if got := runOK(t, workspace, "--root", clone, "status"); got != statusCurrent(2, 2) {
		t.Fatalf("equivalent source = %q", got)
	}
	before := pairBytes(t, workspace)
	detached("ignore", "enable", "go:tests")
	assertIgnore(t, workspace, []string{"go:tests"}, nil)
	requireSameIndex(t, workspace, before)
	if _, err := os.Lstat(filepath.Join(source, ".nodex")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("ignore wrote source control state")
	}
	if got := detached("status"); got != statusStale(2, 1, 2) {
		t.Fatalf("policy status = %q", got)
	}
	detached("generate")
	if got := detached("status"); got != statusCurrent(1, 1) {
		t.Fatalf("policy current = %q", got)
	}
	// Source-local policy and malformed control-directory source never override workspace policy.
	writeIgnore(t, source, nil, []string{"a.go"})
	writeProjectFile(t, source, ".nodex/hidden.go", "package {\n")
	sourcePolicy := readGen(t, source, ignore.DocumentPath)
	detached("generate")
	_, snap = loadIndex(t, workspace)
	if pathsOf(snap) != "a.go" {
		t.Fatalf("workspace policy sources = %s", pathsOf(snap))
	}
	if !bytes.Equal(sourcePolicy, readGen(t, source, ignore.DocumentPath)) {
		t.Fatal("source policy changed")
	}
	detached("ignore", "disable", "go:tests")
	assertIgnore(t, workspace, nil, nil)
	if got := detached("status"); got != statusStale(1, 2, 1) {
		t.Fatalf("disable = %q", got)
	}
	detached("generate")
	// The default local workflow produces exactly the same bytes.
	writeIgnore(t, source, nil, nil)
	generateOK(t, source)
	if !bytes.Equal(pairBytes(t, workspace), pairBytes(t, source)) {
		t.Fatal("local and detached persisted bytes differ")
	}
}

func TestOutputWorkspaceSelection(t *testing.T) {
	source, cwd, out := t.TempDir(), t.TempDir(), t.TempDir()
	writeProjectFile(t, source, "a.go", "// alpha\npackage a\n")
	writeProjectFile(t, source, ".git/HEAD", "ref")
	nested := filepath.Join(source, "nested")
	if err := os.Mkdir(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, cwd string
		flags     []string
	}{
		{"both", cwd, []string{"--root", source, "--out-dir", out}},
		{"out only", nested, []string{"--out-dir", out}},
		{"reverse equals", cwd, []string{"--out-dir=" + out, "--root=" + source}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := runOK(t, tc.cwd, append(tc.flags, "generate")...); got != "generated 1 comment and 1 declaration from 1 source file\n" {
				t.Fatalf("generate = %q", got)
			}
			if got := runOK(t, tc.cwd, append(tc.flags, "status")...); got != statusCurrent(1, 1) {
				t.Fatalf("status = %q", got)
			}
		})
	}
	for _, dir := range []string{source, cwd, nested} {
		if _, err := os.Lstat(filepath.Join(dir, ".nodex")); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("state outside out: %s", dir)
		}
	}
	// An explicit root from inside that source uses the invocation subdirectory.
	runOK(t, nested, "--root", source, "generate")
	_, snap := loadIndex(t, nested)
	if pathsOf(snap) != "a.go" {
		t.Fatalf("nested fingerprints = %s", pathsOf(snap))
	}
	writeProjectFile(t, nested, ".nodex/hidden.go", "package {\n")
	if got := runOK(t, nested, "--root", source, "status"); got != statusCurrent(1, 1) {
		t.Fatalf("nested control included: %q", got)
	}
	// Relative flags use captured cwd even when the process cwd differs.
	base := t.TempDir()
	writeProjectFile(t, base, "source/a.go", "package a\n")
	if err := os.Mkdir(filepath.Join(base, "out"), 0o755); err != nil {
		t.Fatal(err)
	}
	runOK(t, base, "--root", "source", "--out-dir", "out", "generate")
	_, snap = loadIndex(t, filepath.Join(base, "out"))
	if pathsOf(snap) != "a.go" {
		t.Fatalf("relative fingerprints = %s", pathsOf(snap))
	}
}

func TestOutputOptionErrorsAndIndependentCommands(t *testing.T) {
	cwd, source := t.TempDir(), t.TempDir()
	writeProjectFile(t, source, "a.go", "package a\n")
	cases := []struct {
		args    []string
		message string
	}{
		{[]string{"--out-dir"}, "--out-dir requires a path\n"},
		{[]string{"--out-dir="}, "--out-dir requires a path\n"},
		{[]string{"--out-dir", "--root", source, "index", "generate"}, "--out-dir requires a path\n"},
		{[]string{"--out-dir=-x", "index", "status"}, "--out-dir requires a path\n"},
		{[]string{"--out-dir", cwd, "--out-dir=" + cwd, "index", "generate"}, "--out-dir was provided more than once\n"},
		{[]string{"index", "generate", "--out-dir", cwd}, "index generate takes no arguments\n"},
	}
	for _, tc := range cases {
		stdout, stderr, err := runAt(cwd, tc.args...)
		if err == nil || stdout != "" || stderr != tc.message {
			t.Fatalf("%q = %q %q %v", tc.args, stdout, stderr, err)
		}
	}
	file := filepath.Join(cwd, "file")
	writeProjectFile(t, cwd, "file", "file")
	for _, invalid := range []string{filepath.Join(cwd, "missing"), file} {
		stdout, stderr, err := runAt(cwd, "--root", source, "--out-dir", invalid, "index", "generate")
		if err == nil || stdout != "" || !strings.Contains(stderr, "workspace base") {
			t.Fatalf("invalid out = %q %q %v", stdout, stderr, err)
		}
	}
	missing := filepath.Join(cwd, "missing")
	for _, args := range [][]string{{"version"}, {"ignore", "presets"}, {"skill", "targets"}, {"skill", "show"}} {
		stdout, stderr, err := runWith(func() (string, error) { t.Fatal("independent command opened cwd"); return "", nil }, append([]string{"--root", missing, "--out-dir", missing}, args...)...)
		if err != nil || stderr != "" || stdout == "" {
			t.Fatalf("independent %q = %q %q %v", args, stdout, stderr, err)
		}
	}
	// out-dir has no role in either project or global skill destinations.
	runOK(t, cwd, "--root", source, "--out-dir", missing, "skill", "install", "codex")
	if _, err := os.Stat(filepath.Join(source, ".codex/skills/nodex/SKILL.md")); err != nil {
		t.Fatal(err)
	}
	home := t.TempDir()
	oldHome := userHomeDir
	userHomeDir = func() (string, error) { return home, nil }
	t.Cleanup(func() { userHomeDir = oldHome })
	runOK(t, cwd, "--out-dir", missing, "skill", "install", "codex", "--global")
	if _, err := os.Stat(filepath.Join(home, ".codex/skills/nodex/SKILL.md")); err != nil {
		t.Fatal(err)
	}
	for _, dir := range []string{cwd, source, home} {
		if _, err := os.Lstat(filepath.Join(dir, ".nodex")); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("unexpected control state: %s", dir)
		}
	}
	if _, err := os.Lstat(missing); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("missing base created")
	}
}

func TestDetachedControlSafety(t *testing.T) {
	source, cwd := t.TempDir(), t.TempDir()
	writeProjectFile(t, source, "a.go", "// alpha\npackage a\n")
	for _, component := range []string{".nodex", ignore.DocumentPath, index.IndexDir, index.SnapshotPath, index.CommentsPath, index.DeclarationsPath} {
		t.Run(component, func(t *testing.T) {
			out, outside := t.TempDir(), t.TempDir()
			target := filepath.Join(outside, "target")
			path := filepath.Join(out, filepath.FromSlash(component))
			if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
				t.Fatal(err)
			}
			isDir := component == ".nodex" || component == index.IndexDir
			if isDir {
				if err := os.Mkdir(target, 0o755); err != nil {
					t.Fatal(err)
				}
			} else {
				runOK(t, cwd, "--root", source, "--out-dir", out, "generate")
				if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
					t.Fatal(err)
				}
				writeProjectFile(t, outside, "target", "original")
			}
			if err := os.Symlink(target, path); err != nil {
				t.Fatal(err)
			}
			for _, command := range []string{"status", "comments", "declarations", "show"} {
				args := []string{"--root", source, "--out-dir", out, "index", command}
				if command == "show" {
					args = append(args, "C000001")
				}
				stdout, stderr, err := runAt(cwd, args...)
				if component != ".nodex" && component != ignore.DocumentPath && command == "status" {
					if err != nil || stdout != "index: corrupt\n" || stderr != "" {
						t.Fatalf("status = %q %q %v", stdout, stderr, err)
					}
				} else if err == nil || stdout != "" || !strings.Contains(stderr, "symbolic link") {
					t.Fatalf("%s = %q %q %v", command, stdout, stderr, err)
				}
			}
			if component == ".nodex" || component == ignore.DocumentPath {
				for _, args := range [][]string{{"ignore", "list"}, {"ignore", "enable", "go:tests"}, {"ignore", "disable", "go:tests"}, {"index", "generate"}} {
					stdout, stderr, err := runAt(cwd, append([]string{"--root", source, "--out-dir", out}, args...)...)
					if err == nil || stdout != "" || !strings.Contains(stderr, "symbolic link") {
						t.Fatalf("%q = %q %q %v", args, stdout, stderr, err)
					}
				}
			} else if component == index.IndexDir {
				stdout, stderr, err := runAt(cwd, "--root", source, "--out-dir", out, "index", "generate")
				if err == nil || stdout != "" || !strings.Contains(stderr, "symbolic link") {
					t.Fatalf("generate = %q %q %v", stdout, stderr, err)
				}
			}
			if isDir {
				entries, err := os.ReadDir(target)
				if err != nil || len(entries) != 0 {
					t.Fatalf("symlink destination modified: %v %v", entries, err)
				}
			} else if string(readFile(t, target)) != "original" {
				t.Fatal("symlink target modified")
			}
		})
	}
	t.Run("control directory file", func(t *testing.T) {
		out := t.TempDir()
		writeProjectFile(t, out, ".nodex", "original")
		for _, args := range [][]string{{"index", "generate"}, {"index", "status"}, {"ignore", "list"}, {"ignore", "enable", "go:tests"}} {
			stdout, stderr, err := runAt(cwd, append([]string{"--root", source, "--out-dir", out}, args...)...)
			if err == nil || stdout != "" || !strings.Contains(stderr, ".nodex is not a directory") {
				t.Fatalf("%q = %q %q %v", args, stdout, stderr, err)
			}
		}
		if string(readFile(t, filepath.Join(out, ".nodex"))) != "original" {
			t.Fatal("control conflict modified")
		}
	})
}

func TestDetachedBinary(t *testing.T) {
	binary := filepath.Join(t.TempDir(), "nodex")
	if output, err := exec.Command("go", "build", "-o", binary, ".").CombinedOutput(); err != nil {
		t.Fatalf("build: %v\n%s", err, output)
	}
	source, workspace, out := t.TempDir(), t.TempDir(), t.TempDir()
	writeProjectFile(t, source, "a.go", "// alpha\npackage a\n")
	writeProjectFile(t, source, "a_test.go", "// test\npackage a\n")
	runBinary := func(cwd string, args ...string) string {
		t.Helper()
		command := exec.Command(binary, args...)
		command.Dir = cwd
		output, err := command.CombinedOutput()
		if err != nil {
			t.Fatalf("binary %q: %v\n%s", args, err, output)
		}
		return string(output)
	}
	runBinary(workspace, "--root", source, "index", "generate")
	if got := runBinary(workspace, "--root", source, "index", "status"); got != statusCurrent(2, 2) {
		t.Fatalf("status = %q", got)
	}
	loadIndex(t, workspace)
	runBinary(workspace, "--root", source, "ignore", "enable", "go:tests")
	assertIgnore(t, workspace, []string{"go:tests"}, nil)
	runBinary(workspace, "--root", source, "index", "generate")
	_, snap := loadIndex(t, workspace)
	if pathsOf(snap) != "a.go" {
		t.Fatalf("temporary ignores = %s", pathsOf(snap))
	}
	for _, dir := range []string{source, out} {
		if _, err := os.Lstat(filepath.Join(dir, ".nodex")); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("unexpected state in %s", dir)
		}
	}
	otherCWD := t.TempDir()
	runBinary(otherCWD, "--root", source, "--out-dir", out, "index", "generate")
	if got := runBinary(otherCWD, "--root", source, "--out-dir", out, "index", "status"); got != statusCurrent(2, 2) {
		t.Fatalf("override = %q", got)
	}
	loadIndex(t, out)
	for _, dir := range []string{source, otherCWD} {
		if _, err := os.Lstat(filepath.Join(dir, ".nodex")); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("unexpected state in %s", dir)
		}
	}
}

func TestDiscoveryFilters(t *testing.T) {
	for _, mode := range []string{"local", "detached", "out-dir"} {
		t.Run(mode, func(t *testing.T) {
			source, workspace, cwd := t.TempDir(), t.TempDir(), t.TempDir()
			files := map[string]string{
				"internal/governance/g.go":     "// Governance\npackage p\ntype ProjectAuthority struct{}\nfunc Service() {}\ntype Other struct{}\n",
				"internal/history-old/o.go":    "// Old\npackage p\ntype Service struct{}\n",
				"internal/history/a.go":        "// History\npackage p\ntype Service struct { Other }\nfunc (s Service) ProjectAuthority() {}\nvar A, B int\nfunc ABC() {}\nfunc ParseA() {}\nfunc AValue() {}\n",
				"internal/history/nested/b.go": "// Nested\npackage p\ntype ProjectAuthority struct{}\n",
				"internal/history2/x.go":       "// Neighbor\npackage p\ntype Service struct{}\n",
				"outside/o.go":                 "// Outside\npackage p\ntype Service struct{}\n",
			}
			for path, body := range files {
				writeProjectFile(t, source, path, body)
			}
			var options []string
			switch mode {
			case "local":
				cwd, workspace = source, source
			case "detached":
				cwd = workspace
				options = []string{"--root", source}
			case "out-dir":
				options = []string{"--root", source, "--out-dir", workspace}
			}
			invoke := func(args ...string) string {
				t.Helper()
				return runOK(t, cwd, append(slices.Clone(options), append([]string{"index"}, args...)...)...)
			}
			invoke("generate")
			before, tree := pairBytes(t, workspace), nodexTree(t, workspace)

			commentFacts := []struct{ path, text string }{
				{"internal/governance/g.go", "Governance"},
				{"internal/history-old/o.go", "Old"},
				{"internal/history/a.go", "History"},
				{"internal/history/nested/b.go", "Nested"},
				{"internal/history2/x.go", "Neighbor"},
				{"outside/o.go", "Outside"},
			}
			wantComments := func(ordinals ...int) string {
				var blocks []string
				for _, n := range ordinals {
					fact := commentFacts[n-1]
					blocks = append(blocks, fmt.Sprintf("## C%06d\n\nfile: `%s`\n\n%s\n", n, fact.path, fact.text))
				}
				return strings.Join(blocks, "\n")
			}
			declarationFacts := []struct{ path, kind, names, doc string }{
				{"internal/governance/g.go", "package", "p", "C000001"},
				{"internal/governance/g.go", "type", "ProjectAuthority", "none"},
				{"internal/governance/g.go", "function", "Service", "none"},
				{"internal/governance/g.go", "type", "Other", "none"},
				{"internal/history-old/o.go", "package", "p", "C000002"},
				{"internal/history-old/o.go", "type", "Service", "none"},
				{"internal/history/a.go", "package", "p", "C000003"},
				{"internal/history/a.go", "type", "Service", "none"},
				{"internal/history/a.go", "field", "", "none"},
				{"internal/history/a.go", "method", "ProjectAuthority", "none"},
				{"internal/history/a.go", "var", "A, B", "none"},
				{"internal/history/a.go", "function", "ABC", "none"},
				{"internal/history/a.go", "function", "ParseA", "none"},
				{"internal/history/a.go", "function", "AValue", "none"},
				{"internal/history/nested/b.go", "package", "p", "C000004"},
				{"internal/history/nested/b.go", "type", "ProjectAuthority", "none"},
				{"internal/history2/x.go", "package", "p", "C000005"},
				{"internal/history2/x.go", "type", "Service", "none"},
				{"outside/o.go", "package", "p", "C000006"},
				{"outside/o.go", "type", "Service", "none"},
			}
			wantDeclarations := func(ordinals ...int) string {
				var blocks []string
				for _, n := range ordinals {
					fact := declarationFacts[n-1]
					names := "names:"
					if fact.names != "" {
						names += " " + fact.names
					}
					blocks = append(blocks, fmt.Sprintf("## D%06d\n\nfile: `%s`\nkind: %s\n%s\ndoc: %s\n", n, fact.path, fact.kind, names, fact.doc))
				}
				return strings.Join(blocks, "\n")
			}
			for _, tc := range []struct {
				name string
				args []string
				want string
			}{
				{"unfiltered comments", []string{"comments"}, wantComments(1, 2, 3, 4, 5, 6)},
				{"unfiltered declarations", []string{"declarations"}, wantDeclarations(1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20)},
				{"exact file", []string{"comments", "--file", "internal/history/a.go"}, wantComments(3)},
				{"subtree and boundaries", []string{"comments", "--file", "internal/history"}, wantComments(3, 4)},
				{"file OR stable order", []string{"comments", "--file", "internal/history", "internal/governance"}, wantComments(1, 3, 4)},
				{"duplicate files", []string{"comments", "--file", "internal/history", "internal/history", "internal/governance", "internal/history"}, wantComments(1, 3, 4)},
				{"overlapping files", []string{"comments", "--file", "internal/history/a.go", "internal/history"}, wantComments(3, 4)},
				{"nonexistent file", []string{"comments", "--file", "missing/file.go"}, ""},
				{"literal glob", []string{"comments", "--file", "*.go", "internal/**", "history"}, ""},
				{"file case", []string{"comments", "--file", "internal/History"}, ""},
				{"declaration file", []string{"declarations", "--file", "internal/history/a.go"}, wantDeclarations(7, 8, 9, 10, 11, 12, 13, 14)},
				{"declaration subtree", []string{"declarations", "--file", "internal/history"}, wantDeclarations(7, 8, 9, 10, 11, 12, 13, 14, 15, 16)},
				{"kind", []string{"declarations", "--kind", "method"}, wantDeclarations(10)},
				{"kind OR", []string{"declarations", "--kind", "type", "method"}, wantDeclarations(2, 4, 6, 8, 10, 16, 18, 20)},
				{"duplicate kinds", []string{"declarations", "--kind", "type", "type", "method", "type"}, wantDeclarations(2, 4, 6, 8, 10, 16, 18, 20)},
				{"unknown kind", []string{"declarations", "--kind", "future-kind"}, ""},
				{"kind case and no aliases", []string{"declarations", "--kind", "Type", "func", "struct", "interface"}, ""},
				{"no comma syntax", []string{"declarations", "--kind", "type,method"}, ""},
				{"name exact no substrings", []string{"declarations", "--name", "A"}, wantDeclarations(11)},
				{"second indexed name", []string{"declarations", "--name", "B"}, wantDeclarations(11)},
				{"name OR", []string{"declarations", "--name", "Service", "ProjectAuthority"}, wantDeclarations(2, 3, 6, 8, 10, 16, 18, 20)},
				{"duplicate names", []string{"declarations", "--name", "Service", "ProjectAuthority", "Service", "ProjectAuthority"}, wantDeclarations(2, 3, 6, 8, 10, 16, 18, 20)},
				{"two matching names one fact", []string{"declarations", "--name", "B", "A", "B"}, wantDeclarations(11)},
				{"name case", []string{"declarations", "--name", "service"}, ""},
				{"name no match", []string{"declarations", "--name", "DoesNotExist"}, ""},
				{"nameless visible", []string{"declarations", "--kind", "field"}, wantDeclarations(9)},
				{"nameless rejected by name", []string{"declarations", "--kind", "field", "--name", "Other"}, ""},
				{"combined AND", []string{"declarations", "--file", "internal/history", "internal/governance", "--kind", "type", "method", "--name", "Service", "ProjectAuthority"}, wantDeclarations(2, 8, 10, 16)},
				{"equivalent flag and value order", []string{"declarations", "--name", "ProjectAuthority", "Service", "--kind", "method", "type", "--file", "internal/governance", "internal/history"}, wantDeclarations(2, 8, 10, 16)},
				{"duplicates across categories", []string{"declarations", "--file", "internal/history", "internal/governance", "internal/history", "--kind", "type", "type", "method", "--name", "Service", "ProjectAuthority", "Service"}, wantDeclarations(2, 8, 10, 16)},
				{"nonexistent declaration file", []string{"declarations", "--file", "missing/file.go"}, ""},
				{"equals single value", []string{"declarations", "--kind=type", "method", "--name=ProjectAuthority"}, wantDeclarations(2, 10, 16)},
			} {
				t.Run(tc.name, func(t *testing.T) {
					if got := invoke(tc.args...); got != tc.want {
						t.Fatalf("%v = %q\nwant %q", tc.args, got, tc.want)
					}
				})
			}
			shown := invoke("show", "D000010")
			const wantShow = "## D000010\n\nfile: `internal/history/a.go`\nlines: 4\nkind: method\nnames: ProjectAuthority\ndoc: none\n\n### Context\n\n```go\nfunc (s Service) ProjectAuthority() {}\n```\n"
			if shown != wantShow {
				t.Fatalf("show = %q, want %q", shown, wantShow)
			}
			requireSameIndex(t, workspace, before)
			if !slices.Equal(nodexTree(t, workspace), tree) {
				t.Fatal("discovery changed workspace tree")
			}
			if mode != "local" {
				if _, err := os.Stat(filepath.Join(source, ".nodex")); !errors.Is(err, os.ErrNotExist) {
					t.Fatal("detached discovery created source state")
				}
			}
		})
	}
}

func TestDiscoveryFilterUsage(t *testing.T) {
	for _, command := range []string{"comments", "declarations"} {
		cases := [][]string{
			{"extra"}, {"--file"}, {"--file", ""},
			{"--file", "A", "--file", "B"}, {"--file=A", "--file=B"},
			{"--file", "/absolute/path"}, {"--file", "../internal/history"},
			{"--file", "internal/../../other"}, {"--file", "internal/../history"},
			{"--file", "."}, {"--file", "./internal/history"},
			{"--file", "internal//history"}, {"--file", "internal/history/"},
			{"--file", `internal\history`}, {"--file", "bad\x00path"},
			{"--file", string([]byte{0xff})},
			{"--file", "internal/history", "--package", "history"},
			{"--root", "source"}, {"--out-dir", "workspace"}, {"-x"},
		}
		for _, flag := range []string{"--package", "--language", "--text", "--contains", "--regex", "--glob", "--prefix", "--doc", "--documented", "--undocumented", "--limit", "--offset"} {
			cases = append(cases, []string{flag, "value"})
		}
		if command == "comments" {
			cases = append(cases, []string{"--kind", "type"}, []string{"--name", "Service"}, []string{"--file", "A", "--kind", "type"})
		} else {
			cases = append(cases, []string{"--kind"}, []string{"--name"}, []string{"--kind", "--name", "A"}, []string{"--file", "--kind", "type"},
				[]string{"--kind", "type", "--kind", "method"}, []string{"--name", "A", "--name", "B"}, []string{"--kind="}, []string{"--name", ""})
		}
		for _, args := range cases {
			t.Run(command+" "+strings.Join(args, " "), func(t *testing.T) {
				root := t.TempDir()
				var out, errOut bytes.Buffer
				opened := false
				err := run(append([]string{"index", command}, args...), func() (string, error) {
					opened = true
					return root, nil
				}, &out, &errOut)
				if err == nil || err.Error() != "usage" || out.Len() != 0 || errOut.Len() == 0 || opened {
					t.Fatalf("usage = %q %q %v, opened=%v", out.String(), errOut.String(), err, opened)
				}
				entries, err := os.ReadDir(root)
				if err != nil || len(entries) != 0 {
					t.Fatalf("usage changed filesystem: %v %v", entries, err)
				}
			})
		}
	}
	filters, err := parseDiscoveryFilters("declarations", []string{"--file", "B", "A", "B", "--kind", "method", "type", "method", "--name", "Z", "A", "Z"})
	if err != nil || !slices.Equal(filters.files, []string{"B", "A"}) || !slices.Equal(filters.kinds, []string{"method", "type"}) || !slices.Equal(filters.names, []string{"Z", "A"}) {
		t.Fatalf("normalized selectors = %+v, %v", filters, err)
	}
}

func TestFilteredDiscoveryLifecycle(t *testing.T) {
	for _, state := range []string{"missing", "stale", "corrupt"} {
		t.Run(state, func(t *testing.T) {
			root := t.TempDir()
			writeProjectFile(t, root, "a.go", "// hello\npackage p\n")
			var before []byte
			var tree []string
			if state != "missing" {
				generateOK(t, root)
				if state == "stale" {
					writeProjectFile(t, root, "a.go", "// changed\npackage p\n")
				} else {
					writeProjectFile(t, root, index.DeclarationsPath, "corrupt\n")
				}
				before, tree = pairBytes(t, root), nodexTree(t, root)
			}
			for _, args := range [][]string{
				{"comments", "--file", "nonexistent"},
				{"declarations", "--file", "nonexistent", "--kind", "future-kind", "--name", "DoesNotExist"},
			} {
				stdout, stderr, err := runAt(root, append([]string{"index"}, args...)...)
				message := state
				if state == "missing" {
					message = "no generated index"
				}
				if err == nil || stdout != "" || !strings.Contains(stderr, message) {
					t.Fatalf("%v = %q %q %v", args, stdout, stderr, err)
				}
			}
			if state == "missing" {
				if _, err := os.Stat(filepath.Join(root, ".nodex")); !errors.Is(err, os.ErrNotExist) {
					t.Fatal("missing discovery created state")
				}
			} else {
				requireSameIndex(t, root, before)
				if !slices.Equal(nodexTree(t, root), tree) {
					t.Fatal("failed discovery changed state tree")
				}
			}
		})
	}
}
