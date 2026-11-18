package view

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/ochamu/ponchimap/internal/model"
)

func build(t *testing.T, modelJSON string, c *Config) *Scene {
	t.Helper()
	m, diags := model.Parse("model.json", []byte(modelJSON))
	if diags.HasErrors() {
		t.Fatalf("parse model: %v", diags)
	}
	if c == nil {
		c = &Config{}
	}
	s, err := Build(m, c)
	if err != nil {
		t.Fatalf("build scene: %v", err)
	}
	return s
}

func loadSmallModel(t *testing.T, name string) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "..", "testdata", "small-models", name))
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

func node(t *testing.T, s *Scene, id string) Node {
	t.Helper()
	for _, n := range s.Nodes {
		if n.ID == id {
			return n
		}
	}
	t.Fatalf("scene has no node %q", id)
	return Node{}
}

func edge(t *testing.T, s *Scene, id string) Edge {
	t.Helper()
	for _, e := range s.Edges {
		for _, src := range e.SourceIDs {
			if src == id {
				return e
			}
		}
	}
	t.Fatalf("scene has no edge for %q", id)
	return Edge{}
}

func markKinds(n Node) []string {
	out := []string{}
	for _, m := range n.Marks {
		out = append(out, string(m.Kind))
	}
	return out
}

func wantNode(t *testing.T, s *Scene, id string, frame Frame, marks ...string) {
	t.Helper()
	n := node(t, s, id)
	if n.Frame != frame {
		t.Errorf("node %s frame = %s, want %s", id, n.Frame, frame)
	}
	if marks == nil {
		marks = []string{}
	}
	if got := markKinds(n); !reflect.DeepEqual(got, marks) {
		t.Errorf("node %s marks = %v, want %v", id, got, marks)
	}
}

func wantEdge(t *testing.T, s *Scene, id string, line Line, label string) {
	t.Helper()
	e := edge(t, s, id)
	if e.Line != line || e.Label != label {
		t.Errorf("edge %s = %s %q, want %s %q", id, e.Line, e.Label, line, label)
	}
}

func TestWorkerAndD1DeclaredButUnverified(t *testing.T) {
	s := build(t, loadSmallModel(t, "worker-and-d1.json"), nil)
	wantNode(t, s, "n001", FrameSolid, "unverified")
	wantNode(t, s, "n002", FrameSolid, "unverified")
	wantEdge(t, s, "e001", LineSolid, "DB接続設定（未照合）")
}

func TestWorkerD1AndMailMixedEvidence(t *testing.T) {
	s := build(t, loadSmallModel(t, "worker-d1-and-mail.json"), nil)
	wantNode(t, s, "n101", FrameSolid)
	wantNode(t, s, "n102", FrameSolid, "unverified")
	wantNode(t, s, "n103", FrameSolid)
	wantEdge(t, s, "e101", LineSolid, "DB接続設定（未照合）")
	wantEdge(t, s, "e102", LineSolid, "利用者データの保存")
	wantEdge(t, s, "e103", LineSolid, "登録確認メールの送信")

	if len(s.Groups) != 1 || s.Groups[0].Kind != GroupBoundary {
		t.Fatalf("groups = %+v, want one real boundary", s.Groups)
	}
	b := s.Groups[0]
	if node(t, s, "n101").GroupID != b.ID || node(t, s, "n102").GroupID != b.ID || node(t, s, "n103").GroupID != "" {
		t.Errorf("boundary membership is wrong: the account holds n101 and n102 only")
	}
	if !reflect.DeepEqual(b.EvidenceIDs, []string{"ev101", "ev102", "ev104"}) {
		t.Errorf("boundary evidence = %v, want the scope.owner evidence of its members", b.EvidenceIDs)
	}
	for _, text := range append([]string{b.ID, b.Label}, s.Groups[0].EvidenceIDs...) {
		if text == "example-account-01" {
			t.Errorf("the raw account ID leaked into the scene as %q", text)
		}
	}
}

