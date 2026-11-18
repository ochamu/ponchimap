package view

import (
	"cmp"
	"fmt"
	"slices"
	"strings"

	"github.com/ochamu/ponchimap/internal/assets"
	"github.com/ochamu/ponchimap/internal/diag"
	"github.com/ochamu/ponchimap/internal/model"
)

var defaultEdgeLabel = map[model.Relation]string{
	model.RelConfiguration:  "接続設定",
	model.RelImplementation: "呼び出し",
	model.RelObservation:    "通信",
	model.RelDependency:     "依存",
}

const splitNote = "同じ名前の論理グループや集約は、1つの指定を分けて描いたものです。"

// The message and hint of a warning show no value of the input: the pointer
// names the entry of the view config, and the target ID the element. A
// warning about the model alone has no pointer.
type warning struct{ code, message, hint string }

func (w warning) at(pointer, targetID string) diag.Diagnostic {
	return diag.Diagnostic{Severity: diag.Warning, Code: w.code, Message: w.message, Hint: w.hint, TargetID: targetID, Location: diag.Location{Pointer: pointer}}
}

const codeSplit = "view.split"

var (
	warnNoRegion = warning{"view.network_without_region",
		"the evidence of the node names a network and no region; a network is drawn inside the region of its nodes, so the node is drawn outside the network",
		"State the region of the node in the model. A resource that has no region stays outside the network."}
	warnContested = warning{"view.boundary_contested",
		"the evidence of the node names more than one %s, and the figure has no mismatch mark for it; the node is drawn outside the boundaries of that level and of the levels inside it",
		"Check the evidence of the node in the model. A record of an earlier state can be removed, or replaced by the current one. A node that is in two at once stays outside both."}
	warnUnbacked = warning{"view.boundary_unbacked",
		"the node is in no boundary of this level: its evidence names none or more than one, or leaves a level outside this one unsettled; the display name is left out",
		"Name another node that is in the boundary, or remove the entry. A view config names boundaries and cannot add one."}
	warnNamedTwice = warning{"view.boundary_named_twice",
		"an earlier entry names the same boundary; this display name is left out",
		"Keep one entry for the boundary and remove the others."}
	warnLabelShared = warning{"view.boundary_label_shared",
		"the boundary is drawn under the same label as another boundary beside it",
		"Give the boundary a name of its own: add an entry to boundaries whose member is the target ID and whose level is %s."}
	listingGroup = listing{"group", warning{codeSplit,
		"the members of the group are in different real boundaries; the group is drawn once in each, under the same label",
		"Keep the entry if the figure reads well. To draw the group once, list members of one boundary only."}}
	listingAggregate = listing{"aggregate", warning{codeSplit,
		"the members of the aggregate are in different real boundaries or logical groups; the aggregate is drawn once in each, under the same label",
		"Keep the entry if the figure reads well. To draw the aggregate once, list members of one boundary and one logical group only."}}
)

// listing is a kind of entry of the view config that lists nodes: a group or
// an aggregate.
type listing struct {
	word string
	// split is the warning for an entry drawn in parts.
	split warning
}

var markOrder = []Mark{
	{Kind: MarkUnverified}, {Kind: MarkMismatch},
	{Kind: MarkUnverified, Of: MarkOfRelation}, {Kind: MarkMismatch, Of: MarkOfRelation},
}

// Build derives a Scene from a model and a config fitted to it: the config
// names elements of the model only, which Resolve sees to for a config read
// from a file. Build fails on anything that would make the diagram claim more
// than the evidence does: a defect of Assess, hidden element or not, and a
// group or an aggregate that lists a hidden node, or a node another one
// lists. Where it can draw what the evidence says and not what the view
// config asks, it does, and says so in the Warnings of the scene.
func Build(m *model.Model, c *Config) (*Scene, error) {
	b := &builder{m: m, c: c, hidden: map[string]bool{}, real: deriveBoundaries(m)}
	if err := b.checkConfig(); err != nil {
		return nil, err
	}
	b.assessment = Assess(m)
	if err := b.assessment.Err(); err != nil {
		return nil, err
	}
	for _, id := range c.Hidden {
		b.hidden[id] = true
	}

	s := &Scene{Title: c.Title, Direction: c.Layout.Direction, Assessment: b.assessment}
	b.placeNodes(s)
	b.nameBoundaries(s)
	if err := b.applyLogicalGroups(s); err != nil {
		return nil, err
	}
	if err := b.applyAggregates(s); err != nil {
		return nil, err
	}
	b.warnOfSharedLabels(s)
	for _, n := range m.Nodes {
		s.Elements = append(s.Elements, b.nodeElement(n.ID))
	}
	b.addEdges(s)
	s.Notes = b.notes(s)
	if slices.ContainsFunc(s.Warnings, func(d diag.Diagnostic) bool { return d.Code == codeSplit }) {
		s.Notes = append(s.Notes, splitNote)
	}
	s.Legend = legendFor(s)
	return s, nil
}

