// Package ecmascript interprets JavaScript, JSX, TypeScript and TSX source.
// Each capability fixes its grammar before parsing caller-supplied bytes. CST
// nodes remain private and are released after adaptation to neutral facts.
//
// Comments are individual physical parser-recognized units, including hashbangs.
// Declarations come from explicit statement and member slots. Direct JSDoc is
// the ordered collection of eligible blocks in that slot's structural leading
// trivia, before its export, ambient, decorator and modifier tokens. No comment
// meaning, tags, types, symbols, modules or language validity are analyzed.
//
// Detected unsafe recovery preserves observed comments but suppresses all
// declarations in that file. This is a conservative structural safety screen,
// not a syntax validator or a guarantee of comment completeness on broken input.
package ecmascript

import (
	"fmt"
	"math"
	"sort"
	"strings"
	"sync"

	"github.com/edalca/nodex/internal/syntax/contracts"
	"github.com/edalca/nodex/internal/syntax/types"
	"github.com/odvcencio/gotreesitter"
	blobs "github.com/odvcencio/gotreesitter/grammars/grammar_blobs"
	runtime "github.com/odvcencio/gotreesitter/grammars/runtime"
)

// Dialect fixes recognition, identity and grammar for a language capability.
type Dialect uint8

const (
	// JavaScript recognizes .js using the JavaScript grammar (which includes JSX).
	JavaScript Dialect = iota
	// JSX recognizes .jsx using the same JavaScript grammar.
	JSX
	// TypeScript recognizes .ts using the TypeScript grammar.
	TypeScript
	// TSX recognizes .tsx using the distinct TSX grammar.
	TSX
)

type language struct{ dialect Dialect }

var _ contracts.Language = language{}

// New returns an immutable capability. An unknown dialect is a composition error.
func New(dialect Dialect) contracts.Language {
	if dialect > TSX {
		panic("ecmascript: unknown dialect")
	}
	return language{dialect: dialect}
}

// ID returns the configured stable language identity.
func (l language) ID() string {
	return [...]string{"javascript", "jsx", "typescript", "tsx"}[l.dialect]
}

// Recognize uses only the configured case-sensitive path suffix.
func (l language) Recognize(path string) bool {
	return strings.HasSuffix(path, [...]string{".js", ".jsx", ".ts", ".tsx"}[l.dialect])
}

// Presets supplies an explicitly empty catalog; no exclusion is implicit.
func (language) Presets() contracts.PresetCatalog {
	return contracts.PresetCatalog{Concrete: []string{}, Aggregates: []contracts.Aggregate{}}
}

// PathExcluded never excludes files; this family owns no path presets.
func (language) PathExcluded(string, []string) bool { return false }

// SourceExcluded never excludes source; this family owns no structural presets.
func (language) SourceExcluded(string, []byte, []string) bool { return false }

var grammarOnce sync.Once
var grammars [3]*gotreesitter.Language
var grammarError error

func (l language) grammar() (*gotreesitter.Language, error) {
	grammarOnce.Do(func() {
		runtime.RegisterJavascriptSupport()
		runtime.RegisterTypescriptSupport()
		runtime.RegisterTsxSupport()
		// Load embedded bytes directly: the convenience Language loader permits a
		// filesystem blob override. Scanner support and profiles still attach here.
		for i, input := range []struct {
			name string
			data []byte
		}{
			{"javascript", blobs.Javascript()}, {"typescript", blobs.Typescript()}, {"tsx", blobs.Tsx()},
		} {
			var err error
			grammars[i], err = runtime.LoadLanguage(input.name, input.data)
			if err != nil {
				grammarError = fmt.Errorf("load %s grammar: %w", input.name, err)
				return
			}
		}
	})
	i := 0
	if l.dialect == TypeScript {
		i = 1
	} else if l.dialect == TSX {
		i = 2
	}
	return grammars[i], grammarError
}

func failure(message string) error {
	return &contracts.ParseError{Diagnostics: []types.Diagnostic{{Msg: message}}}
}

// Parse returns physical comments and safe declaration facts from one CST.
// Detected unsafe recovery suppresses declarations without diagnosing validity.
func (l language) Parse(source []byte) (*contracts.Document, error) {
	if uint64(len(source)) > math.MaxUint32 {
		return nil, failure("source exceeds parser byte range")
	}
	grammar, err := l.grammar()
	if err != nil {
		return nil, failure(err.Error())
	}
	tree, err := gotreesitter.NewParser(grammar).Parse(source)
	if tree == nil {
		return nil, failure(fmt.Sprintf("source structure unavailable: %v", err))
	}
	defer tree.Release()
	root := tree.RootNode()
	if root == nil {
		return nil, failure("source structure unavailable: missing root")
	}
	rt := tree.ParseRuntime()
	a := adapter{source: source, grammar: grammar, lines: sourceLines(source), bounds: make(map[*gotreesitter.Node]span)}
	a.observe(root)
	doc := &contracts.Document{Comments: a.comments, Declarations: []types.Declaration{}}
	unsafe := err != nil || tree.ParseStopReason() != gotreesitter.ParseStopAccepted || tree.ParseStoppedEarly() || root.HasErrorOrMissing() || rt.Truncated || rt.TokenSourceEOFEarly || rt.CRecoveryDroppedErrorForClean || a.unsafe
	if !unsafe {
		for _, n := range a.nodes {
			if a.isList(n) {
				a.slots(n, &doc.Declarations)
			}
		}
		sort.Slice(doc.Declarations, func(i, j int) bool {
			x, y := doc.Declarations[i], doc.Declarations[j]
			if x.Start.Offset != y.Start.Offset {
				return x.Start.Offset < y.Start.Offset
			}
			if x.End.Offset != y.End.Offset {
				return x.End.Offset < y.End.Offset
			}
			return x.Kind < y.Kind
		})
	}
	return doc, nil
}

type span struct{ start, end int }
type adapter struct {
	source   []byte
	grammar  *gotreesitter.Language
	lines    []int
	nodes    []*gotreesitter.Node
	bounds   map[*gotreesitter.Node]span
	comments []types.Comment
	unsafe   bool
}

func (a *adapter) kind(n *gotreesitter.Node) string { return n.Type(a.grammar) }
func (a *adapter) field(n *gotreesitter.Node, name string) *gotreesitter.Node {
	return n.ChildByFieldName(name, a.grammar)
}
func (a *adapter) raw(n *gotreesitter.Node) string {
	if n == nil {
		return ""
	}
	return string(a.source[n.StartByte():n.EndByte()])
}
