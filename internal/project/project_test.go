package project_test

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
	"testing"

	"github.com/edalca/nodex/internal/ignore"
	"github.com/edalca/nodex/internal/project"
)

// ignore.Policy satisfies the exclusion port project discovery consumes.
var _ project.ExclusionPolicy = ignore.Policy{}

func TestOpenValidDirectory(t *testing.T) {
	dir := t.TempDir()
	p, err := project.Open(dir)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	want, err := filepath.EvalSymlinks(dir)
	if err != nil {
		t.Fatalf("EvalSymlinks: %v", err)
	}
	if p.Root() != want {
		t.Fatalf("Root() = %q, want %q", p.Root(), want)
	}
	if !filepath.IsAbs(p.Root()) {
		t.Fatalf("Root() = %q, want absolute", p.Root())
	}
	files, err := p.Files(ignore.Policy{})
	if err != nil {
		t.Fatalf("Files: %v", err)
	}
	if files == nil {
		t.Fatal("Files() = nil, want empty slice")
	}
	if len(files) != 0 {
		t.Fatalf("Files() = %q, want none", files)
	}
}

func TestOpenRejectsMissingRoot(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "missing")
	p, err := project.Open(missing)
	if err == nil || p != nil {
		t.Fatalf("Open(%q) = (%v, %v), want error", missing, p, err)
	}
	if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("Open(%q) error = %v, want not exist", missing, err)
	}
}

func TestOpenRejectsRegularFile(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "file.txt")
	writeFile(t, file, "not a directory")
	p, err := project.Open(file)
	if err == nil || p != nil {
		t.Fatalf("Open(%q) = (%v, %v), want error", file, p, err)
	}
	if errors.Is(err, os.ErrNotExist) {
		t.Fatalf("Open(%q) error = %v, want a present non-directory", file, err)
	}
}

func TestOpenRejectsEmptyPath(t *testing.T) {
	p, err := project.Open("")
	if err == nil || p != nil {
		t.Fatalf("Open empty = (%v, %v), want error", p, err)
	}
}

func TestOpenRejectsBrokenSymlink(t *testing.T) {
	dir := t.TempDir()
	link := filepath.Join(dir, "broken")
	if err := os.Symlink(filepath.Join(dir, "missing"), link); err != nil {
		t.Fatalf("symlink: %v", err)
	}
	p, err := project.Open(link)
	if err == nil || p != nil {
		t.Fatalf("Open(%q) = (%v, %v), want error", link, p, err)
	}
}

func TestOpenRejectsSymlinkToFile(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "file.txt")
	writeFile(t, file, "x")
	link := filepath.Join(dir, "link")
	if err := os.Symlink(file, link); err != nil {
		t.Fatalf("symlink: %v", err)
	}
	p, err := project.Open(link)
	if err == nil || p != nil {
		t.Fatalf("Open(%q) = (%v, %v), want error", link, p, err)
	}
}

func TestOpenNormalizesRoot(t *testing.T) {
	parent := t.TempDir()
	root := filepath.Join(parent, "proj")
	if err := os.Mkdir(root, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	want, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatalf("EvalSymlinks: %v", err)
	}
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}
	rel, err := filepath.Rel(wd, root)
	if err != nil {
		t.Fatalf("Rel: %v", err)
	}
	sep := string(filepath.Separator)
	messy := rel + sep + ".." + sep + filepath.Base(root) + sep + "."
	withDot := parent + sep + "." + sep + "proj" + sep + "."
	trailing := root + sep

	for _, candidate := range []string{messy, withDot, trailing} {
		p, err := project.Open(candidate)
		if err != nil {
			t.Fatalf("Open(%q): %v", candidate, err)
		}
		if !filepath.IsAbs(p.Root()) {
			t.Fatalf("Open(%q) Root() = %q, want absolute", candidate, p.Root())
		}
		if p.Root() != want {
			t.Fatalf("Open(%q) Root() = %q, want %q", candidate, p.Root(), want)
		}
	}
}

