package view

import (
	"encoding/json"
	"slices"

	"github.com/ochamu/ponchimap/internal/model"
)

// fact is one thing evidence can say: a key, and the value that says it.
// Evidence that gives a known key another value (call.implemented: false)
// denies the fact and backs nothing.
type fact struct {
	key string
	// holds reports whether value states the fact. to is the node an edge
	// points at, and is empty for a node's own facts.
	holds func(value any, to string) bool
}

func isTrue(value any, _ string) bool    { return value == true }
func isTarget(value any, to string) bool { return value == to }

var existence = fact{model.KeyExistence, isTrue}

// nodeKey is a fact that earns a node its solid frame, with the evidence
// kinds that back each kind of node with it: a declaration or the deployed
// entity for a managed resource, a concrete use in code or an observation for
// a general service. A key backs no node kind it is not listed for, so a
// role, a region, or an owner alone never draws a solid frame.
type nodeKey struct {
	fact
	backs map[model.NodeKind][]model.EvidenceKind
}

var nodeKeys = []nodeKey{
	{existence, map[model.NodeKind][]model.EvidenceKind{model.KindResource: {model.EvDeclared, model.EvDeployed}}},
	{fact{model.KeyCallImplemented, isTrue}, map[model.NodeKind][]model.EvidenceKind{model.KindService: {model.EvCode}}},
	{fact{model.KeyTrafficObserved, isTrue}, map[model.NodeKind][]model.EvidenceKind{model.KindService: {model.EvObserved}}},
}

var nodeFacts = func() []fact {
	var facts []fact
	for _, k := range nodeKeys {
		facts = append(facts, k.fact)
	}
	return facts
}()

// relationRule says what backs each relation: which evidence kinds, stating
// which facts. A relation with needsDeployed is checked against the real
// environment, so that a declaration of it alone leaves it unverified. note
// is said on every solid line: an observation always says so, so that it is
// never read as an implementation.
type relationRule struct {
	kinds         []model.EvidenceKind
	facts         []fact
	needsDeployed bool
	note          string
}

var relationRules = map[model.Relation]relationRule{
	model.RelConfiguration: {
		kinds:         []model.EvidenceKind{model.EvDeclared, model.EvDeployed},
		facts:         []fact{{model.KeyConnectionConfigure, isTrue}, {model.KeyConnectionTarget, isTarget}},
		needsDeployed: true,
	},
	model.RelImplementation: {
		kinds: []model.EvidenceKind{model.EvCode},
		facts: []fact{{model.KeyCallImplemented, isTrue}},
	},
	model.RelObservation: {
		kinds: []model.EvidenceKind{model.EvObserved},
		facts: []fact{{model.KeyTrafficObserved, isTrue}},
		note:  "観測",
	},
	model.RelDependency: {
		kinds: []model.EvidenceKind{model.EvDeclared, model.EvDeployed, model.EvCode},
		facts: []fact{{model.KeyLifecycleRequires, isTrue}},
	},
}

var edgeFacts = func() []fact {
	var facts []fact
	for _, r := range relationRules {
		facts = append(facts, r.facts...)
	}
	return facts
}()

// directKinds back a fact by themselves. Stated and hint evidence only ever
// justify a dashed frame or line.
var directKinds = []model.EvidenceKind{model.EvDeclared, model.EvDeployed, model.EvCode, model.EvObserved}

func kindsOf(evs []model.Evidence) map[model.EvidenceKind]bool {
	k := map[model.EvidenceKind]bool{}
	for _, ev := range evs {
		k[ev.Kind] = true
	}
	return k
}

// said reports whether s is about one of facts, and whether it states it.
func said(s model.Support, facts []fact, to string) (known, holds bool) {
	for _, f := range facts {
		if f.key == s.Key {
			known = true
			holds = holds || f.holds(s.Value, to)
		}
	}
	return known, holds
}

