package skill

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"
)

const (
	skillFile = "SKILL.md"
	skillDir  = "nodex"
)

var (
	// ErrUnmanaged means the destination SKILL.md exists and does not contain
	// Marker. Install leaves the file unchanged. Uninstall does not remove it.
	ErrUnmanaged = errors.New("existing SKILL.md is not a Nodex-managed skill")

	// ErrSymlink means a component of the destination beneath the base is a
	// symbolic link. The link is not followed.
	ErrSymlink = errors.New("skill destination contains a symbolic link")

	// ErrNotRegular means SKILL.md exists and is not a regular file.
	ErrNotRegular = errors.New("SKILL.md is not a regular file")
)

// errAbsent means an intermediate destination directory does not exist.
// Uninstall treats that as a missing installation.
var errAbsent = errors.New("skill destination is absent")

// Install writes the canonical skill under base at the target's path for place.
//
// base must already exist and be a directory. Install does not create it.
// Directories beneath base are created when they are missing. A symbolic
// link in any of those components is rejected and is not followed, including
// a final nodex directory or SKILL.md link. A regular SKILL.md that already
// matches the canonical bytes is left in place. A regular file that contains
// Marker and differs is replaced. Any other existing SKILL.md returns
// ErrUnmanaged and is not modified. The new bytes are written to a temporary
// file in the nodex directory, synced, and renamed to SKILL.md. The installed
// bytes are the embedded skill and do not carry a timestamp or a target name.
func Install(base string, target Target, place Place) error {
	rel, err := target.path(place)
	if err != nil {
		return err
	}
	return installRel(base, rel)
}

// Uninstall removes a Nodex-managed skill under base at the target's path.
//
// Resolution follows the same base and relative path rules as Install.
// A missing destination is success. A regular file that contains Marker is
// removed. A file that does not contain Marker returns ErrUnmanaged and is
// left in place. A symbolic link is not followed and is not removed. When
// the nodex directory is empty after the file is removed, that directory is
// removed. Parent directories are left in place, including when they are
// empty. Other files in the nodex directory are left in place.
func Uninstall(base string, target Target, place Place) error {
	rel, err := target.path(place)
	if err != nil {
		return err
	}
	return uninstallRel(base, rel)
}

func installRel(base, rel string) error {
	root, parts, err := resolveRel(base, rel)
	if err != nil {
		return err
	}
	parent, created, err := ensureParents(root, parts[:len(parts)-1])
	if err != nil {
		return err
	}
	dest := filepath.Join(parent, parts[len(parts)-1])
	replace, err := needsWrite(dest)
	if err != nil {
		removeCreated(created)
		return err
	}
	if !replace {
		return nil
	}
	tmp, err := writeTemp(parent, skillDocument)
	if err != nil {
		removeCreated(created)
		return err
	}
	if err := os.Rename(tmp, dest); err != nil {
		_ = os.Remove(tmp)
		removeCreated(created)
		return err
	}
	return syncDir(parent)
}

func uninstallRel(base, rel string) error {
	root, parts, err := resolveRel(base, rel)
	if err != nil {
		return err
	}
	parent, err := existingParents(root, parts[:len(parts)-1])
	if err != nil {
		if errors.Is(err, errAbsent) {
			return nil
		}
		return err
	}
	dest := filepath.Join(parent, parts[len(parts)-1])
	info, err := os.Lstat(dest)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("SKILL.md: %w", ErrSymlink)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("SKILL.md: %w", ErrNotRegular)
	}
	existing, err := readRegular(dest)
	if err != nil {
		return err
	}
	if !isManaged(existing) {
		return ErrUnmanaged
	}
	if err := os.Remove(dest); err != nil {
		return err
	}
	entries, err := os.ReadDir(parent)
	if err != nil {
		return err
	}
	if len(entries) == 0 {
		if err := os.Remove(parent); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	return nil
}

// needsWrite reports whether dest must be replaced with the canonical skill.
// A missing file is written. An identical regular file is left in place.
func needsWrite(dest string) (bool, error) {
	info, err := os.Lstat(dest)
	if errors.Is(err, os.ErrNotExist) {
		return true, nil
	}
	if err != nil {
		return false, err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return false, fmt.Errorf("SKILL.md: %w", ErrSymlink)
	}
	if !info.Mode().IsRegular() {
		return false, fmt.Errorf("SKILL.md: %w", ErrNotRegular)
	}
	existing, err := readRegular(dest)
	if err != nil {
		return false, err
	}
	if bytes.Equal(existing, skillDocument) {
		return false, nil
	}
	if !isManaged(existing) {
		return false, ErrUnmanaged
	}
	return true, nil
}

func resolveRel(base, rel string) (string, []string, error) {
	root, err := cleanBase(base)
	if err != nil {
		return "", nil, err
	}
	parts, err := splitRel(rel)
	if err != nil {
		return "", nil, err
	}
	if err := contained(root, parts); err != nil {
		return "", nil, err
	}
	return root, parts, nil
}

