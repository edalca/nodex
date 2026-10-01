package skill

import (
	"bytes"
	"errors"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"syscall"
	"testing"
	"unicode/utf8"
)

func TestCanonicalSkill(t *testing.T) {
	doc := Document()
	if len(doc) == 0 {
		t.Fatal("embedded skill is empty")
	}
	if !utf8.Valid(doc) {
		t.Fatal("embedded skill is not valid UTF-8")
	}
	if bytes.Contains(doc, []byte{'\r'}) {
		t.Fatal("embedded skill contains CR")
	}
	if !bytes.HasSuffix(doc, []byte("\n")) || bytes.HasSuffix(doc, []byte("\n\n")) {
		t.Fatal("embedded skill must end with exactly one newline")
	}
	if !bytes.HasPrefix(doc, []byte("---\n")) {
		t.Fatal("frontmatter does not begin at byte zero")
	}
	rest := doc[len("---\n"):]
	end := bytes.Index(rest, []byte("\n---\n"))
	if end < 0 {
		t.Fatal("frontmatter has no closing fence")
	}
	front := rest[:end+1]
	if !bytes.HasPrefix(front, []byte("name: nodex\n")) {
		t.Fatalf("name line = %q", firstLine(front))
	}
	const description = "description: Use Nodex when analyzing, reviewing, locating, or reasoning about source-code comments and declarations in a repository. Nodex provides a deterministic index of comments, declarations, compact IDs, and bounded source context.\n"
	if !bytes.Contains(front, []byte(description)) {
		t.Fatalf("description = %q", front)
	}
	if bytes.Count(doc, []byte("name: nodex")) != 1 {
		t.Fatal("name: nodex appears more than once")
	}
	if bytes.Count(doc, []byte(Marker)) != 1 {
		t.Fatalf("marker count = %d", bytes.Count(doc, []byte(Marker)))
	}
	for _, phrase := range []string{
		"nodex index status",
		"nodex index generate",
		"nodex index comments",
		"nodex index declarations",
		"nodex index show",
		"doc: none",
		"`names:` with no value",
		"snapshot-local",
		"IDs may change after regeneration",
		"previously observed IDs must not be assumed to refer to the same comments",
		"Nodex collects, indexes, localizes, and retrieves.",
		"The LLM analyzes.",
		"not a linter",
		"does not decide that a declaration requires documentation",
		"Do not treat `doc: none` as a defect by itself.",
		"Do not infer a source location from `nodex index comments`",
		"bounded structural source context",
		"batch them into one `nodex index show` invocation",
		"nodex ignore list",
		"nodex ignore enable",
		"nodex ignore disable",
		"without explicit user intent",
		"snapshot.json",
		"comments.jsonl",
		"declarations.jsonl",
		".nodex/index/",
		"do not grep or regex",
	} {
		if !bytes.Contains(doc, []byte(phrase)) {
			t.Fatalf("missing %q", phrase)
		}
	}
	command := bytes.Index(doc, []byte("nodex "))
	if command < 0 || !bytes.HasPrefix(doc[command:], []byte("nodex index status")) {
		t.Fatal("primary workflow does not begin with nodex index status")
	}
	lowered := bytes.ToLower(doc)
	for _, word := range []string{"claude", "codex", "gemini", "grok", "anthropic", "openai", "chatgpt"} {
		if bytes.Contains(lowered, []byte(word)) {
			t.Fatalf("provider-specific text %q", word)
		}
	}
	mutated := Document()
	mutated[0] ^= 0xff
	if bytes.Equal(Document(), mutated) || !bytes.Equal(Document(), skillDocument) {
		t.Fatal("Document returned an alias of the embedded bytes")
	}
}

func TestEmbeddedMatchesSourceFile(t *testing.T) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("locate test file")
	}
	data, err := os.ReadFile(filepath.Join(filepath.Dir(file), "assets", "nodex", "SKILL.md"))
	if err != nil {
		t.Fatalf("read source: %v", err)
	}
	if !bytes.Equal(data, skillDocument) {
		t.Fatal("embedded bytes differ from assets/nodex/SKILL.md")
	}
}

