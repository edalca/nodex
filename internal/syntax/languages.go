package syntax

import (
	"fmt"
	"sort"
	"strings"

	"github.com/edalca/nodex/internal/syntax/contracts"
	"github.com/edalca/nodex/internal/syntax/golang"
)

// Go is the stable Go language identifier exposed by the syntax facade.
const Go Language = "go"

// languages is the sole composition point for complete language capabilities.
var languages = mustRegistry([]contracts.Language{
	golang.New(),
})

type registry struct {
	implementations []contracts.Language
	concrete        []string
	selectors       []string
	expansions      map[string][]string
}

func mustRegistry(implementations []contracts.Language) registry {
	r, err := newRegistry(implementations)
	if err != nil {
		panic(err)
	}
	return r
}

// newRegistry validates ownership and derives selector lookups from the same
// language objects used for recognition, parsing, context, and exclusion.
func newRegistry(implementations []contracts.Language) (registry, error) {
	r := registry{
		implementations: append([]contracts.Language(nil), implementations...),
		expansions:      make(map[string][]string),
	}
	ids := make(map[string]bool)
	for _, implementation := range r.implementations {
		if implementation == nil || implementation.ID() == "" {
			return registry{}, fmt.Errorf("syntax registry: missing language identity")
		}
		id := implementation.ID()
		if ids[id] {
			return registry{}, fmt.Errorf("syntax registry: duplicate language %q", id)
		}
		ids[id] = true
		catalog := implementation.Presets()
		owned := make(map[string]bool)
		for _, preset := range catalog.Concrete {
			if preset == "" {
				return registry{}, fmt.Errorf("syntax registry: empty preset in %q", id)
			}
			if _, exists := r.expansions[preset]; exists {
				return registry{}, fmt.Errorf("syntax registry: duplicate selector %q", preset)
			}
			owned[preset] = true
			r.concrete = append(r.concrete, preset)
			r.expansions[preset] = []string{preset}
		}
		for _, aggregate := range catalog.Aggregates {
			if aggregate.ID == "" || len(aggregate.Presets) == 0 {
				return registry{}, fmt.Errorf("syntax registry: empty aggregate in %q", id)
			}
			if _, exists := r.expansions[aggregate.ID]; exists {
				return registry{}, fmt.Errorf("syntax registry: duplicate selector %q", aggregate.ID)
			}
			members := make(map[string]bool)
			for _, preset := range aggregate.Presets {
				if !owned[preset] || members[preset] {
					return registry{}, fmt.Errorf("syntax registry: invalid member %q in aggregate %q", preset, aggregate.ID)
				}
				members[preset] = true
			}
			expansion := append([]string(nil), aggregate.Presets...)
			sort.Strings(expansion)
			r.expansions[aggregate.ID] = expansion
		}
	}
	for selector := range r.expansions {
		r.selectors = append(r.selectors, selector)
	}
	sort.Strings(r.concrete)
	sort.Strings(r.selectors)
	return r, nil
}

// resolve rejects overlapping ownership as an internal invariant. The facade's
// recognition API has no configuration-error result, so ambiguity panics with
// all claiming identities in lexical order instead of choosing an owner.
func (r registry) resolve(logicalPath string) contracts.Language {
	var selected contracts.Language
	var owners []string
	for _, implementation := range r.implementations {
		if implementation.Recognize(logicalPath) {
			selected = implementation
			owners = append(owners, implementation.ID())
		}
	}
	if len(owners) > 1 {
		sort.Strings(owners)
		panic(fmt.Sprintf("syntax registry: ambiguous path %q claimed by %s", logicalPath, strings.Join(owners, ", ")))
	}
	return selected
}

func (r registry) expandSelector(selector string) ([]string, error) {
	ids, ok := r.expansions[selector]
	if !ok {
		return nil, fmt.Errorf("unsupported preset %q", selector)
	}
	return append([]string{}, ids...), nil
}

func (r registry) validatePresets(ids []string) error {
	known := make(map[string]struct{}, len(r.concrete))
	for _, id := range r.concrete {
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

func (r registry) pathExcluded(logicalPath string, enabled []string) bool {
	if len(enabled) == 0 {
		return false
	}
	implementation := r.resolve(logicalPath)
	return implementation != nil && implementation.PathExcluded(logicalPath, enabled)
}

func (r registry) inspectExcluded(logicalPath string, source []byte, enabled []string) bool {
	if len(enabled) == 0 {
		return false
	}
	implementation := r.resolve(logicalPath)
	if implementation == nil {
		return false
	}
	return implementation.PathExcluded(logicalPath, enabled) || implementation.SourceExcluded(logicalPath, source, enabled)
}
