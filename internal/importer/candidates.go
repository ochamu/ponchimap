package importer

import (
	"maps"
	"slices"

	"github.com/ochamu/ponchimap/internal/model"
)

func (w *work) matchNodes(left map[int]bool) {
	existing := make([]known, len(w.nodes))
	at := map[string]int{}
	for i, el := range w.nodes {
		n := el.node
		existing[i].own = describe(string(n.Kind), n.Provider, n.Service, w.environment(n.Environment), n.Identity)
		at[n.ID] = i
	}
	// What the records say of the nodes. An acquisition that stays in the
	// model and is not given again still describes its nodes, as it would if
	// its file were given. One that this run reads again or replaces says
	// where its descriptions went, and the candidates of this run say the
	// rest.
	given := map[string]bool{}
	for _, f := range w.findings {
		given[f.acq.ID] = true
	}
	before := map[key]int{}
	accounted := map[int]map[string]bool{}
	for _, r := range w.records {
		for _, s := range r.Nodes {
			i, inModel := at[s.Node]
			if !inModel {
				continue
			}
			d := s.description()
			if k, has := d.key(); has {
				if _, told := before[k]; !told {
					before[k] = i
				}
			}
			if !given[r.ID] && w.history.live(r.ID) {
				existing[i].stated = append(existing[i].stated, d)
				if accounted[i] == nil {
					accounted[i] = map[string]bool{}
				}
				accounted[i][r.ID] = true
			}
		}
	}
	// Evidence that no record accounts for was written by hand, or came with
	// a model whose records were lost: then the identity is all that states
	// the node.
	for i, el := range w.nodes {
		existing[i].byItself = len(el.items) == 0 || slices.ContainsFunc(el.items, func(it *item) bool {
			id := it.ev.AcquisitionID
			return !it.out && !w.fromSupplements[id] && !accounted[i][id]
		})
	}
	type described struct {
		source
		node        *candNode
		environment string
	}
	var candidates []described
	var descriptions []description
	for _, f := range w.live {
		for i := range f.nodes {
			n := &f.nodes[i]
			env := w.environment(n.environment, f.environment)
			candidates = append(candidates, described{source{f, &n.candidate}, n, env})
			descriptions = append(descriptions, describe(n.kind, n.provider, n.service, env, n.identity))
		}
	}
	sets, ambiguous := match(existing, descriptions, left, before)
	for _, i := range ambiguous {
		w.found = append(w.found, ambiguousIdentity.at(candidates[i].at()))
	}

	type fresh struct {
		el  *element
		key key
	}
	var made []fresh
	// The notice names the node, and a new node has no ID yet.
	var disagree []source
	for _, set := range sets {
		var added *element
		if len(set.existing) > 0 {
			for _, later := range set.existing[1:] {
				w.found = append(w.found, duplicateIdentity.about(w.locationInReadModel("nodes", later), w.nodes[later].id()))
			}
		} else {
			first := candidates[set.candidates[0]].node
			added = &element{node: &model.Node{Kind: model.NodeKind(first.kind), Provider: first.provider, Service: first.service}, at: -1}
			var environments []string
			for _, i := range set.candidates {
				if env := candidates[i].environment; !slices.Contains(environments, env) {
					environments = append(environments, env)
				}
			}
			if len(environments) == 1 {
				added.node.Environment = environments[0]
			}
			made = append(made, fresh{added, set.key})
		}
		for _, i := range set.candidates {
			c, el := candidates[i], added
			if el == nil {
				el = w.nodes[set.modelNodeOf[i]]
			}
			w.nodeOf[candidateRef{c.in, c.c.id}] = el
			w.taken = append(w.taken, taken{c.in.acq.ID, descriptions[i], el})
			el.sources = append(el.sources, c.source)
			if !addIdentity(el.node, c.node.identity) {
				disagree = append(disagree, c.source)
			}
		}
	}
	// The IDs are given in the order of the keys, so that the order of the
	// files cannot decide which of two new nodes gets a number added.
	slices.SortFunc(made, func(a, b fresh) int { return compareKeys(a.key, b.key) })
	for _, m := range made {
		m.el.node.ID = w.ids.issue(nodeID, m.key.fields()...)
		w.nodes = append(w.nodes, m.el)
		w.byID[m.el.id()] = m.el
	}
	for _, src := range disagree {
		w.found = append(w.found, identityConflict.about(src.at(), w.nodeOf[candidateRef{src.in, src.c.id}].id()))
	}
}

