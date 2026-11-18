package importer

import (
	"cmp"
	"fmt"
	"maps"
	"slices"
	"strconv"

	"github.com/ochamu/ponchimap/internal/diag"
	"github.com/ochamu/ponchimap/internal/input"
	"github.com/ochamu/ponchimap/internal/model"
)

// The fold reads no file and no clock: what it gives follows from this alone.
type given struct {
	modelPath   string
	model       *model.Model
	text        *input.Document
	records     []record
	findings    []*findings
	supplements *supplements
}

type folded struct {
	model          *model.Model
	records        []record
	ids            []Mapping
	added, removed []string
	partial        bool
	found          diag.List
	inSaved        diag.List
}

type element struct {
	node           *model.Node
	edge           *model.Edge
	at             int
	items          []*item
	sources        []source
	gained, lost   bool
	gone, heldByR3 bool
}

func (el *element) id() string {
	if el.node != nil {
		return el.node.ID
	}
	return el.edge.ID
}

func (el *element) attributes() *map[string]any {
	if el.node != nil {
		return &el.node.Attributes
	}
	return &el.edge.Attributes
}

type source struct {
	in *findings
	c  *candidate
}

func (s source) at() diag.Location { return locationIn(s.in.path, s.in.doc, s.c.pointer) }

type item struct {
	ev   model.Evidence
	at   int
	out  bool
	from *origin
}

type cover struct {
	c    model.Coverage
	at   int
	out  bool
	from *origin
}

// The fields of origin order the new items as Order does: by acquisition,
// then by the ID of the candidate.
type origin struct {
	input        string
	livePosition int
	candidate    local
	statement    int
}

func (o *origin) compare(other *origin) int {
	return cmp.Or(cmp.Compare(o.livePosition, other.livePosition), cmp.Compare(o.candidate, other.candidate),
		cmp.Compare(o.statement, other.statement))
}

type candidateRef struct {
	in *findings
	id local
}

type work struct {
	given
	target          model.Target
	history         history
	live            []*findings
	fromSupplements map[string]bool
	nodes, edges    []*element
	byID            map[string]*element
	coverage        []*cover
	nodeOf, edgeOf  map[candidateRef]*element
	ids             issuer
	found           diag.List
	about           []remark
	taken           []taken
	said            map[string]map[string]map[string]any
}

// taken is a node candidate of this run: what it says, and the node that it
// was taken as.
type taken struct {
	acquisition string
	says        description
	node        *element
}

type remark struct {
	notice notice
	el     *element
}

// fold applies the rules of the package comment.
//
// A node that leaves the model in this run takes no part in the matching,
// and which nodes leave follows from the matching. So the run is worked out
// again without the nodes that left, from the start and with nothing kept of
// the pass before, until no node that took part leaves. A node that left in
// one pass takes no part in any later one, so the passes end.
func fold(in given) folded {
	left := map[int]bool{}
	for {
		w := &work{given: in, target: in.model.Target, fromSupplements: map[string]bool{}, byID: map[string]*element{},
			nodeOf: map[candidateRef]*element{}, edgeOf: map[candidateRef]*element{}}
		w.read()
		w.replace()
		w.matchNodes(left)
		w.matchEdges()
		w.addEvidence()
		w.unsupported()
		// R5 is judged in the last pass: a candidate that fits two nodes
		// names the one that stays once the other takes no part. When R5
		// ends the run, nothing left the model, so what the steps below
		// find is not reported.
		refused := w.found
		w.restate()
		w.remove()
		again := false
		for i, el := range w.nodes {
			if el.at >= 0 && el.gone && !left[i] {
				left[i], again = true, true
			}
		}
		if again {
			continue
		}
		if refused.HasErrors() {
			return folded{found: refused}
		}
		w.settleCoverage()
		supplementsID := w.applySupplements()
		w.issue()
		w.collectSaid()
		for _, el := range w.elements() {
			w.settleAttributes(el)
		}
		return w.build(supplementsID)
	}
}