func TestOpenResolvesDirectorySymlink(t *testing.T) {
	target := t.TempDir()
	link := filepath.Join(t.TempDir(), "link")
	if err := os.Symlink(target, link); err != nil {
		t.Fatalf("symlink: %v", err)
	}
	p, err := project.Open(link)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	want, err := filepath.EvalSymlinks(target)
	if err != nil {
		t.Fatalf("EvalSymlinks: %v", err)
	}
	if p.Root() != want {
		t.Fatalf("Root() = %q, want %q", p.Root(), want)
	}
}

func TestFilesLexicalOrderRelativeAndRecursive(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "b.rs"), "rs")
	writeFile(t, filepath.Join(root, "dir", "z.go"), "go")
	writeFile(t, filepath.Join(root, "a.py"), "py")
	writeFile(t, filepath.Join(root, "config.yaml"), "yaml")
	writeFile(t, filepath.Join(root, "dir", "a.md"), "md")
	writeFile(t, filepath.Join(root, "README.md"), "md")

	files := discover(t, root, ignore.Policy{})
	want := []string{
		"README.md",
		"a.py",
		"b.rs",
		"config.yaml",
		"dir/a.md",
		"dir/z.go",
	}
	if !slices.Equal(files, want) {
		t.Fatalf("Files() = %q, want %q", files, want)
	}
	for _, rel := range files {
		if filepath.IsAbs(rel) {
			t.Fatalf("path %q is absolute", rel)
		}
	}
}

func TestFilesExcludesControlDirectories(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "keep.txt"), "keep")
	writeFile(t, filepath.Join(root, "sub", "keep.txt"), "keep")
	writeFile(t, filepath.Join(root, ".gitignore"), "not a directory")
	writeFile(t, filepath.Join(root, ".nodex-notes"), "not a directory")
	writeFile(t, filepath.Join(root, ".outputs.txt"), "not a directory")
	writeFile(t, filepath.Join(root, ".github", "workflows", "ci.yml"), "ci")
	writeFile(t, filepath.Join(root, ".git", "HEAD"), "ref")
	writeFile(t, filepath.Join(root, ".git", "objects", "ab"), "obj")
	writeFile(t, filepath.Join(root, ".nodex", "state"), "state")
	writeFile(t, filepath.Join(root, "sub", ".git", "config"), "cfg")
	writeFile(t, filepath.Join(root, "sub", ".nodex", "cache"), "cache")

	files := discover(t, root, ignore.Policy{})
	want := []string{
		".github/workflows/ci.yml",
		".gitignore",
		".nodex-notes",
		".outputs.txt",
		"keep.txt",
		"sub/keep.txt",
	}
	if !slices.Equal(files, want) {
		t.Fatalf("Files() = %q, want %q", files, want)
	}
}

func TestFilesTraversesOutputsDirectory(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "notes", "example.go"), "package p")
	writeFile(t, filepath.Join(root, ".outputs", "example.go"), "package p")
	writeFile(t, filepath.Join(root, ".outputs", "nested", "note.md"), "note")
	writeFile(t, filepath.Join(root, "sub", ".outputs", "log"), "log")
	writeFile(t, filepath.Join(root, ".outputs", ".git", "HEAD"), "ref")
	writeFile(t, filepath.Join(root, ".outputs", ".nodex", "state"), "state")

	files := discover(t, root, ignore.Policy{})
	want := []string{
		".outputs/example.go",
		".outputs/nested/note.md",
		"notes/example.go",
		"sub/.outputs/log",
	}
	if !slices.Equal(files, want) {
		t.Fatalf("Files() = %q, want %q", files, want)
	}
}