const evidenceCases = `{
  "schema_version": "0.1", "target": {"repository": ".", "environment": "dev"},
  "nodes": [
    {"id": "w", "kind": "resource", "provider": "cloudflare", "service": "cloudflare.workers", "identity": {}, "attributes": {},
     "evidence": [{"id": "ev1", "kind": "deployed", "supports": [{"key": "existence", "value": true}], "ref": {"type": "query", "path": "findings/query.json"}, "checked_at": "2026-11-13T00:00:00Z"}]},
    {"id": "a", "kind": "resource", "provider": "cloudflare", "service": "cloudflare.d1", "identity": {}, "attributes": {},
     "evidence": [{"id": "ev2", "kind": "deployed", "supports": [{"key": "existence", "value": true}], "ref": {"type": "query", "path": "findings/query.json"}, "checked_at": "2026-11-13T00:00:00Z"}]},
    {"id": "b", "kind": "resource", "provider": "cloudflare", "service": "cloudflare.d1", "identity": {}, "attributes": {},
     "evidence": [{"id": "ev3", "kind": "deployed", "supports": [{"key": "existence", "value": true}], "ref": {"type": "query", "path": "findings/query.json"}, "checked_at": "2026-11-13T00:00:00Z"}]},
    {"id": "sdk", "kind": "service", "provider": "stripe", "service": "stripe", "identity": {}, "attributes": {},
     "evidence": [{"id": "ev4", "kind": "hint", "supports": [{"key": "existence", "value": true}], "ref": {"type": "file", "path": "package.json"}, "checked_at": "2026-11-13T00:00:00Z"}]}
  ],
  "edges": [
    {"id": "main-declared", "from": "w", "to": "a", "relation": "configuration", "slot": "DB", "attributes": {"purpose": "DB接続設定"},
     "evidence": [{"id": "ev5", "kind": "declared", "supports": [{"key": "connection.target", "value": "a"}], "ref": {"type": "file", "path": "wrangler.toml"}, "checked_at": "2026-11-13T00:00:00Z"}]},
    {"id": "main-deployed", "from": "w", "to": "b", "relation": "configuration", "slot": "DB", "attributes": {"purpose": "DB接続設定"},
     "evidence": [{"id": "ev6", "kind": "deployed", "supports": [{"key": "connection.target", "value": "b"}], "ref": {"type": "query", "path": "findings/query.json"}, "checked_at": "2026-11-13T00:00:00Z"}]},
    {"id": "logs-a", "from": "w", "to": "a", "relation": "configuration", "slot": "LOGS", "attributes": {"purpose": "ログ保存設定"},
     "evidence": [{"id": "ev7", "kind": "deployed", "supports": [{"key": "connection.target", "value": "a"}], "ref": {"type": "query", "path": "findings/query.json"}, "checked_at": "2026-11-13T00:00:00Z"}]},
    {"id": "cache-b", "from": "w", "to": "b", "relation": "configuration", "slot": "CACHE", "attributes": {"purpose": "キャッシュ設定"},
     "evidence": [{"id": "ev8", "kind": "declared", "supports": [{"key": "connection.target", "value": "b"}], "ref": {"type": "file", "path": "wrangler.toml"}, "checked_at": "2026-11-13T00:00:00Z"}]},
    {"id": "sdk-only", "from": "w", "to": "sdk", "relation": "implementation", "attributes": {"purpose": "決済"},
     "evidence": [{"id": "ev9", "kind": "hint", "supports": [{"key": "call.implemented", "value": true}], "ref": {"type": "file", "path": "package.json"}, "checked_at": "2026-11-13T00:00:00Z"}]},
    {"id": "told", "from": "w", "to": "sdk", "relation": "dependency", "attributes": {"purpose": "返金処理"},
     "evidence": [{"id": "ev10", "kind": "stated", "supports": [{"key": "lifecycle.requires", "value": true}], "ref": {"type": "document", "path": "README.md"}, "checked_at": "2026-11-13T00:00:00Z"}]},
    {"id": "seen", "from": "w", "to": "a", "relation": "observation", "attributes": {"purpose": "クエリ"},
     "evidence": [{"id": "ev11", "kind": "observed", "supports": [{"key": "traffic.observed", "value": true}], "ref": {"type": "log", "path": "findings/log.json"}, "checked_at": "2026-11-13T00:00:00Z", "valid_time": "2026-11-12T00:00:00Z/2026-11-13T00:00:00Z"}]}
  ],
  "coverage": []
}`

func TestSlotMismatchIsReportedOnlyForTheSameSlot(t *testing.T) {
	s := build(t, evidenceCases, nil)
	wantEdge(t, s, "main-declared", LineSolid, "DB接続設定（不一致）")
	wantEdge(t, s, "main-deployed", LineSolid, "DB接続設定（不一致）")
	wantEdge(t, s, "logs-a", LineSolid, "ログ保存設定")
	wantEdge(t, s, "cache-b", LineSolid, "キャッシュ設定（未照合）")
	wantNode(t, s, "w", FrameSolid, "mismatch")
}

