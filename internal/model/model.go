// Package model holds the configuration model: the single source of truth
// that diagrams and reports are derived from. It never carries renderer types
// or coordinates.
package model

import (
	"errors"
	"math/big"
	"regexp"
	"strings"
	"time"
)

type Model struct {
	// Schema lets an editor bind the file to the schema. It is kept and never
	// read.
	Schema        string     `json:"$schema,omitempty"`
	SchemaVersion string     `json:"schema_version"`
	Target        Target     `json:"target"`
	Nodes         []Node     `json:"nodes"`
	Edges         []Edge     `json:"edges"`
	Coverage      []Coverage `json:"coverage"`
}

type Target struct {
	Repository  string  `json:"repository"`
	Environment string  `json:"environment"`
	Scope       []Scope `json:"scope,omitempty"`
}

// Scope says which part of the real environment something is about. Keys the
// schema does not define are kept.
type Scope map[string]any

// TargetIDs returns the model IDs listed under target_ids.
func (s Scope) TargetIDs() []string {
	ids, _ := s.Targets()
	return ids
}

// Targets returns the model IDs listed under target_ids. listed is false when
// the scope has no target_ids, and so is not limited to named elements.
func (s Scope) Targets() (ids []string, listed bool) {
	items, listed := s["target_ids"].([]any)
	for _, item := range items {
		if id, ok := item.(string); ok && ValidID(id) {
			ids = append(ids, id)
		}
	}
	return ids, listed
}

// Item returns an item of the scope that the schema gives as text, such as
// provider or account_id.
func (s Scope) Item(key string) (value string, written bool) {
	value, written = s[key].(string)
	return value, written
}

// Fields returns the support keys the scope is narrowed to by filter.fields.
func (s Scope) Fields() (keys []string, narrowed bool) {
	filter, _ := s["filter"].(map[string]any)
	items, narrowed := filter["fields"].([]any)
	for _, item := range items {
		if key, ok := item.(string); ok {
			keys = append(keys, key)
		}
	}
	return keys, narrowed
}

// Slot returns the setting the scope is narrowed to by filter.slot.
func (s Scope) Slot() (slot string, narrowed bool) {
	filter, _ := s["filter"].(map[string]any)
	slot, narrowed = filter["slot"].(string)
	return slot, narrowed
}

type NodeKind string

const (
	KindResource NodeKind = "resource"
	KindService  NodeKind = "service"
)

type Node struct {
	ID          string         `json:"id"`
	Kind        NodeKind       `json:"kind"`
	Provider    string         `json:"provider"`
	Service     string         `json:"service"`
	Environment string         `json:"environment,omitempty"`
	Identity    map[string]any `json:"identity,omitempty"`
	Attributes  map[string]any `json:"attributes,omitempty"`
	Evidence    []Evidence     `json:"evidence"`
}

type Relation string

const (
	RelConfiguration  Relation = "configuration"
	RelImplementation Relation = "implementation"
	RelObservation    Relation = "observation"
	RelDependency     Relation = "dependency"
)

type Edge struct {
	ID          string         `json:"id"`
	From        string         `json:"from"`
	To          string         `json:"to"`
	Relation    Relation       `json:"relation"`
	Slot        string         `json:"slot,omitempty"`
	Environment string         `json:"environment,omitempty"`
	Attributes  map[string]any `json:"attributes,omitempty"`
	Evidence    []Evidence     `json:"evidence"`
}

type EvidenceKind string

const (
	EvDeclared EvidenceKind = "declared"
	EvDeployed EvidenceKind = "deployed"
	EvCode     EvidenceKind = "code"
	EvObserved EvidenceKind = "observed"
	EvStated   EvidenceKind = "stated"
	EvHint     EvidenceKind = "hint"
)

type Evidence struct {
	ID            string       `json:"id"`
	Kind          EvidenceKind `json:"kind"`
	Supports      []Support    `json:"supports"`
	Ref           Ref          `json:"ref"`
	CheckedAt     string       `json:"checked_at"`
	ValidTime     string       `json:"valid_time,omitempty"`
	AcquisitionID string       `json:"acquisition_id,omitempty"`
}

// Checked returns checked_at as a time. Parse has seen to its form.
func (ev Evidence) Checked() time.Time { return instant(ev.CheckedAt) }

