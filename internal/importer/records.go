package importer

import (
	"bytes"
	"cmp"
	"encoding/json"
	"errors"
	"maps"
	"slices"
	"time"

	"github.com/ochamu/ponchimap/internal/model"
)

const (
	supplementRoute  = "supplement"
	supplementSource = "supplements"
)

type record struct {
	ID        string         `json:"id"`
	Source    string         `json:"source"`
	Route     string         `json:"route"`
	Operation string         `json:"operation"`
	Target    map[string]any `json:"target,omitempty"`
	CheckedAt string         `json:"checked_at,omitempty"`
	Complete  bool           `json:"complete"`
	Input     string         `json:"input"`
	Digest    string         `json:"digest"`
	Reported  int            `json:"reported"`
	Nodes     []stated       `json:"nodes,omitempty"`
	// Attributes are what the candidates of the acquisition wrote in the
	// attributes of the elements that they were taken as, by element ID: the
	// model does not say which source wrote a value, and only a newer read
	// of that source may replace it (A2).
	Attributes map[string]map[string]any `json:"attributes,omitempty"`
}

// stated is a node candidate of an acquisition as the matching read it, and
// the node of the model that it was taken as. The model keeps one identity
// and one environment of a node, so the next run could not tell from the
// model what each acquisition said of it.
type stated struct {
	Node        string `json:"node"`
	Kind        string `json:"kind"`
	Provider    string `json:"provider,omitempty"`
	Service     string `json:"service,omitempty"`
	Environment string `json:"environment"`
	NativeID    string `json:"native_id,omitempty"`
	LogicalName string `json:"logical_name,omitempty"`
	AccountID   string `json:"account_id,omitempty"`
	ProjectID   string `json:"project_id,omitempty"`
}

func (s stated) description() description {
	return description{kind: s.Kind, provider: s.Provider, service: s.Service, environment: s.Environment,
		native: s.NativeID, name: s.LogicalName, account: s.AccountID, project: s.ProjectID}
}

func (s stated) fields() []string {
	return []string{s.Node, s.Kind, s.Provider, s.Service, s.Environment, s.NativeID, s.LogicalName, s.AccountID, s.ProjectID}
}

// statedBy is what the record of an acquisition keeps of its node candidates:
// those of this run when the acquisition was given, and otherwise the ones it
// had, for as long as the acquisition and the node stay in the model.
func (w *work) statedBy(r record) []stated {
	var out []stated
	switch {
	case slices.ContainsFunc(w.live, func(f *findings) bool { return f.acq.ID == r.ID }):
		// A candidate of the last pass has a node that stays: a node that
		// leaves took no part in that pass.
		for _, t := range w.taken {
			if d := t.says; t.acquisition == r.ID {
				out = append(out, stated{Node: t.node.id(), Kind: d.kind, Provider: d.provider, Service: d.service,
					Environment: d.environment, NativeID: d.native, LogicalName: d.name, AccountID: d.account, ProjectID: d.project})
			}
		}
	case w.history.live(r.ID):
		for _, s := range r.Nodes {
			if !w.missing(s.Node) {
				out = append(out, s)
			}
		}
	}
	slices.SortFunc(out, func(a, b stated) int { return slices.Compare(a.fields(), b.fields()) })
	return slices.Compact(out)
}

func (r record) supplement() bool { return r.Route == supplementRoute }

func older(a, b record) int {
	return cmp.Or(moment(a.CheckedAt).Compare(moment(b.CheckedAt)), cmp.Compare(a.ID, b.ID))
}

func moment(text string) time.Time {
	t, _ := time.Parse(time.RFC3339, text)
	return t
}

type recordsFile struct {
	SchemaVersion string   `json:"schema_version"`
	ModelDigest   string   `json:"model,omitempty"`
	Acquisitions  []record `json:"acquisitions"`
}

const recordsVersion = "0.1"

var errNotRecords = errors.New("not the form of the acquisition records")

