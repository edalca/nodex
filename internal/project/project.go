// Package project establishes the filesystem boundary of a source project
// and resolves the independent workspace base for Nodex control state.
//
// Open takes a directory the caller has already selected. It does not search
// parent directories. Resolve searches upward from a starting directory for
// the nearest language-neutral project marker and does not read source or an
// ignore document. Neither function treats a language manifest as a project
// root. Discovery is language-neutral. The caller supplies an ExclusionPolicy,
// and Files applies that policy to directories and regular files. Symbolic
// links are not followed. Directories named .git and .nodex are not traversed,
// and a policy cannot re-include them.
//
// ReadFile reads the exact bytes of one regular file addressed by a canonical
// logical path. It rejects control directories, symbolic links, and paths
// that are not already clean. It does not apply an exclusion policy and does
// not parse source.
package project

import (
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
)

// Project is the filesystem boundary of one source project.
//
// The boundary is the absolute, cleaned path of the project root. Paths
// reported by the project are relative to that root and stay inside it.
type Project struct {
	root string
}

// Locations identifies the source boundary and the base for control state.
// Both paths are absolute, cleaned, existing directories with links resolved.
// Nodex control state belongs beneath WorkspaceBase/.nodex. Neither location
// is a persisted source identity.
type Locations struct {
	// SourceRoot is the directory whose files are discovered and read.
	SourceRoot string
	// WorkspaceBase is the directory containing Nodex's .nodex control directory.
	WorkspaceBase string
}

// ResolveLocations selects source and workspace directories from invocationCWD.
// An empty root or outDir means that option was absent. An explicit root
// selects source without marker discovery; otherwise source is discovered
// from invocationCWD. An explicit outDir selects the workspace. Without it,
// an explicit root uses invocationCWD as workspace, and automatic source
// discovery uses the discovered source root as workspace. Relative options
// are interpreted from invocationCWD. No directories are created and no
// control state is read. Callers must still reject unsafe control components.
func ResolveLocations(invocationCWD, root, outDir string) (Locations, error) {
	cwd, err := absoluteDir(invocationCWD)
	if err != nil {
		return Locations{}, fmt.Errorf("invocation directory: %w", err)
	}
	source, err := ResolveSource(cwd, root)
	if err != nil {
		return Locations{}, err
	}
	workspace := source
	if outDir != "" {
		workspace, err = absoluteDir(fromDirectory(cwd, outDir))
		if err != nil {
			return Locations{}, fmt.Errorf("workspace base: %w", err)
		}
	} else if root != "" {
		workspace = cwd
	}
	return Locations{SourceRoot: source, WorkspaceBase: workspace}, nil
}

// ResolveSource selects a source root without resolving a workspace.
// An empty root uses marker discovery from invocationCWD. An explicit root
// uses Open without ancestor discovery, relative to invocationCWD when needed.
// An absolute explicit root does not require invocationCWD. This operation
// also selects the project destination for project-local Agent Skills.
func ResolveSource(invocationCWD, root string) (string, error) {
	if root == "" {
		return Resolve(invocationCWD)
	}
	if !filepath.IsAbs(root) {
		cwd, err := absoluteDir(invocationCWD)
		if err != nil {
			return "", fmt.Errorf("invocation directory: %w", err)
		}
		invocationCWD = cwd
	}
	opened, err := Open(fromDirectory(invocationCWD, root))
	if err != nil {
		return "", err
	}
	return opened.Root(), nil
}

func fromDirectory(cwd, selected string) string {
	if filepath.IsAbs(selected) {
		return selected
	}
	return filepath.Join(cwd, selected)
}

// Open establishes the project boundary at root.
//
// root is resolved to an absolute, cleaned directory. A symbolic link in the
// path is resolved to its target, and the boundary is that directory. Open
// fails when root is empty, does not exist, is not a directory, or cannot be
// resolved to a directory. Open does not search parent directories.
func Open(root string) (*Project, error) {
	resolved, err := absoluteDir(root)
	if err != nil {
		return nil, err
	}
	return &Project{root: resolved}, nil
}

// Resolve returns the project root discovered from start.
//
// start is a directory, normally the process working directory. Resolve walks
// that directory and its ancestors and stops at the nearest marker. A .nodex
// marker is a real directory and takes precedence over .git in the same
// directory. A .git marker is a real directory or a regular file, including
// the gitdir file used by a linked worktree. A symbolic link named .nodex or
// .git is an error and is not followed. A .nodex path that exists and is not
// a real directory is an error. A .git path of any other file type is an
// error. When no marker exists, the result is start itself. Resolve does not
// continue past a selected marker and does not consult a language manifest.
// The result is an absolute cleaned directory. Open remains the authority
// for the project boundary of a path the caller already selected.
func Resolve(start string) (string, error) {
	resolved, err := absoluteDir(start)
	if err != nil {
		return "", err
	}
	dir := resolved
	for {
		found, err := projectMarker(dir)
		if err != nil {
			return "", err
		}
		if found {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return resolved, nil
		}
		dir = parent
	}
}

