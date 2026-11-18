package report

import (
	"cmp"
	"fmt"
	"slices"
	"strings"

	"github.com/ochamu/ponchimap/internal/model"
	"github.com/ochamu/ponchimap/internal/view"
)

// figure finds how the scene shows the elements of the model.
type figure struct {
	s      *view.Scene
	shown  map[string]view.Element
	nodes  map[string]*view.Node
	edges  map[string]*view.Edge
	groups map[string]*view.Group
	// members gives the model nodes drawn in a group or in a group inside
	// it, in the order of the model.
	members map[string][]string
	// hiddenEnds gives the ends of a model edge that the view config hides,
	// its source first.
	hiddenEnds map[string][]string
}

func newFigure(s *view.Scene, m *model.Model) *figure {
	g := &figure{
		s: s, shown: map[string]view.Element{}, nodes: map[string]*view.Node{}, edges: map[string]*view.Edge{},
		groups: map[string]*view.Group{}, members: map[string][]string{}, hiddenEnds: map[string][]string{},
	}
	for _, el := range s.Elements {
		g.shown[el.ID] = el
	}
	for i := range s.Nodes {
		g.nodes[s.Nodes[i].ID] = &s.Nodes[i]
	}
	for i := range s.Edges {
		g.edges[s.Edges[i].ID] = &s.Edges[i]
	}
	for i := range s.Groups {
		g.groups[s.Groups[i].ID] = &s.Groups[i]
	}
	for _, n := range m.Nodes {
		for _, group := range g.around(n.ID) {
			g.members[group.ID] = append(g.members[group.ID], n.ID)
		}
	}
	for _, e := range m.Edges {
		for _, end := range slices.Compact([]string{e.From, e.To}) {
			if g.shown[end].Shown == view.ShownHidden {
				g.hiddenEnds[e.ID] = append(g.hiddenEnds[e.ID], end)
			}
		}
	}
	return g
}

// around returns the groups the figure draws a model node in, innermost
// first. A hidden node is in none.
func (g *figure) around(nodeID string) []*view.Group {
	n := g.nodes[g.shown[nodeID].At]
	if n == nil {
		return nil
	}
	var out []*view.Group
	for at := g.groups[n.GroupID]; at != nil; at = g.groups[at.ParentID] {
		out = append(out, at)
	}
	return out
}

func (g *figure) groupRef(group *view.Group) inline {
	return link(group.ID).text(" " + group.Label)
}

func (g *figure) describe(template string, el view.Element) string {
	fill := []string{hiddenEndsMark, strings.Join(g.hiddenEnds[el.ID], " と ")}
	if n := g.nodes[el.At]; n != nil {
		look := frameNames[n.Frame] + "、" + marksText(n.Marks)
		card := look
		if n.Role != "" {
			card += "。ラベル「" + n.Role + "」"
		}
		fill = append(fill, aggregateMark, fmt.Sprintf("集約「%s」（%s）", n.Title, n.ID), frameMark, look, cardMark, card)
	}
	if e := g.edges[el.At]; e != nil {
		fill = append(fill, lineMark, fmt.Sprintf("%s、ラベル「%s」", lineNames[e.Line], e.Label))
	}
	return strings.NewReplacer(fill...).Replace(template)
}

func (g *figure) drawn(id string) line {
	el := g.shown[id]
	body := text(g.describe(cmp.Or(shownTexts[el.Shown].drawn, string(el.Shown)), el))
	if el.Shown == view.ShownLine && el.At != id {
		body = link(el.At).text(sharedLineText).add(body)
	}
	return labelled("図での表示", body)
}

func states(a *view.Assessment, id string) []inline {
	var out []inline
	for i, f := range a.Findings {
		if slices.Contains(f.About, id) {
			out = append(out, text(f.Kind().Label()+"。"+sentence(f)+"（").add(linkAs(detailLabel, findingAnchor(i+1))).text("）"))
		}
	}
	return out
}

func stateRows(ix *index, a *view.Assessment, id string) []line {
	found := states(a, id)
	if unbacked(ix, a, id) {
		found = append(found, text(weakStateText))
	}
	if len(found) == 0 {
		found = []inline{text(noStateText)}
	}
	var out []line
	for _, said := range found {
		out = append(out, labelled("状態", said))
	}
	return out
}

func namingRows(ix *index, id string) []line {
	var out []line
	for _, cv := range ix.naming[id] {
		c := ix.coverage[cv]
		body := link(cv).text(" " + statusName(c.Status))
		if narrowed := narrowing(c.Scope); len(narrowed) > 0 {
			body = body.text("。").add(narrowed)
		}
		out = append(out, labelled("名指しした調査範囲", body))
	}
	return out
}

// roleAttribute and purposeAttribute are the attributes that say what a node
// and an edge are for. Each has a row of its own, written as prose.
const (
	roleAttribute    = "role"
	purposeAttribute = "purpose"
)

