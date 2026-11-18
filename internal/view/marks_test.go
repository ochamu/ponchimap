package view

import (
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/ochamu/ponchimap/internal/model"
)

func loadCase(t *testing.T, name string) *model.Model {
	t.Helper()
	return loadModel(t, filepath.Join("..", "..", "testdata", "marks", name+".json"))
}

func buildCase(t *testing.T, name string, c *Config) *Scene {
	t.Helper()
	s, err := Build(loadCase(t, name), c)
	if err != nil {
		t.Fatalf("build scene: %v", err)
	}
	return s
}

func elementOf(t *testing.T, s *Scene, id string) Element {
	t.Helper()
	for _, el := range s.Elements {
		if el.ID == id {
			return el
		}
	}
	t.Fatalf("scene has no element %q", id)
	return Element{}
}

func TestALegendExplainsAnUnverifiedStateThatOnlyALineCarries(t *testing.T) {
	s := buildCase(t, "only-a-relation-is-unverified", &Config{})
	wantNode(t, s, "w", FrameSolid)
	wantNode(t, s, "a", FrameSolid)
	if e := edge(t, s, "db"); e.State != MarkUnverified {
		t.Errorf("edge db state = %q, want unverified", e.State)
	}
	if !slices.Contains(s.Legend, LegendUnverified) {
		t.Errorf("legend %v does not explain the unverified line", s.Legend)
	}

	confirmed := buildCase(t, "edge-without-slot-confirmed", &Config{})
	if slices.Contains(confirmed.Legend, LegendUnverified) {
		t.Errorf("legend %v explains an unverified state the figure does not show", confirmed.Legend)
	}
}

func TestARelationFromANodeToItselfIsANoteAndAMark(t *testing.T) {
	s := buildCase(t, "relation-to-itself", &Config{})
	if len(s.Edges) != 0 {
		t.Errorf("edges = %+v, want no line from w to itself", s.Edges)
	}
	if want := []Mark{{Kind: MarkUnverified, Of: MarkOfRelation, Count: 1}}; !reflect.DeepEqual(node(t, s, "w").Marks, want) {
		t.Errorf("node w marks = %+v, want %+v", node(t, s, "w").Marks, want)
	}
	if got, want := elementOf(t, s, "self"), (Element{ID: "self", At: "w", Shown: ShownSelf}); !reflect.DeepEqual(got, want) {
		t.Errorf("element self = %+v, want %+v", got, want)
	}
	if want := "接続元と接続先が同じ関係1件は、線を描いていません。"; !slices.Contains(s.Notes, want) {
		t.Errorf("notes %q do not say %q", s.Notes, want)
	}
	if !slices.Contains(s.Legend, LegendUnverified) {
		t.Errorf("legend %v does not explain the relation mark", s.Legend)
	}
}

func TestNotesCountWhatTheViewConfigLeavesOut(t *testing.T) {
	for _, c := range []struct {
		name, model, hide string
		notes             []string
	}{
		{"an unverified relation goes with its end", "only-a-relation-is-unverified", "a",
			[]string{"表示設定により、構成要素1件と関係1件を図から省いています。", "省いたもののうち、未照合は1件です。"}},
		{"a mismatched relation goes with its end", "slot-declared-a-deployed-b", "b",
			[]string{"表示設定により、構成要素1件と関係2件を図から省いています。", "省いたもののうち、不一致は1件です。"}},
		{"nothing left out is unverified or a mismatch", "edge-without-slot-confirmed", "a",
			[]string{"表示設定により、構成要素1件と関係1件を図から省いています。"}},
		{"a hidden relation alone", "edge-without-slot-confirmed", "to-a",
			[]string{"表示設定により、関係1件を図から省いています。"}},
	} {
		t.Run(c.name, func(t *testing.T) {
			s := buildCase(t, c.model, &Config{Hidden: []string{c.hide}})
			if !reflect.DeepEqual(s.Notes, c.notes) {
				t.Errorf("notes = %q, want %q", s.Notes, c.notes)
			}
		})
	}
}

func TestElementsSayWhereEveryModelElementWent(t *testing.T) {
	configured := func(id, from, to string) string {
		return `{"id":"` + id + `","from":"` + from + `","to":"` + to + `","relation":"configuration","attributes":{},"evidence":[{"id":"ev-` + id + `","kind":"deployed","supports":[{"key":"connection.configured","value":true}],"ref":{"type":"query","path":"findings/query.json"},"checked_at":"2026-11-13T00:00:00Z"}]}`
	}
	nodes := []string{}
	for _, id := range []string{"w", "a", "b", "c", "d"} {
		nodes = append(nodes, deployedNode(id, "p", ""))
	}
	edges := []string{configured("to-a", "w", "a"), configured("a-b", "a", "b"), configured("loop", "w", "w"), configured("to-c", "w", "c"), configured("to-d", "w", "d")}
	m, diags := model.Parse("model.json", []byte(modelOf(strings.Join(nodes, ","), strings.Join(edges, ","))))
	if diags.HasErrors() {
		t.Fatal(diags)
	}
	s, err := Build(m, &Config{
		Aggregates: []AggregateConfig{{ID: "agg", Label: "まとめ", Members: []string{"a", "b"}}},
		Hidden:     []string{"c", "to-d"},
	})
	if err != nil {
		t.Fatal(err)
	}
	want := []Element{
		{ID: "w", At: "w", Shown: ShownCard},
		{ID: "a", At: "agg", Shown: ShownAggregated},
		{ID: "b", At: "agg", Shown: ShownAggregated},
		{ID: "c", Shown: ShownHidden},
		{ID: "d", At: "d", Shown: ShownCard},
		{ID: "to-a", At: "to-a", Shown: ShownLine},
		{ID: "a-b", At: "agg", Shown: ShownFolded},
		{ID: "loop", At: "w", Shown: ShownSelf},
		{ID: "to-c", Shown: ShownEndHidden},
		{ID: "to-d", Shown: ShownHidden},
	}
	if !reflect.DeepEqual(s.Elements, want) {
		t.Errorf("elements\n got  %+v\n want %+v", s.Elements, want)
	}
}

func TestAHiddenElementWithoutEvidenceStillStopsTheFigure(t *testing.T) {
	ghost := `{"id":"ghost","kind":"resource","provider":"p","service":"s","identity":{},"attributes":{},"evidence":[{"id":"ev-ghost","kind":"deployed","supports":[{"key":"existence","value":false}],"ref":{"type":"query","path":"findings/query.json"},"checked_at":"2026-11-13T00:00:00Z"}]}`
	m, diags := model.Parse("model.json", []byte(modelOf(deployedNode("w", "p", "")+","+ghost, "")))
	if diags.HasErrors() {
		t.Fatal(diags)
	}
	if _, err := Build(m, &Config{Hidden: []string{"ghost"}}); err == nil || !strings.Contains(err.Error(), `"ghost"`) {
		t.Errorf("err = %v, want an error naming the hidden node ghost", err)
	}
}
