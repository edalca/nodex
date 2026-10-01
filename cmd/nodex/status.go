package main

import (
	"errors"
	"fmt"

	"github.com/edalca/nodex/internal/index"
)

// status reports whether the generated index is missing, current, stale,
// or corrupt.
//
// Those four results are successful inspections. An error is returned only
// when the project cannot be read far enough to decide. status does not
// create or modify the index.
func status(dir string) (string, error) {
	state, err := openProjectState(dir)
	if err != nil {
		return "", err
	}
	_, snap, err := index.Load(state.root)
	if err != nil {
		if errors.Is(err, index.ErrAbsent) {
			return "index: missing\n", nil
		}
		if errors.Is(err, index.ErrCorrupt) {
			return "index: corrupt\n", nil
		}
		return "", err
	}
	sources := state.sources()
	if index.Current(snap, state.policy.Identity(), sources) {
		return fmt.Sprintf("index: current\nsources: %d\ncomments: %d\n", len(snap.Sources), snap.CommentCount), nil
	}
	return fmt.Sprintf("index: stale\nindexed sources: %d\ncurrent sources: %d\ncomments: %d\n",
		len(snap.Sources), len(sources), snap.CommentCount), nil
}