func TestFilesSkipsSymlinksAndDoesNotEscape(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	writeFile(t, filepath.Join(outside, "secret.txt"), "secret")
	writeFile(t, filepath.Join(outside, "nested", "deep.txt"), "secret")
	writeFile(t, filepath.Join(root, "real", "file.txt"), "inside")
	writeFile(t, filepath.Join(root, "keep.txt"), "keep")

	links := []struct {
		name   string
		target string
	}{
		{name: "alias.txt", target: filepath.Join(root, "real", "file.txt")},
		{name: "external.txt", target: filepath.Join(outside, "secret.txt")},
		{name: "linked-dir", target: outside},
		{name: "inside-alias", target: filepath.Join(root, "real")},
		{name: "loop", target: root},
		{name: "rel-file", target: filepath.Join("..", filepath.Base(outside), "secret.txt")},
		{name: "rel-dir", target: filepath.Join("..", filepath.Base(outside))},
		{name: "broken.txt", target: filepath.Join(root, "missing-target")},
		{name: "broken-dir", target: filepath.Join(root, "missing-dir")},
	}
	for _, link := range links {
		if err := os.Symlink(link.target, filepath.Join(root, link.name)); err != nil {
			t.Fatalf("symlink %s: %v", link.name, err)
		}
	}

	files := discover(t, root, ignore.Policy{})
	want := []string{
		"keep.txt",
		"real/file.txt",
	}
	if !slices.Equal(files, want) {
		t.Fatalf("Files() = %q, want %q", files, want)
	}
}

func TestFilesSkipsNonRegularFiles(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "a.txt"), "a")
	if err := syscall.Mkfifo(filepath.Join(root, "pipe"), 0o644); err != nil {
		t.Fatalf("mkfifo: %v", err)
	}
	writeFile(t, filepath.Join(root, "z.txt"), "z")

	files := discover(t, root, ignore.Policy{})
	want := []string{"a.txt", "z.txt"}
	if !slices.Equal(files, want) {
		t.Fatalf("Files() = %q, want %q", files, want)
	}
}

func discover(t *testing.T, root string, policy project.ExclusionPolicy) []string {
	t.Helper()
	p := mustOpen(t, root)
	files, err := p.Files(policy)
	if err != nil {
		t.Fatalf("Files: %v", err)
	}
	again, err := p.Files(policy)
	if err != nil {
		t.Fatalf("Files again: %v", err)
	}
	if !slices.Equal(files, again) {
		t.Fatalf("Files() = %q then %q", files, again)
	}
	if files == nil {
		t.Fatal("Files() = nil, want a slice")
	}
	for i, rel := range files {
		if filepath.IsAbs(rel) {
			t.Fatalf("path %q is absolute", rel)
		}
		if rel != filepath.ToSlash(filepath.Clean(rel)) || !filepath.IsLocal(rel) {
			t.Fatalf("path %q is not a normalized project-relative path", rel)
		}
		if i > 0 && rel < files[i-1] {
			t.Fatalf("Files() not sorted at %q", rel)
		}
		abs := filepath.Join(p.Root(), filepath.FromSlash(rel))
		resolved, err := filepath.EvalSymlinks(abs)
		if err != nil {
			t.Fatalf("EvalSymlinks %s: %v", rel, err)
		}
		back, err := filepath.Rel(p.Root(), resolved)
		if err != nil {
			t.Fatalf("Rel %s: %v", resolved, err)
		}
		if !filepath.IsLocal(back) {
			t.Fatalf("resolved %q to %q, outside %q", rel, resolved, p.Root())
		}
	}
	return files
}

func TestFilesExcludesOutputsWhenPolicyDoes(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "keep.go"), "package p")
	writeFile(t, filepath.Join(root, ".outputs", "example.go"), "package p")
	writeFile(t, filepath.Join(root, ".outputs", "nested", "note.md"), "note")
	writeFile(t, filepath.Join(root, "notes", "example.go"), "package p")

	wantAll := []string{
		".outputs/example.go",
		".outputs/nested/note.md",
		"keep.go",
		"notes/example.go",
	}
	for _, policy := range []project.ExclusionPolicy{
		ignore.Policy{},
		mustParse(t, `{
  "schema": 1,
  "presets": [],
  "exclude": []
}`),
	} {
		files := discover(t, root, policy)
		if !slices.Equal(files, wantAll) {
			t.Fatalf("empty policy Files() = %q, want %q", files, wantAll)
		}
	}

	policy := mustParse(t, `{
  "schema": 1,
  "presets": [],
  "exclude": [
    ".outputs/"
  ]
}`)
	files := discover(t, root, policy)
	want := []string{
		"keep.go",
		"notes/example.go",
	}
	if !slices.Equal(files, want) {
		t.Fatalf("outputs policy Files() = %q, want %q", files, want)
	}
}