type builder struct {
	m      *model.Model
	c      *Config
	hidden map[string]bool
	real   realBoundaries

	assessment *Assessment
	// sceneID maps a model node ID to the scene node that shows it.
	sceneID map[string]string
}

// checkConfig holds the config to what the schema and Resolve cannot see. The
// form of the config, its own IDs, and what it names in the model are theirs.
func (b *builder) checkConfig() error {
	// A node sits in one container of each kind. Listing it twice would leave
	// the second container pointing at a node the first already replaced.
	memberOf := map[string]string{}
	claim := func(kind, id string, members []string) error {
		for _, mid := range members {
			if other, dup := memberOf[kind+"\x00"+mid]; dup {
				return fmt.Errorf("view config: node %q is listed by %s %q and again by %s %q", mid, kind, other, kind, id)
			}
			memberOf[kind+"\x00"+mid] = id
		}
		return nil
	}
	for _, g := range b.c.Groups {
		if err := claim("group", g.ID, g.Members); err != nil {
			return err
		}
	}
	for _, a := range b.c.Aggregates {
		if err := claim("aggregate", a.ID, a.Members); err != nil {
			return err
		}
	}
	// The direction goes into the scene as it is, so a config that did not
	// come through ParseConfig is held to the same values here.
	switch b.c.Layout.Direction {
	case "", DirRight, DirDown:
	default:
		return fmt.Errorf("view config: layout.direction %q is not right or down", b.c.Layout.Direction)
	}
	return nil
}

func (b *builder) placeNodes(s *Scene) {
	b.sceneID = map[string]string{}
	drawn := map[string]bool{}
	for i := range b.m.Nodes {
		n := &b.m.Nodes[i]
		if b.hidden[n.ID] {
			continue
		}
		group := ""
		for _, in := range b.real.in[n.ID] {
			if in.Boundary != "" {
				group = in.Boundary
				drawn[group] = true
			}
			// A node outside a boundary that its evidence names reads as a
			// node that is not in it. Where no mismatch mark says otherwise,
			// a warning does. A level inside a contested one is left to the
			// word about that one.
			switch {
			case in.Outside == OutsideOuterUnstated:
				s.Warnings = append(s.Warnings, warnNoRegion.at("", n.ID))
			case in.Outside == OutsideContested && !b.mismatchShown(n.ID, in.Level):
				w := warnContested
				w.message = fmt.Sprintf(w.message, in.Level)
				s.Warnings = append(s.Warnings, w.at("", n.ID))
			}
		}
		role, _ := n.Attributes["role"].(string)
		if alias, ok := b.c.Labels[n.ID]; ok {
			role = alias
		}
		var marks []Mark
		for _, kind := range b.assessment.Kinds(n.ID) {
			marks = append(marks, Mark{Kind: kind, Count: 1})
		}
		b.sceneID[n.ID] = n.ID
		s.Nodes = append(s.Nodes, Node{
			ID: n.ID, SourceIDs: []string{n.ID}, GroupID: group,
			Title: TitleOf(n), Role: role, IconKey: iconOf(n),
			Frame: frameOf(n), Marks: marks,
		})
	}
	for _, bd := range b.real.list {
		if drawn[bd.id] {
			s.Groups = append(s.Groups, Group{ID: bd.id, ParentID: bd.parent, Kind: GroupBoundary, Label: bd.label, Level: bd.level, EvidenceIDs: bd.evidence})
		}
	}
}

