// Package index gives language-neutral comments snapshot-local identities.
//
// Build copies comments out of the supplied documents, orders them, and
// assigns IDs. Documents may arrive in any order, and the comments inside a
// document may arrive in any order. That order does not change a successful
// result. The canonical order is the logical path, compared byte for byte,
// then the physical source position: start offset, end offset, start line,
// start column, end line, and end column. IDs are assigned only after that
// order is fixed. The first comment is C000001. Each following comment
// receives the next decimal ordinal. The written form uses at least six
// digits, so an ordinal beyond 999999 stays a longer ID instead of wrapping.
//
// An ID names a comment inside the index that assigned it. It is not a
// permanent name. A later index that includes a comment sorting earlier
// assigns new IDs to the comments that follow.
//
// A path must already be a canonical logical path. It is non-empty, relative,
// and slash-separated. It is not "." and it does not escape above the project
// root. It is equal to its cleaned slash form. Build checks the path and does
// not rewrite it, clean it, or resolve it through the filesystem. A nil
// document is rejected. Two documents with the same path are rejected,
// including when either document has no comments. A document with no comments
// contributes no entries. Two comments in one document that share a physical
// range are rejected, because an ID would otherwise depend on the order those
// comments were supplied.
//
// A comment range is coherent when both offsets are zero or positive, the end
// offset is not before the start offset, and every line and column is at
// least 1. Empty normalized text is stored as supplied. Text is not trimmed
// or interpreted. Build does not read files, match ignore rules, detect a
// language, or parse source.
//
// Declarations are indexed from the same documents. Their IDs are
// independent of comment IDs. The first declaration is D000001. The written
// form uses at least six digits, with the same rule for wider ordinals.
// Declaration order is the logical path, then the physical start offset,
// then the physical end offset, then the declaration kind when those ranges
// are equal. Kind is a structural tie-breaker. Documentation text, names,
// and the order of the input slice are not keys. A declaration's
// documentation relationships resolve each parser-owned physical range to
// exactly one comment in the same document. The declaration stores those
// comment IDs in physical source order, without comment text. A range that
// matches no comment in that document is an indexing failure, and Build
// returns no index. A declaration with no associated documentation comment
// is stored with an empty, non-nil Docs collection. Duplicate ranges fail.
//
// Persist writes a finished index under the workspace base. The persisted
// state uses SchemaVersion and three files: comments.jsonl, declarations.jsonl,
// then snapshot.json. snapshot.json is the commit marker. It records the
// caller-supplied ignore policy identity, the SHA-256 digest and count of
// each derived file, and one fingerprint for each included source file. A
// fingerprint is the logical path, the language, and the SHA-256 digest of
// the exact source bytes. Load reads that set back into an Index and rejects
// a missing derived file, a digest mismatch, a count mismatch, or a record
// that violates the same invariants as Build. Current compares a snapshot
// with a newly computed policy identity and source fingerprints. Comment
// text, declaration facts, and file modification times are not part of that
// comparison.
//
// The package does not discover files, match ignore rules, or parse source.
// It does not store absolute paths, timestamps, or machine identity.
//
// A successful Index is immutable. Lookup and Entries are safe for concurrent
// callers. Lookup answers from the built collection and does not construct
// another index. Entries returns a copy. A failed Build returns no index.
package index

import (
	"cmp"
	"errors"
	"fmt"
	"path"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/edalca/nodex/internal/syntax"
)

const idWidth = 6

