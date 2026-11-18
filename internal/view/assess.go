package view

import (
	"cmp"
	"fmt"
	"slices"
	"strings"

	"github.com/ochamu/ponchimap/internal/model"
)

// Assess reads what the evidence and the coverage say about every node and
// edge of the model. It takes no view config, so hiding or folding an element
// cannot change its state. The rules follow, and the code of this file and of
// evidence.go names them by their labels.
//
// # Units and records
//
// A unit is one thing checked against the real environment: an item of a
// node (existence, region, scope.owner or exposure), or a connection, which
// is the configuration edges of one source, environment and slot. A record
// is the value that one declared, deployed or stated evidence item gives a
// unit. A declaration is a record that calls for a check: declared
// existence: true of a resource, and a declared target of a configuration
// edge that is the end of that edge. Two records are comparable when neither
// has a valid_time, or both have one and their times overlap. Direct
// evidence is of kind declared, deployed, code or observed. Coverage is
// unfinished when its status is partial, denied, unsupported or failed.
// coveredBy says when coverage applies to a unit, and coverage that applies
// and lists target_ids names the unit.
//
// # Deployed records that do not count
//
//   - F1: A deployed absence, existence: false or connection.configured:
//     false, does not count unless complete coverage of the same non-empty
//     acquisition_id applies to the unit.
//   - F2: A deployed record without a valid_time does not count when
//     unfinished coverage that names the unit was checked after it, unless
//     the two share a non-empty acquisition_id.
//
// A deployed record with a value counts without coverage.
//
// # States
//
//   - S1: A declaration is unverified when no counted record comparable with
//     it is in its unit. In a connection only a record of the declared target
//     settles it, apart from S8.
//   - S2: A comparable counted record of the same value leaves no mark.
//   - S3: A counted existence: false comparable with a declaration of
//     existence is a mismatch, not unverified.
//   - S4: A node of kind service is never unverified. S5 still applies.
//   - S5: Two comparable records of an item of a node with different values
//     are a mismatch. Any two of declared, counted deployed and stated records
//     are compared, apart from two stated ones and two of one evidence item.
//   - S6: Records that are not comparable are not compared.
//   - S7: The records of a connection come from all its edges, so a deployed
//     connection.target on any of them is a record of the unit.
//   - S8: When no counted record shows the declared target and a comparable
//     one shows another, the connection is a mismatch on its source, the
//     declaring edge and the edge of that record, and not unverified.
//   - S9: A connection is checked per target when comparable declarations, or
//     comparable counted records that are not absences, show two or more
//     targets, or when unfinished coverage that names it applies and no
//     complete coverage that names it does. Then S8 does not apply.
//   - S10: A comparable counted connection.configured: false of the declared
//     target is a mismatch on the source and that edge, not unverified. S2
//     goes first: a comparable counted record that shows the target leaves no
//     mark.
//   - S11: An edge is unverified when no direct evidence states a fact of a
//     relation on it and stated evidence speaks for it.
//   - S12: Only configuration edges are checked against the real environment.
//     S11 applies to edges of every relation.
//
// An element can be unverified by one unit and a mismatch by another.
//
// # Defects
//
// A model with a defect is not drawn, even when the view config hides the
// element. Evidence that F1 and F2 set aside counts here.
//
//   - D1: No evidence speaks for an element: it has none, or only evidence
//     that denies its facts.
//   - D2: One evidence item both states and denies the relation of its edge.
//   - D3: No evidence backs the relation of an edge, but direct evidence on it
//     states a fact of a relation.
func Assess(m *model.Model) *Assessment {
	a := &Assessment{}
	for i := range m.Nodes {
		a.Defects = append(a.Defects, nodeDefects(&m.Nodes[i])...)
	}
	for i := range m.Edges {
		a.Defects = append(a.Defects, edgeDefects(&m.Edges[i])...)
	}
	for _, u := range units(m) {
		a.Findings = append(a.Findings, u.assess(u.read(m.Coverage))...)
	}
	for i := range m.Edges {
		if f, ok := statedOnly(&m.Edges[i]); ok {
			a.Findings = append(a.Findings, f)
		}
	}
	return a
}

type Assessment struct {
	Findings []Finding // every unverified and mismatch state, each once
	Defects  []Defect  // what makes the model unfit to draw (D1 to D3)
}

