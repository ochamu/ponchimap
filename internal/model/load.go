package model

import (
	"bytes"
	"encoding/json"
	"maps"
	"slices"
	"strconv"
	"strings"
	"sync"

	"github.com/ochamu/ponchimap/internal/diag"
	"github.com/ochamu/ponchimap/internal/input"
	"github.com/ochamu/ponchimap/schema"
)

var modelSchema = sync.OnceValue(func() *input.Schema { return input.MustCompile(schema.Model) })

func schemaIDPattern() string {
	var s struct {
		Defs struct {
			ID struct {
				Pattern string `json:"pattern"`
			} `json:"id"`
		} `json:"$defs"`
	}
	if err := json.Unmarshal(schema.Model, &s); err != nil || s.Defs.ID.Pattern == "" {
		panic("model: the schema gives no $defs.id.pattern")
	}
	return s.Defs.ID.Pattern
}

// Keyed is a part of the model whose keys the schema leaves open: it gives
// some of them a meaning, and keeps the others without saying what their
// values are.
type Keyed int

const (
	InSupports Keyed = iota
	InNodeAttributes
	InEdgeAttributes
	InScope
	InFilter
)

// schemaObject is the part of a JSON Schema that names the keys it defines.
type schemaObject struct {
	Properties map[string]schemaObject `json:"properties"`
	AllOf      []struct {
		If schemaObject `json:"if"`
	} `json:"allOf"`
	Const any `json:"const"`
}

var definedKeys = sync.OnceValue(func() map[Keyed][]string {
	var s struct {
		Defs map[string]schemaObject `json:"$defs"`
	}
	if err := json.Unmarshal(schema.Model, &s); err != nil {
		panic("model: the schema cannot be read: " + err.Error())
	}
	names := func(o schemaObject) []string { return slices.Sorted(maps.Keys(o.Properties)) }
	scope := s.Defs["scope"]
	keys := map[Keyed][]string{
		InSupports:       nil,
		InNodeAttributes: names(s.Defs["node"].Properties["attributes"]),
		InEdgeAttributes: names(s.Defs["edge"].Properties["attributes"]),
		InScope:          names(scope),
		InFilter:         names(scope.Properties["filter"]),
	}
	for _, rule := range s.Defs["support"].AllOf {
		if key, ok := rule.If.Properties["key"].Const.(string); ok {
			keys[InSupports] = append(keys[InSupports], key)
		}
	}
	for _, defined := range keys {
		if len(defined) == 0 {
			panic("model: the schema defines no key in a part where it is known to define some")
		}
	}
	return keys
})

// DefinedKeys are the keys that the schema gives a meaning and a value in
// that part of the model. Those of supports are in the order of the schema,
// the others sorted. Another key is kept, and nothing knows what its value
// is.
func DefinedKeys(in Keyed) []string { return definedKeys()[in] }

// Parse reads a model from JSON text. name is how diagnostics name the text,
// usually a file path. The model is nil exactly when the list has an error.
func Parse(name string, raw []byte) (*Model, diag.List) {
	doc, list := input.DecodeJSON(raw)
	if doc != nil {
		checked := modelSchema().Check(doc)
		list = append(list, checked...)
		if !slices.ContainsFunc(checked, func(d diag.Diagnostic) bool { return d.Code == input.UnsupportedVersion }) {
			list = append(list, checkReferences(doc)...)
		}
	}
	for i := range list {
		list[i].Location.Path = name
		if doc != nil {
			list[i].TargetID = targetIDAt(doc.Value, list[i].Location.Pointer)
		}
	}
	list.Sort()
	if list.HasErrors() {
		return nil, list
	}
	dec := json.NewDecoder(bytes.NewReader(bytes.TrimPrefix(raw, []byte("\xEF\xBB\xBF"))))
	dec.UseNumber()
	dec.DisallowUnknownFields()
	var m Model
	if err := dec.Decode(&m); err != nil {
		return nil, diag.List{{Severity: diag.Error, Code: "tool.defect", Location: diag.Location{Path: name},
			Message: "the model passed every check but the tool could not read it; this is a defect of the tool, please report it"}}
	}
	return &m, list
}

// elements are where the model declares an ID, as JSON Pointer tokens in
// which * stands for any index. All the IDs share one namespace, and a
// reference can name only a node or an edge.
var elements = []struct {
	at         []string
	node, edge bool
}{
	{at: []string{"nodes", "*"}, node: true},
	{at: []string{"nodes", "*", "evidence", "*"}},
	{at: []string{"edges", "*"}, edge: true},
	{at: []string{"edges", "*", "evidence", "*"}},
	{at: []string{"coverage", "*"}},
}

// reference is a place where the model names a node, or a node or an edge.
type reference struct {
	at []string
	// edges says that an edge may be named as well as a node.
	edges bool
	// key, when set, limits the place to an object whose key member is key.
	key string
}

