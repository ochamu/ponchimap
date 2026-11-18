// Package report builds the evidence report that goes with one diagram: what
// every node, edge, and group is, which evidence backs it, what is unverified
// or disagrees, how far the investigation got, and how the figure shows each
// element. Build decides what the report says, and Markdown lays it out.
package report

import (
	"fmt"
	"time"

	"github.com/ochamu/ponchimap/internal/model"
	"github.com/ochamu/ponchimap/internal/view"
)

type Report struct{ doc document }

// Drawing is how the figure that the report goes with was drawn: the layout
// it was placed with, and whether a view config was given.
type Drawing struct {
	Layout view.LayoutConfig
	View   bool
}

// Build is a pure function of the model, the scene, and how the figure was
// drawn.
func Build(m *model.Model, s *view.Scene, d Drawing) *Report {
	ix := newIndex(m)
	a := s.Assessment
	st := readState(ix, a)
	fig := newFigure(s, m)

	elements := section{heading: "構成要素"}
	for i := range m.Nodes {
		elements.items = append(elements.items, nodeItem(ix, a, fig, &m.Nodes[i]))
	}
	for i := range s.Groups {
		elements.items = append(elements.items, groupItem(ix, fig, &s.Groups[i]))
	}
	relations := section{heading: "関係とデータの流れ"}
	for i := range m.Edges {
		relations.items = append(relations.items, edgeItem(ix, a, fig, &m.Edges[i]))
	}
	omitted := section{heading: "図に載せなかったもの", lines: omittedLines(ix, a, fig)}
	if len(omitted.lines) == 0 {
		omitted.lead = noOmittedText
	}

	title := reportTitle
	if s.Title != "" {
		title += ": " + s.Title
	}
	return &Report{document{title: title, preamble: preambleText, sections: []section{
		overview(m, s, st, len(omitted.lines)),
		elements, relations,
		st.mismatchSection(), st.unconfirmedSection(),
		omitted,
		st.coverageSection(),
		redraw(d),
	}}}
}

// Markdown gives the same bytes for the same input.
func (r *Report) Markdown() []byte { return r.doc.markdown() }

func overview(m *model.Model, s *view.Scene, st state, omitted int) section {
	groups := map[view.GroupKind]int{}
	for _, g := range s.Groups {
		groups[g.Kind]++
	}
	count := func(n int) string { return fmt.Sprintf("%d件", n) }
	return section{heading: "概要", lines: rows(
		"対象リポジトリ", m.Target.Repository,
		"主題の環境", m.Target.Environment,
		"調査時点", surveyed(m),
		"構成要素", count(len(m.Nodes)),
		"関係", count(len(m.Edges)),
		"実在する境界", count(groups[view.GroupBoundary]),
		"論理グループ", count(groups[view.GroupLogical]),
		"食い違い", count(len(st.mismatches)),
		"未確認の点", fmt.Sprintf("未照合 %d件、根拠が手がかりや説明だけの要素 %d件、どの未照合にも当たらない完了していない調査範囲 %d件", len(st.unverified), len(st.unbacked), len(st.unlisted)),
		"図に載せなかったもの", count(omitted),
		"調査範囲", fmt.Sprintf("%d件。完了 %d件、完了していないもの %d件", len(st.coverage), len(st.coverage)-st.unfinished, st.unfinished),
		"状態の読み方", readingText,
		"根拠の種類の読み方", evidenceKindsText,
		"対象時点の読み方", validTimeText,
	)}
}

// surveyed is the span of the checked_at of all evidence and coverage. The
// times are compared as times and written as the model has them.
func surveyed(m *model.Model) string {
	var oldest, newest string
	var from, to time.Time
	see := func(at time.Time, written string) {
		if oldest == "" || at.Before(from) {
			oldest, from = written, at
		}
		if newest == "" || at.After(to) {
			newest, to = written, at
		}
	}
	for _, n := range m.Nodes {
		for _, ev := range n.Evidence {
			see(ev.Checked(), ev.CheckedAt)
		}
	}
	for _, e := range m.Edges {
		for _, ev := range e.Evidence {
			see(ev.Checked(), ev.CheckedAt)
		}
	}
	for _, c := range m.Coverage {
		see(c.Checked(), c.CheckedAt)
	}
	switch {
	case oldest == "":
		return "記録なし"
	case from.Equal(to):
		return oldest
	}
	return oldest + " から " + newest + " まで"
}

func redraw(d Drawing) section {
	layout := fmt.Sprintf("配置エンジン %s、向き %s", d.Layout.Engine, d.Layout.Direction)
	// ELK has no seed.
	if d.Layout.Engine == view.EngineTALA {
		layout += fmt.Sprintf("、seed %d", d.Layout.Seed)
	}
	files := "モデル"
	if d.View {
		files = "モデルと表示設定"
	}
	return section{heading: "再描画", lines: rows(
		"配置", layout,
		"再描画に必要なファイル", files+"。"+redrawText,
	)}
}