// For returns the findings whose About lists the element.
func (a *Assessment) For(id string) []Finding {
	var out []Finding
	for _, f := range a.Findings {
		if slices.Contains(f.About, id) {
			out = append(out, f)
		}
	}
	return out
}

// Kinds returns the states the element carries, unverified before mismatch.
func (a *Assessment) Kinds(id string) []MarkKind {
	var out []MarkKind
	for _, kind := range []MarkKind{MarkUnverified, MarkMismatch} {
		if slices.ContainsFunc(a.For(id), func(f Finding) bool { return f.Kind() == kind }) {
			out = append(out, kind)
		}
	}
	return out
}

// Err returns the first defect, or nil.
func (a *Assessment) Err() error {
	if len(a.Defects) == 0 {
		return nil
	}
	return a.Defects[0]
}

// FindingCode names why an element is unverified or a mismatch. The codes are
// fixed identifiers, so that lint can use them as the codes of its warnings.
// The part before the dot is the MarkKind.
type FindingCode string

const (
	UnverifiedNotChecked      FindingCode = "unverified.not_checked"
	UnverifiedSuperseded      FindingCode = "unverified.superseded"
	UnverifiedAbsenceUnbacked FindingCode = "unverified.absence_unbacked"
	UnverifiedStatedOnly      FindingCode = "unverified.stated_only"
	MismatchValues            FindingCode = "mismatch.values"
	MismatchAbsent            FindingCode = "mismatch.absent"
)

// Finding is one state of one unit, with what it rests on.
type Finding struct {
	Code     FindingCode
	About    []string // elements that carry the state; the first is the one to point at
	Key      string   // the support key of the unit
	Slot     string   // set for a connection with a slot
	Evidence []string // evidence IDs the state rests on
	Coverage []string // IDs of every coverage record that applies to the unit
}

// Kind is read from the code, so the two cannot disagree.
func (f Finding) Kind() MarkKind {
	kind, _, _ := strings.Cut(string(f.Code), ".")
	return MarkKind(kind)
}

type DefectCode string

const (
	DefectNoEvidence            DefectCode = "model.no_evidence"
	DefectContradictoryEvidence DefectCode = "model.contradictory_evidence"
	DefectRelationEvidence      DefectCode = "model.relation_evidence"
)

// Defect is one element that no figure may show, with the evidence at fault.
type Defect struct {
	Code     DefectCode
	ID       string
	Evidence []string
}

func (d Defect) Error() string {
	evidence := make([]string, len(d.Evidence))
	for i, id := range d.Evidence {
		evidence[i] = fmt.Sprintf("%q", id)
	}
	switch d.Code {
	case DefectContradictoryEvidence:
		return fmt.Sprintf("edge %q: evidence %s both states and denies the relation, so it backs neither reading", d.ID, strings.Join(evidence, ", "))
	case DefectRelationEvidence:
		return fmt.Sprintf("edge %q: its direct evidence %s backs a different kind of relation", d.ID, strings.Join(evidence, ", "))
	}
	return fmt.Sprintf("element %q has no evidence that speaks for it; a fact without evidence is not drawn, even when the view config hides it", d.ID)
}

// itemKeys are the items of a node that are checked. Each has one value at a
// time, so two values for the same time are a mismatch.
var itemKeys = []string{model.KeyExistence, model.KeyRegion, model.KeyScopeOwner, model.KeyExposure}

// recordKinds are the kinds of evidence whose values are compared.
var recordKinds = []model.EvidenceKind{model.EvDeclared, model.EvDeployed, model.EvStated}

// scopeItems are the items of a coverage scope that are matched with a node.
var scopeItems = []string{"provider", "service", "environment", "account_id", "project_id"}

// unit is one thing checked against the real environment: an item of a node,
// or a connection, which is the configuration edges of one source,
// environment and slot. An edge without a slot is a connection by itself.
type unit struct {
	node    *model.Node // the node of the item, or the source of the connection
	key     string
	env     string
	slot    string
	edges   []*model.Edge // none for an item of a node
	records []record      // in the order of the model
}