func TestWeakEvidenceNeverDrawsSolid(t *testing.T) {
	s := build(t, evidenceCases, nil)
	wantNode(t, s, "sdk", FrameDashed)
	wantEdge(t, s, "sdk-only", LineDashed, "決済（手がかり）")
	wantEdge(t, s, "told", LineDashed, "返金処理（説明・未照合）")
}

func TestObservationIsLabelledAsObservation(t *testing.T) {
	s := build(t, evidenceCases, nil)
	wantEdge(t, s, "seen", LineSolid, "クエリ（観測）")
}

func TestAggregateKeepsSourceIDsAndMarks(t *testing.T) {
	s := build(t, loadSmallModel(t, "worker-and-d1.json"), &Config{Aggregates: []AggregateConfig{{ID: "agg", Label: "バックエンド一式", Members: []string{"n001", "n002"}}}})
	n := node(t, s, "agg")
	if !reflect.DeepEqual(n.SourceIDs, []string{"n001", "n002"}) {
		t.Errorf("aggregate sources = %v", n.SourceIDs)
	}
	want := []Mark{{Kind: MarkUnverified, Count: 2}, {Kind: MarkUnverified, Of: MarkOfRelation, Count: 1}}
	if !reflect.DeepEqual(n.Marks, want) {
		t.Errorf("aggregate marks = %+v, want both members and the folded configuration unverified", n.Marks)
	}
	if len(s.Edges) != 0 {
		t.Errorf("an edge between two folded members should not be drawn, got %+v", s.Edges)
	}
}

func TestElementsWithoutEvidenceAreRejected(t *testing.T) {
	m, diags := model.Parse("model.json", []byte(`{"schema_version":"0.1","target":{"repository":".","environment":"dev"},"nodes":[{"id":"x","kind":"service","provider":"p","service":"s","identity":{},"attributes":{},"evidence":[]}],"edges":[],"coverage":[]}`))
	if diags.HasErrors() {
		t.Fatal(diags)
	}
	if _, err := Build(m, &Config{}); err == nil {
		t.Error("a node without evidence was drawn")
	}
}

func TestRelationMustMatchItsEvidence(t *testing.T) {
	m, diags := model.Parse("model.json", []byte(`{"schema_version":"0.1","target":{"repository":".","environment":"dev"},
	"nodes":[
	 {"id":"a","kind":"service","provider":"p","service":"s","identity":{},"attributes":{},"evidence":[{"id":"ev1","kind":"code","supports":[{"key":"call.implemented","value":true}],"ref":{"type":"file","path":"wrangler.toml"},"checked_at":"2026-11-13T00:00:00Z"}]},
	 {"id":"b","kind":"service","provider":"p","service":"s","identity":{},"attributes":{},"evidence":[{"id":"ev2","kind":"code","supports":[{"key":"call.implemented","value":true}],"ref":{"type":"file","path":"wrangler.toml"},"checked_at":"2026-11-13T00:00:00Z"}]}],
	"edges":[{"id":"e","from":"a","to":"b","relation":"implementation","attributes":{},"evidence":[{"id":"ev3","kind":"declared","supports":[{"key":"connection.configured","value":true}],"ref":{"type":"file","path":"wrangler.toml"},"checked_at":"2026-11-13T00:00:00Z"}]}],
	"coverage":[]}`))
	if diags.HasErrors() {
		t.Fatal(diags)
	}
	if _, err := Build(m, &Config{}); err == nil {
		t.Error("a call implementation backed only by a declaration was drawn")
	}
}

type expectedDisplay struct {
	Nodes map[string]struct {
		Frame Frame    `json:"frame"`
		Marks []string `json:"marks"`
	} `json:"nodes"`
	Edges map[string]struct {
		Line  Line   `json:"line"`
		Label string `json:"label"`
	} `json:"edges"`
	Boundaries []struct {
		Label       string   `json:"label"`
		Members     []string `json:"members"`
		ParentLabel string   `json:"parent_label"`
	} `json:"boundaries"`
	LogicalGroups []struct {
		ID      string   `json:"id"`
		Members []string `json:"members"`
	} `json:"logical_groups"`
	Aggregates []struct {
		ID      string         `json:"id"`
		Members []string       `json:"members"`
		Frame   Frame          `json:"frame"`
		Marks   map[string]int `json:"marks"`
	} `json:"aggregates"`
	Hidden []string `json:"hidden"`
}