func TestTargets(t *testing.T) {
	want := []struct {
		name    string
		project string
		global  string
	}{
		{"agents", ".agents/skills/nodex/SKILL.md", ".agents/skills/nodex/SKILL.md"},
		{"claude", ".claude/skills/nodex/SKILL.md", ".claude/skills/nodex/SKILL.md"},
		{"codex", ".codex/skills/nodex/SKILL.md", ".codex/skills/nodex/SKILL.md"},
		{"gemini", ".agents/skills/nodex/SKILL.md", ".gemini/config/skills/nodex/SKILL.md"},
		{"grok", ".grok/skills/nodex/SKILL.md", ".grok/skills/nodex/SKILL.md"},
	}
	got := Targets()
	if len(got) != len(want) {
		t.Fatalf("Targets len = %d", len(got))
	}
	for i, target := range got {
		if target.Name() != want[i].name || target.ProjectPath() != want[i].project || target.GlobalPath() != want[i].global {
			t.Fatalf("target %d = %s %s %s", i, target.Name(), target.ProjectPath(), target.GlobalPath())
		}
		for _, rel := range []string{target.ProjectPath(), target.GlobalPath()} {
			if rel == "" || path.IsAbs(rel) || filepath.IsAbs(rel) || path.Clean(rel) != rel || strings.Contains(rel, "..") {
				t.Fatalf("unsafe path %q", rel)
			}
			if _, err := splitRel(rel); err != nil {
				t.Fatalf("split %s: %v", rel, err)
			}
		}
		found, err := Lookup(target.Name())
		if err != nil || found != target {
			t.Fatalf("Lookup(%s) = %+v %v", target.Name(), found, err)
		}
	}
	if Targets()[0].ProjectPath() != Targets()[3].ProjectPath() {
		t.Fatal("agents and gemini do not share a project path")
	}
	if Targets()[0].GlobalPath() == Targets()[3].GlobalPath() {
		t.Fatal("agents and gemini share a home path")
	}
	again := Targets()
	again[0] = Target{}
	if Targets()[0].Name() != "agents" {
		t.Fatal("Targets returned the catalog slice")
	}
	for _, name := range []string{"Agents", "CODEX", "agent", "gemini-cli", "", "codex "} {
		if _, err := Lookup(name); err == nil {
			t.Fatalf("Lookup accepted %q", name)
		}
	}
}

func TestInstallFreshAndIdempotent(t *testing.T) {
	for _, place := range []Place{Project, Home} {
		for _, target := range Targets() {
			name := target.Name() + "/project"
			rel := target.ProjectPath()
			if place == Home {
				name = target.Name() + "/home"
				rel = target.GlobalPath()
			}
			t.Run(name, func(t *testing.T) {
				base := t.TempDir()
				if err := Install(base, target, place); err != nil {
					t.Fatalf("install: %v", err)
				}
				dest := filepath.Join(base, filepath.FromSlash(rel))
				if !bytes.Equal(readSkill(t, dest), Document()) {
					t.Fatal("installed bytes differ from the embedded skill")
				}
				if _, err := os.Stat(filepath.Join(base, ".nodex")); !errors.Is(err, os.ErrNotExist) {
					t.Fatal("install created .nodex")
				}
				if tree := treeEntries(t, base); !sameEntries(tree, skillTree(rel)) {
					t.Fatalf("tree = %v, want %v", tree, skillTree(rel))
				}
				ino := inodeOf(t, dest)
				if err := Install(base, target, place); err != nil {
					t.Fatalf("second install: %v", err)
				}
				if inodeOf(t, dest) != ino {
					t.Fatal("idempotent install replaced the file")
				}
				if !bytes.Equal(readSkill(t, dest), Document()) {
					t.Fatal("second install changed bytes")
				}
				assertNoSkillTemp(t, base)
			})
		}
	}
}