var (
	// ErrEmptyPath is returned when a document path is empty.
	ErrEmptyPath = errors.New("path is empty")

	// ErrDotPath is returned when a document path is ".".
	ErrDotPath = errors.New("path is \".\"")

	// ErrAbsolutePath is returned when a document path is slash-absolute.
	ErrAbsolutePath = errors.New("path is absolute")

	// ErrUncleanPath is returned when a document path is not already clean.
	ErrUncleanPath = errors.New("path is not clean")

	// ErrEscapingPath is returned when a document path escapes the project root.
	ErrEscapingPath = errors.New("path escapes the project root")

	// ErrNegativeOffset is returned when a comment offset is negative.
	ErrNegativeOffset = errors.New("offset is negative")

	// ErrEndBeforeStart is returned when a comment end offset precedes its start.
	ErrEndBeforeStart = errors.New("end offset precedes the start offset")

	// ErrPosition is returned when a comment line or column is less than 1.
	ErrPosition = errors.New("line and column must be at least 1")

	// ErrInvalidID is returned when text is not a canonical comment ID.
	ErrInvalidID = errors.New("invalid comment ID")

	// ErrIDOverflow is returned when a comment ID does not fit in uint64.
	ErrIDOverflow = errors.New("comment ID overflows")

	// ErrInvalidDeclID is returned when text is not a canonical declaration ID.
	ErrInvalidDeclID = errors.New("invalid declaration ID")

	// ErrDeclIDOverflow is returned when a declaration ID does not fit in uint64.
	ErrDeclIDOverflow = errors.New("declaration ID overflows")

	// ErrEmptyKind is returned when a declaration kind is empty.
	ErrEmptyKind = errors.New("declaration kind is empty")

	// ErrDuplicateDoc means a declaration repeats a direct documentation range or ID.
	ErrDuplicateDoc = errors.New("duplicate direct documentation relationship")

	// ErrDocsOutOfOrder means persisted documentation IDs are not in physical source order.
	ErrDocsOutOfOrder = errors.New("direct documentation relationships are not in physical source order")
)

// NilDocumentError reports a nil document in the slice passed to Build.
// Index is the position of that document.
type NilDocumentError struct {
	Index int
}

// Error reports the input position.
func (e *NilDocumentError) Error() string {
	if e == nil {
		return "nil document"
	}
	return fmt.Sprintf("document [%d] is nil", e.Index)
}

// PathError reports a logical path Build cannot index.
// Index is the position of the document in the input slice.
type PathError struct {
	Index int
	Path  string
	Err   error
}

// Error reports the document position, the path, and the cause.
func (e *PathError) Error() string {
	if e == nil {
		return "invalid document path"
	}
	if e.Err == nil {
		return fmt.Sprintf("document [%d] path %q is invalid", e.Index, e.Path)
	}
	return fmt.Sprintf("document [%d] path %q: %v", e.Index, e.Path, e.Err)
}

// Unwrap returns the cause of the path failure.
func (e *PathError) Unwrap() error { return e.Err }

// DuplicatePathError reports two documents that use one logical path.
// FirstIndex and Index are their positions in the input slice.
type DuplicatePathError struct {
	Path       string
	FirstIndex int
	Index      int
}

// Error reports the path and both input positions.
func (e *DuplicatePathError) Error() string {
	if e == nil {
		return "duplicate document path"
	}
	return fmt.Sprintf("duplicate document path %q at documents [%d] and [%d]", e.Path, e.FirstIndex, e.Index)
}

// CommentError reports a comment Build cannot index.
// Index is the position of the comment in the document that supplied it.
type CommentError struct {
	Path  string
	Index int
	Range syntax.Range
	Err   error
}

// Error reports the path, the comment position, the physical range, and the cause.
func (e *CommentError) Error() string {
	if e == nil {
		return "invalid comment"
	}
	loc := fmt.Sprintf("%s: comment [%d] bytes [%d,%d)", e.Path, e.Index, e.Range.Start.Offset, e.Range.End.Offset)
	if e.Err == nil {
		return loc + " is invalid"
	}
	return loc + ": " + e.Err.Error()
}

// Unwrap returns the cause of the comment failure.
func (e *CommentError) Unwrap() error { return e.Err }

// DuplicatePositionError reports two comments in one document that share a
// physical range. Index and Other are their positions in that document's
// comment slice, and Index is the earlier position.
type DuplicatePositionError struct {
	Path  string
	Index int
	Other int
	Range syntax.Range
}

// Error reports the path, both comment positions, and the shared range.
func (e *DuplicatePositionError) Error() string {
	if e == nil {
		return "duplicate comment position"
	}
	return fmt.Sprintf("%s: comments [%d] and [%d] share bytes [%d,%d)", e.Path, e.Index, e.Other, e.Range.Start.Offset, e.Range.End.Offset)
}

