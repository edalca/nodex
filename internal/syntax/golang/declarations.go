package golang

import (
	"errors"
	"go/ast"
	"go/token"
	"sort"
)

var (
	// ErrMalformedDeclaration means the supplied range is not a coherent
	// physical range of the source bytes.
	ErrMalformedDeclaration = errors.New("declaration range is malformed")

	// ErrDeclarationNotFound means the range is not exactly one declaration.
	ErrDeclarationNotFound = errors.New("declaration range does not match a declaration")

	// ErrAmbiguousDeclaration means the range matches more than one declaration.
	ErrAmbiguousDeclaration = errors.New("declaration range matches more than one declaration")
)

const (
	// KindPackage is the package clause.
	KindPackage = "package"
	// KindFunction is a function declaration.
	KindFunction = "function"
	// KindMethod is a function declaration with a receiver.
	KindMethod = "method"
	// KindConstGroup is a parenthesized const declaration.
	KindConstGroup = "const-group"
	// KindVarGroup is a parenthesized var declaration.
	KindVarGroup = "var-group"
	// KindTypeGroup is a parenthesized type declaration.
	KindTypeGroup = "type-group"
	// KindConst is one const spec, or one const declaration written without parentheses.
	KindConst = "const"
	// KindVar is one var spec, or one var declaration written without parentheses.
	KindVar = "var"
	// KindType is one type spec, or one type declaration written without parentheses.
	KindType = "type"
	// KindField is a struct field or an interface field.
	KindField = "field"
)

// Declaration is one structural declaration fact.
//
// Kind is one of the Kind constants. Names are the identifiers this node
// declares, in source order. Names is empty when the node declares none,
// which is how an embedded field is represented. Start and End are the
// half-open physical extent of the declaration itself. A documentation
// comment that sits outside that extent is not part of the extent.
//
// HasDoc reports whether the node’s own documentation comment is present.
// DocStart and DocEnd are that comment’s physical range when HasDoc is
// true. They are not a nearby comment, a parent declaration’s comment, or
// a child declaration’s comment. A nil documentation field leaves HasDoc
// false.
type Declaration struct {
	Kind     string
	Names    []string
	Start    Position
	End      Position
	HasDoc   bool
	DocStart Position
	DocEnd   Position
}

// ParseFile interprets src as Go source.
//
// A nil src is empty input. On success the comment slice and the
// declaration slice are non-nil. Comments are ordered by physical start
// offset. Declarations follow the syntax tree in preorder. Both come from
// the same parse. On failure the error is *Error and both slices are nil.
func ParseFile(src []byte) ([]Comment, []Declaration, error) {
	src, tf, file, err := parseSource(src)
	if err != nil {
		return nil, nil, err
	}
	comments, err := commentsFrom(src, tf, file)
	if err != nil {
		return nil, nil, err
	}
	decls, err := declarationsFrom(src, tf, file)
	if err != nil {
		return nil, nil, err
	}
	return comments, decls, nil
}

func commentsFrom(src []byte, tf *token.File, file *ast.File) ([]Comment, error) {
	comments := make([]Comment, 0, len(file.Comments))
	for _, group := range file.Comments {
		comment, err := commentFromGroup(src, tf, group)
		if err != nil {
			return nil, err
		}
		comments = append(comments, comment)
	}
	sort.SliceStable(comments, func(i, j int) bool {
		return comments[i].Start.Offset < comments[j].Start.Offset
	})
	return comments, nil
}