// The expected displays under testdata/samples were derived by hand, apart
// from the code in derive.go. Per-element expectations are checked on a scene
// built without aggregation or hiding, so that every model element is judged
// on its own.
func TestSampleModelsMatchHandDerivedDisplay(t *testing.T) {
	cases, err := filepath.Glob(filepath.Join("..", "..", "testdata", "samples", "*", "expected.json"))
	if err != nil || len(cases) == 0 {
		t.Fatalf("found no samples: %v", err)
	}
	for _, path := range cases {
		dir := filepath.Dir(path)
		t.Run(filepath.Base(dir), func(t *testing.T) {
			raw, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			var want expectedDisplay
			dec := json.NewDecoder(bytes.NewReader(raw))
			dec.DisallowUnknownFields()
			if err := dec.Decode(&want); err != nil {
				t.Fatalf("expected.json: %v", err)
			}
			m := loadModel(t, filepath.Join(dir, "model.json"))
			path := filepath.Join(dir, "view.yaml")
			file, diags := ParseConfig(path, readFile(t, path))
			if diags.HasErrors() {
				t.Fatal(diags)
			}
			cfg, diags := file.Resolve(m)
			if len(diags) > 0 {
				t.Fatal(diags)
			}

			plain := *cfg
			plain.Aggregates, plain.Hidden = nil, nil
			each, err := Build(m, &plain)
			if err != nil {
				t.Fatalf("build without aggregation: %v", err)
			}
			if len(want.Nodes) != len(m.Nodes) || len(want.Edges) != len(m.Edges) {
				t.Errorf("expected.json lists %d nodes and %d edges; the model has %d and %d", len(want.Nodes), len(want.Edges), len(m.Nodes), len(m.Edges))
			}
			for id, w := range want.Nodes {
				wantNode(t, each, id, w.Frame, w.Marks...)
			}
			for id, w := range want.Edges {
				wantEdge(t, each, id, w.Line, w.Label)
			}

			full, err := Build(m, cfg)
			if err != nil {
				t.Fatalf("build with the full view config: %v", err)
			}
			wantWellFormed(t, each)
			wantWellFormed(t, full)
			members := map[string][]string{}
			for _, n := range each.Nodes {
				for g := n.GroupID; g != ""; g = parentOf(each, g) {
					members[g] = append(members[g], n.ID)
				}
			}
			var gotBoundaries, wantBoundaries []string
			for _, g := range each.Groups {
				if g.Kind == GroupBoundary {
					sort.Strings(members[g.ID])
					gotBoundaries = append(gotBoundaries, canonical([]any{g.Label, members[g.ID], labelOf(each, g.ParentID)}))
				}
			}
			for _, b := range want.Boundaries {
				sort.Strings(b.Members)
				wantBoundaries = append(wantBoundaries, canonical([]any{b.Label, b.Members, b.ParentLabel}))
			}
			sort.Strings(gotBoundaries)
			sort.Strings(wantBoundaries)
			if !reflect.DeepEqual(gotBoundaries, wantBoundaries) {
				t.Errorf("boundaries\n got  %v\n want %v", gotBoundaries, wantBoundaries)
			}
			gotLogical, wantLogical := map[string][]string{}, map[string][]string{}
			for _, g := range each.Groups {
				if g.Kind == GroupLogical {
					sort.Strings(members[g.ID])
					gotLogical[g.ID] = members[g.ID]
				}
			}
			for _, g := range want.LogicalGroups {
				sort.Strings(g.Members)
				wantLogical[g.ID] = g.Members
			}
			if !reflect.DeepEqual(gotLogical, wantLogical) {
				t.Errorf("logical groups\n got  %v\n want %v", gotLogical, wantLogical)
			}
			for _, a := range want.Aggregates {
				n := node(t, full, a.ID)
				gotMarks := map[string]int{}
				for _, mk := range n.Marks {
					gotMarks[string(mk.Kind)] = mk.Count
				}
				if a.Marks == nil {
					a.Marks = map[string]int{}
				}
				if n.Frame != a.Frame || !reflect.DeepEqual(gotMarks, a.Marks) || !reflect.DeepEqual(n.SourceIDs, a.Members) {
					t.Errorf("aggregate %s = %s %v %v, want %s %v %v", a.ID, n.Frame, gotMarks, n.SourceIDs, a.Frame, a.Marks, a.Members)
				}
			}
			for _, id := range want.Hidden {
				for _, n := range full.Nodes {
					for _, src := range n.SourceIDs {
						if src == id {
							t.Errorf("hidden element %s is still drawn", id)
						}
					}
				}
			}
		})
	}
}