// backing returns the evidence of the given kinds that states one of facts.
func backing(evs []model.Evidence, kinds []model.EvidenceKind, facts []fact, to string) []model.Evidence {
	var out []model.Evidence
	for _, ev := range evs {
		if !slices.Contains(kinds, ev.Kind) {
			continue
		}
		for _, s := range ev.Supports {
			if _, holds := said(s, facts, to); holds {
				out = append(out, ev)
				break
			}
		}
	}
	return out
}

// speakingFor drops the evidence that only denies facts. What is left is the
// evidence an element can be drawn from, however weak; when nothing is left,
// the element has no more claim to the figure than one without evidence.
func speakingFor(evs []model.Evidence, facts []fact, to string) []model.Evidence {
	var out []model.Evidence
	for _, ev := range evs {
		denials := 0
		for _, s := range ev.Supports {
			if known, holds := said(s, facts, to); known && !holds {
				denials++
			}
		}
		if denials == 0 || denials < len(ev.Supports) {
			out = append(out, ev)
		}
	}
	return out
}

func canonical(v any) string {
	raw, _ := json.Marshal(v)
	return string(raw)
}

func evidenceIDs(evs []model.Evidence) []string {
	var ids []string
	for _, ev := range evs {
		ids = append(ids, ev.ID)
	}
	return ids
}

// frameOf draws a node solid when direct evidence states a fact that backs
// drawing this kind of node.
func frameOf(n *model.Node) Frame {
	for _, k := range nodeKeys {
		if len(backing(n.Evidence, k.backs[n.Kind], []fact{k.fact}, "")) > 0 {
			return FrameSolid
		}
	}
	return FrameDashed
}

// lineOf derives how an edge is drawn. Evidence that backs the relation
// draws a solid line. Anything else, including evidence that only supports
// keys this version does not recognise, draws dashed. statedOnly reports a
// dashed line that a statement speaks for, which is unverified (S11).
func lineOf(e *model.Edge) (line Line, statedOnly bool) {
	rule := relationRules[e.Relation]
	evs := speakingFor(e.Evidence, edgeFacts, e.To)
	switch {
	case len(backing(evs, rule.kinds, rule.facts, e.To)) > 0:
		return LineSolid, false
	case len(backing(evs, directKinds, edgeFacts, e.To)) > 0:
		// A defect (D3): the model is never drawn.
		return LineDashed, false
	}
	return LineDashed, kindsOf(evs)[model.EvStated]
}

// nodeDefects and edgeDefects find what keeps an element out of any figure
// (D1 to D3): drawing it would claim more than its evidence does. No evidence
// is at fault in D1: what is missing is evidence that speaks for the element.
func nodeDefects(n *model.Node) []Defect {
	if len(speakingFor(n.Evidence, nodeFacts, "")) == 0 {
		return []Defect{{Code: DefectNoEvidence, ID: n.ID}}
	}
	return nil
}

func edgeDefects(e *model.Edge) []Defect {
	var out []Defect
	rule := relationRules[e.Relation]
	var contradictory []string
	for _, ev := range e.Evidence {
		stated, denied := false, false
		for _, sup := range ev.Supports {
			if known, holds := said(sup, rule.facts, e.To); known {
				stated, denied = stated || holds, denied || !holds
			}
		}
		if stated && denied {
			contradictory = append(contradictory, ev.ID)
		}
	}
	if len(contradictory) > 0 {
		out = append(out, Defect{Code: DefectContradictoryEvidence, ID: e.ID, Evidence: contradictory})
	}
	evs := speakingFor(e.Evidence, edgeFacts, e.To)
	if len(evs) == 0 {
		return append(out, Defect{Code: DefectNoEvidence, ID: e.ID})
	}
	if len(backing(evs, rule.kinds, rule.facts, e.To)) == 0 {
		if other := backing(evs, directKinds, edgeFacts, e.To); len(other) > 0 {
			out = append(out, Defect{Code: DefectRelationEvidence, ID: e.ID, Evidence: evidenceIDs(other)})
		}
	}
	return out
}