// DeclarationError reports a declaration Build cannot index.
// Index is the position of the declaration in the document that supplied it.
type DeclarationError struct {
	Path  string
	Index int
	Range syntax.Range
	Err   error
}

// Error reports the path, the declaration position, the physical range, and the cause.
func (e *DeclarationError) Error() string {
	if e == nil {
		return "invalid declaration"
	}
	loc := fmt.Sprintf("%s: declaration [%d] bytes [%d,%d)", e.Path, e.Index, e.Range.Start.Offset, e.Range.End.Offset)
	if e.Err == nil {
		return loc + " is invalid"
	}
	return loc + ": " + e.Err.Error()
}

// Unwrap returns the cause of the declaration failure.
func (e *DeclarationError) Unwrap() error { return e.Err }

// DuplicateDeclarationError reports two declarations in one document that
// share a physical range and kind. Index and Other are their positions in
// that document's declaration slice, and Index is the earlier position.
type DuplicateDeclarationError struct {
	Path  string
	Index int
	Other int
	Range syntax.Range
}

// Error reports the path, both declaration positions, and the shared range.
func (e *DuplicateDeclarationError) Error() string {
	if e == nil {
		return "duplicate declaration position"
	}
	return fmt.Sprintf("%s: declarations [%d] and [%d] share bytes [%d,%d)", e.Path, e.Index, e.Other, e.Range.Start.Offset, e.Range.End.Offset)
}

// UnresolvedDocError reports a documentation range that is not the physical
// range of a comment in the same document. Build returns no index.
type UnresolvedDocError struct {
	Path     string
	Index    int
	Range    syntax.Range
	DocRange syntax.Range
}

// Error reports the declaration and the documentation range that did not match.
func (e *UnresolvedDocError) Error() string {
	if e == nil {
		return "documentation range does not match a comment"
	}
	return fmt.Sprintf("%s: declaration [%d] bytes [%d,%d) documentation range [%d,%d) does not match a comment",
		e.Path, e.Index, e.Range.Start.Offset, e.Range.End.Offset, e.DocRange.Start.Offset, e.DocRange.End.Offset)
}

// IDError reports text that is not a comment ID.
type IDError struct {
	Text string
	Err  error
}

// Error reports the rejected text and the cause.
// A very long text is shortened in the message. The Text field keeps the
// original value.
func (e *IDError) Error() string {
	if e == nil {
		return "invalid comment ID"
	}
	shown := e.Text
	if len(shown) > 64 {
		shown = shown[:64] + "..."
	}
	if e.Err == nil {
		return fmt.Sprintf("comment ID %q is invalid", shown)
	}
	return fmt.Sprintf("comment ID %q: %v", shown, e.Err)
}

// Unwrap returns the cause of the ID failure.
func (e *IDError) Unwrap() error { return e.Err }

// ID is the snapshot-local identity of one indexed comment.
//
// The canonical text is the uppercase letter C followed by the decimal
// ordinal, padded with ASCII zeros to at least six digits. Numbering starts
// at one. The zero ID is not valid, and Build does not produce it.
type ID struct {
	n uint64
}

// String returns the canonical text of id.
// The zero ID returns the empty string.
func (id ID) String() string {
	if id.n == 0 {
		return ""
	}
	return "C" + idDigitsText(id.n)
}

// Valid reports whether id is a comment ID produced by Build or ParseID.
// The zero ID is not valid.
func (id ID) Valid() bool { return id.n != 0 }

// ParseID reads the canonical text of a comment ID.
//
// The text must begin with uppercase C and continue with ASCII digits only.
// At least six digits are required. Those digits must be the canonical
// encoding of an ordinal from 1 through the maximum uint64 value: the width
// is six, or wider when the ordinal needs more digits, and a wider encoding
// has no leading zero. A value that does not fit in uint64 is rejected.
// ParseID does not wrap it.
func ParseID(text string) (ID, error) {
	fail := func(err error) (ID, error) {
		return ID{}, &IDError{Text: text, Err: err}
	}
	if !idDigits(text) {
		return fail(ErrInvalidID)
	}
	n, err := strconv.ParseUint(text[1:], 10, 64)
	if err != nil {
		if errors.Is(err, strconv.ErrRange) {
			return fail(ErrIDOverflow)
		}
		return fail(ErrInvalidID)
	}
	if n == 0 {
		return fail(ErrInvalidID)
	}
	id := ID{n: n}
	if id.String() != text {
		return fail(ErrInvalidID)
	}
	return id, nil
}