func parentOf(s *Scene, id string) string {
	for _, g := range s.Groups {
		if g.ID == id {
			return g.ParentID
		}
	}
	return ""
}

func labelOf(s *Scene, id string) string {
	for _, g := range s.Groups {
		if g.ID == id {
			return g.Label
		}
	}
	return ""
}

// edgeCase is two deployed resources, a and b, joined by one edge e from a to
// b with the given evidence.
func edgeCase(relation, evidence string) string {
	return `{"schema_version":"0.1","target":{"repository":".","environment":"dev"},
	"nodes":[
	 {"id":"a","kind":"resource","provider":"p","service":"s","identity":{},"attributes":{},"evidence":[{"id":"n1","kind":"deployed","supports":[{"key":"existence","value":true}],"ref":{"type":"query","path":"findings/query.json"},"checked_at":"2026-11-13T00:00:00Z"}]},
	 {"id":"b","kind":"resource","provider":"p","service":"s","identity":{},"attributes":{},"evidence":[{"id":"n2","kind":"deployed","supports":[{"key":"existence","value":true}],"ref":{"type":"query","path":"findings/query.json"},"checked_at":"2026-11-13T00:00:00Z"}]}],
	"edges":[{"id":"e","from":"a","to":"b","relation":"` + relation + `","slot":"DB","attributes":{"purpose":"接続"},"evidence":[` + evidence + `]}],
	"coverage":[]}`
}

func withEdgeEvidence(t *testing.T, relation, evidence string) (*Scene, error) {
	t.Helper()
	m, diags := model.Parse("model.json", []byte(edgeCase(relation, evidence)))
	if diags.HasErrors() {
		t.Fatal(diags)
	}
	return Build(m, &Config{})
}

func ev(id, kind, key, value string) string {
	return `{"id":"` + id + `","kind":"` + kind + `","supports":[{"key":"` + key + `","value":` + value + `}],"ref":{"type":"file","path":"wrangler.toml"},"checked_at":"2026-11-13T00:00:00Z"}`
}

func TestUnknownSupportKeyNeverDrawsSolid(t *testing.T) {
	s, err := withEdgeEvidence(t, "implementation", ev("x1", "code", "vendor.custom", "true"))
	if err != nil {
		t.Fatal(err)
	}
	wantEdge(t, s, "e", LineDashed, "接続（手がかり）")

	s = build(t, `{"schema_version":"0.1","target":{"repository":".","environment":"dev"},"nodes":[
	 {"id":"x","kind":"service","provider":"p","service":"s","identity":{},"attributes":{},"evidence":[`+ev("x2", "code", "vendor.custom", "true")+`]}],"edges":[],"coverage":[]}`, nil)
	wantNode(t, s, "x", FrameDashed)
}

func TestDeployedEvidenceForAnotherFactDoesNotVerifyAConnection(t *testing.T) {
	s, err := withEdgeEvidence(t, "configuration", ev("x1", "declared", "connection.configured", "true")+","+ev("x2", "deployed", "region", `"apac"`))
	if err != nil {
		t.Fatal(err)
	}
	wantEdge(t, s, "e", LineSolid, "接続（未照合）")
}

const regionAtTimes = `{"schema_version":"0.1","target":{"repository":".","environment":"dev"},"nodes":[
 {"id":"x","kind":"resource","provider":"p","service":"s","identity":{},"attributes":{},"evidence":[
  {"id":"d","kind":"declared","supports":[{"key":"existence","value":true},{"key":"region","value":"tokyo"}],"ref":{"type":"file","path":"wrangler.toml"},"checked_at":"2026-11-13T00:00:00Z"%s},
  {"id":"p","kind":"deployed","supports":[{"key":"existence","value":true},{"key":"region","value":"osaka"}],"ref":{"type":"query","path":"findings/query.json"},"checked_at":"2026-11-13T00:00:00Z"}]}],"edges":[],"coverage":[]}`