func absoluteDir(root string) (string, error) {
	if root == "" {
		return "", errors.New("project root path is empty")
	}
	absolute, err := filepath.Abs(root)
	if err != nil {
		return "", fmt.Errorf("resolve project root: %w", err)
	}
	resolved, err := filepath.EvalSymlinks(absolute)
	if err != nil {
		return "", fmt.Errorf("resolve project root: %w", err)
	}
	resolved = filepath.Clean(resolved)
	info, err := os.Lstat(resolved)
	if err != nil {
		return "", fmt.Errorf("resolve project root: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return "", fmt.Errorf("project root is not a directory: %s", resolved)
	}
	return resolved, nil
}

// projectMarker reports whether dir itself contains a project marker.
// An existing marker of the wrong kind is an error. A missing marker is
// not an error.
func projectMarker(dir string) (bool, error) {
	switch kind, err := markerAt(filepath.Join(dir, ".nodex")); {
	case err != nil:
		return false, err
	case kind == markerDirectory:
		return true, nil
	case kind == markerSymlink:
		return false, errors.New(".nodex is a symbolic link")
	case kind != markerAbsent:
		return false, errors.New(".nodex is not a directory")
	}
	switch kind, err := markerAt(filepath.Join(dir, ".git")); {
	case err != nil:
		return false, err
	case kind == markerDirectory || kind == markerFile:
		return true, nil
	case kind == markerSymlink:
		return false, errors.New(".git is a symbolic link")
	case kind != markerAbsent:
		return false, errors.New(".git is not a project marker")
	default:
		return false, nil
	}
}

type markerKind int

const (
	markerAbsent markerKind = iota
	markerDirectory
	markerFile
	markerSymlink
	markerOther
)

func markerAt(path string) (markerKind, error) {
	info, err := os.Lstat(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return markerAbsent, nil
		}
		return markerAbsent, err
	}
	mode := info.Mode()
	switch {
	case mode&os.ModeSymlink != 0:
		return markerSymlink, nil
	case info.IsDir():
		return markerDirectory, nil
	case mode.IsRegular():
		return markerFile, nil
	default:
		return markerOther, nil
	}
}

// Root returns the absolute, cleaned path of the project boundary.
func (p *Project) Root() string {
	if p == nil {
		return ""
	}
	return p.root
}

// ExclusionPolicy reports whether a logical project path is excluded.
//
// path is relative to the project root, uses "/" separators, and has no
// empty, ".", or ".." element. isDir reports whether path names a directory.
// A true result excludes path. An excluded directory is not traversed.
// Matching, negation, and ancestor sealing belong to the policy. Project
// applies the decision and does not reinterpret it.
type ExclusionPolicy interface {
	Excluded(path string, isDir bool) bool
}

var (
	// ErrEmptyPath is returned when a source path is empty.
	ErrEmptyPath = errors.New("path is empty")

	// ErrDotPath is returned when a source path is ".".
	ErrDotPath = errors.New("path is \".\"")

	// ErrAbsolutePath is returned when a source path is absolute.
	ErrAbsolutePath = errors.New("path is absolute")

	// ErrUncleanPath is returned when a source path is not already clean.
	ErrUncleanPath = errors.New("path is not clean")

	// ErrEscapingPath is returned when a source path escapes the project root.
	ErrEscapingPath = errors.New("path escapes the project root")

	// ErrControlPath is returned when a source path names .git or .nodex,
	// or a file inside either directory.
	ErrControlPath = errors.New("path is inside a control directory")

	// ErrDirectory is returned when a source path names a directory.
	ErrDirectory = errors.New("path is a directory")

	// ErrSymlink is returned when a source path uses a symbolic link.
	ErrSymlink = errors.New("path is a symbolic link")

	// ErrNotRegular is returned when a source path is not a regular file.
	ErrNotRegular = errors.New("path is not a regular file")
)