// mismatchShown reports whether the node carries the mismatch mark for what
// its evidence says about the level. Records of different times are not
// compared, and the networks of a node never are, so two values do not
// always make a mark.
func (b *builder) mismatchShown(id string, level Level) bool {
	key := levels[slices.IndexFunc(levels, func(r levelRule) bool { return r.level == level })].key
	return slices.ContainsFunc(b.assessment.For(id), func(f Finding) bool { return f.Kind() == MarkMismatch && f.Key == key })
}

// The view config cannot make a boundary, so an entry whose member is in none
// is left out, and the figure is still drawn: the evidence may be back after
// the next investigation.
func (b *builder) nameBoundaries(s *Scene) {
	taken := map[string]bool{}
	for i, al := range b.c.Boundaries {
		at := entryPointer(al.at, "boundaries", i)
		level, id := Level(cmp.Or(al.Level, defaultBoundaryLevel)), ""
		for _, in := range b.real.in[al.Member] {
			if in.Level == level {
				id = in.Boundary
			}
		}
		switch {
		case id == "":
			s.Warnings = append(s.Warnings, warnUnbacked.at(at, al.Member))
		case taken[id]:
			s.Warnings = append(s.Warnings, warnNamedTwice.at(at, al.Member))
		default:
			taken[id] = true
			if g := slices.IndexFunc(s.Groups, func(g Group) bool { return g.ID == id }); g >= 0 {
				s.Groups[g].Label = al.Label
			}
		}
	}
}

// A default label says the level and the value of a boundary and nothing
// about the others, so two accounts of one provider share it until the view
// config names them. The warning names the first node of the boundary, which
// is what an entry needs.
func (b *builder) warnOfSharedLabels(s *Scene) {
	type place struct{ parent, label string }
	beside := map[place]int{}
	for _, g := range s.Groups {
		if g.Kind == GroupBoundary {
			beside[place{g.ParentID, g.Label}]++
		}
	}
	first := map[string]string{}
	for _, bd := range b.real.list {
		first[bd.id] = bd.first
	}
	for _, g := range s.Groups {
		if g.Kind == GroupBoundary && beside[place{g.ParentID, g.Label}] > 1 {
			w := warnLabelShared
			w.hint = fmt.Sprintf(w.hint, g.Level)
			s.Warnings = append(s.Warnings, w.at("/boundaries", first[g.ID]))
		}
	}
}

func (b *builder) sceneNode(s *Scene, modelID string) *Node {
	for i := range s.Nodes {
		if s.Nodes[i].ID == b.sceneID[modelID] {
			return &s.Nodes[i]
		}
	}
	return nil
}

type part struct {
	id, container string
	members       []string
}

// One shape cannot wrap members of two containers without drawing one of them
// outside the boundary its evidence puts it in, so each container gets a
// shape of its own.
func (b *builder) parts(s *Scene, kind listing, id, pointer string, members []string) ([]part, error) {
	var parts []part
	for _, mid := range members {
		n := b.sceneNode(s, mid)
		if n == nil {
			return nil, fmt.Errorf("view config: %s %q lists %q, which is hidden or is not a node of the model", kind.word, id, mid)
		}
		at := slices.IndexFunc(parts, func(p part) bool { return p.container == n.GroupID })
		if at < 0 {
			at = len(parts)
			partID := id
			if at > 0 {
				partID = fmt.Sprintf("%s_%d", id, at+1)
			}
			parts = append(parts, part{id: partID, container: n.GroupID})
		}
		parts[at].members = append(parts[at].members, mid)
	}
	if len(parts) > 1 {
		s.Warnings = append(s.Warnings, kind.split.at(pointer, id))
	}
	return parts, nil
}

func (b *builder) applyLogicalGroups(s *Scene) error {
	for i, g := range b.c.Groups {
		parts, err := b.parts(s, listingGroup, g.ID, entryPointer(g.at, "groups", i), g.Members)
		if err != nil {
			return err
		}
		for _, p := range parts {
			s.Groups = append(s.Groups, Group{ID: p.id, ParentID: p.container, Kind: GroupLogical, Label: g.Label})
			for _, mid := range p.members {
				b.sceneNode(s, mid).GroupID = p.id
			}
		}
	}
	return nil
}

