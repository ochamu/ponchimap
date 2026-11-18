package view

import (
	"fmt"
	"reflect"
	"slices"
	"testing"

	"github.com/ochamu/ponchimap/internal/model"
)

// wantWellFormed asserts what every consumer of a Scene relies on: one ID
// names one element, and a group's parent is declared before it, which rules
// out a group that is its own ancestor.
func wantWellFormed(t *testing.T, s *Scene) {
	t.Helper()
	seen := map[string]string{}
	claim := func(id, what string) {
		if prev, dup := seen[id]; dup {
			t.Errorf("scene ID %q names both %s and %s", id, prev, what)
		}
		seen[id] = what
	}
	for _, g := range s.Groups {
		if g.ParentID != "" && seen[g.ParentID] != "a group" {
			t.Errorf("group %q has parent %q, which is not a group declared before it", g.ID, g.ParentID)
		}
		claim(g.ID, "a group")
	}
	for _, n := range s.Nodes {
		if n.GroupID != "" && seen[n.GroupID] != "a group" {
			t.Errorf("node %q sits in %q, which is not a group", n.ID, n.GroupID)
		}
		claim(n.ID, "a node")
	}
	for _, e := range s.Edges {
		claim(e.ID, "an edge")
	}
}

func TestGivenIDsCannotCollideWithGeneratedBoundaryIDs(t *testing.T) {
	members := []string{"n101", "n102"}
	// bd1 is a legal config ID that looks like a numbered boundary.
	for name, cfg := range map[string]*Config{
		"logical group": {Groups: []GroupConfig{{ID: "bd1", Label: "バックエンド", Members: members}}},
		"aggregate":     {Aggregates: []AggregateConfig{{ID: "bd1", Label: "バックエンド一式", Members: members}}},
	} {
		t.Run(name, func(t *testing.T) {
			s := build(t, loadSmallModel(t, "worker-d1-and-mail.json"), cfg)
			wantWellFormed(t, s)
			var boundaries []string
			for _, g := range s.Groups {
				if g.Kind == GroupBoundary {
					boundaries = append(boundaries, g.ID)
				}
			}
			if len(boundaries) != 1 || boundaries[0] == "bd1" {
				t.Errorf("boundaries = %v, want the one account boundary under an ID the config cannot take", boundaries)
			}
		})
	}
}

func TestGeneratedBoundaryIDsAreOutsideTheGivenIDPattern(t *testing.T) {
	s := build(t, loadSmallModel(t, "worker-d1-and-mail.json"), nil)
	for _, g := range s.Groups {
		if model.ValidID(g.ID) {
			t.Errorf("generated boundary ID %q is also a legal model and view config ID", g.ID)
		}
	}
}

func aggregateAll(members ...string) *Config {
	return &Config{Aggregates: []AggregateConfig{{ID: "agg", Label: "まとめ", Members: members}}}
}

func TestStateOfEdgesFoldedIntoAnAggregateIsStillDrawn(t *testing.T) {
	t.Run("unverified configuration between verified members", func(t *testing.T) {
		s := build(t, edgeCase("configuration", ev("x1", "declared", "connection.configured", "true")), aggregateAll("a", "b"))
		n := node(t, s, "agg")
		want := []Mark{{Kind: MarkUnverified, Of: MarkOfRelation, Count: 1}}
		if !reflect.DeepEqual(n.Marks, want) {
			t.Fatalf("aggregate marks = %+v, want %+v", n.Marks, want)
		}
		if !slices.Contains(s.Legend, LegendUnverified) {
			t.Errorf("legend %v does not explain the unverified mark", s.Legend)
		}
	})
	t.Run("confirmed configuration leaves no mark", func(t *testing.T) {
		s := build(t, edgeCase("configuration", ev("x1", "deployed", "connection.configured", "true")), aggregateAll("a", "b"))
		if n := node(t, s, "agg"); len(n.Marks) != 0 {
			t.Errorf("aggregate marks = %+v, want none", n.Marks)
		}
	})
	t.Run("mismatched and unverified relations are counted apart from member state", func(t *testing.T) {
		s := build(t, evidenceCases, aggregateAll("w", "a", "b"))
		want := []Mark{
			{Kind: MarkMismatch, Of: MarkOfElement, Count: 1},
			{Kind: MarkUnverified, Of: MarkOfRelation, Count: 1},
			{Kind: MarkMismatch, Of: MarkOfRelation, Count: 2},
		}
		if n := node(t, s, "agg"); !reflect.DeepEqual(n.Marks, want) {
			t.Errorf("aggregate marks = %+v, want %+v", n.Marks, want)
		}
	})
}

