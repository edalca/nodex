package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/edalca/nodex/internal/ignore"
	"github.com/edalca/nodex/internal/syntax"
)

func dispatchIgnore(inv invocation, args []string, getwd func() (string, error), stdout, stderr io.Writer) error {
	if len(args) == 0 {
		fmt.Fprintln(stderr, "usage: nodex ignore presets|list|enable|disable")
		return errors.New("usage")
	}
	switch args[0] {
	case "presets":
		if len(args) != 1 {
			fmt.Fprintln(stderr, "ignore presets takes no arguments")
			return errors.New("usage")
		}
		return writeStdout(stdout, stderr, formatSelectors(syntax.Selectors()))
	case "list":
		if len(args) != 1 {
			fmt.Fprintln(stderr, "ignore list takes no arguments")
			return errors.New("usage")
		}
		dirs, err := locations(inv, getwd)
		if err != nil {
			fmt.Fprintf(stderr, "nodex: %v\n", err)
			return err
		}
		text, err := ignoreList(dirs.WorkspaceBase)
		if err != nil {
			fmt.Fprintf(stderr, "nodex: %v\n", err)
			return err
		}
		return writeStdout(stdout, stderr, text)
	case "enable", "disable":
		if len(args) != 2 {
			fmt.Fprintf(stderr, "ignore %s requires one preset\n", args[0])
			return errors.New("usage")
		}
		selector := args[1]
		if _, err := syntax.ExpandSelector(selector); err != nil {
			fmt.Fprintf(stderr, "nodex: %v\n", err)
			return err
		}
		dirs, err := locations(inv, getwd)
		if err != nil {
			fmt.Fprintf(stderr, "nodex: %v\n", err)
			return err
		}
		if args[0] == "enable" {
			err = ignoreEnable(dirs.WorkspaceBase, selector)
		} else {
			err = ignoreDisable(dirs.WorkspaceBase, selector)
		}
		if err != nil {
			fmt.Fprintf(stderr, "nodex: %v\n", err)
			return err
		}
		return nil
	default:
		fmt.Fprintf(stderr, "unknown ignore command %q\n", args[0])
		return errors.New("usage")
	}
}

func formatSelectors(selectors []string) string {
	if len(selectors) == 0 {
		return ""
	}
	return strings.Join(selectors, "\n") + "\n"
}

func ignoreList(workspaceBase string) (string, error) {
	policy, err := loadPolicy(workspaceBase)
	if err != nil {
		return "", err
	}
	data, err := policy.Encode()
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func ignoreEnable(workspaceBase, selector string) error {
	add, err := syntax.ExpandSelector(selector)
	if err != nil {
		return err
	}
	return editIgnore(workspaceBase, func(policy ignore.Policy) (ignore.Policy, error) {
		return ignore.New(unionPresets(policy.Presets(), add), policy.Excludes())
	})
}

func ignoreDisable(workspaceBase, selector string) error {
	remove, err := syntax.ExpandSelector(selector)
	if err != nil {
		return err
	}
	return editIgnore(workspaceBase, func(policy ignore.Policy) (ignore.Policy, error) {
		return ignore.New(withoutPresets(policy.Presets(), remove), policy.Excludes())
	})
}

func editIgnore(workspaceBase string, edit func(ignore.Policy) (ignore.Policy, error)) error {
	policy, err := loadPolicy(workspaceBase)
	if err != nil {
		return err
	}
	next, err := edit(policy)
	if err != nil {
		return err
	}
	if err := syntax.ValidatePresets(next.Presets()); err != nil {
		return err
	}
	if policy.Identity() == next.Identity() {
		return nil
	}
	return publishIgnore(workspaceBase, next)
}

func unionPresets(current, add []string) []string {
	seen := make(map[string]struct{}, len(current)+len(add))
	out := make([]string, 0, len(current)+len(add))
	for _, id := range append(append([]string{}, current...), add...) {
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	return out
}

func withoutPresets(current, remove []string) []string {
	drop := make(map[string]struct{}, len(remove))
	for _, id := range remove {
		drop[id] = struct{}{}
	}
	out := make([]string, 0, len(current))
	for _, id := range current {
		if _, ok := drop[id]; !ok {
			out = append(out, id)
		}
	}
	return out
}

// publishIgnore installs the canonical ignore document.
//
// The document is validated before it is published. A temporary file in
// .nodex is written, synced, and closed, then renamed over ignore.json.
// The containing directory is synced afterward. A symbolic link named .nodex
// or ignore.json is rejected and is not followed. .nodex/index is not
// modified. The temporary file is removed if publication fails.
func publishIgnore(workspaceBase string, policy ignore.Policy) error {
	data, err := policy.Encode()
	if err != nil {
		return err
	}
	checked, err := ignore.Parse(data)
	if err != nil {
		return err
	}
	if checked.Identity() != policy.Identity() || !sameStrings(checked.Presets(), policy.Presets()) || !sameStrings(checked.Excludes(), policy.Excludes()) {
		return errors.New("encoded ignore document does not match the policy")
	}
	if err := syntax.ValidatePresets(checked.Presets()); err != nil {
		return err
	}

	nodexPath := filepath.Join(workspaceBase, ".nodex")
	created := false
	info, err := os.Lstat(nodexPath)
	switch {
	case err == nil && info.Mode()&os.ModeSymlink != 0:
		return errors.New(".nodex is a symbolic link")
	case err == nil && !info.IsDir():
		return errors.New(".nodex is not a directory")
	case errors.Is(err, os.ErrNotExist):
		if err := os.Mkdir(nodexPath, 0o755); err != nil {
			return err
		}
		created = true
	case err != nil:
		return err
	}

	docPath := filepath.Join(nodexPath, "ignore.json")
	info, err = os.Lstat(docPath)
	if err == nil {
		if info.Mode()&os.ModeSymlink != 0 {
			return cleanupIgnorePublish(created, nodexPath, "", fmt.Errorf("%s is a symbolic link", ignore.DocumentPath))
		}
		if !info.Mode().IsRegular() {
			return cleanupIgnorePublish(created, nodexPath, "", fmt.Errorf("%s is not a regular file", ignore.DocumentPath))
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return cleanupIgnorePublish(created, nodexPath, "", err)
	}

	tmp, err := writeIgnoreTemp(nodexPath, data)
	if err != nil {
		return cleanupIgnorePublish(created, nodexPath, "", err)
	}
	if err := os.Rename(tmp, docPath); err != nil {
		return cleanupIgnorePublish(created, nodexPath, tmp, err)
	}
	if err := syncIgnoreDir(nodexPath); err != nil {
		return err
	}
	return nil
}

func cleanupIgnorePublish(created bool, nodexPath, tmp string, cause error) error {
	if tmp != "" {
		_ = os.Remove(tmp)
	}
	if created {
		_ = os.Remove(nodexPath)
	}
	return cause
}

func writeIgnoreTemp(dir string, data []byte) (string, error) {
	f, err := os.CreateTemp(dir, ".ignore-*")
	if err != nil {
		return "", err
	}
	name := f.Name()
	remove := true
	defer func() {
		if remove {
			_ = os.Remove(name)
		}
	}()
	if _, err := f.Write(data); err != nil {
		_ = f.Close()
		return "", err
	}
	if err := f.Sync(); err != nil {
		_ = f.Close()
		return "", err
	}
	if err := f.Close(); err != nil {
		return "", err
	}
	remove = false
	return name, nil
}

func syncIgnoreDir(dir string) error {
	f, err := os.Open(dir)
	if err != nil {
		return err
	}
	defer f.Close()
	return f.Sync()
}

func sameStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