func declarationsFrom(src []byte, tf *token.File, file *ast.File) ([]Declaration, error) {
	if file == nil {
		return nil, &Error{Diagnostics: []Diagnostic{{Msg: "parsed file is missing"}}}
	}
	out := make([]Declaration, 0)
	if file.Package.IsValid() && file.Name != nil {
		decl, err := packageDecl(src, tf, file)
		if err != nil {
			return nil, err
		}
		out = append(out, decl)
	}
	var stack []ast.Node
	var walkErr error
	ast.Inspect(file, func(n ast.Node) bool {
		if n == nil {
			if len(stack) > 0 {
				stack = stack[:len(stack)-1]
			}
			return true
		}
		if walkErr != nil {
			return false
		}
		switch node := n.(type) {
		case *ast.FuncDecl:
			decl, err := functionDecl(src, tf, node)
			if err != nil {
				walkErr = err
				return false
			}
			out = append(out, decl)
		case *ast.GenDecl:
			decls, err := generalDecls(src, tf, node)
			if err != nil {
				walkErr = err
				return false
			}
			out = append(out, decls...)
		case *ast.Field:
			if !structOrInterfaceField(stack) {
				break
			}
			decl, err := fieldDecl(src, tf, node)
			if err != nil {
				walkErr = err
				return false
			}
			out = append(out, decl)
		}
		stack = append(stack, n)
		return true
	})
	if walkErr != nil {
		return nil, walkErr
	}
	return out, nil
}

func packageDecl(src []byte, tf *token.File, file *ast.File) (Declaration, error) {
	if file.Name == nil || file.Name.Name == "" {
		return Declaration{}, &Error{Diagnostics: []Diagnostic{{Msg: "package declaration has no name"}}}
	}
	return oneDecl(src, tf, KindPackage, []string{file.Name.Name}, file.Package, file.Name.End(), file.Doc)
}

func functionDecl(src []byte, tf *token.File, node *ast.FuncDecl) (Declaration, error) {
	if node == nil || node.Name == nil || node.Name.Name == "" || node.Type == nil {
		return Declaration{}, &Error{Diagnostics: []Diagnostic{{Msg: "function declaration is incomplete"}}}
	}
	kind := KindFunction
	if node.Recv != nil {
		kind = KindMethod
	}
	return oneDecl(src, tf, kind, []string{node.Name.Name}, node.Pos(), node.End(), node.Doc)
}

func generalDecls(src []byte, tf *token.File, node *ast.GenDecl) ([]Declaration, error) {
	if node == nil {
		return nil, &Error{Diagnostics: []Diagnostic{{Msg: "declaration is missing"}}}
	}
	specKind, groupedKind, ok := declarationKinds(node.Tok)
	if !ok {
		return nil, nil
	}
	if len(node.Specs) == 0 && !node.Lparen.IsValid() {
		return nil, &Error{Diagnostics: []Diagnostic{{Msg: "declaration has no specs"}}}
	}
	names, err := namesOfSpecs(node.Specs)
	if err != nil {
		return nil, err
	}
	if !node.Lparen.IsValid() {
		// The parser stores the documentation of a declaration that has no
		// parentheses on the general declaration and passes a nil
		// documentation comment to its single spec. That general
		// declaration is the one fact. The spec is not a second fact.
		decl, err := oneDecl(src, tf, specKind, names, node.Pos(), node.End(), node.Doc)
		if err != nil {
			return nil, err
		}
		return []Declaration{decl}, nil
	}
	group, err := oneDecl(src, tf, groupedKind, names, node.Pos(), node.End(), node.Doc)
	if err != nil {
		return nil, err
	}
	out := []Declaration{group}
	for _, spec := range node.Specs {
		decl, err := specDecl(src, tf, specKind, spec)
		if err != nil {
			return nil, err
		}
		out = append(out, decl)
	}
	return out, nil
}

func declarationKinds(tok token.Token) (specKind, groupedKind string, ok bool) {
	switch tok {
	case token.CONST:
		return KindConst, KindConstGroup, true
	case token.VAR:
		return KindVar, KindVarGroup, true
	case token.TYPE:
		return KindType, KindTypeGroup, true
	default:
		return "", "", false
	}
}

