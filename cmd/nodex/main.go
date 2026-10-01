// Command nodex indexes a project's comments and declarations and reports
// that index.
//
// The project root is resolved from the process working directory unless
// --root is set before the command. --out-dir selects the existing base for
// .nodex; explicit --root otherwise uses invocation cwd as that base.
// index generate reads the effective ignore policy, discovers included files,
// applies enabled presets, and persists an index of the remaining supported source files. index comments
// writes the current comment corpus as compact Markdown. index declarations
// writes the current declaration facts as compact Markdown. index status
// reports whether that index is missing, current, stale, or corrupt.
// index show resolves snapshot-local comment and declaration IDs from a
// current index to their source location and a bounded structural context.
// version prints the source-defined product version and the build metadata.
// ignore lists and edits the workspace ignore document. skill prints the
// canonical Agent Skill and installs or removes that same document for one
// named target. The index commands do not regenerate an index except for
// index generate. version, ignore presets, skill targets, and skill show do
// not open a project. Project-local skill install and skill uninstall use
// the same root resolution as the other project commands. With --global they
// use the user home directory and do not resolve a project.
package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/edalca/nodex/internal/index"
	"github.com/edalca/nodex/internal/project"
	"github.com/edalca/nodex/internal/syntax"
)

const commandUsage = "usage: nodex [--root <path>] [--out-dir <path>] index|version|ignore|skill\n  --root: source project\n  --out-dir: existing base directory for .nodex workspace"

// invocation is one parsed command line.
//
// root and outDir are set only when their global options were supplied.
// args is the command and the arguments that follow it.
type invocation struct {
	root      string
	hasRoot   bool
	outDir    string
	hasOutDir bool
	args      []string
}

func main() {
	if err := run(os.Args[1:], os.Getwd, os.Stdout, os.Stderr); err != nil {
		os.Exit(1)
	}
}

func run(args []string, getwd func() (string, error), stdout, stderr io.Writer) error {
	inv, err := parseArgs(args)
	if err != nil {
		fmt.Fprintln(stderr, err.Error())
		return err
	}
	if len(inv.args) == 0 {
		fmt.Fprintln(stderr, commandUsage)
		return errors.New("usage")
	}
	switch inv.args[0] {
	case "version":
		if len(inv.args) != 1 {
			fmt.Fprintln(stderr, "version takes no arguments")
			return errors.New("usage")
		}
		return writeStdout(stdout, stderr, versionText())
	case "ignore":
		return dispatchIgnore(inv, inv.args[1:], getwd, stdout, stderr)
	case "skill":
		return dispatchSkill(inv, inv.args[1:], getwd, stdout, stderr)
	case "index":
		return dispatchIndex(inv, inv.args[1:], getwd, stdout, stderr)
	default:
		fmt.Fprintf(stderr, "unknown command %q\n", inv.args[0])
		return errors.New("usage")
	}
}

func parseArgs(args []string) (invocation, error) {
	var inv invocation
	i := 0
	for i < len(args) {
		arg := args[i]
		if arg == "--" {
			// "--" ends global options and is not itself an argument.
			i++
			break
		}
		if !strings.HasPrefix(arg, "-") {
			break
		}
		if !strings.HasPrefix(arg, "--") {
			return invocation{}, fmt.Errorf("unknown option %q", arg)
		}
		name, value, hasValue := splitFlag(arg)
		if name != "root" && name != "out-dir" {
			return invocation{}, fmt.Errorf("unknown option %q", arg)
		}
		if (name == "root" && inv.hasRoot) || (name == "out-dir" && inv.hasOutDir) {
			return invocation{}, fmt.Errorf("--%s was provided more than once", name)
		}
		if !hasValue {
			if i+1 >= len(args) {
				return invocation{}, fmt.Errorf("--%s requires a path", name)
			}
			value = args[i+1]
			i += 2
		} else {
			i++
		}
		if value == "" || strings.HasPrefix(value, "-") {
			return invocation{}, fmt.Errorf("--%s requires a path", name)
		}
		if name == "root" {
			inv.root = value
			inv.hasRoot = true
		} else {
			inv.outDir = value
			inv.hasOutDir = true
		}
	}
	inv.args = args[i:]
	return inv, nil
}

func splitFlag(arg string) (name, value string, hasValue bool) {
	body := strings.TrimPrefix(arg, "--")
	if eq := strings.IndexByte(body, '='); eq >= 0 {
		return body[:eq], body[eq+1:], true
	}
	return body, "", false
}

// locate selects the source destination for project-local skill operations.
// Control-state options do not affect skill installation.
func locate(inv invocation, getwd func() (string, error)) (string, error) {
	if inv.hasRoot && filepath.IsAbs(inv.root) {
		return project.ResolveSource("", inv.root)
	}
	cwd, err := getwd()
	if err != nil {
		return "", err
	}
	return project.ResolveSource(cwd, inv.root)
}

// locations captures invocation cwd once before resolving source and workspace.
func locations(inv invocation, getwd func() (string, error)) (project.Locations, error) {
	cwd, err := getwd()
	if err != nil {
		return project.Locations{}, err
	}
	return project.ResolveLocations(cwd, inv.root, inv.outDir)
}

func writeStdout(stdout, stderr io.Writer, text string) error {
	if _, err := io.WriteString(stdout, text); err != nil {
		fmt.Fprintf(stderr, "nodex: %v\n", err)
		return err
	}
	return nil
}

func countNoun(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}

func generate(dirs project.Locations) (int, int, int, error) {
	state, err := openProjectState(dirs)
	if err != nil {
		return 0, 0, 0, err
	}
	docs := make([]*syntax.Document, len(state.files))
	for i, file := range state.files {
		doc, err := syntax.Parse(file.source.Path, file.body)
		if err != nil {
			return 0, 0, 0, err
		}
		docs[i] = doc
	}
	idx, err := index.Build(docs)
	if err != nil {
		return 0, 0, 0, err
	}
	if _, err := index.Persist(state.workspace, state.policy.Identity(), state.sources(), idx); err != nil {
		return 0, 0, 0, err
	}
	return idx.Len(), idx.DeclarationLen(), len(state.files), nil
}
