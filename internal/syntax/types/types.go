// Package types holds the physical source vocabulary shared inside syntax.
// It contains no parser trees, filesystem state, or language policy.
package types

// Position identifies a physical byte: Offset is zero-based; Line and Column
// are one-based, and Column counts bytes. Source directives do not relocate it.
type Position struct {
	Offset int
	Line   int
	Column int
}

// Range is a half-open physical interval [Start, End) in caller-supplied source.
type Range struct {
	Start Position
	End   Position
}

// Comment is one language-defined comment unit. Raw is the exact source slice;
// Text is normalized content preserving directives. Start and End bound Raw.
type Comment struct {
	Raw   string
	Text  string
	Start Position
	End   Position
}

// Declaration is a structural fact with names in source order and a half-open
// physical extent. Kind names its neutral form. Docs contains only direct
// parser-owned documentation ranges in physical source order, without duplicates.
// Docs is non-nil, including when empty. An unnamed declaration has no synthetic name.
type Declaration struct {
	Kind  string
	Names []string
	Start Position
	End   Position
	Docs  []Range
}

// Snippet is the exact contiguous source slice [Start, End), without formatting
// or inserted ellipses. It carries no syntax tree.
type Snippet struct {
	Start Position
	End   Position
	Text  string
}

// Diagnostic is a parser message at a physical source position.
type Diagnostic struct {
	Position Position
	Msg      string
}