func TestMarkTextTellsRelationStateFromElementState(t *testing.T) {
	for want, m := range map[string]Mark{
		"未照合":       {Kind: MarkUnverified, Count: 1},
		"未照合 2件":    {Kind: MarkUnverified, Count: 2},
		"関係の未照合":    {Kind: MarkUnverified, Of: MarkOfRelation, Count: 1},
		"関係の不一致 2件": {Kind: MarkMismatch, Of: MarkOfRelation, Count: 2},
	} {
		if got := m.Text(); got != want {
			t.Errorf("%+v reads %q, want %q", m, got, want)
		}
	}
}

func TestEvidenceThatDeniesAFactBacksNothing(t *testing.T) {
	hint := "," + ev("h", "hint", "call.implemented", "true")
	for _, c := range []struct {
		name, relation, evidence string
		line                     Line
		label                    string
	}{
		{"call not implemented", "implementation", ev("x1", "code", "call.implemented", "false") + hint, LineDashed, "接続（手がかり）"},
		{"traffic not observed", "observation", ev("x1", "observed", "traffic.observed", "false") + hint, LineDashed, "接続（手がかり）"},
		{"dependency not required", "dependency", ev("x1", "declared", "lifecycle.requires", "false") + hint, LineDashed, "接続（手がかり）"},
		{"connection not configured", "configuration", ev("x1", "declared", "connection.configured", "false") + hint, LineDashed, "接続（手がかり）"},
		{"connection target is another node", "configuration", ev("x1", "declared", "connection.target", `"a"`) + hint, LineDashed, "接続（手がかり）"},
		{"connection target is this edge's node", "configuration", ev("x1", "declared", "connection.target", `"b"`), LineSolid, "接続（未照合）"},
		{"deployed denial does not confirm a declaration", "configuration",
			ev("x1", "declared", "connection.configured", "true") + "," + ev("x2", "deployed", "connection.configured", "false"), LineSolid, "接続（未照合）"},
	} {
		t.Run(c.name, func(t *testing.T) {
			s, err := withEdgeEvidence(t, c.relation, c.evidence)
			if err != nil {
				t.Fatal(err)
			}
			wantEdge(t, s, "e", c.line, c.label)
		})
	}

	t.Run("an edge that its only evidence denies is not drawn", func(t *testing.T) {
		if _, err := withEdgeEvidence(t, "implementation", ev("x1", "code", "call.implemented", "false")); err == nil {
			t.Error("an implementation whose only evidence says it is not implemented was drawn")
		}
	})
}

func oneNode(kind, evidence string) string {
	return `{"schema_version":"0.1","target":{"repository":".","environment":"dev"},"nodes":[
	 {"id":"x","kind":"` + kind + `","provider":"p","service":"s","identity":{},"attributes":{},"evidence":[` + evidence + `]}],"edges":[],"coverage":[]}`
}

func TestSolidFrameNeedsTheFactThatBacksThatKindOfNode(t *testing.T) {
	hint := "," + ev("h", "hint", "existence", "true")
	for _, c := range []struct {
		name, kind, evidence string
		frame                Frame
	}{
		{"resource declared", "resource", ev("x1", "declared", "existence", "true"), FrameSolid},
		{"resource deployed", "resource", ev("x1", "deployed", "existence", "true"), FrameSolid},
		{"resource known only from a role in code", "resource", ev("x1", "code", "role", `"保存"`), FrameDashed},
		{"resource known only from a call in code", "resource", ev("x1", "code", "call.implemented", "true"), FrameDashed},
		{"resource with only a deployed region", "resource", ev("x1", "deployed", "region", `"tokyo"`), FrameDashed},
		{"resource deployed as absent", "resource", ev("x1", "deployed", "existence", "false") + hint, FrameDashed},
		{"service used in code", "service", ev("x1", "code", "call.implemented", "true"), FrameSolid},
		{"service seen in traffic", "service", ev("x1", "observed", "traffic.observed", "true"), FrameSolid},
		{"service known only from a role in code", "service", ev("x1", "code", "role", `"決済"`), FrameDashed},
		{"service only declared", "service", ev("x1", "declared", "existence", "true"), FrameDashed},
		{"service not called", "service", ev("x1", "code", "call.implemented", "false") + hint, FrameDashed},
		{"service not observed", "service", ev("x1", "observed", "traffic.observed", "false") + hint, FrameDashed},
	} {
		t.Run(c.name, func(t *testing.T) {
			s := build(t, oneNode(c.kind, c.evidence), nil)
			if got := node(t, s, "x").Frame; got != c.frame {
				t.Errorf("frame = %s, want %s", got, c.frame)
			}
		})
	}

	t.Run("a node that its only evidence denies is not drawn", func(t *testing.T) {
		m, diags := model.Parse("model.json", []byte(oneNode("resource", ev("x1", "deployed", "existence", "false"))))
		if diags.HasErrors() {
			t.Fatal(diags)
		}
		if _, err := Build(m, &Config{}); err == nil {
			t.Error("a resource whose only evidence says it does not exist was drawn")
		}
	})
}