func parseRecords(text []byte) (*recordsFile, error) {
	dec := json.NewDecoder(bytes.NewReader(text))
	dec.UseNumber()
	dec.DisallowUnknownFields()
	var file recordsFile
	if err := dec.Decode(&file); err != nil || dec.More() || file.SchemaVersion != recordsVersion {
		return nil, errNotRecords
	}
	seen := map[string]bool{}
	for _, r := range file.Acquisitions {
		_, err := time.Parse(time.RFC3339, r.CheckedAt)
		dated := err == nil || (r.supplement() && r.CheckedAt == "")
		if !model.ValidID(r.ID) || seen[r.ID] || r.Source == "" || r.Route == "" || !dated ||
			slices.ContainsFunc(r.Nodes, func(s stated) bool { return !model.ValidID(s.Node) }) ||
			slices.ContainsFunc(slices.Collect(maps.Keys(r.Attributes)), func(id string) bool { return !model.ValidID(id) }) {
			return nil, errNotRecords
		}
		seen[r.ID] = true
	}
	return &file, nil
}

func (f recordsFile) text() []byte {
	if f.Acquisitions == nil {
		f.Acquisitions = []record{}
	}
	slices.SortFunc(f.Acquisitions, func(a, b record) int { return cmp.Compare(a.ID, b.ID) })
	var text bytes.Buffer
	enc := json.NewEncoder(&text)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(f); err != nil {
		panic("importer: the acquisition records hold a value that cannot be written as JSON")
	}
	return text.Bytes()
}

type history struct {
	timelines map[string][]record
	place     map[string]position
	// Nothing of a stale acquisition is taken in, and the history knows
	// nothing else about it: what carries such an ID in the model was written
	// by hand.
	stale map[string]bool
}

type position struct {
	source string
	index  int
	cut    int
}

func newestComplete(line []record) int {
	for i := len(line) - 1; i >= 0; i-- {
		if line[i].Complete {
			return i
		}
	}
	return -1
}

// An incoming acquisition that the records know is the same read sent again.
func newHistory(known, incoming []record) history {
	h := history{timelines: map[string][]record{}, place: map[string]position{}, stale: map[string]bool{}}
	recorded := map[string]bool{}
	for _, r := range known {
		recorded[r.ID] = true
	}
	lines, seen := map[string][]record{}, map[string]bool{}
	for _, r := range slices.Concat(incoming, known) {
		if !seen[r.ID] && !r.supplement() {
			seen[r.ID] = true
			lines[r.Source] = append(lines[r.Source], r)
		}
	}
	for source, all := range lines {
		slices.SortFunc(all, older)
		var line []record
		for i, r := range all {
			if i < newestComplete(all) && !recorded[r.ID] {
				h.stale[r.ID] = true
				continue
			}
			line = append(line, r)
		}
		h.timelines[source] = line
		for i, r := range line {
			h.place[r.ID] = position{source: source, index: i, cut: newestComplete(line)}
		}
	}
	return h
}

func (h history) source(id string) string { return h.place[id].source }

// superseded reports whether a newer acquisition of the same source is
// complete, so that nothing of this one stays in the model (M1).
func (h history) superseded(id string) bool {
	p, known := h.place[id]
	return known && p.index < p.cut
}

func (h history) live(id string) bool { return !h.stale[id] && !h.superseded(id) }

func (h history) newest(id string) bool {
	p, known := h.place[id]
	return known && p.index == len(h.timelines[p.source])-1
}

func (h history) before(a, b string) bool {
	pa, pb := h.place[a], h.place[b]
	return pa.source != "" && pa.source == pb.source && pa.index < pb.index
}

// kept are the acquisitions that the records keep: those that the model
// names, and of each source the newest one and the newest complete one.
func (h history) kept(named map[string]bool) []record {
	var out []record
	for _, line := range h.timelines {
		for _, r := range line {
			if p := h.place[r.ID]; named[r.ID] || p.index == p.cut || h.newest(r.ID) {
				out = append(out, r)
			}
		}
	}
	return out
}