func TestDifferentValuesAtDifferentTimesAreNotAMismatch(t *testing.T) {
	sameTime := build(t, fmt.Sprintf(regionAtTimes, ""), nil)
	wantNode(t, sameTime, "x", FrameSolid, "mismatch")

	// An old plan cannot be compared with the current state, so it is neither
	// a mismatch nor verified.
	oldPlan := build(t, fmt.Sprintf(regionAtTimes, `,"valid_time":"2025-01-01T00:00:00Z/2025-06-30T00:00:00Z"`), nil)
	wantNode(t, oldPlan, "x", FrameSolid, "unverified")
}

func TestEdgesFoldedIntoAnAggregateStayTraceable(t *testing.T) {
	s := build(t, loadSmallModel(t, "worker-and-d1.json"), &Config{Aggregates: []AggregateConfig{{ID: "agg", Label: "バックエンド一式", Members: []string{"n001", "n002"}}}})
	if got := node(t, s, "agg").FoldedEdgeIDs; !reflect.DeepEqual(got, []string{"e001"}) {
		t.Errorf("folded edges = %v, want [e001]", got)
	}
}

// The direction is copied into the scene, so Build accepts only the values a
// loaded view config can hold.
func TestADirectionThatNamesNoLayoutIsRefused(t *testing.T) {
	_, err := buildErr(t, modelOf(deployedNode("x", "cloudflare", ""), ""), &Config{Layout: LayoutConfig{Direction: "sideways"}})
	if err == nil || !strings.Contains(err.Error(), "layout.direction") {
		t.Errorf("error = %v, want one about layout.direction", err)
	}
}

// The tool knows no service: a card takes its name and its icon from what the
// findings wrote on the node, whatever the key.
func TestACardIsNamedAndDrawnByItsFindings(t *testing.T) {
	for _, c := range []struct{ service, category, name, title, icon string }{
		{"cloudflare.workers", "", "", "cloudflare.workers", "generic"},
		{"cloudflare.workers", "compute", "Cloudflare Workers", "Cloudflare Workers", "compute"},
		{"cloudflare.d1", "storage", "Probe Database", "Probe Database", "storage"},
		{"resend", "", "", "resend", "generic"},
		{"acme.queue", "", "", "acme.queue", "generic"},
		{"aws.lambda", "compute", "", "aws.lambda", "compute"},
		{"aws.lambda", "", "AWS Lambda", "AWS Lambda", "generic"},
		{"aws.lambda", "compute", "AWS Lambda", "AWS Lambda", "compute"},
	} {
		t.Run(c.service+"/"+c.category+"/"+c.name, func(t *testing.T) {
			s := build(t, modelOf(cardNode("x", c.service, c.category, c.name), ""), nil)
			if n := node(t, s, "x"); n.Title != c.title || n.IconKey != c.icon {
				t.Errorf("card = %q with icon %q, want %q with icon %q", n.Title, n.IconKey, c.title, c.icon)
			}
		})
	}
}

func cardNode(id, service, category, name string) string {
	attributes := `"role":"処理"`
	if category != "" {
		attributes += fmt.Sprintf(`,"category":%q`, category)
	}
	if name != "" {
		attributes += fmt.Sprintf(`,"service_name":%q`, name)
	}
	return fmt.Sprintf(`{"id":%q,"kind":"resource","provider":"p","service":%q,"attributes":{%s},"evidence":[{"id":"ev-%s","kind":"deployed","supports":[{"key":"existence","value":true}],"ref":{"type":"query","path":"findings/query.json"},"checked_at":"2026-11-13T00:00:00Z"}]}`, id, service, attributes, id)
}

func TestAnAggregateOfNodesOfOneCategoryIsDrawnWithItsIcon(t *testing.T) {
	for _, c := range []struct{ name, service, category, icon string }{
		{"same category", "gcp.pubsub", "queue", "queue"},
		{"two categories", "aws.s3", "storage", "generic"},
	} {
		t.Run(c.name, func(t *testing.T) {
			nodes := cardNode("a", "aws.sqs", "queue", "") + "," + cardNode("b", c.service, c.category, "")
			s := build(t, modelOf(nodes, ""), &Config{Aggregates: []AggregateConfig{{ID: "agg", Label: "キュー", Members: []string{"a", "b"}}}})
			if n := node(t, s, "agg"); n.IconKey != c.icon {
				t.Errorf("aggregate icon = %q, want %q", n.IconKey, c.icon)
			}
		})
	}
}
