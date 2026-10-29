package tui

import (
	"fmt"
	"sort"
	"strings"

	"github.com/hashicorp/hcl"
	"github.com/hashicorp/hcl/hcl/ast"
)

// policyPath is one parsed `path` block of an ACL policy: the path
// pattern, its lead comment lines, its capabilities, and any other
// attributes (allowed/denied parameters, wrapping TTLs, ...) in
// document order.
type policyPath struct {
	path     string
	comments []string
	caps     []string
	attrs    []policyAttr
}

type policyAttr struct {
	name   string
	values []string // one rendered line per entry
}

// capOrder is the canonical display order of capabilities; unknown
// ones are appended alphabetically.
var capOrder = []string{"create", "read", "update", "delete", "list", "patch", "sudo", "deny"}

// capLetter maps each capability to its letter in the inline row
// summary (e.g. create/read/update/delete/list -> "crudl").
var capLetter = map[string]string{
	"create": "c", "read": "r", "update": "u", "delete": "d",
	"list": "l", "patch": "p", "sudo": "s", "deny": "x",
}

// capSummary renders a path block's capabilities as the dim inline
// summary shown next to the path in the list.
func capSummary(caps []string) string {
	if len(caps) == 0 {
		return "(no capabilities)"
	}
	var b strings.Builder
	for _, c := range sortCaps(caps) {
		if l, ok := capLetter[c]; ok {
			b.WriteString(l)
		}
	}
	return b.String()
}

// sortCaps orders capabilities canonically.
func sortCaps(caps []string) []string {
	out := make([]string, 0, len(caps))
	for _, want := range capOrder {
		for _, c := range caps {
			if c == want && !containsStr(out, c) {
				out = append(out, c)
			}
		}
	}
	var extra []string
	for _, c := range caps {
		if !containsStr(capOrder, c) && !containsStr(out, c) {
			extra = append(extra, c)
		}
	}
	sort.Strings(extra)
	return append(out, extra...)
}

func containsStr(s []string, v string) bool {
	for _, x := range s {
		if x == v {
			return true
		}
	}
	return false
}

// parsePolicyDoc parses raw ACL policy rules into path blocks with
// their lead comments attached. It errors when the document is not
// plain path-rule HCL, so callers can fall back to the raw view
// rather than silently dropping content.
func parsePolicyDoc(raw string) ([]policyPath, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, nil
	}
	f, err := hcl.Parse(raw)
	if err != nil {
		return nil, err
	}
	list, ok := f.Node.(*ast.ObjectList)
	if !ok {
		return nil, fmt.Errorf("unexpected policy structure")
	}
	var out []policyPath
	for _, item := range list.Items {
		if len(item.Keys) < 2 || strings.ToLower(item.Keys[0].Token.Text) != "path" {
			// Top-level statements other than path blocks are not
			// representable in the structured view; show the raw
			// document instead of hiding them.
			return nil, fmt.Errorf("unsupported top-level statement")
		}
		pp := policyPath{
			path:     unquote(item.Keys[1].Token.Text),
			comments: commentLines(item.LeadComment),
		}
		obj, ok := item.Val.(*ast.ObjectType)
		if !ok {
			return nil, fmt.Errorf("path %q: not a block", pp.path)
		}
		for _, attr := range obj.List.Items {
			if len(attr.Keys) == 0 {
				continue
			}
			name := strings.ToLower(unquote(attr.Keys[0].Token.Text))
			switch {
			case name == "capabilities":
				caps, err := stringList(attr.Val)
				if err != nil {
					return nil, fmt.Errorf("path %q: %v", pp.path, err)
				}
				pp.caps = caps
			default:
				vals, err := renderAttrValue(attr.Val)
				if err != nil {
					return nil, fmt.Errorf("path %q: %v", pp.path, err)
				}
				pp.attrs = append(pp.attrs, policyAttr{name: name, values: vals})
			}
		}
		out = append(out, pp)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].path < out[j].path })
	return out, nil
}

// commentLines extracts the text of a comment group with its markers
// stripped.
func commentLines(g *ast.CommentGroup) []string {
	if g == nil {
		return nil
	}
	var out []string
	for _, c := range g.List {
		s := strings.TrimSpace(c.Text)
		s = strings.TrimPrefix(s, "//")
		s = strings.TrimPrefix(s, "#")
		s = strings.TrimSuffix(s, "*/")
		s = strings.TrimSpace(s)
		if s != "" {
			out = append(out, s)
		}
	}
	return out
}

// stringList renders an HCL value as a list of strings: a literal or a
// list literal.
func stringList(n ast.Node) ([]string, error) {
	switch v := n.(type) {
	case *ast.LiteralType:
		return []string{unquote(v.Token.Text)}, nil
	case *ast.ListType:
		var out []string
		for _, e := range v.List {
			lt, ok := e.(*ast.LiteralType)
			if !ok {
				return nil, fmt.Errorf("unsupported list element")
			}
			out = append(out, unquote(lt.Token.Text))
		}
		return out, nil
	}
	return nil, fmt.Errorf("unsupported value")
}

// renderAttrValue renders a non-capabilities attribute for display:
// literals and lists as one line, objects as one "key: [values]"
// line per entry (e.g. allowed_parameters).
func renderAttrValue(n ast.Node) ([]string, error) {
	switch v := n.(type) {
	case *ast.LiteralType:
		return []string{unquote(v.Token.Text)}, nil
	case *ast.ListType:
		elems, err := stringList(v)
		if err != nil {
			return nil, err
		}
		return []string{"[" + strings.Join(elems, ", ") + "]"}, nil
	case *ast.ObjectType:
		var out []string
		for _, it := range v.List.Items {
			if len(it.Keys) == 0 {
				continue
			}
			elems, err := stringList(it.Val)
			if err != nil {
				return nil, err
			}
			out = append(out, fmt.Sprintf("%s: [%s]", unquote(it.Keys[0].Token.Text), strings.Join(elems, ", ")))
		}
		return out, nil
	}
	return nil, fmt.Errorf("unsupported value")
}

// unquote strips the surrounding quotes of an HCL literal, if any.
func unquote(s string) string {
	if len(s) >= 2 && s[0] == '"' && s[len(s)-1] == '"' {
		return s[1 : len(s)-1]
	}
	return s
}