// record is the value one piece of evidence gives a unit: the canonical JSON
// of an item, or the target node of a connection.
type record struct {
	ev     *model.Evidence
	edge   *model.Edge // the edge the evidence is on, for a connection
	value  string
	absent bool // deployed existence: false, or deployed connection.configured: false
	when   model.Period
}

func (r record) comparable(o record) bool { return r.when.Comparable(o.when) }

func keep(recs []record, ok func(record) bool) []record {
	var out []record
	for _, r := range recs {
		if ok(r) {
			out = append(out, r)
		}
	}
	return out
}

func units(m *model.Model) []*unit {
	var out []*unit
	nodes := map[string]*model.Node{}
	for i := range m.Nodes {
		n := &m.Nodes[i]
		nodes[n.ID] = n
		for _, key := range itemKeys {
			u := &unit{node: n, key: key, env: cmp.Or(n.Environment, m.Target.Environment)}
			for j := range n.Evidence {
				u.records = append(u.records, itemRecords(&n.Evidence[j], key)...)
			}
			out = append(out, u)
		}
	}
	bySlot := map[[3]string]*unit{}
	for i := range m.Edges {
		e := &m.Edges[i]
		if !relationRules[e.Relation].needsDeployed {
			continue
		}
		env := cmp.Or(e.Environment, m.Target.Environment)
		at := [3]string{e.From, env, e.Slot}
		u := bySlot[at]
		if u == nil {
			u = &unit{node: nodes[e.From], key: model.KeyConnectionTarget, env: env, slot: e.Slot}
			out = append(out, u)
			if e.Slot != "" {
				bySlot[at] = u
			}
		}
		u.edges = append(u.edges, e)
		for j := range e.Evidence {
			if r, ok := connectionRecord(&e.Evidence[j], e); ok {
				u.records = append(u.records, r)
			}
		}
	}
	return out
}

func itemRecords(ev *model.Evidence, key string) []record {
	if !slices.Contains(recordKinds, ev.Kind) {
		return nil
	}
	var out []record
	for _, s := range ev.Supports {
		if s.Key == key {
			absent := ev.Kind == model.EvDeployed && key == model.KeyExistence && s.Value == false
			out = append(out, record{ev: ev, value: canonical(s.Value), absent: absent, when: ev.When()})
		}
	}
	return out
}

// connectionRecord reads the target that evidence on edge e gives: its
// connection.target, or else the end of e when it says the connection is
// configured.
func connectionRecord(ev *model.Evidence, e *model.Edge) (record, bool) {
	if !slices.Contains(recordKinds, ev.Kind) {
		return record{}, false
	}
	r := record{ev: ev, edge: e, when: ev.When()}
	configured, unset := false, false
	for _, s := range ev.Supports {
		switch s.Key {
		case model.KeyConnectionTarget:
			if target, ok := s.Value.(string); ok {
				r.value = target
			}
		case model.KeyConnectionConfigure:
			configured, unset = configured || s.Value == true, unset || s.Value == false
		}
	}
	switch {
	case r.value != "":
	case configured:
		r.value = e.To
	case unset && ev.Kind == model.EvDeployed:
		r.value, r.absent = e.To, true
	default:
		return record{}, false
	}
	return r, true
}

// declarations are the records that call for a check against the real
// environment: a managed resource declared to exist (S4), and a declared
// connection to the end of its own edge.
func (u *unit) declarations() []record {
	return keep(u.records, func(r record) bool {
		if r.ev.Kind != model.EvDeclared {
			return false
		}
		if r.edge != nil {
			return r.value == r.edge.To
		}
		return u.node.Kind == model.KindResource && u.key == model.KeyExistence && r.value == "true"
	})
}

// coveredBy reports whether a coverage record applies to the unit.
func (u *unit) coveredBy(c *model.Coverage) bool {
	slot, bySlot := c.Scope.Slot()
	fields, byField := c.Scope.Fields()
	ids := []string{u.node.ID}
	if len(u.edges) == 0 {
		if bySlot || byField && !slices.Contains(fields, u.key) {
			return false
		}
	} else {
		if bySlot && slot != u.slot {
			return false
		}
		if byField && !slices.Contains(fields, model.KeyConnectionConfigure) && !slices.Contains(fields, model.KeyConnectionTarget) {
			return false
		}
		for _, e := range u.edges {
			ids = append(ids, e.ID)
		}
	}
	return scopeFits(c.Scope, u.node, u.env, ids)
}