func TestFilesPrunesExcludedDirectorySubtree(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "keep.go"), "package p")
	writeFile(t, filepath.Join(root, "src", "also.go"), "package p")
	writeFile(t, filepath.Join(root, "vendor", "lib.go"), "package p")
	writeFile(t, filepath.Join(root, "vendor", "nested", "deep.go"), "package p")

	policy := mustParse(t, `{
  "schema": 1,
  "presets": [],
  "exclude": [
    "vendor/"
  ]
}`)
	files := discover(t, root, policy)
	want := []string{"keep.go", "src/also.go"}
	if !slices.Equal(files, want) {
		t.Fatalf("Files() = %q, want %q", files, want)
	}
}

func TestFilesKeepsNegatedFileUnderTraversableDirectory(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "keep.go"), "package p")
	writeFile(t, filepath.Join(root, "vendor", "lib.go"), "package p")
	writeFile(t, filepath.Join(root, "vendor", "patched.go"), "package p")
	writeFile(t, filepath.Join(root, "vendor", "nested", "skip.go"), "package p")

	policy := mustParse(t, `{
  "schema": 1,
  "presets": [],
  "exclude": [
    "vendor/*",
    "!vendor/patched.go"
  ]
}`)
	files := discover(t, root, policy)
	want := []string{"keep.go", "vendor/patched.go"}
	if !slices.Equal(files, want) {
		t.Fatalf("Files() = %q, want %q", files, want)
	}
}

func TestFilesDoesNotDiscoverFileUnderSealedDirectory(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "keep.go"), "package p")
	writeFile(t, filepath.Join(root, "vendor", "patched.go"), "package p")
	writeFile(t, filepath.Join(root, "vendor", "lib.go"), "package p")
	writeFile(t, filepath.Join(root, "vendor", "nested", "deep.go"), "package p")

	policy := mustParse(t, `{
  "schema": 1,
  "presets": [],
  "exclude": [
    "vendor/",
    "!vendor/patched.go"
  ]
}`)
	files := discover(t, root, policy)
	want := []string{"keep.go"}
	if !slices.Equal(files, want) {
		t.Fatalf("Files() = %q, want %q", files, want)
	}
}

func TestFilesExcludesMatchingFilesWithoutPruningParent(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "keep.go"), "package p")
	writeFile(t, filepath.Join(root, "notes.txt"), "note")
	writeFile(t, filepath.Join(root, "secret.txt"), "secret")
	writeFile(t, filepath.Join(root, "dir", "keep.go"), "package p")
	writeFile(t, filepath.Join(root, "dir", "secret.txt"), "secret")
	writeFile(t, filepath.Join(root, "dir", "nested", "keep.go"), "package p")

	policy := mustParse(t, `{
  "schema": 1,
  "presets": [],
  "exclude": [
    "secret.txt"
  ]
}`)
	files := discover(t, root, policy)
	want := []string{
		"dir/keep.go",
		"dir/nested/keep.go",
		"keep.go",
		"notes.txt",
	}
	if !slices.Equal(files, want) {
		t.Fatalf("Files() = %q, want %q", files, want)
	}
}

func TestFilesDirectoryRuleDoesNotExcludeSameNamedFile(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "build"), "not a directory")
	writeFile(t, filepath.Join(root, "pkg", "keep.go"), "package p")
	writeFile(t, filepath.Join(root, "pkg", "build", "out.go"), "package p")

	policy := mustParse(t, `{
  "schema": 1,
  "presets": [],
  "exclude": [
    "build/"
  ]
}`)
	files := discover(t, root, policy)
	want := []string{"build", "pkg/keep.go"}
	if !slices.Equal(files, want) {
		t.Fatalf("Files() = %q, want %q", files, want)
	}
}

func TestFilesLexicalOrderSurvivesFiltering(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "a.go"), "a")
	writeFile(t, filepath.Join(root, "b.log"), "b")
	writeFile(t, filepath.Join(root, "c.go"), "c")
	writeFile(t, filepath.Join(root, "dir", "a.log"), "a")
	writeFile(t, filepath.Join(root, "dir", "m.go"), "m")
	writeFile(t, filepath.Join(root, "z.go"), "z")

	policy := mustParse(t, `{
  "schema": 1,
  "presets": [],
  "exclude": [
    "*.log"
  ]
}`)
	files := discover(t, root, policy)
	want := []string{
		"a.go",
		"c.go",
		"dir/m.go",
		"z.go",
	}
	if !slices.Equal(files, want) {
		t.Fatalf("Files() = %q, want %q", files, want)
	}
}