func (w *work) read() {
	var incoming []record
	for _, f := range w.findings {
		incoming = append(incoming, f.acq)
	}
	w.history = newHistory(w.records, incoming)
	for _, r := range w.records {
		if r.supplement() {
			w.fromSupplements[r.ID] = true
		}
	}
	for _, f := range w.findings {
		if w.history.live(f.acq.ID) {
			w.live = append(w.live, f)
		}
	}
	slices.SortFunc(w.live, func(a, b *findings) int { return older(a.acq, b.acq) })
	for i, f := range w.live {
		f.livePosition = i
	}

	inModel := map[string]bool{}
	evidence := func(el *element, items []model.Evidence) {
		for i, ev := range items {
			el.items = append(el.items, &item{ev: ev, at: i})
			inModel[ev.ID] = true
		}
		inModel[el.id()] = true
		w.byID[el.id()] = el
	}
	for i := range w.model.Nodes {
		// A pass of fold that is worked out again must find the model as it
		// was read, so each pass writes to a copy of its own: matching adds
		// to the identity, and the last pass settles the attributes.
		n := w.model.Nodes[i]
		n.Identity = maps.Clone(n.Identity)
		n.Attributes = maps.Clone(n.Attributes)
		el := &element{node: &n, at: i}
		evidence(el, el.node.Evidence)
		w.nodes = append(w.nodes, el)
	}
	for i := range w.model.Edges {
		e := w.model.Edges[i]
		e.Attributes = maps.Clone(e.Attributes)
		el := &element{edge: &e, at: i}
		evidence(el, el.edge.Evidence)
		w.edges = append(w.edges, el)
	}
	for i, c := range w.model.Coverage {
		w.coverage = append(w.coverage, &cover{c: c, at: i})
		inModel[c.ID] = true
	}
	// A new node or edge gets its ID before it is known what leaves the
	// model, so it avoids every ID that the model had.
	w.ids = issuer{inModel: inModel, issued: map[string]bool{}}
}

func (w *work) elements() []*element { return slices.Concat(w.nodes, w.edges) }

func (w *work) environment(written ...string) string {
	return cmp.Or(cmp.Or(written...), w.target.Environment)
}

func (w *work) locationInReadModel(list string, index int) diag.Location {
	pointer := fmt.Sprintf("/%s/%d", list, index)
	if w.text == nil {
		return diag.Location{Path: w.modelPath, Pointer: pointer}
	}
	return locationIn(w.modelPath, w.text, pointer)
}

func (w *work) note(n notice, el *element) { w.about = append(w.about, remark{n, el}) }

