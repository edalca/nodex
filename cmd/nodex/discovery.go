package main

import (
	"fmt"
	"io/fs"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/edalca/nodex/internal/index"
)

// discoveryFilters selects loaded facts without changing their order or values.
// Empty categories accept every fact; values within a category are alternatives.
type discoveryFilters struct {
	files []string
	kinds []string
	names []string
}

// parseDiscoveryFilters validates command-local selectors before opening state.
// Each flag occurs once and consumes consecutive values. Values beginning with
// a dash are options, so unsupported options cannot become selectors.
func parseDiscoveryFilters(command string, args []string) (discoveryFilters, error) {
	var filters discoveryFilters
	for i := 0; i < len(args); {
		arg := args[i]
		if !strings.HasPrefix(arg, "--") {
			return discoveryFilters{}, fmt.Errorf("index %s: unexpected argument %q", command, arg)
		}
		name, value, hasValue := splitFlag(arg)
		var values *[]string
		switch {
		case name == "file":
			values = &filters.files
		case name == "kind" && command == "declarations":
			values = &filters.kinds
		case name == "name" && command == "declarations":
			values = &filters.names
		default:
			return discoveryFilters{}, fmt.Errorf("index %s: unsupported filter %q", command, arg)
		}
		if len(*values) > 0 {
			return discoveryFilters{}, fmt.Errorf("--%s was provided more than once", name)
		}
		i++
		var raw []string
		if hasValue {
			raw = append(raw, value)
		}
		for i < len(args) && !strings.HasPrefix(args[i], "-") {
			raw = append(raw, args[i])
			i++
		}
		if len(raw) == 0 {
			return discoveryFilters{}, fmt.Errorf("--%s requires at least one value", name)
		}
		for _, selector := range raw {
			if selector == "" || !utf8.ValidString(selector) || strings.ContainsRune(selector, 0) || strings.HasPrefix(selector, "-") {
				return discoveryFilters{}, fmt.Errorf("--%s: invalid value %q", name, selector)
			}
			if name == "file" && (selector == "." || !fs.ValidPath(selector) || strings.ContainsRune(selector, '\\')) {
				return discoveryFilters{}, fmt.Errorf("--file: invalid logical relative path %q", selector)
			}
			if !slices.Contains(*values, selector) {
				*values = append(*values, selector)
			}
		}
	}
	return filters, nil
}

func (f discoveryFilters) matchFile(logical string) bool {
	if len(f.files) == 0 {
		return true
	}
	for _, selector := range f.files {
		if logical == selector || strings.HasPrefix(logical, selector+"/") {
			return true
		}
	}
	return false
}

func (f discoveryFilters) matchDeclaration(decl index.Declaration) bool {
	if !f.matchFile(decl.Path) || (len(f.kinds) > 0 && !slices.Contains(f.kinds, string(decl.Kind))) {
		return false
	}
	if len(f.names) == 0 {
		return true
	}
	for _, name := range decl.Names {
		if slices.Contains(f.names, name) {
			return true
		}
	}
	return false
}
