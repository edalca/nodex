package syntax

import (
	"fmt"

	"github.com/edalca/nodex/internal/syntax/golang"
)

// Selectors returns the command-line preset selectors known to this build,
// in lexical order. The result includes concrete presets and aggregate
// selectors. It is a copy.
func Selectors() []string {
	return golang.Selectors()
}

// ConcretePresets returns the preset identifiers this build can persist,
// in lexical order. Aggregate selectors are not included. The result is a copy.
func ConcretePresets() []string {
	return golang.ConcretePresets()
}

// ExpandSelector returns the concrete preset identifiers selected by selector.
//
// A concrete identifier selects itself. An aggregate selector expands to the
// concrete presets of that language known to this build. The result is a new
// slice in lexical order and never contains the aggregate selector. An
// unknown selector is rejected.
func ExpandSelector(selector string) ([]string, error) {
	ids, ok := golang.Expand(selector)
	if !ok {
		return nil, fmt.Errorf("unsupported preset %q", selector)
	}
	return ids, nil
}

// ValidatePresets reports whether every id is a concrete preset this build
// can persist. Aggregate selectors and unknown identifiers are rejected.
// An empty list is valid. The list is not reordered.
func ValidatePresets(ids []string) error {
	known := make(map[string]struct{}, len(ConcretePresets()))
	for _, id := range ConcretePresets() {
		known[id] = struct{}{}
	}
	seen := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		if _, ok := known[id]; !ok {
			return fmt.Errorf("unsupported preset %q", id)
		}
		if _, ok := seen[id]; ok {
			return fmt.Errorf("duplicate preset %q", id)
		}
		seen[id] = struct{}{}
	}
	return nil
}

// PathExcluded reports whether enabled presets exclude logicalPath from its
// path alone.
//
// logicalPath is a slash-separated logical project path. enabled holds
// concrete preset identifiers. An unrecognized path is not excluded. Presets
// that need source text are not applied here, and source is not read.
func PathExcluded(logicalPath string, enabled []string) bool {
	if len(enabled) == 0 {
		return false
	}
	lang, ok := Recognize(logicalPath)
	if !ok {
		return false
	}
	switch lang {
	case Go:
		return golang.PathExcluded(logicalPath, enabled)
	default:
		return false
	}
}

// InspectExcluded reports whether enabled presets exclude logicalPath after
// structural inspection.
//
// source is the file bytes. It is parsed only when an enabled preset
// classifies files by structure. With no such preset, source is not
// inspected and may be nil. Path-only presets are also applied, so a caller
// that already dropped path exclusions can still use this as the full
// decision. An unrecognized path is not excluded.
func InspectExcluded(logicalPath string, source []byte, enabled []string) bool {
	if PathExcluded(logicalPath, enabled) {
		return true
	}
	if len(enabled) == 0 {
		return false
	}
	lang, ok := Recognize(logicalPath)
	if !ok {
		return false
	}
	switch lang {
	case Go:
		return golang.SourceExcluded(logicalPath, source, enabled)
	default:
		return false
	}
}
