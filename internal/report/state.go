package report

import (
	"cmp"
	"fmt"
	"slices"
	"strings"

	"github.com/ochamu/ponchimap/internal/model"
	"github.com/ochamu/ponchimap/internal/view"
)

// state is what the evidence and the coverage tell: what disagrees, what is
// unconfirmed, and what was read. readState takes the model and its
// assessment and no scene, so that hiding or folding an element cannot change
// a word of the three sections made from it.
type state struct {
	mismatches []line
	unverified []line
	unbacked   []line // the elements without direct evidence that no finding speaks for
	unfinished int    // the coverage records that are not complete
	unlisted   []line // those of them that stand under no unverified finding
	asked      []line // the scopes the investigation was asked to cover
	coverage   []line
}

func readState(ix *index, a *view.Assessment) state {
	var st state
	listed := map[string]bool{}
	for i, f := range a.Findings {
		l := findingLine(ix, i+1, f)
		if f.Kind() == view.MarkMismatch {
			st.mismatches = append(st.mismatches, l)
			continue
		}
		st.unverified = append(st.unverified, l)
		if hasUnit(f) {
			for _, id := range f.Coverage {
				listed[id] = true
			}
		}
	}
	add := func(id string) {
		if unbacked(ix, a, id) {
			st.unbacked = append(st.unbacked, line{
				body: text(weakLabel + ": ").add(ix.element(id)).text("。" + weakEntryText),
				sub:  rows("確認方法", settleDirectlyText),
			})
		}
	}
	for _, n := range ix.m.Nodes {
		add(n.ID)
	}
	for _, e := range ix.m.Edges {
		add(e.ID)
	}
	for _, scope := range ix.m.Target.Scope {
		asked := labelled(targetScopeLabel, keyed(without(scope, "filter"), model.InScope, scopeOrder...))
		asked.sub = row("絞り込み", narrowing(scope))
		st.asked = append(st.asked, asked)
	}
	for i := range ix.m.Coverage {
		c := &ix.m.Coverage[i]
		st.coverage = append(st.coverage, coverageLine(ix, c))
		if c.Status == model.CovComplete {
			continue
		}
		st.unfinished++
		// A record such as a listing that stopped half way applies to no
		// unverified finding, and would otherwise be left out of the section.
		if !listed[c.ID] {
			st.unlisted = append(st.unlisted, unfinishedLine(c))
		}
	}
	return st
}

// unbacked reports whether the report says of the element itself that only a
// hint or a statement speaks for it. A relation with the stated-only finding
// has that finding say so.
func unbacked(ix *index, a *view.Assessment, id string) bool {
	return ix.dashed(id) && !slices.ContainsFunc(a.For(id), func(f view.Finding) bool { return f.Code == view.UnverifiedStatedOnly })
}

func (st state) mismatchSection() section {
	sec := section{heading: "食い違い", lead: mismatchLeadText, lines: st.mismatches}
	if len(sec.lines) == 0 {
		sec.lead = noMismatchText
	}
	return sec
}

func (st state) unconfirmedSection() section {
	sec := section{heading: "未確認の点", lines: slices.Concat(st.unverified, st.unbacked, st.unlisted)}
	if len(sec.lines) == 0 {
		sec.lead = noUnconfirmedText
	}
	return sec
}

func (st state) coverageSection() section {
	sec := section{heading: "調査範囲", lead: coverageLeadText, lines: slices.Concat(st.asked, st.coverage)}
	if len(st.coverage) == 0 {
		sec.lead += noCoverageText
	}
	return sec
}

// findingAnchor is the link target of the entry of the nth finding of the
// assessment, counted from 1. A model ID holds no underscore and a boundary
// ID opens with bd_, so it is none of them.
func findingAnchor(n int) string { return fmt.Sprintf("finding_%d", n) }

// hasUnit reports whether the finding is about a unit that coverage applies
// to.
func hasUnit(f view.Finding) bool { return f.Key != "" }

func findingLine(ix *index, n int, f view.Finding) line {
	l := line{
		anchor: findingAnchor(n),
		body:   text(f.Kind().Label() + ": ").add(ix.elements(f.About)).text("。" + sentence(f)),
	}
	for _, id := range f.Evidence {
		l.sub = append(l.sub, line{body: citation(ix, id, citedKeys(f))})
	}
	if hasUnit(f) {
		for _, id := range f.Coverage {
			l.sub = append(l.sub, labelled("この項目に当たる調査範囲", unitCoverage(ix, f, id)))
		}
		if len(f.Coverage) == 0 {
			l.sub = append(l.sub, line{body: text(noCoverageOfUnitText)})
		}
	}
	for _, s := range settleTexts(ix, f) {
		l.sub = append(l.sub, labelled("確認方法", text(s)))
	}
	return l
}

