// Package contracts defines the private agreement between the syntax facade
// and a complete language implementation. It does not prescribe parser internals.
package contracts

import (
	"errors"
	"strings"

	"github.com/edalca/nodex/internal/syntax/types"
)

// Language supplies every syntax capability of one supported language.
// Implementations are immutable and safe for concurrent calls. They interpret
// caller-supplied bytes without opening paths, walking files, or applying manual
// exclusions. ID and catalog identifiers are nonempty and globally unique.
type Language interface {
	// ID is the stable language identifier used in source fingerprints.
	ID() string
	// Recognize uses only a case-sensitive logical path suffix, never contents.
	Recognize(logicalPath string) bool
	// Parse returns all comment units in physical order and declaration facts
	// from one parse. Docs contains zero or more direct parser-owned physical
	// documentation ranges in source order, without duplicates, and is non-nil. Nil
	// source means empty input. Failure returns nil and a *ParseError, with no
	// partial document. Languages define their structural recovery policy;
	// success does not certify language validity. Positions remain physical
	// despite source directives.
	Parse(source []byte) (*Document, error)
	// Context identifies exactly one comment by its physical range and returns
	// a contiguous original source slice bounded by limits. Failure returns a
	// zero snippet and a range sentinel below, or *ParseError when source
	// structure is unavailable.
	Context(source []byte, target types.Range, limits Limits) (types.Snippet, error)
	// DeclarationContext identifies exactly one declaration by its physical
	// range. The bounded original slice keeps the declaration's starting line.
	// Failure returns a zero snippet and a declaration sentinel, or *ParseError.
	DeclarationContext(source []byte, target types.Range, limits Limits) (types.Snippet, error)
	// Presets returns a fresh catalog. Aggregates contain only concrete presets
	// owned by this language; aggregate identifiers are never persisted.
	Presets() PresetCatalog
	// PathExcluded evaluates enabled concrete presets without inspecting source.
	PathExcluded(logicalPath string, enabled []string) bool
	// SourceExcluded evaluates structural presets, inspecting source only when
	// one is enabled. Path-only exclusions are coordinated by the facade.
	SourceExcluded(logicalPath string, source []byte, enabled []string) bool
}

// Document is the complete result of one parse, without path, language identity,
// or parser-specific objects. The facade supplies path and identity. Successful
// comment and declaration slices are non-nil, including empty results.
type Document struct {
	Comments     []types.Comment
	Declarations []types.Declaration
}

// Limits bounds context in physical lines and bytes outside the anchored span.
// The anchor is the comment or the declaration's first physical line. Lines at
// or below zero mean one; negative ExtraBytes means zero. An anchor exceeding
// Lines keeps its start and only the following lines that fit.
type Limits struct {
	Lines      int
	ExtraBytes int
}

// PresetCatalog describes the concrete exclusions and command-line aggregates
// contributed by one language. All slices belong to the caller.
type PresetCatalog struct {
	Concrete   []string
	Aggregates []Aggregate
}

// Aggregate expands ID to a nonempty set of this language's concrete presets.
// Members must be unique. ID must differ from every concrete or aggregate ID.
type Aggregate struct {
	ID      string
	Presets []string
}

// ParseError carries language-neutral diagnostics in physical source order.
// It contains no logical filename; the facade adds the caller's path.
type ParseError struct {
	Diagnostics []types.Diagnostic
}

// Error reports diagnostic messages without a filename.
func (e *ParseError) Error() string {
	if e == nil || len(e.Diagnostics) == 0 {
		return "syntax error"
	}
	parts := make([]string, len(e.Diagnostics))
	for i, diagnostic := range e.Diagnostics {
		parts[i] = diagnostic.Msg
	}
	return strings.Join(parts, "\n")
}

var (
	// ErrMalformedRange means the comment range has invalid physical coordinates.
	ErrMalformedRange = errors.New("comment range is malformed")
	// ErrCommentNotFound means the range does not exactly identify a comment unit.
	ErrCommentNotFound = errors.New("comment range does not match a comment")
	// ErrAmbiguousComment means the range identifies multiple comment units.
	ErrAmbiguousComment = errors.New("comment range matches more than one comment")
	// ErrMalformedDeclaration means the declaration range has invalid coordinates.
	ErrMalformedDeclaration = errors.New("declaration range is malformed")
	// ErrDeclarationNotFound means the range does not exactly identify a declaration.
	ErrDeclarationNotFound = errors.New("declaration range does not match a declaration")
	// ErrAmbiguousDeclaration means the range identifies multiple declarations.
	ErrAmbiguousDeclaration = errors.New("declaration range matches more than one declaration")
)
