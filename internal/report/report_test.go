package report_test

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"
	"testing"

	"github.com/ochamu/ponchimap/internal/model"
	"github.com/ochamu/ponchimap/internal/report"
	"github.com/ochamu/ponchimap/internal/view"
)

var reportSections = []string{"概要", "構成要素", "関係とデータの流れ", "食い違い", "未確認の点", "図に載せなかったもの", "調査範囲", "再描画"}

// stateSections say what the evidence and the coverage tell. They are read
// from the model alone, so no view config can change them.
var stateSections = []string{"食い違い", "未確認の点", "調査範囲"}

// expected is the hand-derived display kept beside each sample model. The
// tests read it as the independent statement of what the figure shows.
type expected struct {
	Nodes map[string]struct {
		Frame string   `json:"frame"`
		Marks []string `json:"marks"`
	} `json:"nodes"`
	Edges map[string]struct {
		Line  string `json:"line"`
		Label string `json:"label"`
	} `json:"edges"`
	Aggregates []struct {
		ID      string   `json:"id"`
		Members []string `json:"members"`
	} `json:"aggregates"`
	Hidden []string `json:"hidden"`
}

type fixture struct {
	name  string
	model *model.Model
	scene *view.Scene
	want  *expected
}

func (f fixture) markdown() string {
	return string(report.Build(f.model, f.scene, report.Drawing{}).Markdown())
}

func buildScene(t *testing.T, m *model.Model, c *view.Config) *view.Scene {
	t.Helper()
	s, err := view.Build(m, c)
	if err != nil {
		t.Fatalf("build scene: %v", err)
	}
	return s
}