// scopeFits reports whether a scope is about node n in environment env. Each
// item the scope shares with n has n's value, and the scope names one of ids,
// or names the account or project of n by items that n all has.
func scopeFits(s model.Scope, n *model.Node, env string, ids []string) bool {
	has := map[string]string{"provider": n.Provider, "service": n.Service, "environment": env}
	for _, key := range []string{"account_id", "project_id"} {
		if v, ok := n.Identity[key]; ok && v != nil {
			has[key] = fmt.Sprint(v)
		}
	}
	all := true
	for _, key := range scopeItems {
		want, written := s.Item(key)
		if !written {
			continue
		}
		switch got := has[key]; {
		case got == "":
			all = false
		case got != want:
			return false
		}
	}
	if targets, listed := s.Targets(); listed {
		return slices.ContainsFunc(ids, func(id string) bool { return slices.Contains(targets, id) })
	}
	_, account := s.Item("account_id")
	_, project := s.Item("project_id")
	return (account || project) && all
}

// reading is what the real environment says about a unit: the deployed
// records that count, the ones F1 and F2 set aside, and the coverage records
// that apply to it.
type reading struct {
	counted, unbacked, superseded []record
	coverage                      []string
	// stopped reports that a read naming the unit stopped short and none
	// naming it finished, so the targets found may not be all there are.
	stopped bool
}

// named reports whether a coverage record lists its targets. Only such a read
// was an attempt at those elements. A listing that stopped half way says
// nothing about what a read of an element itself found, so it changes no
// state.
func named(c *model.Coverage) bool {
	_, listed := c.Scope.Targets()
	return listed
}

func (u *unit) read(coverage []model.Coverage) reading {
	var r reading
	var applying []*model.Coverage
	stopped, finished := false, false
	for i := range coverage {
		if c := &coverage[i]; u.coveredBy(c) {
			applying = append(applying, c)
			r.coverage = append(r.coverage, c.ID)
			stopped = stopped || named(c) && c.Unfinished()
			finished = finished || named(c) && c.Status == model.CovComplete
		}
	}
	r.stopped = stopped && !finished
	for _, rec := range u.records {
		if rec.ev.Kind != model.EvDeployed {
			continue
		}
		backs := func(c *model.Coverage) bool {
			return c.Status == model.CovComplete && sameAcquisition(c, rec.ev)
		}
		supersedes := func(c *model.Coverage) bool {
			return named(c) && c.Unfinished() && c.Checked().After(rec.ev.Checked()) && !sameAcquisition(c, rec.ev)
		}
		switch {
		case rec.absent && !slices.ContainsFunc(applying, backs):
			r.unbacked = append(r.unbacked, rec)
		case rec.when.Present() && slices.ContainsFunc(applying, supersedes):
			r.superseded = append(r.superseded, rec)
		default:
			r.counted = append(r.counted, rec)
		}
	}
	return r
}

// sameAcquisition ties evidence to a coverage record. checked_at is not
// enough, since two reads can share a time by chance.
func sameAcquisition(c *model.Coverage, ev *model.Evidence) bool {
	return c.AcquisitionID != "" && c.AcquisitionID == ev.AcquisitionID
}

func (u *unit) assess(r reading) []Finding {
	if len(u.edges) == 0 {
		return u.assessItem(r)
	}
	return u.assessConnection(r)
}

// assessItem applies S1 to S6 to an item of a node.
func (u *unit) assessItem(r reading) []Finding {
	var out []Finding
	var open []record
	for _, d := range u.declarations() {
		if !slices.ContainsFunc(r.counted, d.comparable) {
			open = append(open, d)
		}
	}
	if len(open) > 0 {
		out = append(out, u.unverified(open, r, record.comparable, []string{u.node.ID}))
	}
	compared := slices.Concat(keep(u.records, func(x record) bool { return x.ev.Kind != model.EvDeployed }), r.counted)
	var differ []record
	absent := false
	for i, a := range compared {
		for _, b := range compared[i+1:] {
			bothStated := a.ev.Kind == model.EvStated && b.ev.Kind == model.EvStated
			if a.ev == b.ev || bothStated || a.value == b.value || !a.comparable(b) {
				continue
			}
			differ = append(differ, a, b)
			absent = absent || a.absent || b.absent
		}
	}
	if len(differ) > 0 {
		code := MismatchValues
		if absent {
			code = MismatchAbsent
		}
		out = append(out, u.finding(code, []string{u.node.ID}, differ, r))
	}
	return out
}

