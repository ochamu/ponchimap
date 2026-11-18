package importer

import (
	"bytes"
	"cmp"
	"encoding/json"
	"fmt"
	"maps"
	"slices"
	"sync"

	"github.com/ochamu/ponchimap/internal/diag"
	"github.com/ochamu/ponchimap/internal/input"
	"github.com/ochamu/ponchimap/internal/model"
	"github.com/ochamu/ponchimap/schema"
)

var modelSchema = input.Resource{Name: "model.schema.json", JSON: schema.Model}

var findingsSchema = sync.OnceValue(func() *input.Schema { return input.MustCompile(schema.Findings, modelSchema) })

// The candidates stand in the order of their IDs, as Order has it, whatever
// the order of the file.
type findings struct {
	path         string
	doc          *input.Document
	acq          record
	environment  string
	nodes        []candNode
	edges        []candEdge
	coverage     []candCoverage
	livePosition int
}

type candidate struct {
	id         local
	pointer    string
	attributes map[string]any
	evidence   []candEvidence
}

type candNode struct {
	candidate
	kind, provider, service, environment string
	identity                             map[string]any
}

type candEdge struct {
	candidate
	from, to          local
	relation          model.Relation
	slot, environment string
}

type candEvidence struct {
	id        local
	kind      model.EvidenceKind
	supports  []candSupport
	ref       model.Ref
	checkedAt string
	validTime string
}

type candSupport struct {
	key    string
	value  any
	target local
}

type candCoverage struct {
	id           local
	scope        model.Scope
	targets      []local
	hasTargetIDs bool
	status       model.CoverageStatus
	reason       string
	checkedAt    string
}

// readFindings reads the findings file at path through the stages of Reading
// up to the IDs and references of the candidates.
func readFindings(path string) (*findings, diag.List) {
	raw, list := input.ReadFile(path)
	if list != nil {
		return nil, list
	}
	doc, list := input.DecodeJSON(raw)
	var f *findings
	if doc != nil {
		readable := !list.HasErrors()
		checked := findingsSchema().Check(doc)
		list = append(list, checked...)
		if readable && !checked.HasErrors() {
			wire, err := decodeFindings(doc.Value)
			if err != nil {
				list = append(list, unreadableInput.at(diag.Location{}))
			} else {
				list = append(list, wire.checkCandidates(doc)...)
				f = wire.findings(path, doc)
			}
		}
	}
	for i := range list {
		list[i].Location.Path = path
	}
	if list.HasErrors() {
		return nil, list
	}
	return f, list
}

type wireFindings struct {
	Acquisition struct {
		ID        string         `json:"id"`
		Source    string         `json:"source"`
		Route     string         `json:"route"`
		Operation string         `json:"operation"`
		Target    map[string]any `json:"target"`
		CheckedAt string         `json:"checked_at"`
		Complete  bool           `json:"complete"`
	} `json:"acquisition"`
	Nodes       []model.Node     `json:"nodes"`
	Edges       []model.Edge     `json:"edges"`
	Coverage    []model.Coverage `json:"coverage"`
	Diagnostics []struct {
		TargetIDs []string `json:"target_ids"`
	} `json:"diagnostics"`
}

// The value goes through JSON so that the findings hold exactly what was
// checked.
func decodeFindings(value any) (*wireFindings, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var w wireFindings
	if err := dec.Decode(&w); err != nil {
		return nil, err
	}
	return &w, nil
}

func (w *wireFindings) checkCandidates(doc *input.Document) diag.List {
	type declaration struct {
		id string
		at diag.Location
	}
	var declared []declaration
	nodes, elements := map[string]bool{}, map[string]bool{}
	declare := func(pointer, id string) {
		declared = append(declared, declaration{id, doc.Locate(pointer+"/id", false)})
	}
	type reference struct {
		pointer, id string
		missing     notice
		among       map[string]bool
	}
	var references []reference
	evidence := func(pointer string, items []model.Evidence) {
		for i, ev := range items {
			at := fmt.Sprintf("%s/evidence/%d", pointer, i)
			declare(at, ev.ID)
			for j, s := range ev.Supports {
				if id, ok := s.Value.(string); ok && s.Key == model.KeyConnectionTarget {
					references = append(references, reference{fmt.Sprintf("%s/supports/%d/value", at, j), id, unknownNode, nodes})
				}
			}
		}
	}
	for i, n := range w.Nodes {
		at := fmt.Sprintf("/nodes/%d", i)
		declare(at, n.ID)
		nodes[n.ID], elements[n.ID] = true, true
		evidence(at, n.Evidence)
	}
	for i, e := range w.Edges {
		at := fmt.Sprintf("/edges/%d", i)
		declare(at, e.ID)
		elements[e.ID] = true
		references = append(references,
			reference{at + "/from", e.From, unknownNode, nodes}, reference{at + "/to", e.To, unknownNode, nodes})
		evidence(at, e.Evidence)
	}
	for i, c := range w.Coverage {
		at := fmt.Sprintf("/coverage/%d", i)
		declare(at, c.ID)
		for j, id := range c.Scope.TargetIDs() {
			references = append(references, reference{fmt.Sprintf("%s/scope/target_ids/%d", at, j), id, unknownElement, elements})
		}
	}
	for i, d := range w.Diagnostics {
		for j, id := range d.TargetIDs {
			references = append(references, reference{fmt.Sprintf("/diagnostics/%d/target_ids/%d", i, j), id, unknownElement, elements})
		}
	}

	// The later of two declarations is the one later in the text.
	slices.SortStableFunc(declared, func(a, b declaration) int {
		return cmp.Or(cmp.Compare(a.at.Line, b.at.Line), cmp.Compare(a.at.Column, b.at.Column))
	})
	var list diag.List
	seen := map[string]bool{}
	for _, d := range declared {
		if seen[d.id] {
			list = append(list, duplicateID.at(d.at))
		}
		seen[d.id] = true
	}
	for _, r := range references {
		if !r.among[r.id] {
			list = append(list, r.missing.at(doc.Locate(r.pointer, false)))
		}
	}
	return list
}