// addMarks sums marks of the same kind and subject, in markOrder.
func addMarks(marks []Mark, more ...Mark) []Mark {
	var out []Mark
	for _, sum := range markOrder {
		for _, m := range append(slices.Clone(marks), more...) {
			if m.Kind == sum.Kind && m.Of == sum.Of {
				sum.Count += m.Count
			}
		}
		if sum.Count > 0 {
			out = append(out, sum)
		}
	}
	return out
}

func (b *builder) applyAggregates(s *Scene) error {
	for i, a := range b.c.Aggregates {
		parts, err := b.parts(s, listingAggregate, a.ID, entryPointer(a.at, "aggregates", i), a.Members)
		if err != nil {
			return err
		}
		for _, p := range parts {
			b.fold(s, a, p)
		}
	}
	return nil
}

// fold draws the members of one part of an aggregate as one node. The node
// keeps every source ID and every mark, and is drawn solid only when all
// members are, so that folding never makes the whole look more verified than
// its parts.
func (b *builder) fold(s *Scene, a AggregateConfig, p part) {
	agg := Node{ID: p.id, GroupID: p.container, Title: a.Label, Frame: FrameSolid}
	icons := map[string]bool{}
	member := map[string]bool{}
	for _, mid := range p.members {
		n := b.sceneNode(s, mid)
		member[n.ID] = true
		agg.SourceIDs = append(agg.SourceIDs, n.SourceIDs...)
		icons[n.IconKey] = true
		agg.IconKey = n.IconKey
		if n.Frame == FrameDashed {
			agg.Frame = FrameDashed
		}
		agg.Marks = addMarks(agg.Marks, n.Marks...)
	}
	if len(icons) != 1 {
		agg.IconKey = assets.GenericIcon
	}
	agg.Role = a.Role
	if agg.Role == "" {
		agg.Role = fmt.Sprintf("%d件をまとめて表示", len(p.members))
	}
	kept := s.Nodes[:0]
	placed := false
	for _, n := range s.Nodes {
		if !member[n.ID] {
			kept = append(kept, n)
			continue
		}
		if !placed {
			kept, placed = append(kept, agg), true
		}
	}
	s.Nodes = kept
	for _, mid := range p.members {
		b.sceneID[mid] = p.id
	}
}

type edgeDisplay struct {
	label string
	line  Line
	state MarkKind
}

// display derives how an edge is shown: its line from its evidence, and the
// state its label announces from the assessment. A dashed line names why it
// is dashed instead of a state.
func (b *builder) display(e *model.Edge) edgeDisplay {
	d := edgeDisplay{}
	if kinds := b.assessment.Kinds(e.ID); len(kinds) > 0 {
		d.state = kinds[len(kinds)-1]
	}
	line, statedOnly := lineOf(e)
	d.line = line
	d.label, _ = e.Attributes["purpose"].(string)
	if alias, ok := b.c.Labels[e.ID]; ok {
		d.label = alias
	}
	if d.label == "" {
		d.label = defaultEdgeLabel[e.Relation]
	}
	note := "手がかり"
	switch {
	case line == LineSolid && d.state != "":
		note = d.state.Label()
	case line == LineSolid:
		note = relationRules[e.Relation].note
	case statedOnly:
		note = "説明・未照合"
	}
	if note != "" {
		d.label += "（" + note + "）"
	}
	return d
}

func (b *builder) nodeElement(id string) Element {
	switch at := b.sceneID[id]; {
	case b.hidden[id]:
		return Element{ID: id, Shown: ShownHidden}
	case at != id:
		return Element{ID: id, At: at, Shown: ShownAggregated}
	}
	return Element{ID: id, At: id, Shown: ShownCard}
}