func cleanBase(base string) (string, error) {
	if base == "" {
		return "", errors.New("skill base path is empty")
	}
	abs, err := filepath.Abs(base)
	if err != nil {
		return "", err
	}
	abs = filepath.Clean(abs)
	info, err := os.Stat(abs)
	if err != nil {
		return "", err
	}
	if !info.IsDir() {
		return "", fmt.Errorf("skill base is not a directory: %s", abs)
	}
	return abs, nil
}

func splitRel(rel string) ([]string, error) {
	if rel == "" || path.IsAbs(rel) || filepath.IsAbs(rel) {
		return nil, errors.New("skill path must be relative")
	}
	if strings.Contains(rel, "\\") || strings.ContainsRune(rel, 0) {
		return nil, errors.New("skill path is invalid")
	}
	if path.Clean(rel) != rel {
		return nil, errors.New("skill path is not clean")
	}
	parts := strings.Split(rel, "/")
	if len(parts) < 2 {
		return nil, errors.New("skill path is invalid")
	}
	for _, part := range parts {
		if part == "" || part == "." || part == ".." {
			return nil, errors.New("skill path escapes the base")
		}
	}
	if parts[len(parts)-1] != skillFile || parts[len(parts)-2] != skillDir {
		return nil, errors.New("skill path must end in nodex/SKILL.md")
	}
	return parts, nil
}

func contained(root string, parts []string) error {
	full := root
	for _, part := range parts {
		full = filepath.Join(full, part)
		rel, err := filepath.Rel(root, full)
		if err != nil {
			return err
		}
		if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return errors.New("skill path escapes the base")
		}
	}
	return nil
}

// ensureParents creates missing directories for parts and returns the final
// directory. Created directories are removed if a later component is rejected.
func ensureParents(root string, parts []string) (string, []string, error) {
	current := root
	var created []string
	for _, part := range parts {
		next := filepath.Join(current, part)
		info, err := os.Lstat(next)
		if errors.Is(err, os.ErrNotExist) {
			if mkErr := os.Mkdir(next, 0o755); mkErr != nil {
				if !errors.Is(mkErr, os.ErrExist) {
					removeCreated(created)
					return "", nil, mkErr
				}
			} else {
				created = append(created, next)
				if syncErr := syncDir(filepath.Dir(next)); syncErr != nil {
					removeCreated(created)
					return "", nil, syncErr
				}
			}
			info, err = os.Lstat(next)
		}
		if err != nil {
			removeCreated(created)
			return "", nil, err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			removeCreated(created)
			return "", nil, fmt.Errorf("%s: %w", part, ErrSymlink)
		}
		if !info.IsDir() {
			removeCreated(created)
			return "", nil, fmt.Errorf("%s is not a directory", part)
		}
		current = next
	}
	return current, created, nil
}

// existingParents walks parts without creating directories. A missing
// component returns errAbsent. A symbolic link is rejected.
func existingParents(root string, parts []string) (string, error) {
	current := root
	for _, part := range parts {
		next := filepath.Join(current, part)
		info, err := os.Lstat(next)
		if errors.Is(err, os.ErrNotExist) {
			return "", errAbsent
		}
		if err != nil {
			return "", err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return "", fmt.Errorf("%s: %w", part, ErrSymlink)
		}
		if !info.IsDir() {
			return "", fmt.Errorf("%s is not a directory", part)
		}
		current = next
	}
	return current, nil
}

func removeCreated(dirs []string) {
	for i := len(dirs) - 1; i >= 0; i-- {
		entries, err := os.ReadDir(dirs[i])
		if err != nil || len(entries) != 0 {
			return
		}
		if err := os.Remove(dirs[i]); err != nil {
			return
		}
	}
}

func readRegular(path string) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("SKILL.md: %w", ErrNotRegular)
	}
	return io.ReadAll(f)
}

func isManaged(data []byte) bool {
	return bytes.Contains(data, []byte(Marker))
}

func writeTemp(dir string, data []byte) (string, error) {
	f, err := os.CreateTemp(dir, ".skill-*")
	if err != nil {
		return "", err
	}
	name := f.Name()
	remove := true
	defer func() {
		if remove {
			_ = f.Close()
			_ = os.Remove(name)
		}
	}()
	n, err := f.Write(data)
	if err != nil {
		return "", err
	}
	if n != len(data) {
		return "", io.ErrShortWrite
	}
	if err := f.Chmod(0o644); err != nil {
		return "", err
	}
	if err := f.Sync(); err != nil {
		return "", err
	}
	if err := f.Close(); err != nil {
		return "", err
	}
	remove = false
	return name, nil
}

func syncDir(dir string) error {
	f, err := os.Open(dir)
	if err != nil {
		return err
	}
	defer f.Close()
	return f.Sync()
}