func specDecl(src []byte, tf *token.File, kind string, spec ast.Spec) (Declaration, error) {
	names, err := namesOfSpec(spec)
	if err != nil {
		return Declaration{}, err
	}
	switch s := spec.(type) {
	case *ast.ValueSpec:
		return oneDecl(src, tf, kind, names, s.Pos(), s.End(), s.Doc)
	case *ast.TypeSpec:
		return oneDecl(src, tf, kind, names, s.Pos(), s.End(), s.Doc)
	default:
		return Declaration{}, &Error{Diagnostics: []Diagnostic{{Msg: "declaration spec is not a value or type spec"}}}
	}
}

func fieldDecl(src []byte, tf *token.File, node *ast.Field) (Declaration, error) {
	if node == nil {
		return Declaration{}, &Error{Diagnostics: []Diagnostic{{Msg: "field is missing"}}}
	}
	names, err := identNames(node.Names)
	if err != nil {
		return Declaration{}, err
	}
	return oneDecl(src, tf, KindField, names, node.Pos(), node.End(), node.Doc)
}

func structOrInterfaceField(stack []ast.Node) bool {
	if len(stack) < 2 {
		return false
	}
	if _, ok := stack[len(stack)-1].(*ast.FieldList); !ok {
		return false
	}
	switch stack[len(stack)-2].(type) {
	case *ast.StructType, *ast.InterfaceType:
		return true
	default:
		return false
	}
}

func namesOfSpecs(specs []ast.Spec) ([]string, error) {
	var names []string
	for _, spec := range specs {
		got, err := namesOfSpec(spec)
		if err != nil {
			return nil, err
		}
		names = append(names, got...)
	}
	return names, nil
}

func namesOfSpec(spec ast.Spec) ([]string, error) {
	switch s := spec.(type) {
	case *ast.ValueSpec:
		if len(s.Names) == 0 {
			return nil, &Error{Diagnostics: []Diagnostic{{Msg: "value declaration has no names"}}}
		}
		return identNames(s.Names)
	case *ast.TypeSpec:
		if s.Name == nil || s.Name.Name == "" {
			return nil, &Error{Diagnostics: []Diagnostic{{Msg: "type declaration has no name"}}}
		}
		return []string{s.Name.Name}, nil
	default:
		return nil, &Error{Diagnostics: []Diagnostic{{Msg: "declaration spec is not a value or type spec"}}}
	}
}

func identNames(list []*ast.Ident) ([]string, error) {
	if len(list) == 0 {
		return nil, nil
	}
	out := make([]string, len(list))
	for i, id := range list {
		if id == nil || id.Name == "" {
			return nil, &Error{Diagnostics: []Diagnostic{{Msg: "declaration identifier is empty"}}}
		}
		out[i] = id.Name
	}
	return out, nil
}

func oneDecl(src []byte, tf *token.File, kind string, names []string, pos, end token.Pos, doc *ast.CommentGroup) (Declaration, error) {
	start, finish, err := nodeRange(tf, pos, end)
	if err != nil {
		return Declaration{}, err
	}
	decl := Declaration{Kind: kind, Names: names, Start: start, End: finish}
	if err := attachDoc(src, tf, &decl, doc); err != nil {
		return Declaration{}, err
	}
	return decl, nil
}

func nodeRange(tf *token.File, pos, end token.Pos) (Position, Position, error) {
	if tf == nil || !pos.IsValid() || !end.IsValid() {
		return Position{}, Position{}, &Error{Diagnostics: []Diagnostic{{Msg: "declaration position is invalid"}}}
	}
	startOff := tf.Offset(pos)
	endOff := tf.Offset(end)
	if startOff < 0 || endOff < startOff || endOff > tf.Size() {
		return Position{}, Position{}, &Error{Diagnostics: []Diagnostic{{Msg: "declaration range is outside the source"}}}
	}
	return positionAt(tf, startOff), positionAt(tf, endOff), nil
}

func attachDoc(src []byte, tf *token.File, decl *Declaration, group *ast.CommentGroup) error {
	if group == nil {
		return nil
	}
	comment, err := commentFromGroup(src, tf, group)
	if err != nil {
		return err
	}
	decl.HasDoc = true
	decl.DocStart = comment.Start
	decl.DocEnd = comment.End
	return nil
}