// digestOfFindings is the digest of what a read says. The order of the
// candidates is no part of that, so the same read given again with its
// candidates in another order is the same read.
func digestOfFindings(value any) string {
	byID := func(candidates []any) []any {
		inOrder := slices.Clone(candidates)
		slices.SortStableFunc(inOrder, func(a, b any) int { return cmp.Compare(candidateID(a), candidateID(b)) })
		return inOrder
	}
	read, _ := value.(map[string]any)
	read = maps.Clone(read)
	for _, list := range []string{"nodes", "edges", "coverage"} {
		candidates, has := read[list].([]any)
		if !has {
			continue
		}
		candidates = byID(candidates)
		for i, c := range candidates {
			element, _ := c.(map[string]any)
			if evidence, has := element["evidence"].([]any); has {
				element = maps.Clone(element)
				element["evidence"] = byID(evidence)
				candidates[i] = element
			}
		}
		read[list] = candidates
	}
	return digest(read)
}

func candidateID(candidate any) string {
	object, _ := candidate.(map[string]any)
	id, _ := object["id"].(string)
	return id
}

func (w *wireFindings) findings(path string, doc *input.Document) *findings {
	a := w.Acquisition
	f := &findings{path: path, doc: doc, acq: record{
		ID: a.ID, Source: a.Source, Route: a.Route, Operation: a.Operation, Target: a.Target,
		CheckedAt: a.CheckedAt, Complete: a.Complete, Input: path, Digest: digestOfFindings(doc.Value), Reported: len(w.Diagnostics),
	}}
	f.environment, _ = a.Target["environment"].(string)
	evidence := func(items []model.Evidence) []candEvidence {
		out := make([]candEvidence, len(items))
		for i, ev := range items {
			out[i] = candEvidence{id: local(ev.ID), kind: ev.Kind, ref: ev.Ref, checkedAt: ev.CheckedAt, validTime: ev.ValidTime}
			for _, s := range ev.Supports {
				if id, ok := s.Value.(string); ok && s.Key == model.KeyConnectionTarget {
					out[i].supports = append(out[i].supports, candSupport{key: s.Key, target: local(id)})
				} else {
					out[i].supports = append(out[i].supports, candSupport{key: s.Key, value: s.Value})
				}
			}
		}
		slices.SortFunc(out, func(a, b candEvidence) int { return cmp.Compare(a.id, b.id) })
		return out
	}
	for i, n := range w.Nodes {
		f.nodes = append(f.nodes, candNode{
			candidate: candidate{id: local(n.ID), pointer: fmt.Sprintf("/nodes/%d", i), attributes: n.Attributes, evidence: evidence(n.Evidence)},
			kind:      string(n.Kind), provider: n.Provider, service: n.Service, environment: n.Environment, identity: n.Identity,
		})
	}
	for i, e := range w.Edges {
		f.edges = append(f.edges, candEdge{
			candidate: candidate{id: local(e.ID), pointer: fmt.Sprintf("/edges/%d", i), attributes: e.Attributes, evidence: evidence(e.Evidence)},
			from:      local(e.From), to: local(e.To), relation: e.Relation, slot: e.Slot, environment: e.Environment,
		})
	}
	for _, c := range w.Coverage {
		cov := candCoverage{id: local(c.ID), scope: model.Scope{}, status: c.Status, reason: c.Reason, checkedAt: c.CheckedAt}
		for key, value := range c.Scope {
			if key != "target_ids" {
				cov.scope[key] = value
				continue
			}
			cov.hasTargetIDs = true
			for _, id := range c.Scope.TargetIDs() {
				cov.targets = append(cov.targets, local(id))
			}
		}
		f.coverage = append(f.coverage, cov)
	}
	slices.SortFunc(f.nodes, func(a, b candNode) int { return cmp.Compare(a.id, b.id) })
	slices.SortFunc(f.edges, func(a, b candEdge) int { return cmp.Compare(a.id, b.id) })
	slices.SortFunc(f.coverage, func(a, b candCoverage) int { return cmp.Compare(a.id, b.id) })
	return f
}