func TestFilesControlDirectoriesStayExcludedWhenPolicyIncludesThem(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "keep.go"), "keep")
	writeFile(t, filepath.Join(root, ".git", "HEAD"), "ref")
	writeFile(t, filepath.Join(root, ".git", "objects", "ab"), "obj")
	writeFile(t, filepath.Join(root, ".nodex", "state"), "state")
	writeFile(t, filepath.Join(root, "sub", "keep.go"), "keep")
	writeFile(t, filepath.Join(root, "sub", ".git", "config"), "cfg")
	writeFile(t, filepath.Join(root, "sub", ".nodex", "cache"), "cache")

	policy := mustParse(t, `{
  "schema": 1,
  "presets": [],
  "exclude": [
    "!.git/**",
    "!.nodex/**"
  ]
}`)
	included := []struct {
		path  string
		isDir bool
	}{
		{path: ".git", isDir: true},
		{path: ".git/HEAD", isDir: false},
		{path: ".git/objects", isDir: true},
		{path: ".git/objects/ab", isDir: false},
		{path: ".nodex", isDir: true},
		{path: ".nodex/state", isDir: false},
		{path: "sub/.git", isDir: true},
		{path: "sub/.git/config", isDir: false},
		{path: "sub/.nodex", isDir: true},
		{path: "sub/.nodex/cache", isDir: false},
	}
	for _, item := range included {
		if policy.Excluded(item.path, item.isDir) {
			t.Fatalf("Excluded(%q, dir=%v) = true, want false", item.path, item.isDir)
		}
	}

	files := discover(t, root, policy)
	want := []string{"keep.go", "sub/keep.go"}
	if !slices.Equal(files, want) {
		t.Fatalf("Files() = %q, want %q", files, want)
	}
}

func mustOpen(t *testing.T, root string) *project.Project {
	t.Helper()
	p, err := project.Open(root)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	return p
}

func mustParse(t *testing.T, document string) ignore.Policy {
	t.Helper()
	policy, err := ignore.Parse([]byte(document))
	if err != nil {
		t.Fatalf("Parse(%s): %v", document, err)
	}
	return policy
}

func writeFile(t *testing.T, path, data string) {
	t.Helper()
	writeBytes(t, path, []byte(data))
}

