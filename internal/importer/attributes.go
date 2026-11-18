package importer

import (
	"maps"
	"slices"

	"github.com/ochamu/ponchimap/internal/model"
)

// evidenceKeys are the attributes of a node that follow its evidence (A1):
// the support key of the same name states their value.
var evidenceKeys = []string{model.KeyRole, model.KeyRegion}

// An element that the run did not touch keeps what a person wrote.
func (w *work) settleAttributes(el *element) {
	if el.gone || (!el.gained && !el.lost && len(el.sources) == 0) {
		return
	}
	attributes := el.attributes()
	set := func(key string, value any) {
		if *attributes == nil {
			*attributes = map[string]any{}
		}
		(*attributes)[key] = value
	}
	settled := map[string]bool{}
	if el.node != nil {
		standingUndated := func(it *item) bool { return !it.out && it.ev.ValidTime == "" }
		for _, key := range evidenceKeys {
			// A3: what the user states about the role wins over the wording
			// of every other source, which would only ever dispute it.
			if said := w.values(el, key, func(it *item) bool { return standingUndated(it) && w.fromSupplements[it.ev.AcquisitionID] }); key == model.KeyRole && len(said) == 1 {
				set(key, said[0])
				settled[key] = true
				continue
			}
			switch said := w.values(el, key, standingUndated); len(said) {
			case 0:
				// The value goes with the evidence that gave it. A value that
				// a person wrote, which no evidence that left stated, stays.
				// An item that a new one took the ID of did not leave: the
				// same read came again.
				gone := w.values(el, key, func(it *item) bool {
					return it.out && !slices.ContainsFunc(el.items, func(other *item) bool { return !other.out && other.ev.ID == it.ev.ID })
				})
				if now, has := (*attributes)[key]; has && holds(gone, now) {
					delete(*attributes, key)
				}
			case 1:
				set(key, said[0])
				settled[key] = true
			default:
				delete(*attributes, key)
				settled[key] = true
				w.note(disputed[key], el)
			}
		}
	}
	// A2: a candidate writes what the element lacks, and what an older read
	// of its own source wrote. What another source wrote stays.
	for _, src := range el.sources {
		for _, key := range slices.Sorted(maps.Keys(src.c.attributes)) {
			if now, has := (*attributes)[key]; !settled[key] && (!has || w.saidBefore(src.in.acq.ID, el.id(), key, now)) {
				set(key, src.c.attributes[key])
			}
		}
	}
}

// collectSaid gathers what the candidates of each acquisition wrote in the
// attributes of the elements they were taken as: by acquisition, element ID
// and key. An acquisition of this run says it again through its candidates;
// any other says what its record kept, the record of a read that this run
// replaces included.
func (w *work) collectSaid() {
	w.said = map[string]map[string]map[string]any{}
	for _, r := range w.records {
		w.said[r.ID] = r.Attributes
	}
	for _, f := range w.live {
		delete(w.said, f.acq.ID)
	}
	for _, el := range w.elements() {
		if el.gone {
			continue
		}
		for _, src := range el.sources {
			for key, value := range src.c.attributes {
				byElement := w.said[src.in.acq.ID]
				if byElement == nil {
					byElement = map[string]map[string]any{}
					w.said[src.in.acq.ID] = byElement
				}
				if byElement[el.id()] == nil {
					byElement[el.id()] = map[string]any{}
				}
				// Of two candidates of one read that are the same element,
				// the first in Order is what the read wrote, as it is the
				// value that A2 writes.
				if _, has := byElement[el.id()][key]; !has {
					byElement[el.id()][key] = value
				}
			}
		}
	}
}

// saidBefore reports whether an older acquisition of the source of
// acquisition wrote value as the key of the element.
func (w *work) saidBefore(acquisition, id, key string, value any) bool {
	for other, byElement := range w.said {
		if said, has := byElement[id][key]; has && w.history.before(other, acquisition) && canonical(said) == canonical(value) {
			return true
		}
	}
	return false
}

// attributesBy is what the record of an acquisition keeps of the attributes
// that its candidates wrote, for as long as the acquisition and the element
// stay in the model, as statedBy keeps what they said of the nodes.
func (w *work) attributesBy(r record) map[string]map[string]any {
	if !w.history.live(r.ID) {
		return nil
	}
	var out map[string]map[string]any
	for id, attributes := range w.said[r.ID] {
		if !w.missing(id) {
			if out == nil {
				out = map[string]map[string]any{}
			}
			out[id] = attributes
		}
	}
	return out
}

func (w *work) values(el *element, key string, chosen func(*item) bool) []any {
	var out []any
	for _, it := range el.items {
		if !chosen(it) {
			continue
		}
		for _, s := range it.ev.Supports {
			if s.Key == key && !holds(out, s.Value) {
				out = append(out, s.Value)
			}
		}
	}
	return out
}

func holds(values []any, v any) bool {
	return slices.ContainsFunc(values, func(other any) bool { return canonical(other) == canonical(v) })
}