func addIdentity(n *model.Node, identity map[string]any) bool {
	agrees := true
	for _, key := range slices.Sorted(maps.Keys(identity)) {
		have, has := n.Identity[key]
		switch {
		case !has:
			if n.Identity == nil {
				n.Identity = map[string]any{}
			}
			n.Identity[key] = identity[key]
		case !sameIdentity(have, identity[key]):
			agrees = false
		}
	}
	return agrees
}

func sameIdentity(a, b any) bool {
	if ta, tb := identityText(a), identityText(b); ta != "" && tb != "" {
		return ta == tb
	}
	return canonical(a) == canonical(b)
}

// edgeKey is what makes two edges the same.
type edgeKey struct {
	from, to    string
	relation    model.Relation
	environment string
	slot        string
}

func (w *work) edgeKey(e *model.Edge) edgeKey {
	return edgeKey{e.From, e.To, e.Relation, w.environment(e.Environment), e.Slot}
}

func (w *work) matchEdges() {
	byKey := map[edgeKey]*element{}
	for _, el := range w.edges {
		if k := w.edgeKey(el.edge); byKey[k] == nil {
			byKey[k] = el
		}
	}
	var made []*element
	for _, f := range w.live {
		for i := range f.edges {
			e := &f.edges[i]
			k := edgeKey{w.nodeOf[candidateRef{f, e.from}].id(), w.nodeOf[candidateRef{f, e.to}].id(), e.relation,
				w.environment(e.environment, f.environment), e.slot}
			el := byKey[k]
			if el == nil {
				el = &element{edge: &model.Edge{From: k.from, To: k.to, Relation: k.relation, Slot: k.slot}, at: -1}
				if k.environment != w.target.Environment {
					el.edge.Environment = k.environment
				}
				byKey[k] = el
				made = append(made, el)
			}
			w.edgeOf[candidateRef{f, e.id}] = el
			el.sources = append(el.sources, source{f, &e.candidate})
		}
	}
	fields := func(el *element) []string {
		k := w.edgeKey(el.edge)
		return []string{k.from, k.to, string(k.relation), k.environment, k.slot}
	}
	slices.SortFunc(made, func(a, b *element) int { return slices.Compare(fields(a), fields(b)) })
	for _, el := range made {
		el.edge.ID = w.ids.issue(edgeID, fields(el)...)
		w.edges = append(w.edges, el)
		w.byID[el.id()] = el
	}
}

func (w *work) addEvidence() {
	for _, el := range w.elements() {
		for _, src := range el.sources {
			for _, c := range src.c.evidence {
				ev := model.Evidence{Kind: c.kind, Ref: c.ref, CheckedAt: c.checkedAt, ValidTime: c.validTime, AcquisitionID: src.in.acq.ID}
				for _, s := range c.supports {
					if s.target != "" {
						s.value = w.nodeOf[candidateRef{src.in, s.target}].id()
					}
					ev.Supports = append(ev.Supports, model.Support{Key: s.key, Value: s.value})
				}
				el.items = append(el.items, &item{ev: ev, at: -1,
					from: &origin{input: src.in.path, livePosition: src.in.livePosition, candidate: c.id}})
				el.gained = true
			}
		}
	}
}

// R5: a candidate without evidence may only name an element that the model
// has.
func (w *work) unsupported() {
	for _, el := range w.elements() {
		carries := slices.ContainsFunc(el.sources, func(s source) bool { return len(s.c.evidence) > 0 })
		if el.at < 0 && !carries {
			w.found = append(w.found, noEvidence.at(el.sources[0].at()))
		}
	}
}