func writeBytes(t *testing.T, path string, data []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func TestReadFileExactBytes(t *testing.T) {
	root := t.TempDir()
	body := []byte("a\r\nb\x00c")
	writeBytes(t, filepath.Join(root, "dir", "sub", "a.go"), body)
	p := mustOpen(t, root)
	got, err := p.ReadFile("dir/sub/a.go")
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if !bytes.Equal(got, body) {
		t.Fatalf("ReadFile = %q, want %q", got, body)
	}
	files := discover(t, root, ignore.Policy{})
	if !slices.Equal(files, []string{"dir/sub/a.go"}) {
		t.Fatalf("Files() = %q", files)
	}
}

func TestReadFileRejectsInvalidPaths(t *testing.T) {
	root := t.TempDir()
	body := []byte("inside")
	writeBytes(t, filepath.Join(root, "a.go"), body)
	writeBytes(t, filepath.Join(root, "dir", "b.go"), body)
	outside := t.TempDir()
	writeBytes(t, filepath.Join(outside, "secret.txt"), []byte("secret"))
	writeBytes(t, filepath.Join(root, ".git", "HEAD"), []byte("ref"))
	writeBytes(t, filepath.Join(root, ".git", "objects", "ab"), []byte("obj"))
	writeBytes(t, filepath.Join(root, ".nodex", "ignore.json"), []byte("{}"))
	writeBytes(t, filepath.Join(root, "sub", ".git", "config"), []byte("cfg"))
	writeBytes(t, filepath.Join(root, "sub", ".nodex", "cache"), []byte("cache"))
	if err := os.Mkdir(filepath.Join(root, "empty-dir"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.Symlink(filepath.Join(root, "a.go"), filepath.Join(root, "link.go")); err != nil {
		t.Fatalf("symlink file: %v", err)
	}
	if err := os.Symlink(filepath.Join(root, "dir"), filepath.Join(root, "dir-link")); err != nil {
		t.Fatalf("symlink dir: %v", err)
	}
	if err := os.Symlink(filepath.Join(outside, "secret.txt"), filepath.Join(root, "outside.txt")); err != nil {
		t.Fatalf("symlink outside: %v", err)
	}
	if err := syscall.Mkfifo(filepath.Join(root, "pipe"), 0o644); err != nil {
		t.Fatalf("mkfifo: %v", err)
	}
	writeBytes(t, filepath.Join(root, ".gitfile"), []byte("not-control"))

	p := mustOpen(t, root)
	cases := []struct {
		path string
		want error
	}{
		{path: "", want: project.ErrEmptyPath},
		{path: ".", want: project.ErrDotPath},
		{path: "/etc/passwd", want: project.ErrAbsolutePath},
		{path: filepath.Join(root, "a.go"), want: project.ErrAbsolutePath},
		{path: "a/../a.go", want: project.ErrUncleanPath},
		{path: "./a.go", want: project.ErrUncleanPath},
		{path: "dir//b.go", want: project.ErrUncleanPath},
		{path: "dir/./b.go", want: project.ErrUncleanPath},
		{path: "../secret.txt", want: project.ErrEscapingPath},
		{path: "..", want: project.ErrEscapingPath},
		{path: "empty-dir", want: project.ErrDirectory},
		{path: "dir", want: project.ErrDirectory},
		{path: "link.go", want: project.ErrSymlink},
		{path: "dir-link/b.go", want: project.ErrSymlink},
		{path: "outside.txt", want: project.ErrSymlink},
		{path: "pipe", want: project.ErrNotRegular},
		{path: ".git", want: project.ErrControlPath},
		{path: ".git/HEAD", want: project.ErrControlPath},
		{path: ".git/objects/ab", want: project.ErrControlPath},
		{path: ".nodex", want: project.ErrControlPath},
		{path: ".nodex/ignore.json", want: project.ErrControlPath},
		{path: "sub/.git/config", want: project.ErrControlPath},
		{path: "sub/.nodex/cache", want: project.ErrControlPath},
	}
	for _, tc := range cases {
		t.Run(tc.path, func(t *testing.T) {
			got, err := p.ReadFile(tc.path)
			if err == nil || got != nil {
				t.Fatalf("ReadFile(%q) = %q, %v, want error", tc.path, got, err)
			}
			if !errors.Is(err, tc.want) {
				t.Fatalf("ReadFile(%q) error = %v, want %v", tc.path, err, tc.want)
			}
		})
	}
	got, err := p.ReadFile("a.go")
	if err != nil {
		t.Fatalf("ReadFile a.go: %v", err)
	}
	if !bytes.Equal(got, body) {
		t.Fatalf("ReadFile a.go = %q", got)
	}
	got, err = p.ReadFile(".gitfile")
	if err != nil {
		t.Fatalf("ReadFile .gitfile: %v", err)
	}
	if string(got) != "not-control" {
		t.Fatalf("ReadFile .gitfile = %q", got)
	}
	var none *project.Project
	if _, err := none.ReadFile("a.go"); err == nil {
		t.Fatal("nil project ReadFile succeeded")
	}
}

func TestResolve(t *testing.T) {
	t.Run("nodex in start", func(t *testing.T) {
		root := t.TempDir()
		if err := os.Mkdir(filepath.Join(root, ".nodex"), 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		if got := mustResolve(t, root); got != resolvedDir(t, root) {
			t.Fatalf("Resolve = %s, want %s", got, resolvedDir(t, root))
		}
	})
	t.Run("git directory in start", func(t *testing.T) {
		root := t.TempDir()
		if err := os.Mkdir(filepath.Join(root, ".git"), 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		if got := mustResolve(t, root); got != resolvedDir(t, root) {
			t.Fatalf("Resolve = %s", got)
		}
	})
	t.Run("ancestor nodex", func(t *testing.T) {
		root := t.TempDir()
		if err := os.Mkdir(filepath.Join(root, ".nodex"), 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		child := filepath.Join(root, "a", "b")
		if err := os.MkdirAll(child, 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		if got := mustResolve(t, child); got != resolvedDir(t, root) {
			t.Fatalf("Resolve = %s, want %s", got, resolvedDir(t, root))
		}
	})
	t.Run("ancestor git", func(t *testing.T) {
		root := t.TempDir()
		if err := os.Mkdir(filepath.Join(root, ".git"), 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		child := filepath.Join(root, "a", "b")
		if err := os.MkdirAll(child, 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		if got := mustResolve(t, child); got != resolvedDir(t, root) {
			t.Fatalf("Resolve = %s, want %s", got, resolvedDir(t, root))
		}
	})
	t.Run("nearer marker wins", func(t *testing.T) {
		root := t.TempDir()
		if err := os.Mkdir(filepath.Join(root, ".git"), 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		child := filepath.Join(root, "child")
		if err := os.MkdirAll(filepath.Join(child, ".git"), 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		if got := mustResolve(t, child); got != resolvedDir(t, child) {
			t.Fatalf("Resolve = %s, want child %s", got, resolvedDir(t, child))
		}
		if got := mustResolve(t, root); got != resolvedDir(t, root) {
			t.Fatalf("Resolve(parent) = %s", got)
		}
	})
	t.Run("nodex wins over git in the same directory", func(t *testing.T) {
		root := t.TempDir()
		if err := os.Mkdir(filepath.Join(root, ".git"), 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		child := filepath.Join(root, "child")
		if err := os.MkdirAll(filepath.Join(child, ".nodex"), 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		if err := os.WriteFile(filepath.Join(child, ".git"), []byte("gitdir: /somewhere\n"), 0o644); err != nil {
			t.Fatalf("write: %v", err)
		}
		if got := mustResolve(t, child); got != resolvedDir(t, child) {
			t.Fatalf("Resolve = %s, want %s", got, resolvedDir(t, child))
		}
	})
	t.Run("malformed nodex is not skipped for git", func(t *testing.T) {
		root := t.TempDir()
		if err := os.WriteFile(filepath.Join(root, ".nodex"), []byte("nope"), 0o644); err != nil {
			t.Fatalf("write: %v", err)
		}
		if err := os.Mkdir(filepath.Join(root, ".git"), 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		_, err := project.Resolve(root)
		if err == nil || !strings.Contains(err.Error(), ".nodex") {
			t.Fatalf("Resolve = %v", err)
		}
	})
	t.Run("gitdir file", func(t *testing.T) {
		root := t.TempDir()
		if err := os.WriteFile(filepath.Join(root, ".git"), []byte("gitdir: /repo/.git/worktrees/name\n"), 0o644); err != nil {
			t.Fatalf("write: %v", err)
		}
		if got := mustResolve(t, root); got != resolvedDir(t, root) {
			t.Fatalf("Resolve = %s", got)
		}
	})
	t.Run("nodex symlink", func(t *testing.T) {
		root := t.TempDir()
		outside := t.TempDir()
		if err := os.Mkdir(filepath.Join(outside, ".nodex"), 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		if err := os.Symlink(filepath.Join(outside, ".nodex"), filepath.Join(root, ".nodex")); err != nil {
			t.Fatalf("symlink: %v", err)
		}
		_, err := project.Resolve(root)
		if err == nil || !strings.Contains(err.Error(), "symbolic link") {
			t.Fatalf("Resolve = %v", err)
		}
	})
	t.Run("git symlink", func(t *testing.T) {
		root := t.TempDir()
		outside := t.TempDir()
		if err := os.Mkdir(filepath.Join(outside, ".git"), 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		if err := os.Symlink(filepath.Join(outside, ".git"), filepath.Join(root, ".git")); err != nil {
			t.Fatalf("symlink: %v", err)
		}
		_, err := project.Resolve(root)
		if err == nil || !strings.Contains(err.Error(), "symbolic link") {
			t.Fatalf("Resolve = %v", err)
		}
	})
	t.Run("nodex file", func(t *testing.T) {
		root := t.TempDir()
		if err := os.WriteFile(filepath.Join(root, ".nodex"), []byte("x"), 0o644); err != nil {
			t.Fatalf("write: %v", err)
		}
		_, err := project.Resolve(root)
		if err == nil || !strings.Contains(err.Error(), ".nodex") {
			t.Fatalf("Resolve = %v", err)
		}
	})
	t.Run("git fifo", func(t *testing.T) {
		root := t.TempDir()
		if err := syscall.Mkfifo(filepath.Join(root, ".git"), 0o644); err != nil {
			t.Fatalf("mkfifo: %v", err)
		}
		_, err := project.Resolve(root)
		if err == nil || !strings.Contains(err.Error(), ".git") {
			t.Fatalf("Resolve = %v", err)
		}
	})
	t.Run("no marker uses start", func(t *testing.T) {
		root := t.TempDir()
		child := filepath.Join(root, "child")
		if err := os.Mkdir(child, 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		if got := mustResolve(t, child); got != resolvedDir(t, child) {
			t.Fatalf("Resolve = %s, want %s", got, resolvedDir(t, child))
		}
	})
	t.Run("go.mod is not a root", func(t *testing.T) {
		root := t.TempDir()
		if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/root\n\ngo 1.27.1\n"), 0o644); err != nil {
			t.Fatalf("write: %v", err)
		}
		child := filepath.Join(root, "mod")
		if err := os.Mkdir(child, 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		if err := os.WriteFile(filepath.Join(child, "go.mod"), []byte("module example.com/mod\n\ngo 1.27.1\n"), 0o644); err != nil {
			t.Fatalf("write: %v", err)
		}
		if got := mustResolve(t, child); got != resolvedDir(t, child) {
			t.Fatalf("Resolve = %s, want the start directory", got)
		}
	})
	t.Run("go.work is not several roots", func(t *testing.T) {
		root := t.TempDir()
		if err := os.WriteFile(filepath.Join(root, "go.work"), []byte("go 1.27.1\n\nuse ./mod\n"), 0o644); err != nil {
			t.Fatalf("write: %v", err)
		}
		child := filepath.Join(root, "mod")
		if err := os.Mkdir(child, 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		if err := os.WriteFile(filepath.Join(child, "go.mod"), []byte("module example.com/mod\n\ngo 1.27.1\n"), 0o644); err != nil {
			t.Fatalf("write: %v", err)
		}
		if got := mustResolve(t, child); got != resolvedDir(t, child) {
			t.Fatalf("Resolve without a marker = %s", got)
		}
		if err := os.Mkdir(filepath.Join(root, ".git"), 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		if got := mustResolve(t, child); got != resolvedDir(t, root) {
			t.Fatalf("Resolve with .git = %s, want one workspace root", got)
		}
	})
	t.Run("start symlink resolves before the walk", func(t *testing.T) {
		root := t.TempDir()
		if err := os.Mkdir(filepath.Join(root, ".nodex"), 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		link := filepath.Join(t.TempDir(), "link")
		if err := os.Symlink(root, link); err != nil {
			t.Fatalf("symlink: %v", err)
		}
		if got := mustResolve(t, link); got != resolvedDir(t, root) {
			t.Fatalf("Resolve = %s, want %s", got, resolvedDir(t, root))
		}
	})
	t.Run("empty", func(t *testing.T) {
		if _, err := project.Resolve(""); err == nil {
			t.Fatal("empty path was accepted")
		}
	})
}

func mustResolve(t *testing.T, start string) string {
	t.Helper()
	got, err := project.Resolve(start)
	if err != nil {
		t.Fatalf("Resolve(%s): %v", start, err)
	}
	opened, err := project.Open(got)
	if err != nil {
		t.Fatalf("Open(Resolve): %v", err)
	}
	if opened.Root() != got {
		t.Fatalf("Open = %s, Resolve = %s", opened.Root(), got)
	}
	return got
}

func resolvedDir(t *testing.T, dir string) string {
	t.Helper()
	got, err := filepath.EvalSymlinks(dir)
	if err != nil {
		t.Fatalf("EvalSymlinks %s: %v", dir, err)
	}
	return filepath.Clean(got)
}