// assessConnection applies S1, S2 and S6 to S10 to a connection.
func (u *unit) assessConnection(r reading) []Finding {
	declared := u.declarations()
	found := keep(r.counted, func(x record) bool { return !x.absent })
	perTarget := r.stopped || severalTargets(declared) || severalTargets(found)
	var open, mismatched []record
	absent := false
	for _, d := range declared {
		same := func(x record) bool { return d.comparable(x) && x.value == d.value }
		unset := keep(r.counted, func(x record) bool { return x.absent && same(x) })
		other := keep(found, d.comparable)
		switch {
		case slices.ContainsFunc(found, same):
		case len(unset) > 0:
			mismatched = append(append(mismatched, d), unset...)
			absent = true
		case !perTarget && len(other) > 0:
			mismatched = append(append(mismatched, d), other...)
		default:
			open = append(open, d)
		}
	}
	// A record set aside matters to a declaration when counting it could
	// have settled the declaration: the same target, or in a setting of one
	// target, any target found.
	related := func(d, x record) bool {
		return d.comparable(x) && (x.value == d.value || !perTarget && !x.absent)
	}
	var out []Finding
	if len(open) > 0 {
		out = append(out, u.unverified(open, r, related, u.edgesOf(open)))
	}
	if len(mismatched) > 0 {
		code := MismatchValues
		if absent {
			code = MismatchAbsent
		}
		out = append(out, u.finding(code, append([]string{u.node.ID}, u.edgesOf(mismatched)...), mismatched, r))
	}
	return out
}

// severalTargets reports whether records of one time point at more than one
// target, which only a setting that takes several targets does (S9).
func severalTargets(recs []record) bool {
	for i, a := range recs {
		for _, b := range recs[i+1:] {
			if a.value != b.value && a.comparable(b) {
				return true
			}
		}
	}
	return false
}

// unverified is the finding for declarations that no counted record settles
// (S1). Its code says why: F1 set aside an absence, F2 set aside an earlier
// record, or nothing was read.
func (u *unit) unverified(open []record, r reading, related func(d, x record) bool, about []string) Finding {
	matters := func(x record) bool {
		return slices.ContainsFunc(open, func(d record) bool { return related(d, x) })
	}
	unbacked, superseded := keep(r.unbacked, matters), keep(r.superseded, matters)
	code := UnverifiedNotChecked
	switch {
	case len(unbacked) > 0:
		code = UnverifiedAbsenceUnbacked
	case len(superseded) > 0:
		code = UnverifiedSuperseded
	}
	return u.finding(code, about, slices.Concat(open, unbacked, superseded), r)
}

func (u *unit) finding(code FindingCode, about []string, recs []record, r reading) Finding {
	f := Finding{Code: code, About: about, Key: u.key, Slot: u.slot, Coverage: r.coverage}
	for _, rec := range u.records {
		if slices.ContainsFunc(recs, func(x record) bool { return x.ev == rec.ev }) && !slices.Contains(f.Evidence, rec.ev.ID) {
			f.Evidence = append(f.Evidence, rec.ev.ID)
		}
	}
	return f
}

// edgesOf returns the edges the records are on, in the order of the model.
func (u *unit) edgesOf(recs []record) []string {
	var ids []string
	for _, e := range u.edges {
		if slices.ContainsFunc(recs, func(x record) bool { return x.edge == e }) {
			ids = append(ids, e.ID)
		}
	}
	return ids
}

// statedOnly is the finding for a relation that only a statement speaks for
// (S11). It holds for every relation, since no statement is ever checked.
func statedOnly(e *model.Edge) (Finding, bool) {
	if _, stated := lineOf(e); !stated {
		return Finding{}, false
	}
	f := Finding{Code: UnverifiedStatedOnly, About: []string{e.ID}}
	for _, ev := range speakingFor(e.Evidence, edgeFacts, e.To) {
		if ev.Kind == model.EvStated {
			f.Evidence = append(f.Evidence, ev.ID)
		}
	}
	return f, true
}