// settleTexts are the ways to check a finding. A query of its unit that did
// not finish says better than the finding what to do next, such as checking
// a permission the query was denied.
func settleTexts(ix *index, f view.Finding) []string {
	own := findingTexts[f.Code].settle
	if own == "" {
		return nil
	}
	var out []string
	if hasUnit(f) {
		for _, id := range f.Coverage {
			if s := coverageTexts[ix.coverage[id].Status].settle; s != "" && !slices.Contains(out, s) {
				out = append(out, s)
			}
		}
	}
	if len(out) == 0 {
		return []string{own}
	}
	return out
}

func citation(ix *index, id string, keys []string) inline {
	ev := ix.evidence[id]
	out := ix.cite(id, ix.element)
	var values []inline
	for _, s := range ev.Supports {
		if slices.Contains(keys, s.Key) {
			values = append(values, support(s))
		}
	}
	if len(values) > 0 {
		out = out.text("。").add(joined(values, "、"))
	}
	out = out.text("。出典 ").add(refText(ev.Ref)).text("。確認時刻 " + ev.CheckedAt)
	if ev.ValidTime != "" {
		out = out.text("。対象時点 " + ev.ValidTime)
	}
	if ev.AcquisitionID != "" {
		out = out.text("。取得記録 " + ev.AcquisitionID)
	}
	return out
}

func reasonOf(c *model.Coverage) string { return cmp.Or(c.Reason, noReasonText) }

// unitCoverage says how far a coverage record of the unit of a finding got,
// and which evidence of the finding came from the same acquisition.
func unitCoverage(ix *index, f view.Finding, id string) inline {
	c := ix.coverage[id]
	var same []string
	for _, ev := range f.Evidence {
		if c.AcquisitionID != "" && ix.evidence[ev].AcquisitionID == c.AcquisitionID {
			same = append(same, ev)
		}
	}
	out := link(id).text(" " + statusName(c.Status) + "。")
	if len(same) == 0 {
		return out.text(reasonOf(c))
	}
	// A reason may close its own sentence.
	return out.text(strings.TrimSuffix(reasonOf(c), "。") + "。").add(links(same)).text(" と同じ取得記録 " + c.AcquisitionID)
}

func unfinishedLine(c *model.Coverage) line {
	body := text("調査範囲 ").link(c.ID).text(" " + statusName(c.Status) + ": ")
	if ids := c.Scope.TargetIDs(); len(ids) > 0 {
		body = body.text("対象 ").add(links(ids)).text("。")
	}
	return line{body: body.text(reasonOf(c)), sub: rows("確認方法", coverageTexts[c.Status].settle)}
}

// scopeOrder are the items of a scope that say where it is, in the order a
// reader narrows down: the rest follow in the order of the keys.
var scopeOrder = []string{"provider", "account_id", "project_id", "environment", "service"}

func coverageLine(ix *index, c *model.Coverage) line {
	l := line{anchor: c.ID, body: text(c.ID + " " + statusName(c.Status) + ": " + reasonOf(c))}
	if ids := c.Scope.TargetIDs(); len(ids) > 0 {
		l.sub = append(l.sub, labelled("対象の要素", links(ids)))
	}
	l.sub = slices.Concat(l.sub,
		row("対象範囲", keyed(without(c.Scope, "target_ids", "filter"), model.InScope, scopeOrder...)),
		row("絞り込み", narrowing(c.Scope)),
		rows("確認時刻", c.CheckedAt),
	)
	acquisition := text(cmp.Or(c.AcquisitionID, "記録なし"))
	if c.AcquisitionID != "" {
		var given []inline
		for _, id := range ix.acquired[c.AcquisitionID] {
			given = append(given, link(id).text("（").link(ix.evidence[id].owner).text("）"))
		}
		if len(given) == 0 {
			acquisition = acquisition.text("。同じ取得記録を持つ根拠はない")
		} else {
			acquisition = acquisition.text("。同じ取得記録を持つ根拠: ").add(joined(given, "、"))
		}
	}
	l.sub = append(l.sub, labelled("取得記録", acquisition))
	return l
}

// narrowing says which items of its scope a coverage record is about.
func narrowing(s model.Scope) inline {
	var parts []inline
	if slot, ok := s.Slot(); ok {
		parts = append(parts, text("設定項目（slot）"+slot))
	}
	if fields, ok := s.Fields(); ok {
		parts = append(parts, text("項目 "+cmp.Or(strings.Join(fields, "、"), "なし")))
	}
	filter, _ := s["filter"].(map[string]any)
	if rest := keyed(without(filter, "slot", "fields"), model.InFilter); len(rest) > 0 {
		parts = append(parts, rest)
	}
	return joined(parts, "。")
}
