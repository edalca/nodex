package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/edalca/nodex/internal/ignore"
	"github.com/edalca/nodex/internal/index"
	"github.com/edalca/nodex/internal/project"
	"github.com/edalca/nodex/internal/syntax"
)

// errNoGeneratedIndex means a command was asked to read an index that has
// not been generated.
var errNoGeneratedIndex = errors.New("no generated index; run nodex generate")

// errStaleIndex means the generated index does not match the current
// project inputs. The index is not regenerated.
var errStaleIndex = errors.New("index is stale; run nodex generate")

// projectState is the current project input used to compare a snapshot.
//
// files are the included supported sources in discovery order. Each body is
// the exact byte sequence its fingerprint was computed from.
type projectState struct {
	root   string
	policy ignore.Policy
	files  []supportedFile
}

// supportedFile is one included source file recognized by syntax.
type supportedFile struct {
	source index.Source
	body   []byte
}

// sources returns the fingerprints in discovery order.
func (s projectState) sources() []index.Source {
	out := make([]index.Source, len(s.files))
	for i, file := range s.files {
		out[i] = file.source
	}
	return out
}

// openProjectState reads the inputs that decide whether a snapshot is current.
//
// dir is opened as the project root. The effective ignore policy is
// .nodex/ignore.json, or the zero policy when that document is absent.
// Included files come from the project. Manual exclusions are applied there.
// Only paths the syntax facade recognizes are candidates. Enabled presets
// then drop supported source. A path-only preset is applied before the file
// is read. A structural preset may inspect the file bytes. Each remaining
// file becomes a fingerprint of its logical path, language, and exact bytes.
// Comment text is not parsed.
func openProjectState(dir string) (projectState, error) {
	opened, err := project.Open(dir)
	if err != nil {
		return projectState{}, err
	}
	policy, err := loadPolicy(opened.Root())
	if err != nil {
		return projectState{}, err
	}
	files, err := selectedSources(opened, policy)
	if err != nil {
		return projectState{}, err
	}
	return projectState{root: opened.Root(), policy: policy, files: files}, nil
}

// selectedSources returns the supported sources that remain after manual
// exclusions and enabled presets.
//
// Discovery order is preserved. A file excluded by a preset is omitted
// before its fingerprint is recorded. Source bytes are parsed only when an
// enabled preset classifies files by structure.
func selectedSources(opened *project.Project, policy ignore.Policy) ([]supportedFile, error) {
	enabled := policy.Presets()
	if err := syntax.ValidatePresets(enabled); err != nil {
		return nil, err
	}
	files, err := opened.Files(policy)
	if err != nil {
		return nil, err
	}
	supported := make([]supportedFile, 0)
	for _, file := range files {
		language, ok := syntax.Recognize(file)
		if !ok {
			continue
		}
		if syntax.PathExcluded(file, enabled) {
			continue
		}
		body, err := opened.ReadFile(file)
		if err != nil {
			return nil, err
		}
		if syntax.InspectExcluded(file, body, enabled) {
			continue
		}
		supported = append(supported, supportedFile{
			source: index.Source{
				Path:     file,
				Language: language,
				Digest:   index.DigestBytes(body),
			},
			body: body,
		})
	}
	return supported, nil
}

// openCurrentIndex loads the generated index and accepts it only when the
// snapshot matches the current project inputs.
//
// A missing index, a corrupt index, and a stale index are errors. Nothing
// is regenerated. Comment text is not parsed. An enabled structural preset
// may classify source bytes while the source set is selected.
func openCurrentIndex(dir string) (*index.Index, index.Snapshot, projectState, error) {
	state, err := openProjectState(dir)
	if err != nil {
		return nil, index.Snapshot{}, projectState{}, err
	}
	idx, snap, err := index.Load(state.root)
	if err != nil {
		if errors.Is(err, index.ErrAbsent) {
			return nil, index.Snapshot{}, projectState{}, errNoGeneratedIndex
		}
		return nil, index.Snapshot{}, projectState{}, err
	}
	if !index.Current(snap, state.policy.Identity(), state.sources()) {
		return nil, index.Snapshot{}, projectState{}, errStaleIndex
	}
	return idx, snap, state, nil
}

// loadPolicy reads .nodex/ignore.json. A missing document is the zero policy.
// The file is not created. A symbolic link is not followed.
func loadPolicy(root string) (ignore.Policy, error) {
	nodex := filepath.Join(root, ".nodex")
	info, err := os.Lstat(nodex)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return ignore.Policy{}, nil
		}
		return ignore.Policy{}, err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return ignore.Policy{}, errors.New(".nodex is a symbolic link")
	}
	if !info.IsDir() {
		return ignore.Policy{}, errors.New(".nodex is not a directory")
	}
	document := filepath.Join(root, filepath.FromSlash(ignore.DocumentPath))
	info, err = os.Lstat(document)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return ignore.Policy{}, nil
		}
		return ignore.Policy{}, err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return ignore.Policy{}, fmt.Errorf("%s is a symbolic link", ignore.DocumentPath)
	}
	if !info.Mode().IsRegular() {
		return ignore.Policy{}, fmt.Errorf("%s is not a regular file", ignore.DocumentPath)
	}
	data, err := os.ReadFile(document)
	if err != nil {
		return ignore.Policy{}, err
	}
	policy, err := ignore.Parse(data)
	if err != nil {
		return ignore.Policy{}, err
	}
	if err := syntax.ValidatePresets(policy.Presets()); err != nil {
		return ignore.Policy{}, err
	}
	return policy, nil
}
