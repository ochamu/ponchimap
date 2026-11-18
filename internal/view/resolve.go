package view

import (
	"maps"
	"slices"
	"strconv"
	"strings"

	"github.com/ochamu/ponchimap/internal/diag"
	"github.com/ochamu/ponchimap/internal/model"
)

// Resolve fits the config to a model. A view config is written by hand and
// kept while the model changes, so an entry that names an ID the model no
// longer has is left out with a warning, and the rest of the config still
// draws. An ID that the config declares and the model already uses is an
// error. So is what cannot be drawn as asked, judged on the entries that are
// left: a node that two groups or two aggregates list, and a member of a
// group or an aggregate that the config hides. The returned config is nil
// exactly when the list has an error. Each boundary alias, group, and
// aggregate it keeps knows where it stands in the file, for the warnings of
// Build.
func (f *ConfigFile) Resolve(m *model.Model) (*Config, diag.List) {
	c := &f.written
	r := resolver{file: f, nodes: map[string]bool{}, edges: map[string]bool{}, hidden: map[string]bool{},
		listed: map[string]map[string]bool{"group": {}, "aggregate": {}}}
	for _, n := range m.Nodes {
		r.nodes[n.ID] = true
	}
	for _, e := range m.Edges {
		r.edges[e.ID] = true
	}

	out := *c
	out.Labels = maps.Clone(c.Labels)
	for _, id := range slices.Sorted(maps.Keys(c.Labels)) {
		if !r.has(id, anyElement) {
			r.missing("/labels/"+pointerToken(id), true, id, anyElement)
			delete(out.Labels, id)
		}
	}
	out.Hidden = r.keep("/hidden", c.Hidden, anyElement)
	for _, id := range out.Hidden {
		r.hidden[id] = true
	}
	out.Boundaries = nil
	for i, al := range c.Boundaries {
		al.at = "/boundaries/" + strconv.Itoa(i)
		// A hidden member still says which boundary the name is for: the
		// boundaries are derived from the model, whatever the config hides.
		if r.nodes[al.Member] {
			out.Boundaries = append(out.Boundaries, al)
			continue
		}
		r.missing(al.at+"/member", false, al.Member, boundaryMember)
	}
	out.Groups = nil
	for i, g := range c.Groups {
		g.at = "/groups/" + strconv.Itoa(i)
		if g.Members = r.members(g.at, "group", g.ID, g.Members); g.Members != nil {
			out.Groups = append(out.Groups, g)
		}
	}
	out.Aggregates = nil
	for i, a := range c.Aggregates {
		a.at = "/aggregates/" + strconv.Itoa(i)
		if a.Members = r.members(a.at, "aggregate", a.ID, a.Members); a.Members != nil {
			out.Aggregates = append(out.Aggregates, a)
		}
	}
	r.found.Sort()
	if r.found.HasErrors() {
		return nil, r.found
	}
	return &out, r.found
}

type resolver struct {
	file         *ConfigFile
	nodes, edges map[string]bool
	hidden       map[string]bool
	listed       map[string]map[string]bool
	found        diag.List
}

// referent is what an entry of the config may name: a node, or a node or an
// edge.
type referent struct {
	edges         bool
	message, hint string
}

var (
	nodeOnly = referent{
		message: "no node of the model has this ID; the entry is left out",
		hint:    "Remove the entry, or name a node of the model.",
	}
	// A display name holds for as long as the node it names is in the
	// boundary, so the way to keep one is to name another node of the boundary.
	boundaryMember = referent{
		message: "no node of the model has this ID; the display name is left out",
		hint:    "Name another node that is in the boundary, or remove the entry.",
	}
	anyElement = referent{
		edges:   true,
		message: "no node or edge of the model has this ID; the entry is left out",
		hint:    "Remove the entry, or name a node or an edge of the model.",
	}
)

const hiddenMember = "the member is hidden by the view config"

func (r *resolver) has(id string, want referent) bool {
	return r.nodes[id] || (want.edges && r.edges[id])
}

func (r *resolver) report(severity diag.Severity, code, pointer string, key bool, id, message, hint string) {
	at := r.file.doc.Locate(pointer, key)
	at.Path = r.file.name
	// The schema holds every name of an element to the ID form, so id is a
	// name and not a value written here by mistake.
	r.found = append(r.found, diag.Diagnostic{Severity: severity, Code: code, Message: message, Hint: hint, Location: at, TargetID: id})
}

func (r *resolver) missing(pointer string, key bool, id string, want referent) {
	r.report(diag.Warning, "view.unknown_reference", pointer, key, id, want.message, want.hint)
}

// keep returns the IDs of the list at pointer that the model has.
func (r *resolver) keep(pointer string, ids []string, want referent) []string {
	var kept []string
	for i, id := range ids {
		if r.has(id, want) {
			kept = append(kept, id)
			continue
		}
		r.missing(pointer+"/"+strconv.Itoa(i), false, id, want)
	}
	return kept
}

// members returns the members of the group or the aggregate at pointer that
// the model has. A container left without members is not drawn, and nil is
// returned for it.
func (r *resolver) members(pointer, kind, id string, members []string) []string {
	kept := r.keep(pointer+"/members", members, nodeOnly)
	if kept == nil {
		r.report(diag.Warning, "view.no_members", pointer, false, id,
			"no member of the "+kind+" is in the model; the "+kind+" is left out",
			"Name members that the model has, or remove the "+kind+". When no "+kind+" is left, remove the "+kind+"s key as well.")
		return nil
	}
	if r.has(id, anyElement) {
		r.report(diag.Error, "view.duplicate_id", pointer+"/id", false, id,
			"the ID is already used by a node or an edge of the model",
			"Give the "+kind+" an ID that no node, edge, group, or aggregate uses.")
	}
	for i, member := range members {
		if !r.nodes[member] {
			continue
		}
		at := pointer + "/members/" + strconv.Itoa(i)
		if r.hidden[member] {
			r.report(diag.Error, "view.hidden_member", at, false, id, hiddenMember,
				"Remove the member from the "+kind+", or remove its ID from hidden.")
		}
		if r.listed[kind][member] {
			r.report(diag.Error, "view.listed_twice", at, false, id,
				"the node is already listed by another "+kind,
				"A node is drawn inside one "+kind+". Remove the node from one of the two "+kind+"s.")
		}
		r.listed[kind][member] = true
	}
	return kept
}

// pointerToken makes key one token of a JSON Pointer (RFC 6901).
func pointerToken(key string) string {
	return strings.ReplaceAll(strings.ReplaceAll(key, "~", "~0"), "/", "~1")
}