// idDigits reports whether text is C followed by at least six ASCII digits.
func idDigits(text string) bool {
	return prefixedDigits(text, 'C')
}

// DeclID is the snapshot-local identity of one indexed declaration.
//
// The canonical text is the uppercase letter D followed by the decimal
// ordinal, padded with ASCII zeros to at least six digits. Numbering starts
// at one. The zero DeclID is not valid, and Build does not produce it.
// Declaration IDs are a separate sequence from comment IDs.
type DeclID struct {
	n uint64
}

// String returns the canonical text of id.
// The zero DeclID returns the empty string.
func (id DeclID) String() string {
	if id.n == 0 {
		return ""
	}
	return "D" + idDigitsText(id.n)
}

// Valid reports whether id is a declaration ID produced by Build or ParseDeclID.
// The zero DeclID is not valid.
func (id DeclID) Valid() bool { return id.n != 0 }

// ParseDeclID reads the canonical text of a declaration ID.
//
// The text must begin with uppercase D and continue with ASCII digits only.
// At least six digits are required. Those digits must be the canonical
// encoding of an ordinal from 1 through the maximum uint64 value: the width
// is six, or wider when the ordinal needs more digits, and a wider encoding
// has no leading zero. A value that does not fit in uint64 is rejected.
// ParseDeclID does not wrap it. A comment ID is not a declaration ID.
func ParseDeclID(text string) (DeclID, error) {
	fail := func(err error) (DeclID, error) {
		return DeclID{}, &DeclIDError{Text: text, Err: err}
	}
	if !prefixedDigits(text, 'D') {
		return fail(ErrInvalidDeclID)
	}
	n, err := strconv.ParseUint(text[1:], 10, 64)
	if err != nil {
		if errors.Is(err, strconv.ErrRange) {
			return fail(ErrDeclIDOverflow)
		}
		return fail(ErrInvalidDeclID)
	}
	if n == 0 {
		return fail(ErrInvalidDeclID)
	}
	id := DeclID{n: n}
	if id.String() != text {
		return fail(ErrInvalidDeclID)
	}
	return id, nil
}

// DeclIDError reports text that is not a declaration ID.
type DeclIDError struct {
	Text string
	Err  error
}

// Error reports the rejected text and the cause.
// A very long text is shortened in the message. The Text field keeps the
// original value.
func (e *DeclIDError) Error() string {
	if e == nil {
		return "invalid declaration ID"
	}
	shown := e.Text
	if len(shown) > 64 {
		shown = shown[:64] + "..."
	}
	if e.Err == nil {
		return fmt.Sprintf("declaration ID %q is invalid", shown)
	}
	return fmt.Sprintf("declaration ID %q: %v", shown, e.Err)
}

// Unwrap returns the cause of the ID failure.
func (e *DeclIDError) Unwrap() error { return e.Err }

func prefixedDigits(text string, prefix byte) bool {
	if len(text) < idWidth+1 || text[0] != prefix {
		return false
	}
	for i := 1; i < len(text); i++ {
		if text[i] < '0' || text[i] > '9' {
			return false
		}
	}
	return true
}

func idDigitsText(n uint64) string {
	digits := strconv.FormatUint(n, 10)
	if pad := idWidth - len(digits); pad > 0 {
		digits = strings.Repeat("0", pad) + digits
	}
	return digits
}

// Entry is one indexed comment.
//
// Path, Language, Range, and Text are copies of the document values. ID is
// assigned from the canonical order. An entry does not carry raw source, a
// syntax tree, or a classification of the comment.
type Entry struct {
	ID       ID
	Path     string
	Language syntax.Language
	Range    syntax.Range
	Text     string
}