// prose takes the attribute key out of attrs when it is text. An attribute
// of another type stays among the rest, so that no value is dropped.
func prose(attrs map[string]any, key string) (said string, rest map[string]any) {
	said, ok := attrs[key].(string)
	if !ok {
		return "", attrs
	}
	return said, without(attrs, key)
}

func nodeItem(ix *index, a *view.Assessment, g *figure, n *model.Node) item {
	role, attributes := prose(n.Attributes, roleAttribute)
	it := item{id: n.ID, heading: n.ID + " " + ix.name(n.ID)}
	it.lines = slices.Concat(
		rows(
			"種別", named(nodeKindNames, n.Kind),
			"環境", n.Environment,
			"役割", cmp.Or(role, noRoleText),
		),
		// identity is where a model keeps the identifiers of a node, whatever
		// their keys are, and the report prints identifiers.
		row("識別情報", pairs(n.Identity)),
		row("属性", keyed(attributes, model.InNodeAttributes)),
		stateRows(ix, a, n.ID),
		[]line{g.drawn(n.ID)},
		namingRows(ix, n.ID),
	)
	for _, group := range g.around(n.ID) {
		it.lines = append(it.lines, labelled("所属", g.membership(group, n)))
	}
	for i := range n.Evidence {
		it.lines = append(it.lines, ix.evidenceLine(&n.Evidence[i]))
	}
	return it
}

// membership names a group a node is drawn in, with the evidence of the node
// that puts it there. A logical group has no such evidence.
func (g *figure) membership(group *view.Group, n *model.Node) inline {
	if group.Kind == view.GroupLogical {
		return g.groupRef(group).text("。" + logicalMembershipText)
	}
	var own []string
	for _, ev := range n.Evidence {
		if slices.Contains(group.EvidenceIDs, ev.ID) {
			own = append(own, ev.ID)
		}
	}
	if len(own) == 0 {
		return g.groupRef(group)
	}
	return g.groupRef(group).text("。根拠 ").add(links(own))
}

func edgeItem(ix *index, a *view.Assessment, g *figure, e *model.Edge) item {
	purpose, attributes := prose(e.Attributes, purposeAttribute)
	it := item{id: e.ID, heading: e.ID + " " + ix.name(e.ID)}
	it.lines = slices.Concat(
		rows("関係の種類", named(relationNames, e.Relation)),
		[]line{labelled("接続元", ix.element(e.From)), labelled("接続先", ix.element(e.To))},
		rows(
			"設定項目（slot）", e.Slot,
			"環境", e.Environment,
			"目的", purpose,
		),
		row("属性", keyed(attributes, model.InEdgeAttributes)),
		stateRows(ix, a, e.ID),
		[]line{g.drawn(e.ID)},
		namingRows(ix, e.ID),
	)
	for i := range e.Evidence {
		it.lines = append(it.lines, ix.evidenceLine(&e.Evidence[i]))
	}
	return it
}

func groupItem(ix *index, g *figure, group *view.Group) item {
	it := item{id: group.ID, heading: group.ID + " " + group.Label}
	it.lines = rows("種別", groupKindNames[group.Kind])
	if parent := g.groups[group.ParentID]; parent != nil {
		it.lines = append(it.lines, labelled("外側のグループ", g.groupRef(parent)))
	}
	var inner []inline
	for i := range g.s.Groups {
		if child := &g.s.Groups[i]; child.ParentID == group.ID {
			inner = append(inner, g.groupRef(child))
		}
	}
	if len(inner) > 0 {
		it.lines = append(it.lines, labelled("内側のグループ", joined(inner, "、")))
	}
	if members := g.members[group.ID]; len(members) > 0 {
		it.lines = append(it.lines, labelled("図に載せた構成要素", links(members)))
	}
	if len(group.EvidenceIDs) > 0 {
		backing := line{body: text("境界と所属を支える根拠:")}
		for _, id := range group.EvidenceIDs {
			backing.sub = append(backing.sub, line{body: ix.cite(id, link).text("。出典 ").add(refText(ix.evidence[id].Ref))})
		}
		it.lines = append(it.lines, backing)
	}
	return it
}

// omittedLines lists the elements without a shape of their own in the
// figure, each with why and with its state, so that leaving an element off
// the figure never drops what is unverified or disagrees.
func omittedLines(ix *index, a *view.Assessment, g *figure) []line {
	var out []line
	for _, el := range g.s.Elements {
		t := shownTexts[el.Shown]
		if t.omittedAs == "" {
			continue
		}
		why := g.describe(cmp.Or(t.omitted, t.drawn), el)
		said := states(a, el.ID)
		if len(said) == 0 {
			said = []inline{text(noStateText)}
		}
		out = append(out, line{body: text(t.omittedAs + ": ").add(ix.element(el.ID)).text("。" + why + "。状態: ").add(joined(said, "。"))})
	}
	return out
}
