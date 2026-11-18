package view

import (
	"strings"
	"testing"

	"github.com/ochamu/ponchimap/internal/model"
)

func deployedNode(id, provider, extra string) string {
	return `{"id":"` + id + `","kind":"resource","provider":"` + provider + `","service":"s","identity":{},"attributes":{},"evidence":[{"id":"ev-` + id + `","kind":"deployed","supports":[{"key":"existence","value":true}` + extra + `],"ref":{"type":"query","path":"findings/query.json"},"checked_at":"2026-11-13T00:00:00Z"}]}`
}

func modelOf(nodes, edges string) string {
	return `{"schema_version":"0.1","target":{"repository":".","environment":"dev"},"nodes":[` + nodes + `],"edges":[` + edges + `],"coverage":[]}`
}

func buildErr(t *testing.T, modelJSON string, c *Config) (*Scene, error) {
	t.Helper()
	m, diags := model.Parse("model.json", []byte(modelJSON))
	if diags.HasErrors() {
		t.Fatalf("parse model: %v", diags)
	}
	return Build(m, c)
}

func TestAStatedRelationFoldedIntoAnAggregateStillShowsAsUnverified(t *testing.T) {
	edge := `{"id":"told","from":"a","to":"b","relation":"dependency","attributes":{"purpose":"連携"},"evidence":[{"id":"ev-told","kind":"stated","supports":[{"key":"lifecycle.requires","value":true}],"ref":{"type":"document","path":"README.md"},"checked_at":"2026-11-13T00:00:00Z"}]}`
	s := build(t, modelOf(deployedNode("a", "p", "")+","+deployedNode("b", "p", ""), edge), aggregateAll("a", "b"))
	got := node(t, s, "agg").Marks
	want := Mark{Kind: MarkUnverified, Of: MarkOfRelation, Count: 1}
	if len(got) != 1 || got[0] != want {
		t.Errorf("aggregate marks = %+v, want %+v: a relation known only from a statement was folded away without a trace", got, want)
	}
}

func TestEvidenceThatStatesAndDeniesTheSameRelationIsRejected(t *testing.T) {
	for name, supports := range map[string]string{
		"not configured, yet names the target": `{"key":"connection.configured","value":false},{"key":"connection.target","value":"b"}`,
		"configured, yet names another target": `{"key":"connection.configured","value":true},{"key":"connection.target","value":"a"}`,
	} {
		edge := `{"id":"e","from":"a","to":"b","relation":"configuration","slot":"DB","attributes":{},"evidence":[{"id":"ev-e","kind":"deployed","supports":[` + supports + `],"ref":{"type":"query","path":"findings/query.json"},"checked_at":"2026-11-13T00:00:00Z"}]}`
		_, err := buildErr(t, modelOf(deployedNode("a", "p", "")+","+deployedNode("b", "p", ""), edge), &Config{})
		if err == nil || !strings.Contains(err.Error(), "ev-e") {
			t.Errorf("%s: err = %v, want an error naming evidence ev-e", name, err)
		}
	}
}

func TestSameRegionUnderDifferentProvidersIsTwoBoundaries(t *testing.T) {
	region := `,{"key":"region","value":"tokyo"}`
	s := build(t, modelOf(deployedNode("a", "p1", region)+","+deployedNode("b", "p2", region)+","+deployedNode("c", "p1", region), ""), &Config{})
	if node(t, s, "a").GroupID == node(t, s, "b").GroupID {
		t.Error("nodes of different providers share one region boundary")
	}
	if node(t, s, "a").GroupID != node(t, s, "c").GroupID {
		t.Error("nodes of the same provider and region are in different boundaries")
	}
}

func TestANodeCannotBelongToTwoAggregatesOrTwoGroups(t *testing.T) {
	nodes := deployedNode("a", "p", "") + "," + deployedNode("b", "p", "") + "," + deployedNode("c", "p", "")
	cases := map[string]*Config{
		"two aggregates share a member":     {Aggregates: []AggregateConfig{{ID: "x", Label: "X", Members: []string{"a", "b"}}, {ID: "y", Label: "Y", Members: []string{"a", "c"}}}},
		"an aggregate lists a member twice": {Aggregates: []AggregateConfig{{ID: "x", Label: "X", Members: []string{"a", "a"}}}},
		"two groups share a member":         {Groups: []GroupConfig{{ID: "x", Label: "X", Members: []string{"a", "b"}}, {ID: "y", Label: "Y", Members: []string{"b", "c"}}}},
	}
	for name, cfg := range cases {
		if _, err := buildErr(t, modelOf(nodes, ""), cfg); err == nil {
			t.Errorf("%s: the view config was accepted", name)
		}
	}
}