func TestInstallSharedProjectPath(t *testing.T) {
	base := t.TempDir()
	agents := mustTarget(t, "agents")
	gemini := mustTarget(t, "gemini")
	if err := Install(base, agents, Project); err != nil {
		t.Fatalf("agents: %v", err)
	}
	dest := filepath.Join(base, filepath.FromSlash(agents.ProjectPath()))
	ino := inodeOf(t, dest)
	if err := Install(base, gemini, Project); err != nil {
		t.Fatalf("gemini: %v", err)
	}
	if inodeOf(t, dest) != ino {
		t.Fatal("gemini install rewrote the shared skill")
	}
	files := regularFiles(t, base)
	if len(files) != 1 || files[0] != agents.ProjectPath() {
		t.Fatalf("files = %v", files)
	}
	if agents.ProjectPath() != gemini.ProjectPath() {
		t.Fatal("project paths differ")
	}
}

func TestInstallRequiresExistingDirectoryBase(t *testing.T) {
	target := mustTarget(t, "codex")
	missing := filepath.Join(t.TempDir(), "missing")
	if err := Install(missing, target, Project); err == nil {
		t.Fatal("missing base was accepted")
	}
	if _, err := os.Stat(missing); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("missing base was created")
	}
	if err := Install("", target, Project); err == nil {
		t.Fatal("empty base was accepted")
	}
	dir := t.TempDir()
	file := filepath.Join(dir, "file")
	if err := os.WriteFile(file, []byte("keep"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := Install(file, target, Project); err == nil || !strings.Contains(err.Error(), "not a directory") {
		t.Fatalf("file base: %v", err)
	}
	if string(readSkill(t, file)) != "keep" {
		t.Fatal("file base was modified")
	}
	if _, err := os.Stat(filepath.Join(dir, ".codex")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("file base gained a skill directory")
	}
	parent := t.TempDir()
	base := filepath.Join(parent, "project")
	if err := os.Mkdir(base, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	t.Chdir(parent)
	if err := Install("project", target, Project); err != nil {
		t.Fatalf("relative base: %v", err)
	}
	if !bytes.Equal(readSkill(t, filepath.Join(base, ".codex", "skills", "nodex", "SKILL.md")), Document()) {
		t.Fatal("relative base wrote unexpected bytes")
	}
}

func TestInstallUpgradesManagedSkill(t *testing.T) {
	base := t.TempDir()
	target := mustTarget(t, "grok")
	dest := filepath.Join(base, filepath.FromSlash(target.ProjectPath()))
	old := []byte("---\nname: nodex\ndescription: old\n---\n\n" + Marker + "\n\nold workflow\n")
	writeAt(t, dest, old)
	if err := Install(base, target, Project); err != nil {
		t.Fatalf("upgrade: %v", err)
	}
	if !bytes.Equal(readSkill(t, dest), Document()) {
		t.Fatal("upgrade did not install the canonical skill")
	}
	assertNoSkillTemp(t, base)
}

func TestInstallLeavesUnmanagedSkill(t *testing.T) {
	base := t.TempDir()
	target := mustTarget(t, "claude")
	dest := filepath.Join(base, filepath.FromSlash(target.ProjectPath()))
	const original = "# my skill\n\nnot managed\n"
	writeAt(t, dest, []byte(original))
	err := Install(base, target, Project)
	if !errors.Is(err, ErrUnmanaged) {
		t.Fatalf("install = %v", err)
	}
	if string(readSkill(t, dest)) != original {
		t.Fatal("unmanaged skill was modified")
	}
	near := []byte("<!-- nodex-managed-skill:v2 -->\n")
	writeAt(t, dest, near)
	err = Install(base, target, Home)
	if !errors.Is(err, ErrUnmanaged) {
		t.Fatalf("home install = %v", err)
	}
	if !bytes.Equal(readSkill(t, filepath.Join(base, filepath.FromSlash(target.GlobalPath()))), near) {
		t.Fatal("near-marker skill was modified")
	}
	assertNoSkillTemp(t, base)
}

func TestInstallRejectsSymlinks(t *testing.T) {
	outside := t.TempDir()
	secret := filepath.Join(outside, "secret")
	if err := os.WriteFile(secret, []byte("keep"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	outsideSkill := filepath.Join(outside, skillFile)
	if err := os.WriteFile(outsideSkill, []byte("outside"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	t.Run("intermediate", func(t *testing.T) {
		base := t.TempDir()
		if err := os.Symlink(outside, filepath.Join(base, ".codex")); err != nil {
			t.Fatalf("symlink: %v", err)
		}
		err := Install(base, mustTarget(t, "codex"), Project)
		if !errors.Is(err, ErrSymlink) {
			t.Fatalf("install = %v", err)
		}
		if string(readSkill(t, secret)) != "keep" || string(readSkill(t, outsideSkill)) != "outside" {
			t.Fatal("install followed an intermediate symlink")
		}
	})
	t.Run("nodex directory", func(t *testing.T) {
		base := t.TempDir()
		skills := filepath.Join(base, ".grok", "skills")
		if err := os.MkdirAll(skills, 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		if err := os.Symlink(outside, filepath.Join(skills, skillDir)); err != nil {
			t.Fatalf("symlink: %v", err)
		}
		err := Install(base, mustTarget(t, "grok"), Project)
		if !errors.Is(err, ErrSymlink) {
			t.Fatalf("install = %v", err)
		}
		if _, statErr := os.Stat(filepath.Join(outside, skillFile)); statErr != nil || string(readSkill(t, outsideSkill)) != "outside" {
			t.Fatal("install followed a nodex symlink")
		}
	})
	t.Run("skill file", func(t *testing.T) {
		base := t.TempDir()
		dir := filepath.Join(base, ".claude", "skills", skillDir)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		if err := os.Symlink(outsideSkill, filepath.Join(dir, skillFile)); err != nil {
			t.Fatalf("symlink: %v", err)
		}
		err := Install(base, mustTarget(t, "claude"), Project)
		if !errors.Is(err, ErrSymlink) {
			t.Fatalf("install = %v", err)
		}
		info, statErr := os.Lstat(filepath.Join(dir, skillFile))
		if statErr != nil || info.Mode()&os.ModeSymlink == 0 {
			t.Fatal("SKILL.md symlink was replaced")
		}
		if string(readSkill(t, outsideSkill)) != "outside" {
			t.Fatal("install followed a SKILL.md symlink")
		}
	})
}

func TestInstallRejectsNonRegularDestination(t *testing.T) {
	target := mustTarget(t, "codex")
	t.Run("directory", func(t *testing.T) {
		base := t.TempDir()
		dest := filepath.Join(base, ".codex", "skills", skillDir, skillFile)
		if err := os.MkdirAll(dest, 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		err := Install(base, target, Project)
		if !errors.Is(err, ErrNotRegular) {
			t.Fatalf("install = %v", err)
		}
		info, statErr := os.Stat(dest)
		if statErr != nil || !info.IsDir() {
			t.Fatal("directory destination was removed")
		}
	})
	t.Run("fifo", func(t *testing.T) {
		base := t.TempDir()
		dir := filepath.Join(base, ".codex", "skills", skillDir)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		dest := filepath.Join(dir, skillFile)
		if err := syscall.Mkfifo(dest, 0o644); err != nil {
			t.Fatalf("mkfifo: %v", err)
		}
		err := Install(base, target, Project)
		if !errors.Is(err, ErrNotRegular) {
			t.Fatalf("install = %v", err)
		}
		info, statErr := os.Lstat(dest)
		if statErr != nil || info.Mode()&os.ModeNamedPipe == 0 {
			t.Fatal("fifo destination was replaced")
		}
	})
}

func TestInstallWriteFailureLeavesNoPartialFile(t *testing.T) {
	target := mustTarget(t, "codex")
	t.Run("read only base", func(t *testing.T) {
		base := t.TempDir()
		restoreMode(t, base, 0o755)
		if err := os.Chmod(base, 0o555); err != nil {
			t.Fatalf("chmod: %v", err)
		}
		if err := Install(base, target, Project); err == nil {
			t.Fatal("read-only base was accepted")
		}
		if err := os.Chmod(base, 0o755); err != nil {
			t.Fatalf("restore: %v", err)
		}
		if _, err := os.Stat(filepath.Join(base, ".codex")); !errors.Is(err, os.ErrNotExist) {
			t.Fatal("failed install created .codex")
		}
		assertNoSkillTemp(t, base)
	})
	t.Run("read only skill directory", func(t *testing.T) {
		base := t.TempDir()
		dir := filepath.Join(base, ".codex", "skills", skillDir)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		old := []byte(Marker + "\nold\n")
		if err := os.WriteFile(filepath.Join(dir, skillFile), old, 0o644); err != nil {
			t.Fatalf("write: %v", err)
		}
		restoreMode(t, dir, 0o755)
		if err := os.Chmod(dir, 0o555); err != nil {
			t.Fatalf("chmod: %v", err)
		}
		err := Install(base, target, Project)
		if err == nil {
			t.Fatal("read-only skill directory was accepted")
		}
		if err := os.Chmod(dir, 0o755); err != nil {
			t.Fatalf("restore: %v", err)
		}
		if !bytes.Equal(readSkill(t, filepath.Join(dir, skillFile)), old) {
			t.Fatal("failed upgrade replaced SKILL.md")
		}
		assertNoSkillTemp(t, base)
	})
}

func TestInstallRejectsEscape(t *testing.T) {
	base := t.TempDir()
	parent := filepath.Dir(base)
	for _, rel := range []string{
		"",
		"/tmp/nodex/SKILL.md",
		"../nodex/SKILL.md",
		"foo/../../nodex/SKILL.md",
		"foo/../nodex/SKILL.md",
		"./nodex/SKILL.md",
		"skills/nodex/SKILL.md/..",
		`skills\nodex\SKILL.md`,
		"skills/nodex/OTHER.md",
		"skills/notnodex/SKILL.md",
		"nodex/SKILL.md/extra",
	} {
		if _, err := splitRel(rel); err == nil {
			t.Fatalf("splitRel accepted %q", rel)
		}
	}
	bad := Target{name: "bad", project: "../nodex/SKILL.md", global: "/etc/nodex/SKILL.md"}
	if err := Install(base, bad, Project); err == nil {
		t.Fatal("project escape was accepted")
	}
	if err := Install(base, bad, Home); err == nil {
		t.Fatal("home escape was accepted")
	}
	if err := Install(base, Target{}, Project); err == nil {
		t.Fatal("zero target was accepted")
	}
	if err := Install(base, mustTarget(t, "codex"), 0); err == nil {
		t.Fatal("zero place was accepted")
	}
	entries, err := os.ReadDir(base)
	if err != nil {
		t.Fatalf("read base: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("base gained entries: %d", len(entries))
	}
	if _, err := os.Stat(filepath.Join(parent, skillFile)); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("install wrote SKILL.md beside the base")
	}
}

func TestUninstall(t *testing.T) {
	t.Run("removes managed skill and empty nodex directory", func(t *testing.T) {
		base := t.TempDir()
		target := mustTarget(t, "codex")
		if err := Install(base, target, Project); err != nil {
			t.Fatalf("install: %v", err)
		}
		if err := Uninstall(base, target, Project); err != nil {
			t.Fatalf("uninstall: %v", err)
		}
		if _, err := os.Stat(filepath.Join(base, ".codex", "skills", skillDir)); !errors.Is(err, os.ErrNotExist) {
			t.Fatal("empty nodex directory remained")
		}
		if _, err := os.Stat(filepath.Join(base, ".codex", "skills")); err != nil {
			t.Fatalf("skills directory was removed: %v", err)
		}
		if _, err := os.Stat(filepath.Join(base, ".codex")); err != nil {
			t.Fatalf(".codex was removed: %v", err)
		}
		if err := Uninstall(base, target, Project); err != nil {
			t.Fatalf("second uninstall: %v", err)
		}
	})
	t.Run("missing installation succeeds", func(t *testing.T) {
		base := t.TempDir()
		if err := Uninstall(base, mustTarget(t, "grok"), Home); err != nil {
			t.Fatalf("uninstall: %v", err)
		}
		entries, err := os.ReadDir(base)
		if err != nil {
			t.Fatalf("read: %v", err)
		}
		if len(entries) != 0 {
			t.Fatal("missing uninstall created entries")
		}
	})
	t.Run("leaves an unmanaged skill", func(t *testing.T) {
		base := t.TempDir()
		target := mustTarget(t, "claude")
		dest := filepath.Join(base, filepath.FromSlash(target.GlobalPath()))
		const original = "# mine\n"
		writeAt(t, dest, []byte(original))
		err := Uninstall(base, target, Home)
		if !errors.Is(err, ErrUnmanaged) {
			t.Fatalf("uninstall = %v", err)
		}
		if string(readSkill(t, dest)) != original {
			t.Fatal("unmanaged skill was removed")
		}
	})
	t.Run("removes an older managed skill", func(t *testing.T) {
		base := t.TempDir()
		target := mustTarget(t, "agents")
		dest := filepath.Join(base, filepath.FromSlash(target.ProjectPath()))
		writeAt(t, dest, []byte(Marker+"\nold\n"))
		if err := Uninstall(base, target, Project); err != nil {
			t.Fatalf("uninstall: %v", err)
		}
		if _, err := os.Stat(dest); !errors.Is(err, os.ErrNotExist) {
			t.Fatal("older managed skill remained")
		}
	})
	t.Run("keeps sibling files and the nodex directory", func(t *testing.T) {
		base := t.TempDir()
		target := mustTarget(t, "gemini")
		if err := Install(base, target, Home); err != nil {
			t.Fatalf("install: %v", err)
		}
		dir := filepath.Dir(filepath.Join(base, filepath.FromSlash(target.GlobalPath())))
		notes := filepath.Join(dir, "notes.txt")
		if err := os.WriteFile(notes, []byte("keep"), 0o644); err != nil {
			t.Fatalf("write: %v", err)
		}
		if err := Uninstall(base, target, Home); err != nil {
			t.Fatalf("uninstall: %v", err)
		}
		if string(readSkill(t, notes)) != "keep" {
			t.Fatal("sibling file was removed")
		}
		if _, err := os.Stat(filepath.Join(dir, skillFile)); !errors.Is(err, os.ErrNotExist) {
			t.Fatal("SKILL.md remained")
		}
		if _, err := os.Stat(dir); err != nil {
			t.Fatal("nodex directory with a sibling was removed")
		}
		parent := filepath.Dir(dir)
		if _, err := os.Stat(parent); err != nil {
			t.Fatal("parent directory was removed")
		}
	})
	t.Run("does not follow a symlink", func(t *testing.T) {
		outside := t.TempDir()
		outsideSkill := filepath.Join(outside, skillFile)
		body := []byte(Marker + "\noutside\n")
		if err := os.WriteFile(outsideSkill, body, 0o644); err != nil {
			t.Fatalf("write: %v", err)
		}
		base := t.TempDir()
		if err := os.Symlink(outside, filepath.Join(base, ".agents")); err != nil {
			t.Fatalf("symlink: %v", err)
		}
		err := Uninstall(base, mustTarget(t, "agents"), Project)
		if !errors.Is(err, ErrSymlink) {
			t.Fatalf("uninstall = %v", err)
		}
		if !bytes.Equal(readSkill(t, outsideSkill), body) {
			t.Fatal("uninstall followed an intermediate symlink")
		}

		base = t.TempDir()
		dir := filepath.Join(base, ".claude", "skills", skillDir)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		link := filepath.Join(dir, skillFile)
		if err := os.Symlink(outsideSkill, link); err != nil {
			t.Fatalf("symlink: %v", err)
		}
		err = Uninstall(base, mustTarget(t, "claude"), Project)
		if !errors.Is(err, ErrSymlink) {
			t.Fatalf("file symlink = %v", err)
		}
		info, statErr := os.Lstat(link)
		if statErr != nil || info.Mode()&os.ModeSymlink == 0 {
			t.Fatal("SKILL.md symlink was removed")
		}
		if !bytes.Equal(readSkill(t, outsideSkill), body) {
			t.Fatal("uninstall followed a SKILL.md symlink")
		}

		base = t.TempDir()
		skills := filepath.Join(base, ".grok", "skills")
		if err := os.MkdirAll(skills, 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		if err := os.Symlink(outside, filepath.Join(skills, skillDir)); err != nil {
			t.Fatalf("symlink: %v", err)
		}
		err = Uninstall(base, mustTarget(t, "grok"), Project)
		if !errors.Is(err, ErrSymlink) {
			t.Fatalf("dir symlink = %v", err)
		}
		if !bytes.Equal(readSkill(t, outsideSkill), body) {
			t.Fatal("uninstall followed a nodex symlink")
		}
	})
	t.Run("refuses a non-regular destination", func(t *testing.T) {
		base := t.TempDir()
		dest := filepath.Join(base, ".codex", "skills", skillDir, skillFile)
		if err := os.MkdirAll(dest, 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		err := Uninstall(base, mustTarget(t, "codex"), Project)
		if !errors.Is(err, ErrNotRegular) {
			t.Fatalf("uninstall = %v", err)
		}
		info, statErr := os.Stat(dest)
		if statErr != nil || !info.IsDir() {
			t.Fatal("directory destination was removed")
		}
	})
}

func mustTarget(t *testing.T, name string) Target {
	t.Helper()
	target, err := Lookup(name)
	if err != nil {
		t.Fatalf("Lookup(%s): %v", name, err)
	}
	return target
}

func writeAt(t *testing.T, path string, data []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
}

func readSkill(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return data
}

func inodeOf(t *testing.T, path string) uint64 {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat %s: %v", path, err)
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		t.Fatalf("stat type %T", info.Sys())
	}
	return stat.Ino
}

func regularFiles(t *testing.T, root string) []string {
	t.Helper()
	var files []string
	for _, entry := range treeEntries(t, root) {
		info, err := os.Lstat(filepath.Join(root, filepath.FromSlash(entry)))
		if err != nil {
			t.Fatalf("stat %s: %v", entry, err)
		}
		if info.Mode().IsRegular() {
			files = append(files, entry)
		}
	}
	return files
}

func treeEntries(t *testing.T, root string) []string {
	t.Helper()
	var entries []string
	err := filepath.WalkDir(root, func(candidate string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if candidate == root {
			return nil
		}
		rel, err := filepath.Rel(root, candidate)
		if err != nil {
			return err
		}
		entries = append(entries, filepath.ToSlash(rel))
		return nil
	})
	if err != nil {
		t.Fatalf("walk: %v", err)
	}
	sort.Strings(entries)
	return entries
}

func skillTree(rel string) []string {
	var entries []string
	current := ""
	for _, part := range strings.Split(rel, "/") {
		if current == "" {
			current = part
		} else {
			current += "/" + part
		}
		entries = append(entries, current)
	}
	return entries
}

func sameEntries(got, want []string) bool {
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

func assertNoSkillTemp(t *testing.T, root string) {
	t.Helper()
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if strings.HasPrefix(entry.Name(), ".skill-") {
			t.Fatalf("temporary file remained: %s", path)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk: %v", err)
	}
}

func restoreMode(t *testing.T, path string, mode os.FileMode) {
	t.Helper()
	t.Cleanup(func() {
		_ = os.Chmod(path, mode)
	})
}

func firstLine(data []byte) string {
	line, _, _ := bytes.Cut(data, []byte("\n"))
	return string(line)
}