func readFile(t *testing.T, path string) []byte {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func loadModel(t *testing.T, path string) *model.Model {
	t.Helper()
	m, diags := model.Parse(path, readFile(t, path))
	if diags.HasErrors() {
		t.Fatal(diags)
	}
	return m
}

func loadConfig(t *testing.T, path string, m *model.Model) *view.Config {
	t.Helper()
	file, diags := view.ParseConfig(path, readFile(t, path))
	if diags.HasErrors() {
		t.Fatal(diags)
	}
	c, diags := file.Resolve(m)
	if len(diags) > 0 {
		t.Fatal(diags)
	}
	return c
}

func loadSmallModel(t *testing.T, name string) fixture {
	t.Helper()
	m := loadModel(t, filepath.Join("..", "..", "testdata", "small-models", name))
	return fixture{name: name, model: m, scene: buildScene(t, m, &view.Config{})}
}

func loadSample(t *testing.T, name string) fixture {
	t.Helper()
	dir := filepath.Join("..", "..", "testdata", "samples", name)
	m := loadModel(t, filepath.Join(dir, "model.json"))
	raw, err := os.ReadFile(filepath.Join(dir, "expected.json"))
	if err != nil {
		t.Fatal(err)
	}
	var want expected
	if err := json.Unmarshal(raw, &want); err != nil {
		t.Fatalf("%s expected.json: %v", name, err)
	}
	return fixture{name: name, model: m, scene: buildScene(t, m, loadConfig(t, filepath.Join(dir, "view.yaml"), m)), want: &want}
}

func samples(t *testing.T) []fixture {
	t.Helper()
	dirs, _ := filepath.Glob(filepath.Join("..", "..", "testdata", "samples", "*", "model.json"))
	if len(dirs) < 5 {
		t.Fatalf("found %d sample models, want the five samples at least", len(dirs))
	}
	var out []fixture
	for _, d := range dirs {
		out = append(out, loadSample(t, filepath.Base(filepath.Dir(d))))
	}
	return out
}

const mixedModel = `{
  "schema_version": "0.1",
  "target": {"repository": ".", "environment": "dev"},
  "nodes": [
    {"id": "na", "kind": "resource", "provider": "cloudflare", "service": "cloudflare.kv",
     "identity": {"account_id": "raw-account-777", "native_id": "raw-kv-one"},
     "attributes": {"role": "下書きを保存する"},
     "evidence": [{"id": "eva", "kind": "deployed", "supports": [
        {"key": "existence", "value": true},
        {"key": "role", "value": "下書きを保存する"},
        {"key": "scope.owner", "value": {"kind": "account", "id": "raw-account-777"}},
        {"key": "scope.network", "value": "raw-network-555"},
        {"key": "x.vendor.tag", "value": "raw-extension-333"},
        {"key": "x.vendor.detail", "value": {"token": "raw-nested-888", "ports": [844388, "raw-listed-111"]}}],
       "ref": {"type": "query", "path": "findings/a.json"}, "checked_at": "2026-11-13T00:00:00Z"}]},
    {"id": "nb", "kind": "resource", "provider": "cloudflare", "service": "cloudflare.kv",
     "identity": {"account_id": "raw-account-777", "native_id": "raw-kv-two"},
     "attributes": {},
     "evidence": [{"id": "evb", "kind": "deployed", "supports": [
        {"key": "existence", "value": true},
        {"key": "scope.owner", "value": {"kind": "account", "id": "raw-account-777"}},
        {"key": "scope.network", "value": "raw-network-555"}],
       "ref": {"type": "query", "path": "findings/b.json"}, "checked_at": "2026-11-13T00:00:00Z"}]},
    {"id": "nc", "kind": "service", "provider": "acme", "service": "acme.mail", "identity": {}, "attributes": {"service_name": "Acme _Mail_ &copy; &#65;"},
     "evidence": [{"id": "evc", "kind": "code", "supports": [{"key": "call.implemented", "value": true}],
       "ref": {"type": "file", "path": "src/c.ts", "start_line": 3, "end_line": 9}, "checked_at": "2026-11-13T00:00:00Z"}]},
    {"id": "nd", "kind": "service", "provider": "stripe", "service": "stripe", "identity": {}, "attributes": {},
     "evidence": [{"id": "evd", "kind": "code", "supports": [{"key": "call.implemented", "value": true}],
       "ref": {"type": "file", "path": "src/d.ts"}, "checked_at": "2026-11-13T00:00:00Z"}]}
  ],
  "edges": [
    {"id": "e1", "from": "na", "to": "nb", "relation": "configuration", "slot": "COPY", "attributes": {},
     "evidence": [{"id": "ev1", "kind": "declared", "supports": [{"key": "connection.configured", "value": true}, {"key": "connection.target", "value": "nb"},
        {"key": "env.SESSION_SEED", "value": "raw-seed-4f9a2c71"}],
       "ref": {"type": "file", "path": "wrangler.toml"}, "checked_at": "2026-11-13T00:00:00Z"}]},
    {"id": "e2", "from": "nc", "to": "na", "relation": "implementation", "attributes": {"purpose": "保存する"},
     "evidence": [{"id": "ev2", "kind": "code", "supports": [{"key": "call.implemented", "value": true}],
       "ref": {"type": "file", "path": "src/c.ts"}, "checked_at": "2026-11-13T00:00:00Z"}]},
    {"id": "e3", "from": "nc", "to": "nb", "relation": "implementation", "attributes": {"purpose": "保存する"},
     "evidence": [{"id": "ev3", "kind": "code", "supports": [{"key": "call.implemented", "value": true}],
       "ref": {"type": "file", "path": "src/c.ts"}, "checked_at": "2026-11-13T00:00:00Z"}]},
    {"id": "e4", "from": "nc", "to": "nd", "relation": "implementation", "attributes": {},
     "evidence": [{"id": "ev4", "kind": "code", "supports": [{"key": "call.implemented", "value": true}],
       "ref": {"type": "file", "path": "src/c.ts"}, "checked_at": "2026-11-13T00:00:00Z"}]},
    {"id": "e5", "from": "nc", "to": "na", "relation": "observation", "attributes": {},
     "evidence": [{"id": "ev5", "kind": "observed", "supports": [{"key": "traffic.observed", "value": true}],
       "ref": {"type": "log", "path": "findings/log.json"}, "checked_at": "2026-11-13T00:00:00Z"}]}
  ],
  "coverage": [
    {"id": "cv1", "scope": {"provider": "cloudflare", "account_id": "raw-account-777", "target_ids": ["na"], "cluster": "raw-target-999"},
     "status": "denied", "reason": "照会する権限がない", "checked_at": "2026-11-13T00:00:00Z"},
    {"id": "cv2", "scope": {"provider": "cloudflare", "target_ids": ["nb"]},
     "status": "complete", "reason": "取得完了の記録", "checked_at": "2026-11-13T00:00:00Z"}
  ]
}`

// mixed folds na and nb into one aggregate, which folds e1 inside it and
// makes e3 share the line of e2, and hides nd and e5, which leaves e4
// without one of its ends.
func mixed(t *testing.T) fixture {
	t.Helper()
	m, diags := model.Parse("model.json", []byte(mixedModel))
	if diags.HasErrors() {
		t.Fatal(diags)
	}
	s := buildScene(t, m, &view.Config{
		Aggregates: []view.AggregateConfig{{ID: "agg-store", Label: "保存先", Members: []string{"na", "nb"}}},
		Hidden:     []string{"nd", "e5"},
	})
	if len(s.Nodes) != 2 || len(s.Edges) != 1 {
		t.Fatalf("fixture premise: scene has %d nodes and %d edges, want the aggregate, nc, and one shared line", len(s.Nodes), len(s.Edges))
	}
	return fixture{name: "mixed", model: m, scene: s}
}

func fixtures(t *testing.T) []fixture {
	t.Helper()
	out := []fixture{loadSmallModel(t, "worker-and-d1.json"), loadSmallModel(t, "worker-d1-and-mail.json")}
	return append(append(out, samples(t)...), mixed(t))
}

// finding is one line of a .want file of testdata/marks: a state that was
// derived by hand from the rules of view.Assess, with the elements, evidence
// and coverage records it involves.
type finding struct {
	code                      string
	about, evidence, coverage []string
}

func (f finding) ids() []string { return slices.Concat(f.about, f.evidence, f.coverage) }

// where is the section that reports the finding, and the state its entry is
// labelled with.
func (f finding) where() (heading, label string) {
	if strings.HasPrefix(f.code, "mismatch.") {
		return "食い違い", view.MarkMismatch.Label()
	}
	return "未確認の点", view.MarkUnverified.Label()
}

type markCase struct {
	name     string
	model    *model.Model
	findings []finding
}

// markCases are the models of testdata/marks that can be drawn, each with
// the findings derived for it by hand.
func markCases(t *testing.T) []markCase {
	t.Helper()
	paths, _ := filepath.Glob(filepath.Join("..", "..", "testdata", "marks", "*.json"))
	if len(paths) < 60 {
		t.Fatalf("found %d cases under testdata/marks, want 60 at least", len(paths))
	}
	list := func(field string) []string {
		if field == "-" {
			return nil
		}
		return strings.Split(field, ",")
	}
	var out []markCase
	for _, path := range paths {
		raw, err := os.ReadFile(strings.TrimSuffix(path, ".json") + ".want")
		if err != nil {
			t.Fatal(err)
		}
		c := markCase{name: strings.TrimSuffix(filepath.Base(path), ".json"), model: loadModel(t, path)}
		defect := false
		for _, line := range strings.Split(strings.TrimSpace(string(raw)), "\n") {
			if line == "" {
				continue
			}
			fields := strings.Split(line, "\t")
			if len(fields) != 6 {
				t.Fatalf("%s.want: a line has %d fields, want 6", c.name, len(fields))
			}
			defect = defect || strings.HasPrefix(fields[0], "model.")
			c.findings = append(c.findings, finding{code: fields[0], about: list(fields[1]), evidence: list(fields[4]), coverage: list(fields[5])})
		}
		if !defect {
			out = append(out, c)
		}
	}
	return out
}

func headings(md string) []string {
	var out []string
	for _, line := range strings.Split(md, "\n") {
		if heading, ok := strings.CutPrefix(line, "## "); ok {
			out = append(out, heading)
		}
	}
	return out
}

// sectionText is what a reader sees under one section heading.
func sectionText(t *testing.T, md, heading string) string {
	t.Helper()
	_, rest, found := strings.Cut(md, "\n## "+heading+"\n")
	if !found {
		t.Fatalf("the report has no section %q", heading)
	}
	text, _, _ := strings.Cut(rest, "\n## ")
	return text
}

func anchorTag(id string) string { return `<a id="` + id + `"></a>` }

// itemText is what a reader sees under the heading of one element: the
// Markdown from its anchor up to the next heading anchor or section. The
// anchor of an item stands on a line of its own.
func itemText(t *testing.T, md, id string) string {
	t.Helper()
	_, rest, found := strings.Cut(md, "\n"+anchorTag(id)+"\n")
	if !found {
		t.Fatalf("the report has no item for %s", id)
	}
	end := len(rest)
	for _, stop := range []string{"\n<a id=", "\n## "} {
		if i := strings.Index(rest, stop); i >= 0 && i < end {
			end = i
		}
	}
	return rest[:end]
}

// entries splits text into its top-level list entries, each with the lines
// nested under it.
func entries(text string) []string {
	var out []string
	for _, line := range strings.Split(text, "\n") {
		switch {
		case strings.HasPrefix(line, "- "):
			out = append(out, line)
		case strings.HasPrefix(line, " ") && len(out) > 0:
			out[len(out)-1] += "\n" + line
		}
	}
	return out
}

var linkTarget = regexp.MustCompile(`\]\(#([^)]+)\)`)

// subject is the element or record an entry is about: the first one it links
// to.
func subject(entry string) string {
	if m := linkTarget.FindStringSubmatch(entry); m != nil {
		return m[1]
	}
	return ""
}

func subjects(text string) []string {
	var out []string
	for _, e := range entries(text) {
		if id := subject(e); id != "" {
			out = append(out, id)
		}
	}
	return sorted(out)
}

var ownAnchor = regexp.MustCompile(`^- <a id="([^"]+)"></a>`)

// labelled returns the entries that open with the label. An entry may hold,
// before its label, the anchor that other places link to it by.
func labelled(text, label string) []string {
	var out []string
	for _, e := range entries(text) {
		if strings.HasPrefix(ownAnchor.ReplaceAllString(e, "- "), "- "+label) {
			out = append(out, e)
		}
	}
	return out
}

// preamble is what stands between the title and the first section.
func preamble(md string) string {
	text, _, _ := strings.Cut(md, "\n## ")
	return text
}

// mentions reports whether text names id as a whole identifier, so that ev1
// is not found inside ev10.
func mentions(text, id string) bool {
	return regexp.MustCompile(`(^|[^0-9A-Za-z_-])` + regexp.QuoteMeta(id) + `($|[^0-9A-Za-z_-])`).MatchString(text)
}

func mentionsAll(text string, ids []string) bool {
	return !slices.ContainsFunc(ids, func(id string) bool { return !mentions(text, id) })
}

func sorted(ids []string) []string {
	out := slices.Clone(ids)
	sort.Strings(out)
	return out
}

func TestReportHasTheEightSectionsInOrder(t *testing.T) {
	for _, f := range fixtures(t) {
		if got := headings(f.markdown()); !slices.Equal(got, reportSections) {
			t.Errorf("%s: sections are %v, want %v", f.name, got, reportSections)
		}
	}
}

func TestEveryModelElementAndGroupHasExactlyOneItem(t *testing.T) {
	for _, f := range fixtures(t) {
		t.Run(f.name, func(t *testing.T) {
			r := report.Build(f.model, f.scene, report.Drawing{})
			md := string(r.Markdown())
			home := map[string]string{}
			for _, n := range f.model.Nodes {
				home[n.ID] = "構成要素"
			}
			for _, g := range f.scene.Groups {
				home[g.ID] = "構成要素"
			}
			for _, e := range f.model.Edges {
				home[e.ID] = "関係とデータの流れ"
			}
			for id, heading := range home {
				if n := strings.Count(md, anchorTag(id)); n != 1 {
					t.Errorf("%s is a link target %d times, want once", id, n)
				}
				if !strings.Contains(sectionText(t, md, heading), "\n"+anchorTag(id)+"\n") {
					t.Errorf("the item of %s is not in the section %q", id, heading)
				}
			}
			if items := strings.Count(sectionText(t, md, "構成要素"), "\n### ") + strings.Count(sectionText(t, md, "関係とデータの流れ"), "\n### "); items != len(home) {
				t.Errorf("the report has %d items for %d model elements and groups", items, len(home))
			}
		})
	}
}

func TestEachItemListsEveryEvidenceOfItsElement(t *testing.T) {
	for _, f := range fixtures(t) {
		t.Run(f.name, func(t *testing.T) {
			md := f.markdown()
			want := map[string][]string{}
			for _, n := range f.model.Nodes {
				for _, ev := range n.Evidence {
					want[n.ID] = append(want[n.ID], ev.ID)
				}
			}
			for _, e := range f.model.Edges {
				for _, ev := range e.Evidence {
					want[e.ID] = append(want[e.ID], ev.ID)
				}
			}
			for id, evidence := range want {
				text := itemText(t, md, id)
				for _, ev := range evidence {
					if !strings.Contains(text, anchorTag(ev)) {
						t.Errorf("the item of %s does not hold evidence %s", id, ev)
					}
					if n := strings.Count(md, anchorTag(ev)); n != 1 {
						t.Errorf("evidence %s is a link target %d times, want once", ev, n)
					}
				}
			}
			for _, g := range f.scene.Groups {
				if text := itemText(t, md, g.ID); !mentionsAll(text, g.EvidenceIDs) {
					t.Errorf("the item of %s does not name all of its evidence %v", g.ID, g.EvidenceIDs)
				}
			}
		})
	}
}

func TestEveryLinkLeadsSomewhere(t *testing.T) {
	for _, f := range fixtures(t) {
		md := f.markdown()
		for _, m := range linkTarget.FindAllStringSubmatch(md, -1) {
			if !strings.Contains(md, anchorTag(m[1])) {
				t.Errorf("%s: a link points at #%s, which the report does not hold", f.name, m[1])
			}
		}
	}
}

// The findings of testdata/marks were derived by hand from the rules of
// view.Assess.
// Each one must have an entry of its own that names every element, evidence
// and coverage record it involves, and the report must claim no state beyond
// them.
func TestEveryFindingDerivedByHandHasItsEntry(t *testing.T) {
	for _, c := range markCases(t) {
		t.Run(c.name, func(t *testing.T) {
			md := string(report.Build(c.model, buildScene(t, c.model, &view.Config{}), report.Drawing{}).Markdown())
			want := map[string]int{}
			for _, f := range c.findings {
				heading, label := f.where()
				want[heading]++
				if !slices.ContainsFunc(labelled(sectionText(t, md, heading), label), func(e string) bool { return mentionsAll(e, f.ids()) }) {
					t.Errorf("no %q entry of %q names all of %v (%s)", label, heading, f.ids(), f.code)
				}
			}
			for heading, label := range map[string]string{"食い違い": view.MarkMismatch.Label(), "未確認の点": view.MarkUnverified.Label()} {
				if got := len(labelled(sectionText(t, md, heading), label)); got != want[heading] {
					t.Errorf("%q has %d entries labelled %q, want %d", heading, got, label, want[heading])
				}
			}
		})
	}
}

func TestHidingOrFoldingAnElementChangesNoStateSection(t *testing.T) {
	for _, tc := range []struct {
		name                 string
		model                *model.Model
		node, edge, neighbor string
	}{
		{"unverified with denied coverage", mixed(t).model, "na", "e1", "nb"},
		{"slot mismatch", loadModel(t, "../../testdata/marks/slot-declared-a-deployed-b.json"), "w", "db-declared", "a"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := fixture{model: tc.model, scene: buildScene(t, tc.model, &view.Config{})}
			plain := f.markdown()
			for change, cfg := range map[string]*view.Config{
				"hide node": {Hidden: []string{tc.node}},
				"hide edge": {Hidden: []string{tc.edge}},
				"fold pair": {Aggregates: []view.AggregateConfig{{ID: "pair", Label: "まとめ", Members: []string{tc.node, tc.neighbor}}}},
			} {
				f.scene = buildScene(t, tc.model, cfg)
				md := f.markdown()
				for _, heading := range stateSections {
					if sectionText(t, md, heading) != sectionText(t, plain, heading) {
						t.Errorf("%s changed the %q section", change, heading)
					}
				}
			}
		})
	}
}