// addEdges draws one line per distinct relation between two scene nodes, and
// records where each model edge went. An edge whose ends fall in the same
// scene node has no line, so the state its label would have announced is
// counted on that node instead.
func (b *builder) addEdges(s *Scene) {
	type shape struct {
		from, to, label string
		line            Line
		state           MarkKind
	}
	merged := map[shape]int{}
	for i := range b.m.Edges {
		e := &b.m.Edges[i]
		el := Element{ID: e.ID}
		from, to := b.sceneID[e.From], b.sceneID[e.To]
		switch {
		case b.hidden[e.ID]:
			el.Shown = ShownHidden
		case b.hidden[e.From] || b.hidden[e.To]:
			el.Shown = ShownEndHidden
		case from == to:
			d := b.display(e)
			n := b.sceneNode(s, e.From)
			n.FoldedEdgeIDs = append(n.FoldedEdgeIDs, e.ID)
			if d.state != "" {
				n.Marks = addMarks(n.Marks, Mark{Kind: d.state, Of: MarkOfRelation, Count: 1})
			}
			el.At, el.Shown = n.ID, ShownFolded
			if e.From == e.To {
				el.Shown = ShownSelf
			}
		default:
			d := b.display(e)
			k := shape{from, to, d.label, d.line, d.state}
			at, ok := merged[k]
			if ok {
				s.Edges[at].SourceIDs = append(s.Edges[at].SourceIDs, e.ID)
			} else {
				at = len(s.Edges)
				merged[k] = at
				s.Edges = append(s.Edges, Edge{ID: e.ID, SourceIDs: []string{e.ID}, From: from, To: to, Label: d.label, Line: d.line, State: d.state})
			}
			el.At, el.Shown = s.Edges[at].ID, ShownLine
		}
		s.Elements = append(s.Elements, el)
	}
}

// notes tells what the figure leaves out: the elements the view config keeps
// off it, and how many of them are unverified or a mismatch, so that hiding
// never hides a discrepancy without a trace, and the edges from a node to
// itself.
func (b *builder) notes(s *Scene) []string {
	var notes, omitted, states []string
	nodes, edges, unverified, mismatch, self := 0, 0, 0, 0, 0
	for i, el := range s.Elements {
		if el.Shown == ShownSelf {
			self++
		}
		if el.Shown != ShownHidden && el.Shown != ShownEndHidden {
			continue
		}
		if i < len(b.m.Nodes) {
			nodes++
		} else {
			edges++
		}
		kinds := b.assessment.Kinds(el.ID)
		if slices.Contains(kinds, MarkUnverified) {
			unverified++
		}
		if slices.Contains(kinds, MarkMismatch) {
			mismatch++
		}
	}
	if nodes > 0 {
		omitted = append(omitted, fmt.Sprintf("構成要素%d件", nodes))
	}
	if edges > 0 {
		omitted = append(omitted, fmt.Sprintf("関係%d件", edges))
	}
	if len(omitted) > 0 {
		notes = append(notes, "表示設定により、"+strings.Join(omitted, "と")+"を図から省いています。")
	}
	if unverified > 0 {
		states = append(states, fmt.Sprintf("未照合は%d件", unverified))
	}
	if mismatch > 0 {
		states = append(states, fmt.Sprintf("不一致は%d件", mismatch))
	}
	if len(states) > 0 {
		notes = append(notes, "省いたもののうち、"+strings.Join(states, "、")+"です。")
	}
	if self > 0 {
		notes = append(notes, fmt.Sprintf("接続元と接続先が同じ関係%d件は、線を描いていません。", self))
	}
	return notes
}

var stateLegend = map[MarkKind]LegendItem{MarkUnverified: LegendUnverified, MarkMismatch: LegendMismatch}

func legendFor(s *Scene) []LegendItem {
	used := map[LegendItem]bool{}
	for _, n := range s.Nodes {
		used[map[Frame]LegendItem{FrameSolid: LegendSolidFrame, FrameDashed: LegendDashedFrame}[n.Frame]] = true
		for _, mk := range n.Marks {
			used[stateLegend[mk.Kind]] = true
		}
	}
	for _, e := range s.Edges {
		used[map[Line]LegendItem{LineSolid: LegendSolidLine, LineDashed: LegendDashedLine}[e.Line]] = true
		if e.State != "" {
			used[stateLegend[e.State]] = true
		}
	}
	for _, g := range s.Groups {
		used[map[GroupKind]LegendItem{GroupBoundary: LegendBoundary, GroupLogical: LegendLogical}[g.Kind]] = true
	}
	var out []LegendItem
	for _, item := range LegendOrder {
		if used[item] {
			out = append(out, item)
		}
	}
	return out
}
