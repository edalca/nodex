// Command nodex indexes a project's comments and declarations and reports
// that index.
//
// The project root is resolved from the process working directory unless
// --root is set before the command. index generate reads the effective
// ignore policy, discovers included files, applies enabled presets, and
// persists an index of the remaining supported source files. index comments
// writes the current comment corpus as compact Markdown. index declarations
// writes the current declaration facts as compact Markdown. index status
// reports whether that index is missing, current, stale, or corrupt.
// index show resolves snapshot-local comment and declaration IDs from a
// current index to their source location and a bounded structural context.
// version prints the source-defined product version and the build metadata.
// ignore lists and edits the project ignore document. skill prints the
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
	"strings"

	"github.com/edalca/nodex/internal/index"
	"github.com/edalca/nodex/internal/project"
	"github.com/edalca/nodex/internal/syntax"
)

const commandUsage = "usage: nodex [--root <path>] index|version|ignore|skill"

// invocation is one parsed command line.
//
// root is set only when --root was supplied. args is the command and the
// arguments that follow it.
type invocation struct {
	root    string
	hasRoot bool
	args    []string
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
		if name != "root" {
			return invocation{}, fmt.Errorf("unknown option %q", arg)
		}
		if inv.hasRoot {
			return invocation{}, errors.New("--root was provided more than once")
		}
		if !hasValue {
			if i+1 >= len(args) {
				return invocation{}, errors.New("--root requires a path")
			}
			value = args[i+1]
			i += 2
		} else {
			i++
		}
		if value == "" || strings.HasPrefix(value, "-") {
			return invocation{}, errors.New("--root requires a path")
		}
		inv.root = value
		inv.hasRoot = true
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

// locate returns the directory project commands open.
//
// An explicit --root is returned unchanged so Open applies the caller's path
// and does not walk ancestors. Otherwise the working directory is resolved
// to the nearest project marker.
func locate(inv invocation, getwd func() (string, error)) (string, error) {
	if inv.hasRoot {
		return inv.root, nil
	}
	wd, err := getwd()
	if err != nil {
		return "", err
	}
	return project.Resolve(wd)
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

func generate(dir string) (int, int, int, error) {
	state, err := openProjectState(dir)
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
	if _, err := index.Persist(state.root, state.policy.Identity(), state.sources(), idx); err != nil {
		return 0, 0, 0, err
	}
	return idx.Len(), idx.DeclarationLen(), len(state.files), nil
}