// A slot whose declared and deployed targets differ, with its source hidden:
// the binding that disagrees leaves the figure with both of its relations,
// and the report still tells of it in every place a reader may look.
func TestMismatchOfAHiddenElementStaysInTheReport(t *testing.T) {
	m := loadModel(t, filepath.Join("..", "..", "testdata", "marks", "slot-declared-a-deployed-b.json"))
	md := string(report.Build(m, buildScene(t, m, &view.Config{Hidden: []string{"w"}}), report.Drawing{}).Markdown())
	label := view.MarkMismatch.Label()
	involved := []string{"w", "db-declared", "db-deployed"}
	if got := labelled(sectionText(t, md, "食い違い"), label); len(got) != 1 || !mentionsAll(got[0], involved) {
		t.Errorf("the mismatch entries are %q, want one that names %v", got, involved)
	}
	omitted := entries(sectionText(t, md, "図に載せなかったもの"))
	for _, id := range involved {
		if !strings.Contains(itemText(t, md, id), label) {
			t.Errorf("the item of %s does not say %q", id, label)
		}
		if !slices.ContainsFunc(omitted, func(e string) bool { return subject(e) == id && strings.Contains(e, label) }) {
			t.Errorf("no entry of the omitted section is about %s and says %q", id, label)
		}
	}
}