// Declaration is one indexed declaration.
//
// Path, Language, Kind, Names, and Range are copies of the document values.
// ID is assigned from the canonical declaration order. Docs contains the IDs
// of zero or more direct parser-owned documentation comments in the same source,
// ordered by physical position, without duplicates. Docs is non-nil, including
// when empty. A declaration carries no comment text, raw source, or syntax tree.
type Declaration struct {
	ID       DeclID
	Path     string
	Language syntax.Language
	Kind     syntax.Kind
	Names    []string
	Range    syntax.Range
	Docs     []ID
}

// Index is an immutable in-memory collection of indexed comments and
// declarations.
//
// The zero value and a nil pointer contain neither. Lookup, Entries, Len,
// LookupDeclaration, Declarations, and DeclarationLen do not modify the
// collection and are safe for concurrent callers.
type Index struct {
	entries  []Entry
	byID     map[uint64]int
	decls    []Declaration
	declByID map[uint64]int
}

// Len reports how many comments the index holds.
func (x *Index) Len() int {
	if x == nil {
		return 0
	}
	return len(x.entries)
}

// Lookup returns the comment named by id.
//
// The second result is false when id is not in the index, including when id
// is the zero ID or x is nil. Lookup does not rebuild the index.
func (x *Index) Lookup(id ID) (Entry, bool) {
	if x == nil || !id.Valid() {
		return Entry{}, false
	}
	i, ok := x.byID[id.n]
	if !ok || i < 0 || i >= len(x.entries) {
		return Entry{}, false
	}
	return x.entries[i], true
}

// Entries returns a copy of the comments in canonical order.
// The caller may change the returned slice without affecting the index.
func (x *Index) Entries() []Entry {
	n := 0
	if x != nil {
		n = len(x.entries)
	}
	out := make([]Entry, n)
	if n > 0 {
		copy(out, x.entries)
	}
	return out
}

// DeclarationLen reports how many declarations the index holds.
func (x *Index) DeclarationLen() int {
	if x == nil {
		return 0
	}
	return len(x.decls)
}

// LookupDeclaration returns the declaration named by id.
//
// The second result is false when id is not in the index, including when id
// is the zero DeclID or x is nil. LookupDeclaration does not rebuild the
// index. The Names slice of the result is a copy.
func (x *Index) LookupDeclaration(id DeclID) (Declaration, bool) {
	if x == nil || !id.Valid() {
		return Declaration{}, false
	}
	i, ok := x.declByID[id.n]
	if !ok || i < 0 || i >= len(x.decls) {
		return Declaration{}, false
	}
	return copyDeclaration(x.decls[i]), true
}

// Declarations returns a copy of the declarations in canonical order.
// The caller may change the returned slice, including its name slices,
// without affecting the index.
func (x *Index) Declarations() []Declaration {
	n := 0
	if x != nil {
		n = len(x.decls)
	}
	out := make([]Declaration, n)
	for i := 0; i < n; i++ {
		out[i] = copyDeclaration(x.decls[i])
	}
	return out
}

func copyDeclaration(d Declaration) Declaration {
	names := make([]string, len(d.Names))
	copy(names, d.Names)
	d.Names = names
	docs := make([]ID, len(d.Docs))
	copy(docs, d.Docs)
	d.Docs = docs
	return d
}

