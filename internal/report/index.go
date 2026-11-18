package report

import (
	"bytes"
	"encoding/json"
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/ochamu/ponchimap/internal/model"
	"github.com/ochamu/ponchimap/internal/view"
)

// index finds the parts of a model by ID. It reads the model alone.
type index struct {
	m        *model.Model
	nodes    map[string]*model.Node
	edges    map[string]*model.Edge
	evidence map[string]owned
	coverage map[string]*model.Coverage
	// acquired gives the evidence that names an acquisition, and covering
	// the coverage records that name it, both in the order of the model.
	acquired, covering map[string][]string
	// naming gives the coverage records that list an element under
	// target_ids, in the order of the model.
	naming map[string][]string
}

type owned struct {
	*model.Evidence
	owner string
}

func newIndex(m *model.Model) *index {
	ix := &index{
		m: m, nodes: map[string]*model.Node{}, edges: map[string]*model.Edge{},
		evidence: map[string]owned{}, coverage: map[string]*model.Coverage{},
		acquired: map[string][]string{}, covering: map[string][]string{}, naming: map[string][]string{},
	}
	add := func(owner string, evs []model.Evidence) {
		for i := range evs {
			ev := &evs[i]
			ix.evidence[ev.ID] = owned{ev, owner}
			if ev.AcquisitionID != "" {
				ix.acquired[ev.AcquisitionID] = append(ix.acquired[ev.AcquisitionID], ev.ID)
			}
		}
	}
	for i := range m.Nodes {
		n := &m.Nodes[i]
		ix.nodes[n.ID] = n
		add(n.ID, n.Evidence)
	}
	for i := range m.Edges {
		e := &m.Edges[i]
		ix.edges[e.ID] = e
		add(e.ID, e.Evidence)
	}
	for i := range m.Coverage {
		c := &m.Coverage[i]
		ix.coverage[c.ID] = c
		if c.AcquisitionID != "" {
			ix.covering[c.AcquisitionID] = append(ix.covering[c.AcquisitionID], c.ID)
		}
		for _, id := range c.Scope.TargetIDs() {
			if !slices.Contains(ix.naming[id], c.ID) {
				ix.naming[id] = append(ix.naming[id], c.ID)
			}
		}
	}
	return ix
}

// name is the short name that follows a link to a node or an edge.
func (ix *index) name(id string) string {
	if e := ix.edges[id]; e != nil {
		relation := string(e.Relation)
		if name, ok := relationNames[e.Relation]; ok {
			relation = name
		}
		return fmt.Sprintf("%s（%s → %s）", relation, e.From, e.To)
	}
	return view.TitleOf(ix.nodes[id])
}

func (ix *index) element(id string) inline {
	return link(id).text(" " + ix.name(id))
}

func (ix *index) elements(ids []string) inline {
	parts := make([]inline, len(ids))
	for i, id := range ids {
		parts[i] = ix.element(id)
	}
	return joined(parts, "、")
}

// dashed reports whether a figure draws the node or edge dashed: no evidence
// backs it directly.
func (ix *index) dashed(id string) bool {
	if e := ix.edges[id]; e != nil {
		return view.LineOf(e) == view.LineDashed
	}
	return view.FrameOf(ix.nodes[id]) == view.FrameDashed
}

func (ix *index) cite(id string, owner func(id string) inline) inline {
	ev := ix.evidence[id]
	return link(id).text(": ").add(owner(ev.owner)).text("の" + named(evidenceKindNames, ev.Kind))
}

func (ix *index) evidenceLine(ev *model.Evidence) line {
	l := line{anchor: ev.ID, body: text("根拠 " + ev.ID + ": " + named(evidenceKindNames, ev.Kind))}
	for _, s := range ev.Supports {
		l.sub = append(l.sub, labelled("支える内容", support(s)))
	}
	l.sub = append(l.sub, labelled("出典", refText(ev.Ref)))
	l.sub = append(l.sub, rows("確認時刻", ev.CheckedAt, "対象時点", ev.ValidTime)...)
	if ev.AcquisitionID != "" {
		acquisition := text(ev.AcquisitionID)
		if records := ix.covering[ev.AcquisitionID]; len(records) > 0 {
			acquisition = acquisition.text("（調査範囲 ").add(links(records)).text("）")
		}
		l.sub = append(l.sub, labelled("取得記録", acquisition))
	}
	return l
}

func refText(ref model.Ref) inline {
	out := text(ref.Type + " ").add(code(ref.Path))
	switch {
	case ref.StartLine > 0 && ref.EndLine > ref.StartLine:
		out = out.text(fmt.Sprintf(" %d〜%d行", ref.StartLine, ref.EndLine))
	case ref.StartLine > 0:
		out = out.text(fmt.Sprintf(" %d行", ref.StartLine))
	}
	return out
}

// Nothing says what the value under a key the schema does not define is, so
// it may be one that must not be shown. support and keyed are the ways the
// parts of the model that keep such keys are printed: they name the key and
// leave the value out.

func support(s model.Support) inline {
	if !slices.Contains(model.DefinedKeys(model.InSupports), s.Key) {
		return code(s.Key).text("。" + undefinedKeyText)
	}
	return pair(s.Key, s.Value)
}

// keyed writes the items of m, which is the part in of the model. The items
// under the keys the schema defines are written as pairs, and the other keys
// follow by name alone.
func keyed(m map[string]any, in model.Keyed, first ...string) inline {
	defined := map[string]any{}
	var withheld, parts []inline
	for _, key := range slices.Sorted(maps.Keys(m)) {
		if slices.Contains(model.DefinedKeys(in), key) {
			defined[key] = m[key]
		} else {
			withheld = append(withheld, code(key))
		}
	}
	if shown := pairs(defined, first...); len(shown) > 0 {
		parts = append(parts, shown)
	}
	if len(withheld) > 0 {
		parts = append(parts, joined(withheld, "、"), text(undefinedKeyText))
	}
	return joined(parts, "。")
}

// pair is printed as code, so that no sign of a key or a value is read as
// Markdown.
func pair(key string, value any) inline { return code(key + " = " + jsonText(value)) }

// pairs writes every item of m as "key = value": the keys of first in that
// order, then the rest in the order of the keys.
func pairs(m map[string]any, first ...string) inline {
	var parts []inline
	for _, key := range first {
		if value, ok := m[key]; ok {
			parts = append(parts, pair(key, value))
		}
	}
	for _, key := range slices.Sorted(maps.Keys(m)) {
		if !slices.Contains(first, key) {
			parts = append(parts, pair(key, m[key]))
		}
	}
	return joined(parts, "、")
}

func without(m map[string]any, keys ...string) map[string]any {
	out := maps.Clone(m)
	for _, key := range keys {
		delete(out, key)
	}
	return out
}

// jsonText writes a value as the model has it.
func jsonText(v any) string {
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return fmt.Sprint(v)
	}
	return strings.TrimSpace(b.String())
}