// In worker-d1-and-mail.json the D1 n102 and the binding e101 stay unverified,
// nothing disagrees, and the complete coverage cv101 does not stand in for
// the two records that are not complete.
func TestReportOfWorkerD1AndMail(t *testing.T) {
	f := loadSmallModel(t, "worker-d1-and-mail.json")
	md := f.markdown()

	overview := sectionText(t, md, "概要")
	for _, want := range []string{"dev", "2026-11-13T09:00:00Z", "2026-11-13T09:03:00Z"} {
		if !strings.Contains(overview, want) {
			t.Errorf("the overview does not say %q", want)
		}
	}

	if got := labelled(sectionText(t, md, "食い違い"), view.MarkMismatch.Label()); len(got) != 0 {
		t.Errorf("the mismatch entries are %q, want none", got)
	}

	unconfirmed := sectionText(t, md, "未確認の点")
	unverified := labelled(unconfirmed, view.MarkUnverified.Label())
	var about []string
	for _, e := range unverified {
		about = append(about, subject(e))
	}
	if want := []string{"e101", "n102"}; !slices.Equal(sorted(about), want) {
		t.Errorf("the unverified entries are about %v, want %v", sorted(about), want)
	}
	for id, reason := range map[string][]string{
		"n102": {"ev104", "cv103", "対象D1の照会権限がなく、存在を確認できなかった"},
		"e101": {"ev108", "cv102", "DBバインディングの実環境設定は照会していない"},
	} {
		for _, e := range unverified {
			if subject(e) != id {
				continue
			}
			for _, want := range reason {
				if !strings.Contains(e, want) {
					t.Errorf("the unverified entry of %s does not say %q", id, want)
				}
			}
		}
	}
	// cv102 and cv103 are not complete, and each already stands under the
	// unverified entry it applies to.
	if got := subjectsOutside(unconfirmed, unverified); len(got) != 0 {
		t.Errorf("the other unconfirmed entries are about %v, want none", got)
	}
	for node, coverage := range map[string][]string{"n101": {"cv101", "cv102"}, "n102": {"cv103"}} {
		if !mentionsAll(itemText(t, md, node), coverage) {
			t.Errorf("the item of %s does not name the coverage records %v that list it as a target", node, coverage)
		}
	}

	scope := sectionText(t, md, "調査範囲")
	for id, status := range map[string]string{"cv101": "complete", "cv102": "not_attempted", "cv103": "denied"} {
		found := false
		for _, e := range entries(scope) {
			if strings.Contains(e, anchorTag(id)) {
				found = true
				if !strings.Contains(readAs(e), status) {
					t.Errorf("the coverage entry of %s does not say %q", id, status)
				}
			}
		}
		if !found {
			t.Errorf("the coverage section has no entry for %s", id)
		}
	}
	// aq101 is the acquisition behind both cv101 and ev102.
	for _, e := range entries(scope) {
		if strings.Contains(e, anchorTag("cv101")) && !mentionsAll(e, []string{"aq101", "ev102", "n101"}) {
			t.Error("the entry of cv101 does not lead to its acquisition aq101, the evidence ev102 it gave, and its target n101")
		}
	}
}

func subjectsOutside(text string, except []string) []string {
	var out []string
	for _, e := range entries(text) {
		if !slices.Contains(except, e) {
			out = append(out, subject(e))
		}
	}
	return out
}