var references = []reference{
	{at: []string{"edges", "*", "from"}},
	{at: []string{"edges", "*", "to"}},
	{at: []string{"nodes", "*", "evidence", "*", "supports", "*", "value"}, key: KeyConnectionTarget},
	{at: []string{"edges", "*", "evidence", "*", "supports", "*", "value"}, key: KeyConnectionTarget},
	{at: []string{"coverage", "*", "scope", "target_ids", "*"}, edges: true},
	{at: []string{"target", "scope", "*", "target_ids", "*"}, edges: true},
}

// checkReferences reports IDs declared twice and references that name no
// element. It reads the decoded value defensively and skips what has the
// wrong shape or lacks the ID form, which the schema has reported, so that it
// runs on a document that failed the schema too.
func checkReferences(doc *input.Document) diag.List {
	type declaration struct {
		pointer, id string
		at          diag.Location
	}
	var declared []declaration
	nodes, edges := map[string]bool{}, map[string]bool{}
	for _, e := range elements {
		each(doc.Value, "", e.at, nil, func(pointer string, v any, _ map[string]any) {
			id := idOf(v)
			if id == "" {
				return
			}
			declared = append(declared, declaration{pointer + "/id", id, doc.Locate(pointer+"/id", false)})
			nodes[id] = nodes[id] || e.node
			edges[id] = edges[id] || e.edge
		})
	}
	// The first declaration of an ID is the one first in the text.
	slices.SortStableFunc(declared, func(a, b declaration) int {
		if a.at.Line != b.at.Line {
			return a.at.Line - b.at.Line
		}
		return a.at.Column - b.at.Column
	})
	var list diag.List
	first := map[string]string{}
	for _, d := range declared {
		if at, dup := first[d.id]; dup {
			list = append(list, diag.Diagnostic{Severity: diag.Error, Code: "model.duplicate_id", Location: d.at,
				Message: "the ID is already used at " + at, Hint: "Give each node, edge, evidence, and coverage record an ID of its own."})
			continue
		}
		first[d.id] = d.pointer
	}
	for _, r := range references {
		each(doc.Value, "", r.at, nil, func(pointer string, v any, parent map[string]any) {
			id, ok := v.(string)
			if !ok || !ValidID(id) || (r.key != "" && parent["key"] != r.key) || nodes[id] || (r.edges && edges[id]) {
				return
			}
			message, hint := "no node of the model has this ID", "Name a node of the model."
			if r.edges {
				message, hint = "no node or edge of the model has this ID", "Name a node or an edge of the model."
			}
			list = append(list, diag.Diagnostic{Severity: diag.Error, Code: "model.unknown_reference",
				Location: doc.Locate(pointer, false), Message: message, Hint: hint})
		})
	}
	return list
}

// each calls fn for every value whose pointer matches pattern, with the
// object that holds it. Values of another shape are skipped.
func each(v any, pointer string, pattern []string, parent map[string]any, fn func(pointer string, v any, parent map[string]any)) {
	if len(pattern) == 0 {
		fn(pointer, v, parent)
		return
	}
	switch x := v.(type) {
	case []any:
		if pattern[0] == "*" {
			for i, item := range x {
				each(item, pointer+"/"+strconv.Itoa(i), pattern[1:], nil, fn)
			}
		}
	case map[string]any:
		if inner, ok := x[pattern[0]]; ok && pattern[0] != "*" {
			each(inner, pointer+"/"+pattern[0], pattern[1:], x, fn)
		}
	}
}

// targetIDAt is the ID of the innermost element on the way to pointer whose
// ID has the ID form, in the decoded model document root. Only elements
// count: other objects with an id member, such as a scope.owner, hold
// identifiers of the provider.
func targetIDAt(root any, pointer string) string {
	tokens := strings.Split(pointer, "/")[1:]
	target, depth := "", 0
	for _, e := range elements {
		if len(e.at) <= depth {
			continue
		}
		if id := idOf(elementAt(root, e.at, tokens)); id != "" {
			target, depth = id, len(e.at)
		}
	}
	return target
}

// elementAt follows the first tokens of a pointer along pattern and returns
// what stands there, or nil when the pointer does not lead through it. A * is
// an index of an array: the member "0" of an object written where the schema
// wants an array is not an element.
func elementAt(v any, pattern, tokens []string) any {
	if len(tokens) < len(pattern) {
		return nil
	}
	for i, p := range pattern {
		switch x := v.(type) {
		case []any:
			index, err := strconv.Atoi(tokens[i])
			if p != "*" || err != nil || index < 0 || index >= len(x) {
				return nil
			}
			v = x[index]
		case map[string]any:
			if p == "*" || p != tokens[i] {
				return nil
			}
			v = x[p]
		default:
			return nil
		}
	}
	return v
}

func idOf(v any) string {
	obj, _ := v.(map[string]any)
	id, _ := obj["id"].(string)
	if !ValidID(id) {
		return ""
	}
	return id
}