// ReadFile reads the exact bytes of one regular file inside the project.
//
// path is a canonical logical project-relative path: non-empty, relative,
// slash-separated, and equal to its cleaned form. ReadFile does not clean
// an invalid path and does not rewrite it. An absolute path, a path that
// escapes the project root, a directory, a symbolic link, and any other
// non-regular file are rejected. A path whose elements include .git or
// .nodex is rejected. Symbolic links are not followed, including when an
// intermediate element is a link.
func (p *Project) ReadFile(logical string) ([]byte, error) {
	if p == nil || p.root == "" {
		return nil, errors.New("project root is not established")
	}
	if err := validateLogical(logical); err != nil {
		return nil, err
	}
	current := p.root
	elements := strings.Split(logical, "/")
	for i, elem := range elements {
		if elem == "" || elem == "." || elem == ".." {
			return nil, fmt.Errorf("%w: %s", ErrUncleanPath, logical)
		}
		if elem == ".git" || elem == ".nodex" {
			return nil, fmt.Errorf("%w: %s", ErrControlPath, logical)
		}
		current = filepath.Join(current, elem)
		rel, err := filepath.Rel(p.root, current)
		if err != nil || !filepath.IsLocal(rel) {
			return nil, fmt.Errorf("%w: %s", ErrEscapingPath, logical)
		}
		info, err := os.Lstat(current)
		if err != nil {
			return nil, err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return nil, fmt.Errorf("%w: %s", ErrSymlink, logical)
		}
		last := i == len(elements)-1
		if !last {
			if !info.IsDir() {
				return nil, fmt.Errorf("%w: %s", ErrNotRegular, logical)
			}
			continue
		}
		if info.IsDir() {
			return nil, fmt.Errorf("%w: %s", ErrDirectory, logical)
		}
		if !info.Mode().IsRegular() {
			return nil, fmt.Errorf("%w: %s", ErrNotRegular, logical)
		}
	}
	return os.ReadFile(current)
}

func validateLogical(logical string) error {
	switch {
	case logical == "":
		return ErrEmptyPath
	case logical == ".":
		return ErrDotPath
	case path.IsAbs(logical) || filepath.IsAbs(logical):
		return ErrAbsolutePath
	case path.Clean(logical) != logical:
		return ErrUncleanPath
	case logicalEscapes(logical):
		return ErrEscapingPath
	default:
		return nil
	}
}

func logicalEscapes(logical string) bool {
	for _, elem := range strings.Split(logical, "/") {
		if elem == ".." {
			return true
		}
	}
	return false
}

// Files lists the regular files inside the project that policy does not
// exclude.
//
// policy decides project-specific exclusions. An empty policy excludes no
// ordinary file or directory. Files does not open or parse an ignore document.
//
// Each returned path is relative to the project root, uses slash separators,
// and has no ".." element. The result is sorted lexicographically and is
// empty when no regular file remains. Symbolic links are omitted, directory
// symbolic links are not followed, and directories named .git and .nodex are
// not traversed. Those control directories stay excluded even when policy
// would include them. A directory that policy excludes is not traversed. A
// regular file that policy excludes is omitted.
func (p *Project) Files(policy ExclusionPolicy) ([]string, error) {
	if p == nil || p.root == "" {
		return nil, errors.New("project root is not established")
	}
	var files []string
	if err := walkFiles(p.root, p.root, policy, &files); err != nil {
		return nil, err
	}
	sort.Strings(files)
	if files == nil {
		files = []string{}
	}
	return files, nil
}

func walkFiles(root, dir string, policy ExclusionPolicy, files *[]string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		candidate := filepath.Join(dir, entry.Name())
		rel, err := relativeToRoot(root, candidate)
		if err != nil {
			return err
		}
		// Info reports the directory entry itself. A symbolic link is not a
		// directory and is not a regular file, so the walk neither follows
		// it nor returns it.
		info, err := entry.Info()
		if err != nil {
			return err
		}
		mode := info.Mode()
		if mode&os.ModeSymlink != 0 {
			continue
		}
		if mode.IsDir() {
			if controlDirectory(entry.Name()) {
				continue
			}
			if policy.Excluded(rel, true) {
				continue
			}
			if err := walkFiles(root, candidate, policy, files); err != nil {
				return err
			}
			continue
		}
		if !mode.IsRegular() {
			continue
		}
		if policy.Excluded(rel, false) {
			continue
		}
		*files = append(*files, rel)
	}
	return nil
}

func relativeToRoot(root, candidate string) (string, error) {
	rel, err := filepath.Rel(root, candidate)
	if err != nil {
		return "", fmt.Errorf("relativize %s: %w", candidate, err)
	}
	rel = filepath.Clean(rel)
	if rel == "." || !filepath.IsLocal(rel) {
		return "", fmt.Errorf("path %s is outside the project root", candidate)
	}
	return filepath.ToSlash(rel), nil
}

func controlDirectory(name string) bool {
	switch name {
	case ".git", ".nodex":
		return true
	default:
		return false
	}
}