// In worker-d1-and-mail.json the account boundary rests on ev102 for the
// Worker and on ev104 for the D1, and the report must show the D1 side is
// only a declaration. worker-and-d1.view.yaml adds a group that only the view
// config asks for.
func TestBoundariesAreTracedToEvidenceAndLogicalGroupsAreNot(t *testing.T) {
	f := loadSmallModel(t, "worker-d1-and-mail.json")
	if len(f.scene.Groups) != 1 {
		t.Fatalf("worker-d1-and-mail.json derives %d groups, want the one account boundary", len(f.scene.Groups))
	}
	md := f.markdown()
	boundary := f.scene.Groups[0].ID
	text := itemText(t, md, boundary)
	for _, line := range [][]string{{"ev102", "n101", "deployed"}, {"ev104", "n102", "declared"}} {
		if !slices.ContainsFunc(strings.Split(text, "\n"), func(l string) bool { return mentionsAll(l, line) }) {
			t.Errorf("no line of the boundary item names %v together", line)
		}
	}
	for node, evidence := range map[string]string{"n101": "ev102", "n102": "ev104"} {
		if !slices.ContainsFunc(strings.Split(itemText(t, md, node), "\n"), func(l string) bool { return mentionsAll(l, []string{boundary, evidence}) }) {
			t.Errorf("no line of the item of %s names its boundary %s with the evidence %s", node, boundary, evidence)
		}
	}
	if strings.Contains(itemText(t, md, "n103"), boundary) {
		t.Error("the item of n103 names the account boundary, which nothing says it belongs to")
	}

	m := loadModel(t, filepath.Join("..", "..", "testdata", "small-models", "worker-and-d1.json"))
	c := loadConfig(t, filepath.Join("..", "..", "testdata", "small-models", "worker-and-d1.view.yaml"), m)
	logical := itemText(t, string(report.Build(m, buildScene(t, m, c), report.Drawing{}).Markdown()), "g-app")
	if !strings.Contains(logical, "論理グループ") || strings.Contains(logical, "実在する境界") {
		t.Errorf("the item of g-app does not read as a logical group:\n%s", logical)
	}
	if mentions(logical, "ev001") || mentions(logical, "ev002") {
		t.Error("the item of the logical group g-app names evidence, and a logical group has none")
	}
}

// long-labels-and-mismatches holds a region two pieces of evidence disagree
// on, and a binding that points at one database in the settings and another
// one when deployed.
func TestMismatchEntriesGiveValuesSourcesTimesAndCoverage(t *testing.T) {
	f := loadSample(t, "long-labels-and-mismatches")
	found := labelled(sectionText(t, f.markdown(), "食い違い"), view.MarkMismatch.Label())
	for _, want := range [][]string{
		{"n305", "ev308", "ev309", "us-central1", "asia-northeast1", "infra/delivery.tf", "findings/long-labels-and-mismatches/delivery-function.json", "2026-11-13T09:00:00Z", "cv308", "complete"},
		{"n301", "e301", "e302", "DB_PRIMARY", "ev315", "ev316", "n302", "n303", "wrangler.toml", "findings/long-labels-and-mismatches/primary-binding.json", "2026-11-13T09:00:00Z", "cv303", "complete"},
	} {
		entry := ""
		for _, e := range found {
			if subject(e) == want[0] {
				entry = e
			}
		}
		for _, text := range want {
			if !strings.Contains(readAs(entry), text) {
				t.Errorf("the mismatch entry of %s does not say %q", want[0], text)
			}
		}
	}
	if len(found) != 2 {
		t.Errorf("long-labels-and-mismatches has %d mismatch entries, want 2", len(found))
	}
}

// An element that only a hint or a statement speaks for is unconfirmed even
// when no declaration waits to be checked. The hand-derived display draws it
// dashed.
func TestElementsKnownOnlyFromHintsOrStatementsAreUnconfirmed(t *testing.T) {
	for _, f := range samples(t) {
		t.Run(f.name, func(t *testing.T) {
			listed := subjects(sectionText(t, f.markdown(), "未確認の点"))
			for id, n := range f.want.Nodes {
				if n.Frame == string(view.FrameDashed) && !slices.Contains(listed, id) {
					t.Errorf("node %s is drawn dashed but no unconfirmed entry is about it", id)
				}
			}
			for id, e := range f.want.Edges {
				if e.Line == string(view.LineDashed) && !slices.Contains(listed, id) {
					t.Errorf("edge %s is drawn dashed but no unconfirmed entry is about it", id)
				}
			}
			// A relation that only a statement speaks for is unverified and
			// dashed at once, and is still one entry. No element of the samples
			// has two findings.
			for i, id := range listed {
				if i > 0 && listed[i-1] == id {
					t.Errorf("%s is the subject of more than one unconfirmed entry", id)
				}
			}
		})
	}
}

func TestOmittedSectionOfTheSampleWithHiddenAndFoldedElements(t *testing.T) {
	f := loadSample(t, "order-service")
	md := f.markdown()
	where := map[string]string{"e431": "n425"}
	for _, aggregate := range f.want.Aggregates {
		for _, id := range aggregate.Members {
			where[id] = aggregate.ID
		}
	}
	want := slices.Clone(f.want.Hidden)
	for id, destination := range where {
		want = append(want, id)
		if !mentions(itemText(t, md, id), destination) {
			t.Errorf("the item of %s does not name %s", id, destination)
		}
	}
	if got, want := subjects(sectionText(t, md, "図に載せなかったもの")), sorted(want); !slices.Equal(got, want) {
		t.Errorf("the omitted section is about %v, want %v", got, want)
	}
}

func TestFoldedSharedHiddenAndSelfEdgesSayWhereTheyWent(t *testing.T) {
	f := mixed(t)
	md := f.markdown()
	for id, where := range map[string]string{
		"na": "agg-store",
		"nb": "agg-store",
		"e1": "agg-store",
		"e3": "e2",
	} {
		if !mentions(itemText(t, md, id), where) {
			t.Errorf("the item of %s does not name %s", id, where)
		}
	}
	omitted := sectionText(t, md, "図に載せなかったもの")
	if got, want := subjects(omitted), []string{"e1", "e4", "e5", "na", "nb", "nd"}; !slices.Equal(got, want) {
		t.Errorf("the omitted section is about %v, want %v", got, want)
	}

	drawn := labelled(itemText(t, md, "e4"), "図での表示")
	if len(drawn) != 1 || !mentions(drawn[0], "nd") {
		t.Errorf("the display row of e4 does not name its hidden end nd: %q", drawn)
	}
	for _, row := range entries(omitted) {
		if subject(row) == "e4" {
			_, why, _ := strings.Cut(row, "）")
			if !mentions(why, "nd") {
				t.Errorf("the omitted entry of e4 does not explain which end is hidden: %q", row)
			}
		}
	}

	m := loadModel(t, filepath.Join("..", "..", "testdata", "marks", "relation-to-itself.json"))
	md = string(report.Build(m, buildScene(t, m, &view.Config{}), report.Drawing{}).Markdown())
	self := entries(sectionText(t, md, "図に載せなかったもの"))
	if len(self) != 1 || subject(self[0]) != "self" {
		t.Fatalf("the omitted entries are %q, want one about the relation from w to itself", self)
	}
	for _, text := range []string{self[0], itemText(t, md, "self")} {
		if strings.Contains(text, "集約") {
			t.Errorf("a relation from a node to itself is said to be inside an aggregate:\n%s", text)
		}
		if !strings.Contains(text, view.MarkUnverified.Label()) {
			t.Errorf("the unverified state of the relation is missing:\n%s", text)
		}
	}
}