// Build indexes the comments and declarations in docs.
//
// docs may be nil or empty, and then the index has no entries. A nil element
// is rejected. On any error the returned index is nil and holds none of the
// comments or declarations. The input slice and the documents are left
// unchanged.
func Build(docs []*syntax.Document) (*Index, error) {
	seen := make(map[string]int, len(docs))
	var items []collected
	var declItems []collectedDecl
	for i, doc := range docs {
		if doc == nil {
			return nil, &NilDocumentError{Index: i}
		}
		if err := validatePath(i, doc.Path); err != nil {
			return nil, err
		}
		if first, ok := seen[doc.Path]; ok {
			return nil, &DuplicatePathError{Path: doc.Path, FirstIndex: first, Index: i}
		}
		seen[doc.Path] = i
		placed, err := collect(doc)
		if err != nil {
			return nil, err
		}
		items = append(items, placed...)
		declared, err := collectDecls(doc)
		if err != nil {
			return nil, err
		}
		declItems = append(declItems, declared...)
	}
	slices.SortFunc(items, compareCollected)
	entries := make([]Entry, len(items))
	byID := make(map[uint64]int, len(items))
	commentAt := make(map[positionKey]ID, len(items))
	for i, item := range items {
		n := uint64(i) + 1
		id := ID{n: n}
		entries[i] = Entry{
			ID:       id,
			Path:     item.path,
			Language: item.language,
			Range:    item.rng,
			Text:     item.text,
		}
		byID[n] = i
		commentAt[positionKey{path: item.path, rng: item.rng}] = id
	}
	for i := range declItems {
		item := &declItems[i]
		item.docIDs = make([]ID, len(item.docs))
		for j, rng := range item.docs {
			id, ok := commentAt[positionKey{path: item.path, rng: rng}]
			if !ok {
				return nil, &UnresolvedDocError{
					Path: item.path, Index: item.index, Range: item.rng, DocRange: rng,
				}
			}
			item.docIDs[j] = id
		}
	}
	slices.SortFunc(declItems, compareDecl)
	decls := make([]Declaration, len(declItems))
	declByID := make(map[uint64]int, len(declItems))
	for i, item := range declItems {
		if i > 0 && compareDecl(declItems[i-1], item) == 0 {
			return nil, &DuplicateDeclarationError{
				Path:  item.path,
				Index: declItems[i-1].index,
				Other: item.index,
				Range: item.rng,
			}
		}
		n := uint64(i) + 1
		decls[i] = Declaration{
			ID:       DeclID{n: n},
			Path:     item.path,
			Language: item.language,
			Kind:     item.kind,
			Names:    item.names,
			Range:    item.rng,
			Docs:     item.docIDs,
		}
		declByID[n] = i
	}
	return &Index{entries: entries, byID: byID, decls: decls, declByID: declByID}, nil
}

// collected is one comment retained for ordering. It does not share a header
// with the caller's comment slice.
type collected struct {
	path     string
	language syntax.Language
	text     string
	rng      syntax.Range
}

func collect(doc *syntax.Document) ([]collected, error) {
	seen := make(map[syntax.Range]int, len(doc.Comments))
	items := make([]collected, 0, len(doc.Comments))
	for j, comment := range doc.Comments {
		if err := validateRange(comment.Range); err != nil {
			return nil, &CommentError{Path: doc.Path, Index: j, Range: comment.Range, Err: err}
		}
		if prev, ok := seen[comment.Range]; ok {
			return nil, &DuplicatePositionError{Path: doc.Path, Index: prev, Other: j, Range: comment.Range}
		}
		seen[comment.Range] = j
		items = append(items, collected{
			path:     doc.Path,
			language: doc.Language,
			text:     comment.Text,
			rng:      comment.Range,
		})
	}
	return items, nil
}

// collectedDecl is one declaration retained for ordering.
type collectedDecl struct {
	path     string
	language syntax.Language
	kind     syntax.Kind
	names    []string
	rng      syntax.Range
	docs     []syntax.Range
	docIDs   []ID
	index    int
}

type declKey struct {
	rng  syntax.Range
	kind syntax.Kind
}

func collectDecls(doc *syntax.Document) ([]collectedDecl, error) {
	seen := make(map[declKey]int, len(doc.Declarations))
	items := make([]collectedDecl, 0, len(doc.Declarations))
	for j, decl := range doc.Declarations {
		if err := validateDeclFact(decl); err != nil {
			return nil, &DeclarationError{Path: doc.Path, Index: j, Range: decl.Range, Err: err}
		}
		key := declKey{rng: decl.Range, kind: decl.Kind}
		if prev, ok := seen[key]; ok {
			return nil, &DuplicateDeclarationError{Path: doc.Path, Index: prev, Other: j, Range: decl.Range}
		}
		seen[key] = j
		names := make([]string, len(decl.Names))
		copy(names, decl.Names)
		docs := make([]syntax.Range, len(decl.Docs))
		copy(docs, decl.Docs)
		slices.SortFunc(docs, func(a, b syntax.Range) int {
			return compareCollected(collected{rng: a}, collected{rng: b})
		})
		item := collectedDecl{
			path:     doc.Path,
			language: doc.Language,
			kind:     decl.Kind,
			names:    names,
			rng:      decl.Range,
			index:    j,
			docs:     docs,
		}
		items = append(items, item)
	}
	return items, nil
}

