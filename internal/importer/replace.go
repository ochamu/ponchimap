package importer

import (
	"maps"
	"slices"

	"github.com/ochamu/ponchimap/internal/model"
)

// replace applies M1 and M5. Evidence written by hand carries none of the IDs
// that it takes out, and stays (M4).
func (w *work) replace() {
	incoming := map[string]bool{}
	for _, f := range w.live {
		incoming[f.acq.ID] = true
	}
	for _, el := range w.elements() {
		for _, it := range el.items {
			id := it.ev.AcquisitionID
			if id != "" && (incoming[id] || w.history.superseded(id) || (w.supplements != nil && w.fromSupplements[id])) {
				w.takeOut(el, it)
			}
		}
	}
	for _, c := range w.coverage {
		c.out = incoming[c.c.AcquisitionID]
	}
}

func (w *work) takeOut(el *element, it *item) { it.out, el.lost = true, true }

// spot is where M2 looks for a restatement: the evidence of one kind and one
// valid_time on one element. The edges of one slot are not one spot: a read
// that did not finish may have stopped before the other targets of the slot.
type spot struct {
	el        *element
	kind      model.EvidenceKind
	validTime string
}

// restate applies M2. All items are judged as they stand before any is taken
// out. An item that an earlier run took out was restated by items that still
// state its keys, so the runs in which the acquisitions came do not change
// what stays.
func (w *work) restate() {
	type statedBy struct {
		spot
		acquisition string
	}
	keys := map[statedBy]map[string]bool{}
	at := map[spot][]string{}
	for _, el := range w.elements() {
		for _, it := range el.items {
			if it.out {
				continue
			}
			by := statedBy{spot{el, it.ev.Kind, it.ev.ValidTime}, it.ev.AcquisitionID}
			if keys[by] == nil {
				keys[by] = map[string]bool{}
				at[by.spot] = append(at[by.spot], by.acquisition)
			}
			for _, s := range it.ev.Supports {
				keys[by][s.Key] = true
			}
		}
	}
	var restated []func()
	for _, el := range w.elements() {
		for _, it := range el.items {
			if it.out {
				continue
			}
			here := spot{el, it.ev.Kind, it.ev.ValidTime}
			stated := map[string]bool{}
			for _, newer := range at[here] {
				if w.history.before(it.ev.AcquisitionID, newer) {
					maps.Copy(stated, keys[statedBy{here, newer}])
				}
			}
			if !slices.ContainsFunc(it.ev.Supports, func(s model.Support) bool { return !stated[s.Key] }) {
				restated = append(restated, func() { w.takeOut(el, it) })
			}
		}
	}
	for _, takeOut := range restated {
		takeOut()
	}
}

// remove applies R1 to R3.
func (w *work) remove() {
	stands := func(it *item) bool { return !it.out && !w.fromSupplements[it.ev.AcquisitionID] }
	var doomed []*element
	for _, el := range w.elements() {
		if el.lost && !slices.ContainsFunc(el.items, stands) {
			el.gone = true
			doomed = append(doomed, el)
		}
	}
	// Keeping an element can name another one, through the ends of a kept
	// edge or the connection.target of evidence that is kept, so the naming
	// is worked out again until nothing changes.
	for changed := true; changed; {
		changed = false
		names := map[string]bool{}
		for _, scope := range w.target.Scope {
			for _, id := range scope.TargetIDs() {
				names[id] = true
			}
		}
		for _, el := range w.elements() {
			if el.gone {
				continue
			}
			if el.edge != nil {
				names[el.edge.From], names[el.edge.To] = true, true
			}
			for _, it := range el.items {
				if stands(it) {
					for _, id := range connectionTargets(it.ev) {
						names[id] = true
					}
				}
			}
		}
		for _, el := range doomed {
			if el.gone && names[el.id()] {
				el.gone, el.heldByR3, changed = false, true, true
				for _, it := range el.items {
					it.out = it.out && w.fromSupplements[it.ev.AcquisitionID]
				}
			}
		}
	}
	for _, el := range doomed {
		switch {
		case el.heldByR3:
			w.note(keptSuperseded, el)
		case el.at >= 0 && el.node != nil:
			w.found = append(w.found, removed.about(w.locationInReadModel("nodes", el.at), el.id()))
		case el.at >= 0:
			w.found = append(w.found, removed.about(w.locationInReadModel("edges", el.at), el.id()))
		}
	}
	// A statement about a connection to a node that left names nothing any
	// more. The supplements file still holds it, and the next run that is
	// given the file reports it.
	for _, el := range w.elements() {
		for _, it := range el.items {
			if !it.out && w.fromSupplements[it.ev.AcquisitionID] && slices.ContainsFunc(connectionTargets(it.ev), w.missing) {
				w.takeOut(el, it)
			}
		}
	}
}

func (w *work) missing(id string) bool {
	el := w.byID[id]
	return el == nil || el.gone
}

func connectionTargets(ev model.Evidence) []string {
	var ids []string
	for _, s := range ev.Supports {
		if id, ok := s.Value.(string); ok && s.Key == model.KeyConnectionTarget {
			ids = append(ids, id)
		}
	}
	return ids
}

func (w *work) settleCoverage() {
	for _, f := range w.live {
		for _, c := range f.coverage {
			scope := maps.Clone(c.scope)
			if c.hasTargetIDs {
				ids := []any{}
				for _, target := range c.targets {
					el := w.nodeOf[candidateRef{f, target}]
					if el == nil {
						el = w.edgeOf[candidateRef{f, target}]
					}
					// Two candidates can be one element, which the list names once.
					if id := any(el.id()); !slices.Contains(ids, id) {
						ids = append(ids, id)
					}
				}
				scope["target_ids"] = ids
			}
			w.coverage = append(w.coverage, &cover{at: -1,
				c:    model.Coverage{Scope: scope, Status: c.status, Reason: c.reason, CheckedAt: c.checkedAt, AcquisitionID: f.acq.ID},
				from: &origin{input: f.path, livePosition: f.livePosition, candidate: c.id}})
		}
	}

	stated := map[string]bool{}
	for _, el := range w.elements() {
		for _, it := range el.items {
			if !el.gone && !it.out {
				stated[it.ev.AcquisitionID] = true
			}
		}
	}
	for _, c := range w.coverage {
		// M3: an older acquisition keeps its complete records while evidence of
		// it stays in the model, which the view derivation needs to count that
		// evidence.
		if id := c.c.AcquisitionID; w.history.source(id) != "" {
			c.out = c.out || !(w.history.newest(id) || (c.c.Status == model.CovComplete && stated[id]))
		}
		if c.out {
			continue
		}
		ids, hasTargetIDs := c.c.Scope["target_ids"].([]any)
		stay := slices.DeleteFunc(slices.Clone(ids), func(id any) bool {
			text, _ := id.(string)
			el := w.byID[text]
			return el != nil && el.gone
		})
		if !hasTargetIDs || len(stay) == len(ids) {
			continue
		}
		// R4: a record that is left with no target goes, since a record
		// without target_ids would cover more than it did.
		if c.out = len(stay) == 0; !c.out {
			c.c.Scope = maps.Clone(c.c.Scope)
			c.c.Scope["target_ids"] = stay
		}
	}
}