// The report is kept on the developer's machine, so it prints the values an
// investigation needs to be traced, identifiers included, and says so.
func TestIdentifierValuesArePrintedAndTheReportSaysItIsNotForSharing(t *testing.T) {
	md := mixed(t).markdown()
	for _, value := range []string{"raw-account-777", "raw-kv-one", "下書きを保存する", "照会する権限がない"} {
		if !strings.Contains(md, value) {
			t.Errorf("the report does not print %q", value)
		}
	}
	for _, word := range []string{"識別子", "共有"} {
		if !strings.Contains(preamble(md), word) {
			t.Errorf("the lines under the title do not mention %q", word)
		}
	}
}

// A coverage record that is not complete tells the reader that more may
// exist than was read. It stands under the unverified entry it applies to,
// and by itself when no such entry holds it.
// openModel is the mixed model with keys the schema does not define in every
// place that keeps them: supports, attributes, scopes and their filters. It
// also has a key of its own under identity, where the schema defines none.
func openModel(t *testing.T) string {
	t.Helper()
	raw := mixedModel
	for old, repl := range map[string]string{
		`"target": {"repository": ".", "environment": "dev"}`: `"target": {"repository": ".", "environment": "dev", "scope": [
		   {"provider": "cloudflare", "zone": "raw-zone-123", "filter": {"slot": "COPY", "namespace": "raw-filter-777"}}]}`,
		`"identity": {"account_id": "raw-account-777", "native_id": "raw-kv-one"},
     "attributes": {"role": "下書きを保存する"},`: `"identity": {"account_id": "raw-account-777", "native_id": "raw-kv-one", "x.vendor.id": "raw-ident-321"},
     "attributes": {"role": "下書きを保存する", "region": "raw-region-apac", "machine_type": "raw-attr-222", "labels": {"team": "raw-attr-nested-223"}},`,
		`{"id": "e2", "from": "nc", "to": "na", "relation": "implementation", "attributes": {"purpose": "保存する"},`: `{"id": "e2", "from": "nc", "to": "na", "relation": "implementation", "attributes": {"purpose": "保存する", "weight": "raw-edge-attr-444"},`,
		`{"id": "cv2", "scope": {"provider": "cloudflare", "target_ids": ["nb"]},`:                                `{"id": "cv2", "scope": {"provider": "cloudflare", "target_ids": ["nb"], "filter": {"fields": ["existence"], "namespace": "raw-filter-666"}},`,
	} {
		if strings.Count(raw, old) != 1 {
			t.Fatalf("fixture premise: the mixed model holds %q %d times, want 1", old, strings.Count(raw, old))
		}
		raw = strings.Replace(raw, old, repl, 1)
	}
	return raw
}

// rowsNaming returns the lines of text that print key as code without a
// value after it.
func rowsNaming(text, key string) []string {
	var out []string
	for _, l := range strings.Split(text, "\n") {
		if strings.Contains(l, "`"+key+"`") {
			out = append(out, l)
		}
	}
	return out
}

// The schema gives no meaning to keys such as x.vendor.tag, so the tool
// cannot tell what their values are. Wherever the model keeps such a key, the
// report names the key and leaves its value in the model.
func TestValueUnderAKeyTheSchemaDoesNotDefineIsNotPrinted(t *testing.T) {
	m, diags := model.Parse("model.json", []byte(openModel(t)))
	if diags.HasErrors() {
		t.Fatal(diags)
	}
	md := string(report.Build(m, buildScene(t, m, &view.Config{}), report.Drawing{}).Markdown())
	for _, value := range []string{
		"raw-extension-333", "raw-nested-888", "raw-listed-111", "844388", "raw-seed-4f9a2c71",
		"raw-attr-222", "raw-attr-nested-223", "raw-edge-attr-444",
		"raw-target-999", "raw-zone-123", "raw-filter-666", "raw-filter-777",
	} {
		if strings.Contains(md, value) {
			t.Errorf("the report prints %q, a value under a key the schema does not define", value)
		}
	}

	coverage := sectionText(t, md, "調査範囲")
	record := func(id string) string {
		for _, e := range entries(coverage) {
			if strings.Contains(e, anchorTag(id)) {
				return e
			}
		}
		t.Fatalf("the coverage section has no record %s", id)
		return ""
	}
	asked, _, _ := strings.Cut(coverage, anchorTag("cv1"))
	for _, c := range []struct {
		where, text string
		keys        []string
	}{
		{"the item of na", itemText(t, md, "na"), []string{"x.vendor.tag", "x.vendor.detail", "machine_type", "labels"}},
		{"the item of e1", itemText(t, md, "e1"), []string{"env.SESSION_SEED"}},
		{"the item of e2", itemText(t, md, "e2"), []string{"weight"}},
		{"the item of nb", itemText(t, md, "nb"), []string{"namespace"}},
		{"the scope of the target", asked, []string{"zone", "namespace"}},
		{"the record cv1", record("cv1"), []string{"cluster"}},
		{"the record cv2", record("cv2"), []string{"namespace"}},
	} {
		for _, key := range c.keys {
			rows := rowsNaming(c.text, key)
			if len(rows) != 1 {
				t.Errorf("%s names the key %s in %d rows, want 1", c.where, key, len(rows))
				continue
			}
			for _, word := range []string{"値は載せていない", "モデル"} {
				if !strings.Contains(rows[0], word) {
					t.Errorf("%s: the row of the key %s does not say %q: %s", c.where, key, word, rows[0])
				}
			}
		}
	}
}

// What the schema defines stays readable beside the keys it does not: the
// values of its own keys, scope.network among them since the schema says
// what it is, and every value under identity, which is where a model keeps
// the identifiers of a node whatever their keys are.
func TestValuesUnderDefinedKeysAndIdentityArePrintedBesideWithheldOnes(t *testing.T) {
	m, diags := model.Parse("model.json", []byte(openModel(t)))
	if diags.HasErrors() {
		t.Fatal(diags)
	}
	md := string(report.Build(m, buildScene(t, m, &view.Config{}), report.Drawing{}).Markdown())
	coverage := sectionText(t, md, "調査範囲")
	for _, c := range []struct{ where, text, want string }{
		{"the item of na", itemText(t, md, "na"), "x.vendor.id = \"raw-ident-321\""},
		{"the item of na", itemText(t, md, "na"), "region = \"raw-region-apac\""},
		{"the item of na", itemText(t, md, "na"), "scope.network = \"raw-network-555\""},
		{"the item of e2", itemText(t, md, "e2"), "保存する"},
		{"the coverage section", coverage, "provider = \"cloudflare\""},
		{"the coverage section", coverage, "account_id = \"raw-account-777\""},
		{"the coverage section", coverage, "COPY"},
		{"the coverage section", coverage, "existence"},
	} {
		if !strings.Contains(c.text, c.want) {
			t.Errorf("%s does not print %s", c.where, c.want)
		}
	}
}

