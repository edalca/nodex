package skill

import (
	"fmt"
	"sort"
)

// Place selects the base a target path is relative to.
//
// Project paths are relative to a project root. Home paths are relative to
// the user home directory. The caller resolves that base. The zero Place is
// not a valid selection.
type Place int

const (
	// Project installs under a project root.
	Project Place = iota + 1

	// Home installs under the user home directory.
	Home
)

// Target is one installation location for the canonical skill.
//
// Two targets may share a path. They still receive the same skill bytes.
type Target struct {
	name    string
	project string
	global  string
}

// catalog is the supported targets. agents and gemini share one project-local
// path. gemini's home path is not that shared path.
var catalog = []Target{
	{name: "agents", project: ".agents/skills/nodex/SKILL.md", global: ".agents/skills/nodex/SKILL.md"},
	{name: "claude", project: ".claude/skills/nodex/SKILL.md", global: ".claude/skills/nodex/SKILL.md"},
	{name: "codex", project: ".codex/skills/nodex/SKILL.md", global: ".codex/skills/nodex/SKILL.md"},
	{name: "gemini", project: ".agents/skills/nodex/SKILL.md", global: ".gemini/config/skills/nodex/SKILL.md"},
	{name: "grok", project: ".grok/skills/nodex/SKILL.md", global: ".grok/skills/nodex/SKILL.md"},
}

// Targets returns the supported targets in lexical name order.
//
// The result is a copy. Reordering it does not change later calls.
func Targets() []Target {
	out := make([]Target, len(catalog))
	copy(out, catalog)
	sort.Slice(out, func(i, j int) bool { return out[i].name < out[j].name })
	return out
}

// Lookup returns the target with the case-sensitive name.
//
// Names have no aliases. An unknown name, including a different case, is an
// error.
func Lookup(name string) (Target, error) {
	for _, target := range catalog {
		if target.name == name {
			return target, nil
		}
	}
	return Target{}, fmt.Errorf("unknown skill target %q", name)
}

// Name returns the target identifier.
func (t Target) Name() string {
	return t.name
}

// ProjectPath returns the slash-separated path relative to a project root.
func (t Target) ProjectPath() string {
	return t.project
}

// GlobalPath returns the slash-separated path relative to the user home.
func (t Target) GlobalPath() string {
	return t.global
}

func (t Target) path(place Place) (string, error) {
	switch place {
	case Project:
		if t.project == "" {
			return "", fmt.Errorf("skill target %q has no project path", t.name)
		}
		return t.project, nil
	case Home:
		if t.global == "" {
			return "", fmt.Errorf("skill target %q has no home path", t.name)
		}
		return t.global, nil
	default:
		return "", fmt.Errorf("unknown skill place %d", place)
	}
}