func validateDeclFact(decl syntax.Declaration) error {
	if err := validateRange(decl.Range); err != nil {
		return err
	}
	if decl.Kind == "" {
		return ErrEmptyKind
	}
	if !utf8.ValidString(string(decl.Kind)) {
		return errors.New("kind is not valid UTF-8")
	}
	for i, name := range decl.Names {
		if name == "" {
			return fmt.Errorf("name [%d] is empty", i)
		}
		if !utf8.ValidString(name) {
			return fmt.Errorf("name [%d] is not valid UTF-8", i)
		}
	}
	seenDocs := make(map[syntax.Range]bool, len(decl.Docs))
	for i, rng := range decl.Docs {
		if err := validateRange(rng); err != nil {
			return fmt.Errorf("docs[%d]: %w", i, err)
		}
		if seenDocs[rng] {
			return fmt.Errorf("docs[%d]: %w", i, ErrDuplicateDoc)
		}
		seenDocs[rng] = true
	}
	return nil
}

// compareDecl orders by logical path, then by physical byte range, then by
// kind when the byte ranges are equal. Kind is the structural tie-breaker.
func compareDecl(a, b collectedDecl) int {
	if c := cmp.Compare(a.path, b.path); c != 0 {
		return c
	}
	if c := cmp.Compare(a.rng.Start.Offset, b.rng.Start.Offset); c != 0 {
		return c
	}
	if c := cmp.Compare(a.rng.End.Offset, b.rng.End.Offset); c != 0 {
		return c
	}
	return cmp.Compare(string(a.kind), string(b.kind))
}

// compareCollected orders by logical path, then by physical position.
// The position key is structural: offsets, then line and column. Comment
// text is not a key.
func compareCollected(a, b collected) int {
	if c := cmp.Compare(a.path, b.path); c != 0 {
		return c
	}
	if c := cmp.Compare(a.rng.Start.Offset, b.rng.Start.Offset); c != 0 {
		return c
	}
	if c := cmp.Compare(a.rng.End.Offset, b.rng.End.Offset); c != 0 {
		return c
	}
	if c := cmp.Compare(a.rng.Start.Line, b.rng.Start.Line); c != 0 {
		return c
	}
	if c := cmp.Compare(a.rng.Start.Column, b.rng.Start.Column); c != 0 {
		return c
	}
	if c := cmp.Compare(a.rng.End.Line, b.rng.End.Line); c != 0 {
		return c
	}
	return cmp.Compare(a.rng.End.Column, b.rng.End.Column)
}

func validatePath(docIndex int, logical string) error {
	if err := logicalPathError(logical); err != nil {
		return &PathError{Index: docIndex, Path: logical, Err: err}
	}
	return nil
}

// logicalPathError reports why logical is not a canonical project-relative
// path. A nil error means the path is acceptable as stored. The path is not
// cleaned or rewritten.
func logicalPathError(logical string) error {
	switch {
	case logical == "":
		return ErrEmptyPath
	case logical == ".":
		return ErrDotPath
	case path.IsAbs(logical):
		return ErrAbsolutePath
	case path.Clean(logical) != logical:
		return ErrUncleanPath
	case escapesRoot(logical):
		return ErrEscapingPath
	default:
		return nil
	}
}

// escapesRoot reports whether a cleaned relative path still contains a ".."
// element. path.Clean leaves that element only when it names a parent of the
// path's start.
func escapesRoot(logical string) bool {
	for _, elem := range strings.Split(logical, "/") {
		if elem == ".." {
			return true
		}
	}
	return false
}

func validateRange(r syntax.Range) error {
	if r.Start.Offset < 0 || r.End.Offset < 0 {
		return ErrNegativeOffset
	}
	if r.End.Offset < r.Start.Offset {
		return ErrEndBeforeStart
	}
	if r.Start.Line < 1 || r.End.Line < 1 || r.Start.Column < 1 || r.End.Column < 1 {
		return ErrPosition
	}
	return nil
}
