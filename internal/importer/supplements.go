package importer

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"sync"

	"github.com/ochamu/ponchimap/internal/diag"
	"github.com/ochamu/ponchimap/internal/input"
	"github.com/ochamu/ponchimap/internal/model"
	"github.com/ochamu/ponchimap/schema"
)

var supplementsSchema = sync.OnceValue(func() *input.Schema { return input.MustCompile(schema.Supplements, modelSchema) })

type supplements struct {
	path       string
	doc        *input.Document
	statements []statement
}

type statement struct {
	Target      string          `json:"target_id"`
	Supports    []model.Support `json:"supports"`
	CheckedAt   string          `json:"checked_at"`
	ValidTime   string          `json:"valid_time"`
	first, last int
}

// readSupplements reads the supplements file at path through the stages of
// Reading up to the schema.
func readSupplements(path string) (*supplements, diag.List) {
	raw, list := input.ReadFile(path)
	if list != nil {
		return nil, list
	}
	doc, list := input.DecodeYAML(raw)
	var s *supplements
	if doc != nil {
		readable := !list.HasErrors()
		checked := supplementsSchema().Check(doc)
		list = append(list, checked...)
		if readable && !checked.HasErrors() {
			var err error
			if s, err = decodeSupplements(path, doc); err != nil {
				list = append(list, unreadableInput.at(diag.Location{}))
			}
		}
	}
	for i := range list {
		list[i].Location.Path = path
	}
	if list.HasErrors() {
		return nil, list
	}
	return s, list
}

// The value goes through JSON so that the supplements hold exactly what was
// checked.
func decodeSupplements(path string, doc *input.Document) (*supplements, error) {
	raw, err := json.Marshal(doc.Value)
	if err != nil {
		return nil, err
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var wire struct {
		Statements []statement `json:"statements"`
	}
	if err := dec.Decode(&wire); err != nil {
		return nil, err
	}
	written, _ := doc.Value.(map[string]any)["statements"].([]any)
	for i := range wire.Statements {
		at := fmt.Sprintf("/statements/%d", i)
		st := &wire.Statements[i]
		st.first = doc.Locate(at, false).Line
		st.last = lastLine(doc, at, written[i])
	}
	return &supplements{path: path, doc: doc, statements: wire.Statements}, nil
}

func lastLine(doc *input.Document, pointer string, v any) int {
	last := doc.Locate(pointer, false).Line
	switch x := v.(type) {
	case map[string]any:
		for key, inner := range x {
			at := pointer + "/" + strings.NewReplacer("~", "~0", "/", "~1").Replace(key)
			last = max(last, doc.Locate(at, true).Line, lastLine(doc, at, inner))
		}
	case []any:
		for i, inner := range x {
			last = max(last, lastLine(doc, pointer+"/"+strconv.Itoa(i), inner))
		}
	}
	return last
}

func (s *supplements) record(id string) record {
	r := record{ID: id, Source: supplementSource, Route: supplementRoute, Operation: "read the supplements",
		Complete: true, Input: s.path, Digest: digest(s.doc.Value)}
	for _, st := range s.statements {
		if r.CheckedAt == "" || moment(st.CheckedAt).After(moment(r.CheckedAt)) {
			r.CheckedAt = st.CheckedAt
		}
	}
	return r
}

// The statements go on the model as it stands after the elements that lost
// their evidence left it (M5).
func (w *work) applySupplements() string {
	s := w.supplements
	if s == nil {
		return ""
	}
	id := newID(supplementsID, canonical(s.doc.Value))
	// The same statements keep the acquisition ID that they were recorded
	// under.
	for _, r := range w.records {
		if r.supplement() && r.Digest == digest(s.doc.Value) {
			id = r.ID
		}
	}
	w.fromSupplements[id] = true
	for i, st := range s.statements {
		at := fmt.Sprintf("/statements/%d", i)
		el, names := w.byID[st.Target], true
		if w.missing(st.Target) {
			w.found = append(w.found, unknownTarget.about(locationIn(s.path, s.doc, at+"/target_id"), st.Target))
			names = false
		}
		for j, sup := range st.Supports {
			target, isText := sup.Value.(string)
			if sup.Key != model.KeyConnectionTarget || !isText {
				continue
			}
			if node := w.byID[target]; w.missing(target) || node.node == nil {
				w.found = append(w.found, unknownTarget.about(locationIn(s.path, s.doc, fmt.Sprintf("%s/supports/%d/value", at, j)), target))
				names = false
			}
		}
		if !names {
			continue
		}
		el.items = append(el.items, &item{at: -1, from: &origin{input: s.path, livePosition: len(w.live), statement: i},
			ev: model.Evidence{Kind: model.EvStated, Supports: st.Supports, CheckedAt: st.CheckedAt, ValidTime: st.ValidTime, AcquisitionID: id,
				Ref: model.Ref{Type: "document", Path: s.path, StartLine: model.Line(st.first), EndLine: model.Line(st.last)}}})
		el.gained = true
	}
	return id
}