// When returns the time the evidence speaks of.
func (ev Evidence) When() Period {
	if ev.ValidTime == "" {
		return Period{}
	}
	start, end, isPeriod := strings.Cut(ev.ValidTime, "/")
	if !isPeriod {
		end = start
	}
	return Period{start: instant(start), end: instant(end), dated: true}
}

func instant(text string) time.Time {
	t, _ := time.Parse(time.RFC3339, text)
	return t
}

// Period is the time a piece of evidence speaks of. The zero Period is the
// present: evidence without valid_time speaks of the state the model
// describes, whenever it was checked. A period includes its start and not its
// end, so that the end of an old state and the start of a new one can share a
// moment without the two being read as one time. A period whose start and end
// are the same is that instant.
type Period struct {
	start, end time.Time
	dated      bool
}

// Present reports whether the period is the present.
func (p Period) Present() bool { return !p.dated }

// Comparable reports whether facts of the two periods can be compared: both
// are the present, or both are dated and share a moment. The present is never
// compared with a dated period, since deciding when the present is would make
// the result depend on how checked_at happens to be written.
func (p Period) Comparable(q Period) bool {
	if !p.dated || !q.dated {
		return p.dated == q.dated
	}
	switch {
	case p.instant() && q.instant():
		return p.start.Equal(q.start)
	case p.instant():
		return q.holds(p.start)
	case q.instant():
		return p.holds(q.start)
	}
	return p.start.Before(q.end) && q.start.Before(p.end)
}

func (p Period) instant() bool { return p.start.Equal(p.end) }

func (p Period) holds(t time.Time) bool { return !t.Before(p.start) && t.Before(p.end) }

const (
	KeyExistence           = "existence"
	KeyRole                = "role"
	KeyConnectionConfigure = "connection.configured"
	KeyConnectionTarget    = "connection.target"
	KeyCallImplemented     = "call.implemented"
	KeyTrafficObserved     = "traffic.observed"
	KeyRegion              = "region"
	KeyScopeOwner          = "scope.owner"
	KeyScopeNetwork        = "scope.network"
	KeyExposure            = "exposure"
	KeyLifecycleRequires   = "lifecycle.requires"
)

type Support struct {
	Key   string `json:"key"`
	Value any    `json:"value"`
}

type Ref struct {
	Type      string `json:"type"`
	Path      string `json:"path"`
	StartLine Line   `json:"start_line,omitempty"`
	EndLine   Line   `json:"end_line,omitempty"`
}

// Line is a line number. The schema takes any JSON number whose value is a
// whole number, such as 1e3, which encoding/json refuses for an int.
type Line int

func (l *Line) UnmarshalJSON(raw []byte) error {
	r, ok := new(big.Rat).SetString(string(raw))
	if !ok || !r.IsInt() || !r.Num().IsInt64() {
		return errors.New("a line number is a whole number")
	}
	*l = Line(r.Num().Int64())
	return nil
}

type CoverageStatus string

const (
	CovComplete     CoverageStatus = "complete"
	CovPartial      CoverageStatus = "partial"
	CovDenied       CoverageStatus = "denied"
	CovUnsupported  CoverageStatus = "unsupported"
	CovNotAttempted CoverageStatus = "not_attempted"
	CovFailed       CoverageStatus = "failed"
)

type Coverage struct {
	ID            string         `json:"id"`
	Scope         Scope          `json:"scope"`
	Status        CoverageStatus `json:"status"`
	Reason        string         `json:"reason,omitempty"`
	CheckedAt     string         `json:"checked_at"`
	AcquisitionID string         `json:"acquisition_id,omitempty"`
}

// Checked returns checked_at as a time. Parse has seen to its form.
func (c Coverage) Checked() time.Time { return instant(c.CheckedAt) }

// Unfinished reports whether the acquisition stopped short of its scope.
// not_attempted read nothing, so it says nothing about what was read before.
func (c Coverage) Unfinished() bool {
	switch c.Status {
	case CovPartial, CovDenied, CovUnsupported, CovFailed:
		return true
	}
	return false
}

// idPattern is the ID form of the schema, $defs.id.pattern. IDs are published
// as SVG class names and diagram keys, so the form is deliberately narrow.
var idPattern = regexp.MustCompile(schemaIDPattern())

// ValidID reports whether id may name a model element. The schema of the view
// config holds its own IDs to the same pattern, since they share the
// diagram's namespace.
func ValidID(id string) bool { return idPattern.MatchString(id) }