func TestUnfinishedCoverageIsListedOnce(t *testing.T) {
	unconfirmed := func(name string) string {
		m := loadModel(t, filepath.Join("..", "..", "testdata", "marks", name+".json"))
		return sectionText(t, string(report.Build(m, buildScene(t, m, &view.Config{}), report.Drawing{}).Markdown()), "未確認の点")
	}
	for _, name := range []string{"confirmed-then-a-listing-stopped-later", "slot-declared-a-deployed-b-and-a-listing-stopped"} {
		got := entries(unconfirmed(name))
		if len(got) != 1 || subject(got[0]) != "cv-list" || !strings.Contains(got[0], "partial") {
			t.Errorf("%s: the unconfirmed entries are %q, want one about the listing cv-list that stopped half way", name, got)
		}
	}
	got := entries(unconfirmed("listing-stopped-half-way"))
	if len(got) != 1 || subject(got[0]) != "x" || !mentions(got[0], "cv-partial") {
		t.Errorf("listing-stopped-half-way: the unconfirmed entries are %q, want the one about x, with cv-partial under it", got)
	}
}

var findingLink = regexp.MustCompile(`\]\(#(finding_[0-9]+)\)`)

// The item of an element says its state in a line and links to the entry
// that explains it.
func TestStateOfAnElementLinksToTheEntryThatExplainsIt(t *testing.T) {
	for _, c := range markCases(t) {
		t.Run(c.name, func(t *testing.T) {
			md := string(report.Build(c.model, buildScene(t, c.model, &view.Config{}), report.Drawing{}).Markdown())
			explained := map[string]string{}
			for _, heading := range []string{"食い違い", "未確認の点"} {
				for _, e := range entries(sectionText(t, md, heading)) {
					if m := ownAnchor.FindStringSubmatch(e); m != nil {
						explained[m[1]] = e
					}
				}
			}
			for _, f := range c.findings {
				for _, id := range f.about {
					linked := false
					for _, m := range findingLink.FindAllStringSubmatch(itemText(t, md, id), -1) {
						linked = linked || mentionsAll(explained[m[1]], f.ids())
					}
					if !linked {
						t.Errorf("the item of %s does not link to the entry of its finding %s", id, f.code)
					}
				}
			}
		})
	}
}

// The absence that counts was read by the acquisition aq-get, which the
// complete coverage record cv-complete also names.
func TestMismatchEntryTiesEvidenceToTheCoverageOfTheSameAcquisition(t *testing.T) {
	m := loadModel(t, filepath.Join("..", "..", "testdata", "marks", "absent-named-target-complete.json"))
	md := string(report.Build(m, buildScene(t, m, &view.Config{}), report.Drawing{}).Markdown())
	found := labelled(sectionText(t, md, "食い違い"), view.MarkMismatch.Label())
	if len(found) != 1 {
		t.Fatalf("the mismatch entries are %q, want one", found)
	}
	if !slices.ContainsFunc(strings.Split(found[0], "\n"), func(l string) bool { return mentionsAll(l, []string{"cv-complete", "p", "aq-get"}) }) {
		t.Errorf("no line of the entry ties cv-complete to the evidence p through the acquisition aq-get:\n%s", found[0])
	}
	if !slices.ContainsFunc(strings.Split(itemText(t, md, "x"), "\n"), func(l string) bool { return mentionsAll(l, []string{"aq-get", "cv-complete"}) }) {
		t.Error("the evidence p in the item of x does not lead from its acquisition aq-get to cv-complete")
	}
}

func TestRedrawSectionSaysHowTheFigureWasDrawn(t *testing.T) {
	f := loadSmallModel(t, "worker-d1-and-mail.json")
	redraw := func(d report.Drawing) string {
		return sectionText(t, string(report.Build(f.model, f.scene, d).Markdown()), "再描画")
	}
	text := redraw(report.Drawing{Layout: view.LayoutConfig{Engine: view.EngineTALA, Direction: view.DirRight, Seed: 4242}, View: true})
	for _, want := range []string{"tala", "right", "4242", "表示設定"} {
		if !strings.Contains(text, want) {
			t.Errorf("the redraw section does not say %q", want)
		}
	}

	// ELK draws without a seed, and a render may have no view config.
	text = redraw(report.Drawing{Layout: view.LayoutConfig{Engine: view.EngineELK, Direction: view.DirDown, Seed: 4242}})
	for _, gone := range []string{"seed", "4242", "表示設定"} {
		if strings.Contains(text, gone) {
			t.Errorf("the redraw section of a render without it says %q", gone)
		}
	}
	for _, want := range []string{"elk", "down"} {
		if !strings.Contains(text, want) {
			t.Errorf("the redraw section does not say %q", want)
		}
	}
}

// A path such as src/__init__.py and a value such as "_draft_" hold signs
// that Markdown reads as emphasis. They are printed as code, so that a
// reader sees them as the model has them.
func TestValuesAndPathsArePrintedAsTheModelHasThem(t *testing.T) {
	raw := strings.NewReplacer("src/d.ts", "src/__init__.py", "raw-kv-two", "_raw*kv`two_").Replace(mixedModel)
	m, diags := model.Parse("model.json", []byte(raw))
	if diags.HasErrors() {
		t.Fatal(diags)
	}
	md := string(report.Build(m, buildScene(t, m, &view.Config{}), report.Drawing{}).Markdown())
	for _, want := range []string{"`src/__init__.py`", "native_id = \"_raw*kv`two_\""} {
		if !strings.Contains(md, want) {
			t.Errorf("the report does not print %s", want)
		}
	}
}

// readAs drops the backslash before an escaped ASCII punctuation character,
// as a Markdown reader shows it.
func readAs(markdown string) string {
	return regexp.MustCompile("\\\\([!-/:-@[-`{-~])").ReplaceAllString(markdown, "$1")
}

