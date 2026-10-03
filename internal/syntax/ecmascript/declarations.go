package ecmascript

import (
	"github.com/edalca/nodex/internal/syntax/types"
	"github.com/odvcencio/gotreesitter"
)

func (a *adapter) isList(n *gotreesitter.Node) bool {
	switch a.kind(n) {
	case "program", "statement_block", "class_body", "interface_body", "object", "object_type", "enum_body":
		return true
	}
	return false
}

// slots treats wrappers as part of their declaration, never as independent
// owners. A separate decorator sequence starts its following member's slot.
func (a *adapter) slots(list *gotreesitter.Node, out *[]types.Declaration) {
	previous := int(list.StartByte())
	if a.kind(list) == "program" {
		previous = 0
	}
	var decorators *span
	for i := 0; i < list.ChildCount(); i++ {
		child := list.Child(i)
		if child == nil {
			continue
		}
		k := a.kind(child)
		if k == "comment" {
			continue
		}
		if k == "hash_bang_line" {
			previous = int(child.EndByte())
			continue
		}
		b, ok := a.bounds[child]
		if !ok {
			continue
		}
		if k == "decorator" && a.kind(list) == "class_body" {
			if decorators == nil {
				s := b
				decorators = &s
			} else {
				decorators.end = b.end
			}
			continue
		}
		node := a.owner(child)
		kind, names := a.declaration(node, list)
		if kind != "" {
			if decorators != nil {
				b.start = decorators.start
			}
			switch a.kind(node) {
			case "field_definition", "public_field_definition", "property_signature", "method_signature", "abstract_method_signature":
				// These grammars put owned delimiters outside the member node.
				// Only comment siblings and whitespace may precede that delimiter;
				// any other sibling ends the member's slot.
				cursor := b.end
				for j := i + 1; j < list.ChildCount(); j++ {
					next := list.Child(j)
					if next == nil || int(next.StartByte()) < cursor || !onlyWhitespace(a.source[cursor:next.StartByte()]) {
						break
					}
					if a.kind(next) == "comment" {
						cursor = int(next.EndByte())
						continue
					}
					if !next.IsNamed() && (a.kind(next) == ";" || a.kind(next) == ",") {
						b.end = int(next.EndByte())
					}
					break
				}
			}
			r := a.physical(b)
			*out = append(*out, types.Declaration{Kind: kind, Names: names, Start: r.Start, End: r.End, Docs: a.leadingDocs(previous, b.start)})
		}
		decorators = nil
		previous = b.end
	}
}

// owner unwraps only carriers with one direct declaration child. Expression
// statements are carriers for namespaces, not arbitrary function expressions.
func (a *adapter) owner(n *gotreesitter.Node) *gotreesitter.Node {
	for n != nil {
		switch a.kind(n) {
		case "export_statement", "ambient_declaration", "expression_statement", "statement", "declaration":
			var candidate *gotreesitter.Node
			for i := 0; i < n.ChildCount(); i++ {
				c := n.Child(i)
				if c == nil || !c.IsNamed() || a.kind(c) == "comment" || a.kind(c) == "decorator" {
					continue
				}
				if candidate != nil {
					return n
				}
				candidate = c
			}
			if candidate == nil {
				return n
			}
			if a.kind(n) == "expression_statement" && a.kind(candidate) != "internal_module" && a.kind(candidate) != "module" {
				return n
			}
			n = candidate
		default:
			return n
		}
	}
	return nil
}

func (a *adapter) declaration(n, list *gotreesitter.Node) (string, []string) {
	if n == nil {
		return "", nil
	}
	k, parent := a.kind(n), a.kind(list)
	statement := parent == "program" || parent == "statement_block"
	member := parent == "class_body" || parent == "object" || parent == "interface_body" || parent == "object_type"
	kind := ""
	switch {
	case statement:
		switch k {
		case "function_declaration", "generator_function_declaration", "function_signature":
			kind = "function"
		case "class_declaration", "abstract_class_declaration":
			kind = "class"
		case "class", "function_expression", "generator_function":
			// Anonymous defaults have no declaration-shaped inner node.
			p := n.Parent()
			if p != nil && a.kind(p) == "export_statement" {
				if k == "class" {
					kind = "class"
				} else {
					kind = "function"
				}
			}
		case "lexical_declaration":
			kind = a.raw(a.field(n, "kind"))
		case "variable_declaration":
			kind = "var"
		case "interface_declaration":
			kind = "interface"
		case "type_alias_declaration":
			kind = "type-alias"
		case "enum_declaration":
			kind = "enum"
		case "internal_module", "module":
			kind = "namespace"
		}
	case member:
		switch k {
		case "method_definition", "method_signature", "abstract_method_signature":
			kind = "method"
			static := false
			for i := 0; i < n.ChildCount(); i++ {
				c := n.Child(i)
				if c == nil || c.IsNamed() {
					continue
				}
				switch a.kind(c) {
				case "get", "set":
					kind = "accessor"
				case "static":
					static = true
				}
			}
			if kind == "method" && !static && parent == "class_body" && a.raw(a.field(n, "name")) == "constructor" {
				return "constructor", []string{}
			}
		case "field_definition", "public_field_definition", "property_signature", "pair":
			kind = "property"
		}
	case parent == "enum_body":
		if k == "enum_assignment" || k == "property_identifier" || k == "string" || k == "number" {
			kind = "enum-member"
		}
	}
	if kind == "" {
		return "", nil
	}
	names := []string{}
	if kind == "const" || kind == "let" || kind == "var" {
		for i := 0; i < n.ChildCount(); i++ {
			c := n.Child(i)
			if c != nil && a.kind(c) == "variable_declarator" {
				a.bindings(a.field(c, "name"), &names)
			}
		}
		return kind, names
	}
	name := a.field(n, "name")
	if name == nil {
		name = a.field(n, "property")
	}
	if name == nil {
		name = a.field(n, "key")
	}
	if kind == "enum-member" && k != "enum_assignment" {
		name = n
	}
	if name != nil && a.kind(name) != "computed_property_name" {
		names = append(names, a.raw(name))
	}
	return kind, names
}

// bindings follows binding children only. Renamed keys, computed keys, default
// initializers and type annotations are never mistaken for declared names.
func (a *adapter) bindings(n *gotreesitter.Node, names *[]string) {
	if n == nil {
		return
	}
	switch a.kind(n) {
	case "identifier", "shorthand_property_identifier_pattern":
		*names = append(*names, a.raw(n))
	case "pair_pattern":
		a.bindings(a.field(n, "value"), names)
	case "assignment_pattern", "object_assignment_pattern":
		a.bindings(a.field(n, "left"), names)
	case "rest_pattern", "object_pattern", "array_pattern":
		for i := 0; i < n.ChildCount(); i++ {
			c := n.Child(i)
			if c != nil && c.IsNamed() {
				a.bindings(c, names)
			}
		}
	}
}