func evAt(id, kind, validTime string) string {
	at := ""
	if validTime != "" {
		at = `,"valid_time":"` + validTime + `"`
	}
	return `{"id":"` + id + `","kind":"` + kind + `","supports":[{"key":"existence","value":true}],"ref":{"type":"file","path":"wrangler.toml"},"checked_at":"2026-11-13T00:00:00Z"` + at + `}`
}

func TestOnlyASameMomentDeployedExistenceClearsUnverified(t *testing.T) {
	const past = "2025-01-01T00:00:00Z/2025-06-30T00:00:00Z"
	for _, c := range []struct {
		name, declaredAt, deployedAt string
		marks                        []string
	}{
		{"both current", "", "", nil},
		{"both about the same past period", past, past, nil},
		{"current declaration, past existence", "", past, []string{"unverified"}},
		{"past declaration, current existence", past, "", []string{"unverified"}},
	} {
		t.Run(c.name, func(t *testing.T) {
			s := build(t, oneNode("resource", evAt("d", "declared", c.declaredAt)+","+evAt("p", "deployed", c.deployedAt)), nil)
			wantNode(t, s, "x", FrameSolid, c.marks...)
		})
	}

	t.Run("every declaration needs its own moment confirmed", func(t *testing.T) {
		s := build(t, oneNode("resource", evAt("d1", "declared", "")+","+evAt("d2", "declared", past)+","+evAt("p", "deployed", past)), nil)
		wantNode(t, s, "x", FrameSolid, "unverified")
	})
}

func TestSameOwnerIDUnderDifferentProvidersIsTwoBoundaries(t *testing.T) {
	owned := func(id, provider string) string {
		return fmt.Sprintf(`{"id":%q,"kind":"resource","provider":%q,"service":"s","identity":{},"attributes":{},"evidence":[
		 {"id":"ev-%s","kind":"deployed","supports":[{"key":"existence","value":true},{"key":"scope.owner","value":{"kind":"account","id":"acct-1"}}],"ref":{"type":"query","path":"findings/query.json"},"checked_at":"2026-11-13T00:00:00Z"}]}`, id, provider, id)
	}
	twoNodes := func(providerA, providerB string) *Scene {
		return build(t, `{"schema_version":"0.1","target":{"repository":".","environment":"dev"},"nodes":[`+
			owned("a", providerA)+","+owned("b", providerB)+`],"edges":[],"coverage":[]}`, nil)
	}

	same := twoNodes("cloudflare", "cloudflare")
	if len(same.Groups) != 1 || node(t, same, "a").GroupID != node(t, same, "b").GroupID {
		t.Errorf("one provider, one account: groups = %+v, want one shared boundary", same.Groups)
	}

	different := twoNodes("cloudflare", "vercel")
	wantWellFormed(t, different)
	if len(different.Groups) != 2 || node(t, different, "a").GroupID == node(t, different, "b").GroupID {
		t.Errorf("two providers with the same account id: groups = %+v, want two boundaries", different.Groups)
	}
}

func TestOnlyASameMomentDeployedConfigurationConfirmsADeclaration(t *testing.T) {
	const past = "2025-01-01T00:00:00Z/2025-06-30T00:00:00Z"
	configured := func(id, kind, validTime string) string {
		at := ""
		if validTime != "" {
			at = `,"valid_time":"` + validTime + `"`
		}
		return `{"id":"` + id + `","kind":"` + kind + `","supports":[{"key":"connection.configured","value":true}],"ref":{"type":"file","path":"wrangler.toml"},"checked_at":"2026-11-13T00:00:00Z"` + at + `}`
	}
	for _, c := range []struct{ name, declaredAt, deployedAt, label string }{
		{"both current", "", "", "接続"},
		{"current declaration, past deployment", "", past, "接続（未照合）"},
		{"past declaration, current deployment", past, "", "接続（未照合）"},
	} {
		t.Run(c.name, func(t *testing.T) {
			s, err := withEdgeEvidence(t, "configuration", configured("x1", "declared", c.declaredAt)+","+configured("x2", "deployed", c.deployedAt))
			if err != nil {
				t.Fatal(err)
			}
			wantEdge(t, s, "e", LineSolid, c.label)
		})
	}

	s, err := withEdgeEvidence(t, "configuration", configured("x2", "deployed", past))
	if err != nil {
		t.Fatal(err)
	}
	wantEdge(t, s, "e", LineSolid, "接続")
}