func TestANodeIsNamedByItsEscapedServiceName(t *testing.T) {
	md := mixed(t).markdown()
	const escaped = `Acme \_Mail\_ \&copy; \&#65;`
	if item := itemText(t, md, "nc"); !strings.HasPrefix(item, "\n### nc "+escaped+"\n") {
		t.Errorf("the heading of nc does not give its escaped service name:\n%s", item)
	}
	if from := labelled(itemText(t, md, "e2"), "接続元"); len(from) != 1 || !strings.HasSuffix(from[0], "[nc](#nc) "+escaped) {
		t.Errorf("the source of e2 does not link to nc with its escaped service name: %q", from)
	}
}

// long-labels-and-mismatches holds a relation that only a statement speaks
// for. Its unverified state says so, and no second row repeats it.
func TestRelationKnownOnlyFromAStatementHasOneStateRow(t *testing.T) {
	item := itemText(t, loadSample(t, "long-labels-and-mismatches").markdown(), "e308")
	if n := strings.Count(item, "\n- 状態: "); n != 1 || !strings.Contains(item, view.MarkUnverified.Label()) {
		t.Errorf("the item of e308 has %d state rows, want the one that says it is unverified:\n%s", n, item)
	}
}

// The scope the investigation was asked to cover is part of what was read.
func TestCoverageSectionGivesTheScopeOfTheTarget(t *testing.T) {
	raw := strings.Replace(mixedModel, `"target": {"repository": ".", "environment": "dev"}`,
		`"target": {"repository": ".", "environment": "dev", "scope": [{"provider": "cloudflare", "account_id": "raw-account-777"}]}`, 1)
	m, diags := model.Parse("model.json", []byte(raw))
	if diags.HasErrors() {
		t.Fatal(diags)
	}
	scope := sectionText(t, string(report.Build(m, buildScene(t, m, &view.Config{}), report.Drawing{}).Markdown()), "調査範囲")
	if before, _, _ := strings.Cut(scope, anchorTag("cv1")); !strings.Contains(before, `account_id = "raw-account-777"`) {
		t.Errorf("the coverage section does not give the scope of the target before its records:\n%s", before)
	}
}

// settleRows are the 確認方法 rows of an entry, in the order printed.
func settleRows(entry string) []string {
	var out []string
	for _, l := range strings.Split(entry, "\n") {
		if v, ok := strings.CutPrefix(l, "  - 確認方法: "); ok {
			out = append(out, v)
		}
	}
	return out
}

// declaredOnlyModel is a D1 database that a file declares and that no
// query confirmed, with one coverage record of its existence per status.
func declaredOnlyModel(t *testing.T, statuses []string) *model.Model {
	t.Helper()
	var records []string
	for i, s := range statuses {
		records = append(records, `{"id": "cv`+string(rune('a'+i))+`", "scope": {"provider": "cloudflare", "target_ids": ["x"], "filter": {"fields": ["existence"]}},
		  "status": "`+s+`", "reason": "記録した理由", "checked_at": "2026-11-13T09:00:00Z"}`)
	}
	raw := `{"schema_version": "0.1", "target": {"repository": ".", "environment": "dev"},
	  "nodes": [{"id": "x", "kind": "resource", "provider": "cloudflare", "service": "cloudflare.d1", "identity": {}, "attributes": {},
	    "evidence": [{"id": "d", "kind": "declared", "supports": [{"key": "existence", "value": true}],
	      "ref": {"type": "file", "path": "wrangler.toml"}, "checked_at": "2026-11-13T09:00:00Z"}]}],
	  "edges": [], "coverage": [` + strings.Join(records, ",") + `]}`
	m, diags := model.Parse("model.json", []byte(raw))
	if diags.HasErrors() {
		t.Fatal(diags)
	}
	return m
}

// An unverified entry tells how to check it from how far the queries of its
// unit got: a denied query needs its permission checked, not just a query.
func TestUnverifiedEntrySaysHowToCheckFromTheCoverageOfItsUnit(t *testing.T) {
	const (
		generic  = "この項目を実環境で照会し、結果を実環境の根拠（deployed）として取り込む"
		denied   = "照会に必要な権限を確かめ、照会をやり直す"
		failed   = "照会をやり直し、結果を取り込む"
		partial  = "残りを取得し、結果を取り込む"
		notTried = "照会を実施し、結果を取り込む"
	)
	for _, c := range []struct {
		statuses []string
		want     []string
	}{
		{nil, []string{generic}},
		{[]string{"complete"}, []string{generic}},
		{[]string{"denied"}, []string{denied}},
		{[]string{"failed"}, []string{failed}},
		{[]string{"not_attempted"}, []string{notTried}},
		{[]string{"denied", "failed"}, []string{denied, failed}},
		{[]string{"partial", "complete", "denied", "partial"}, []string{partial, denied}},
	} {
		m := declaredOnlyModel(t, c.statuses)
		md := string(report.Build(m, buildScene(t, m, &view.Config{}), report.Drawing{}).Markdown())
		found := labelled(sectionText(t, md, "未確認の点"), view.MarkUnverified.Label())
		if len(found) != 1 || subject(found[0]) != "x" || strings.Count(found[0], "この項目に当たる調査範囲: ") != len(c.statuses) {
			t.Fatalf("%v: the unverified entries are %q, want one about x with each coverage record under it", c.statuses, found)
		}
		if got := settleRows(found[0]); !slices.Equal(got, c.want) {
			t.Errorf("%v: the entry says to check it by %q, want %q", c.statuses, got, c.want)
		}
	}
}

// Only an unverified state has a way to settle it, whatever the coverage
// records under a mismatch say.
func TestMismatchEntryUnderADeniedRecordHasNoWayToCheck(t *testing.T) {
	m := loadModel(t, filepath.Join("..", "..", "testdata", "marks", "slot-declared-a-deployed-b-after-an-earlier-read-was-denied.json"))
	md := string(report.Build(m, buildScene(t, m, &view.Config{}), report.Drawing{}).Markdown())
	found := labelled(sectionText(t, md, "食い違い"), view.MarkMismatch.Label())
	if len(found) != 1 || !strings.Contains(found[0], "denied") {
		t.Fatalf("the mismatch entries are %q, want one with the denied record cv-old under it", found)
	}
	if got := settleRows(found[0]); len(got) != 0 {
		t.Errorf("the mismatch entry says to check it by %q, want no such row", got)
	}
}

func TestMarkdownIsByteIdenticalAcrossBuilds(t *testing.T) {
	first, second := fixtures(t), fixtures(t)
	for i := range first {
		a, b := []byte(first[i].markdown()), []byte(second[i].markdown())
		if len(a) == 0 {
			t.Errorf("%s: the report is empty", first[i].name)
		}
		if !bytes.Equal(a, b) {
			t.Errorf("%s: two builds of the same input differ", first[i].name)
		}
	}
}