func (w *work) build(supplementsID string) folded {
	out := folded{found: w.found, model: &model.Model{
		Schema: w.model.Schema, SchemaVersion: w.model.SchemaVersion, Target: w.target,
		Nodes: []model.Node{}, Edges: []model.Edge{}, Coverage: []model.Coverage{}}}
	m := out.model
	acquisitions, became := map[string]bool{}, map[origin]string{}
	pointer := map[*element]string{}
	evidence := func(el *element) []model.Evidence {
		list := []model.Evidence{}
		for _, it := range inOrder(el.items, func(it *item) (int, string, bool) { return it.at, it.ev.ID, !it.out }) {
			list = append(list, it.ev)
			acquisitions[it.ev.AcquisitionID] = true
			if it.from != nil {
				became[*it.from] = it.ev.ID
			}
		}
		return list
	}
	place := func(el *element) (int, string, bool) { return el.at, el.id(), !el.gone }
	for _, el := range inOrder(w.nodes, place) {
		pointer[el] = fmt.Sprintf("/nodes/%d", len(m.Nodes))
		n := *el.node
		n.Evidence = evidence(el)
		m.Nodes = append(m.Nodes, n)
	}
	for _, el := range inOrder(w.edges, place) {
		pointer[el] = fmt.Sprintf("/edges/%d", len(m.Edges))
		e := *el.edge
		e.Evidence = evidence(el)
		m.Edges = append(m.Edges, e)
	}
	for _, c := range inOrder(w.coverage, func(c *cover) (int, string, bool) { return c.at, c.c.ID, !c.out }) {
		m.Coverage = append(m.Coverage, c.c)
		acquisitions[c.c.AcquisitionID] = true
		if c.from != nil {
			became[*c.from] = c.c.ID
		}
	}

	for _, el := range w.elements() {
		switch {
		case el.at < 0 && !el.gone:
			out.added = append(out.added, el.id())
		case el.at >= 0 && el.gone:
			out.removed = append(out.removed, el.id())
		}
	}
	for _, a := range w.about {
		out.inSaved = append(out.inSaved, a.notice.about(diag.Location{Path: w.modelPath, Pointer: pointer[a.el]}, a.el.id()))
	}

	out.records = w.history.kept(acquisitions)
	for i := range out.records {
		out.records[i].Nodes = w.statedBy(out.records[i])
		out.records[i].Attributes = w.attributesBy(out.records[i])
	}
	if w.supplements != nil {
		out.records = append(out.records, w.supplements.record(supplementsID))
	} else {
		out.records = append(out.records, slices.DeleteFunc(slices.Clone(w.records), func(r record) bool { return !r.supplement() })...)
	}

	for _, f := range w.live {
		out.partial = out.partial || !f.acq.Complete || f.acq.Reported > 0 ||
			slices.ContainsFunc(f.coverage, func(c candCoverage) bool { return slices.Contains(unfinished, c.status) })
		if !f.acq.Complete {
			out.found = append(out.found, incompleteAcquisition.at(locationIn(f.path, f.doc, "/acquisition/complete")))
		}
		for i := range f.acq.Reported {
			out.found = append(out.found, reported.at(locationIn(f.path, f.doc, fmt.Sprintf("/diagnostics/%d", i))))
		}
	}

	for _, f := range w.findings {
		row := func(kind string, id local, modelID string) {
			out.ids = append(out.ids, Mapping{Input: f.path, Kind: kind, Candidate: string(id), ID: modelID})
		}
		element := func(kind string, c *candidate, el *element) {
			if el == nil || el.gone {
				row(kind, c.id, "")
			} else {
				row(kind, c.id, el.id())
			}
			for _, ev := range c.evidence {
				row("evidence", ev.id, became[origin{input: f.path, livePosition: f.livePosition, candidate: ev.id}])
			}
		}
		for i := range f.nodes {
			element("node", &f.nodes[i].candidate, w.nodeOf[candidateRef{f, f.nodes[i].id}])
		}
		for i := range f.edges {
			element("edge", &f.edges[i].candidate, w.edgeOf[candidateRef{f, f.edges[i].id}])
		}
		for _, c := range f.coverage {
			row("coverage", c.id, became[origin{input: f.path, livePosition: f.livePosition, candidate: c.id}])
		}
	}
	if s := w.supplements; s != nil {
		for i := range s.statements {
			out.ids = append(out.ids, Mapping{Input: s.path, Kind: "evidence", Candidate: strconv.Itoa(i),
				ID: became[origin{input: s.path, livePosition: len(w.live), statement: i}]})
		}
	}
	return out
}

var unfinished = []model.CoverageStatus{model.CovPartial, model.CovDenied, model.CovUnsupported, model.CovFailed}

// inOrder is the order of the saved model: what the model had keeps its
// order, and what is new follows in the order of its ID.
func inOrder[T any](entries []T, place func(T) (at int, id string, stays bool)) []T {
	var out []T
	for _, e := range entries {
		if _, _, stays := place(e); stays {
			out = append(out, e)
		}
	}
	slices.SortStableFunc(out, func(a, b T) int {
		atA, idA, _ := place(a)
		atB, idB, _ := place(b)
		switch {
		case atA >= 0 && atB >= 0:
			return cmp.Compare(atA, atB)
		case atA >= 0:
			return -1
		case atB >= 0:
			return 1
		}
		return cmp.Compare(idA, idB)
	})
	return out
}
