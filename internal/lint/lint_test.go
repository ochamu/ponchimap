package lint_test

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/ochamu/ponchimap/internal/diag"
	"github.com/ochamu/ponchimap/internal/lint"
	"github.com/ochamu/ponchimap/internal/view"
)

const cases = "../../testdata/lint"

func check(t *testing.T, in lint.Input) (*lint.Checked, diag.List) {
	t.Helper()
	checked, diags := lint.Check(in)
	if (checked == nil) != diags.HasErrors() {
		t.Fatalf("Checked returned = %v with errors = %v\n%v", checked != nil, diags.HasErrors(), diags)
	}
	if checked != nil && (checked.Model() == nil || checked.Scene() == nil) {
		t.Fatalf("Checked holds model = %v and scene = %v", checked.Model(), checked.Scene())
	}
	for _, d := range diags {
		if d.Hint == "" {
			t.Errorf("a diagnostic has no hint: %v", d)
		}
	}
	sorted := slices.Clone(diags)
	if sorted.Sort(); !reflect.DeepEqual(sorted, diags) {
		t.Errorf("the list is not in its fixed order:\n%v", diags)
	}
	return checked, diags
}

func caseNames(t *testing.T) []string {
	t.Helper()
	paths, _ := filepath.Glob(filepath.Join(cases, "*", "want"))
	if len(paths) < 23 {
		t.Fatalf("found %d cases, want twenty-three at least", len(paths))
	}
	names := make([]string, len(paths))
	for i, path := range paths {
		names[i] = filepath.Base(filepath.Dir(path))
	}
	return names
}

func caseInput(name string) lint.Input {
	dir := filepath.Join(cases, name)
	in := lint.Input{Model: lint.Source{Path: filepath.Join(dir, "model.json")}}
	if config := filepath.Join(dir, "view.yaml"); exists(config) {
		in.View = lint.Source{Path: config}
	}
	return in
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func wantLine(d diag.Diagnostic, in lint.Input) []string {
	dash := func(s string) string {
		if s == "" {
			return "-"
		}
		return s
	}
	file := dash(d.Location.Path)
	switch d.Location.Path {
	case in.Model.Path:
		file = "model"
	case in.View.Path:
		file = "view"
	}
	place := "-"
	if d.Location.Line > 0 {
		place = fmt.Sprintf("%d:%d", d.Location.Line, d.Location.Column)
	}
	return []string{string(d.Severity), file, place, d.Code, dash(d.Location.Pointer), dash(d.TargetID)}
}

func TestEveryCaseGivesTheDiagnosticsItsWantFileLists(t *testing.T) {
	for _, name := range caseNames(t) {
		t.Run(name, func(t *testing.T) {
			raw, err := os.ReadFile(filepath.Join(cases, name, "want"))
			if err != nil {
				t.Fatal(err)
			}
			var want [][]string
			for line := range strings.Lines(string(raw)) {
				if line = strings.TrimRight(line, "\n"); line != "" {
					want = append(want, strings.Split(line, "\t"))
				}
			}
			in := caseInput(name)
			_, diags := check(t, in)
			var got []string
			for _, d := range diags {
				got = append(got, strings.Join(wantLine(d, in), "\t")+"\t"+d.Message)
			}
			report := fmt.Sprintf("got:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.TrimSpace(string(raw)))
			if len(diags) != len(want) {
				t.Fatalf("%d diagnostics, want %d\n%s", len(diags), len(want), report)
			}
			for i, d := range diags {
				if len(want[i]) != 7 {
					t.Fatalf("line %d of the want file does not have seven fields", i+1)
				}
				if !slices.Equal(wantLine(d, in), want[i][:6]) || (want[i][6] != "-" && !strings.Contains(d.Message, want[i][6])) {
					t.Errorf("diagnostic %d differs\n%s", i+1, report)
				}
			}
		})
	}
}

func TestTheSameInputAlwaysGivesTheSameDiagnostics(t *testing.T) {
	for _, name := range caseNames(t) {
		in := caseInput(name)
		_, first := check(t, in)
		for range 20 {
			if _, again := check(t, in); !reflect.DeepEqual(first, again) {
				t.Fatalf("%s: the diagnostics differ between two checks:\n%v\n%v", name, first, again)
			}
		}
	}
}

func shown(t *testing.T, diags diag.List) []string {
	t.Helper()
	asJSON, err := json.Marshal(diags)
	if err != nil {
		t.Fatal(err)
	}
	texts := []string{string(asJSON)}
	for _, d := range diags {
		texts = append(texts, d.String(), d.Message, d.Hint, d.TargetID, d.Location.Pointer, d.Location.Path)
	}
	return texts
}

func TestTheSamplesAndTheSmallModelsHaveNoError(t *testing.T) {
	const small = "../../testdata/small-models"
	inputs := map[string]lint.Input{
		"worker-and-d1":                      {Model: lint.Source{Path: filepath.Join(small, "worker-and-d1.json")}},
		"worker-and-d1 with its view config": {Model: lint.Source{Path: filepath.Join(small, "worker-and-d1.json")}, View: lint.Source{Path: filepath.Join(small, "worker-and-d1.view.yaml")}},
		"worker-d1-and-mail":                 {Model: lint.Source{Path: filepath.Join(small, "worker-d1-and-mail.json")}},
	}
	samples, _ := filepath.Glob("../../testdata/samples/*/model.json")
	if len(samples) < 5 {
		t.Fatalf("found %d sample models, want the five samples at least", len(samples))
	}
	for _, path := range samples {
		dir := filepath.Dir(path)
		inputs[filepath.Base(dir)] = lint.Input{Model: lint.Source{Path: path}, View: lint.Source{Path: filepath.Join(dir, "view.yaml")}}
	}
	for name, in := range inputs {
		if checked, diags := check(t, in); checked == nil {
			t.Errorf("%s: %v", name, diags)
		}
	}
}

const (
	serviceNode = `{"id":%q,"kind":"service","provider":"example","service":"example.api","evidence":[{"id":"ev-%[1]s","kind":"code","supports":[{"key":"call.implemented","value":true}],"ref":{"type":"file","path":"src/api.ts"},"checked_at":"2026-11-13T00:00:00Z"}]}`
	modelOf     = `{"schema_version":"0.1","target":{"repository":".","environment":"dev"},"nodes":[%s],"edges":[],"coverage":[%s]}`
)

func TestAModelWithoutAViewConfigIsDrawnWithTheDefaults(t *testing.T) {
	checked, diags := check(t, caseInput("no-view"))
	if checked == nil {
		t.Fatal(diags)
	}
	if nodes, edges := len(checked.Scene().Nodes), len(checked.Scene().Edges); nodes != 2 || edges != 1 {
		t.Errorf("the scene has %d nodes and %d edges, want the 2 nodes and the 1 edge of the model", nodes, edges)
	}
	if want := (view.LayoutConfig{Direction: view.DirRight, Engine: view.EngineTALA, Seed: 1}); checked.Layout() != want {
		t.Errorf("layout = %+v, want the defaults %+v", checked.Layout(), want)
	}
}

func TestTheLayoutOverrideReplacesTheFieldsItSets(t *testing.T) {
	const config = "schema_version: \"0.1\"\nlayout:\n  engine: elk\n  direction: down\n  seed: 7\n"
	written := view.LayoutConfig{Engine: view.EngineELK, Direction: view.DirDown, Seed: 7}
	for name, c := range map[string]struct{ override, want view.LayoutConfig }{
		"no field":      {view.LayoutConfig{}, written},
		"the engine":    {view.LayoutConfig{Engine: view.EngineTALA}, view.LayoutConfig{Engine: view.EngineTALA, Direction: view.DirDown, Seed: 7}},
		"the direction": {view.LayoutConfig{Direction: view.DirRight}, view.LayoutConfig{Engine: view.EngineELK, Direction: view.DirRight, Seed: 7}},
		"the seed":      {view.LayoutConfig{Seed: 9}, view.LayoutConfig{Engine: view.EngineELK, Direction: view.DirDown, Seed: 9}},
		"every field": {view.LayoutConfig{Engine: view.EngineTALA, Direction: view.DirRight, Seed: 9},
			view.LayoutConfig{Engine: view.EngineTALA, Direction: view.DirRight, Seed: 9}},
	} {
		checked, diags := check(t, lint.Input{
			Model:  lint.Source{Path: "model.json", Text: fmt.Appendf(nil, modelOf, fmt.Sprintf(serviceNode, "n001"), "")},
			View:   lint.Source{Path: "view.yaml", Text: []byte(config)},
			Layout: c.override,
		})
		if checked == nil || len(diags) > 0 {
			t.Fatalf("%s: %v", name, diags)
		}
		if checked.Layout() != c.want {
			t.Errorf("overriding %s: layout = %+v, want %+v", name, checked.Layout(), c.want)
		}
		if checked.Scene().Direction != c.want.Direction {
			t.Errorf("overriding %s: the scene grows %s, want %s", name, checked.Scene().Direction, c.want.Direction)
		}
	}
}

func TestTheTextOfASourceIsReadInsteadOfItsFile(t *testing.T) {
	const path = "no-such-directory/model.json"
	text := fmt.Appendf(nil, modelOf, fmt.Sprintf(serviceNode, "n001"),
		`{"id":"cv001","scope":{"provider":"example"},"status":"partial","checked_at":"2026-11-13T00:00:00Z"}`)
	checked, diags := check(t, lint.Input{Model: lint.Source{Path: path, Text: text}})
	if checked == nil || len(diags) != 1 || diags[0].Code != "coverage.partial" {
		t.Fatalf("got %v, want the one warning of the text", diags)
	}
	if diags[0].Location.Path != path {
		t.Errorf("the diagnostic names the file %q, want %q", diags[0].Location.Path, path)
	}
}

func TestAFileThatCannotBeReadIsAnErrorOfItsOwn(t *testing.T) {
	valid := caseInput("no-view").Model
	missing := filepath.Join(t.TempDir(), "missing")
	for name, in := range map[string]lint.Input{
		"the model":       {Model: lint.Source{Path: missing}},
		"the view config": {Model: valid, View: lint.Source{Path: missing}},
	} {
		_, diags := check(t, in)
		if len(diags) != 1 || diags[0].Code != "input.unreadable" || diags[0].Location.Path != missing {
			t.Errorf("%s cannot be read: got %v, want one input.unreadable that names the file", name, diags)
		}
	}
}

// Check reads a text whether or not it has the form of a model or a view
// config. The cases of the two loaders hold texts of every wrong form: no
// object, wrong types, keys written twice, YAML that is not read.
func TestATextOfAnyFormIsChecked(t *testing.T) {
	models, _ := filepath.Glob("../../testdata/model/*.json")
	configs, _ := filepath.Glob("../../testdata/view/*.yaml")
	if len(models) == 0 || len(configs) == 0 {
		t.Fatalf("found %d model cases and %d view config cases", len(models), len(configs))
	}
	for _, path := range models {
		check(t, lint.Input{Model: lint.Source{Path: path}})
	}
	valid := caseInput("no-view").Model
	for _, path := range configs {
		check(t, lint.Input{Model: valid, View: lint.Source{Path: path}})
	}
}

// owned is a node whose evidence states its owner and, when one is given, its
// region, so that it stands in the boundaries of both.
func owned(id, owner, region string) string {
	supports := `"supports":[{"key":"scope.owner","value":{"kind":"team","id":"` + owner + `"}},`
	if region != "" {
		supports += `{"key":"region","value":"` + region + `"},`
	}
	return strings.Replace(fmt.Sprintf(serviceNode, id), `"supports":[`, supports, 1)
}

// The entry left out for naming nothing shifts the indices of the fitted
// config away from those of the file.
func TestAWarningAboutTheFigureIsPlacedAtTheEntryOfTheFileThatItIsAbout(t *testing.T) {
	for name, c := range map[string]struct {
		nodes  []string
		config string
		want   []string
	}{
		"a label among labels of the same member and of the same level": {
			nodes: []string{owned("n-a", "team-x", "tokyo"), owned("n-b", "team-x", "")},
			config: `boundaries:
  - member: n-a
    level: region
    label: "a region"
  - member: n-gone
    label: "nothing"
  - member: n-b
    label: "an owner"
  - member: n-b
    level: region
    label: "no region"
`,
			want: []string{
				`warning view.unknown_reference /boundaries/1/member "n-gone"`,
				`warning view.boundary_unbacked /boundaries/3 "n-b"`,
			},
		},
		"a second label for one boundary": {
			nodes: []string{owned("n-a", "team-x", ""), owned("n-b", "team-x", "")},
			config: `boundaries:
  - member: n-a
    label: "one"
  - member: n-b
    label: "two"
`,
			want: []string{`warning view.boundary_named_twice /boundaries/1 "n-b"`},
		},
		"a member of an aggregate that a group lists too": {
			nodes: []string{owned("n-a", "team-x", ""), owned("n-b", "team-x", ""), owned("n-c", "team-x", "")},
			config: `groups:
  - id: g-one
    label: "one"
    members: [n-a, n-b]
aggregates:
  - id: ag-two
    label: "two"
    members: [n-gone, n-c, n-b]
`,
			// The entry begins above its members.
			want: []string{
				`warning view.split /aggregates/0 "ag-two"`,
				`warning view.unknown_reference /aggregates/0/members/0 "n-gone"`,
			},
		},
	} {
		checked, diags := check(t, lint.Input{
			Model: lint.Source{Path: "model.json", Text: fmt.Appendf(nil, modelOf, strings.Join(c.nodes, ","), "")},
			View:  lint.Source{Path: "view.yaml", Text: []byte("schema_version: \"0.1\"\n" + c.config)},
		})
		if checked == nil {
			t.Errorf("%s: no figure is drawn", name)
		}
		var got []string
		for _, d := range diags {
			got = append(got, fmt.Sprintf("%s %s %s %q", d.Severity, d.Code, d.Location.Pointer, d.TargetID))
			if d.Location.Path != "view.yaml" || d.Location.Line == 0 {
				t.Errorf("%s: %v is placed at %+v, want a line of the view config", name, d.Code, d.Location)
			}
		}
		if !slices.Equal(got, c.want) {
			t.Errorf("%s: got %q, want %q", name, got, c.want)
		}
	}
}

// The boundaries are derived from the model, so a node that the view config
// hides still says which boundary a name is for.
func TestABoundaryNamedThroughAHiddenNodeKeepsItsName(t *testing.T) {
	checked, diags := check(t, lint.Input{
		Model: lint.Source{Path: "model.json", Text: fmt.Appendf(nil, modelOf, owned("n-a", "team-x", "")+","+owned("n-b", "team-x", ""), "")},
		View:  lint.Source{Path: "view.yaml", Text: []byte("schema_version: \"0.1\"\nhidden: [n-a]\nboundaries:\n  - member: n-a\n    label: \"named\"\n")},
	})
	if checked == nil || len(diags) != 0 {
		t.Fatalf("got %+v, want a figure and no diagnostic", diags)
	}
	if groups := checked.Scene().Groups; len(groups) != 1 || groups[0].Label != "named" {
		t.Errorf("the boundaries are %+v, want the one of the two nodes under the name of the view config", groups)
	}
}

// Two owners of one provider are drawn under one label until the view config
// names them. The warning says where a name goes: the boundaries of the view
// config, or the node in the model when there is no view config to point at.
func TestABoundaryThatSharesItsLabelIsReportedWhereAFileCanBePointedAt(t *testing.T) {
	text := fmt.Appendf(nil, modelOf, owned("n-a", "team-x", "")+","+owned("n-b", "team-y", ""), "")
	for name, c := range map[string]struct {
		config string
		want   []string
	}{
		"no view config":                       {"", []string{"model.json /nodes/0 line", "model.json /nodes/1 line"}},
		"a view config without boundaries":     {"schema_version: \"0.1\"\n", []string{"view.yaml /boundaries", "view.yaml /boundaries"}},
		"a view config that names one of them": {"schema_version: \"0.1\"\nboundaries:\n  - member: n-a\n    label: \"one\"\n", nil},
	} {
		in := lint.Input{Model: lint.Source{Path: "model.json", Text: text}}
		if c.config != "" {
			in.View = lint.Source{Path: "view.yaml", Text: []byte(c.config)}
		}
		_, diags := check(t, in)
		var got []string
		for i, d := range diags {
			if d.Code != "view.boundary_label_shared" || d.Severity != diag.Warning || d.TargetID != []string{"n-a", "n-b"}[i%2] {
				t.Errorf("%s: got %+v, want a warning view.boundary_label_shared about each node in turn", name, d)
			}
			place := d.Location.Path + " " + d.Location.Pointer
			if d.Location.Line > 0 {
				place += " line"
			}
			got = append(got, place)
		}
		if !slices.Equal(got, c.want) {
			t.Errorf("%s: placed at %q, want %q", name, got, c.want)
		}
	}
}

func TestLintLocatesBothAnUnverifiedNodeAndAnEvidenceDefect(t *testing.T) {
	const nodes = `
  {"id":"declared","kind":"resource","provider":"example","service":"example.api","evidence":[
    {"id":"ev-api","kind":"declared","supports":[{"key":"existence","value":true}],
     "ref":{"type":"file","path":"app.yaml"},"checked_at":"2026-11-13T09:00:00Z"}]},
  {"id":"unbacked","kind":"resource","provider":"example","service":"example.db","evidence":
    []}`
	in := lint.Input{Model: lint.Source{Path: "infra.json", Text: fmt.Appendf(nil, modelOf, nodes, "")}}
	_, diags := check(t, in)
	var got []string
	for _, d := range diags {
		got = append(got, strings.Join(wantLine(d, in), " "))
	}
	want := []string{
		"warning model 2:3 unverified.not_checked /nodes/0 declared",
		"error model 6:5 model.no_evidence /nodes/1/evidence unbacked",
	}
	if !slices.Equal(got, want) {
		t.Errorf("diagnostics = %v, want %v", got, want)
	}
}

func TestALayoutOverrideThatNamesNoLayoutIsAnError(t *testing.T) {
	valid := caseInput("no-view").Model
	for name, override := range map[string]view.LayoutConfig{
		"the engine":    {Engine: "nope"},
		"the direction": {Direction: "nope"},
		"the seed":      {Seed: -4107},
	} {
		checked, diags := check(t, lint.Input{Model: valid, Layout: override})
		if checked != nil || len(diags) != 1 || diags[0].Severity != diag.Error || diags[0].Code != "lint.invalid_layout" || diags[0].Location != (diag.Location{}) || diags[0].TargetID != "" {
			t.Fatalf("%s names no layout: got %+v, want one error lint.invalid_layout without a place", name, diags)
		}
		for _, text := range shown(t, diags) {
			if strings.Contains(text, "nope") || strings.Contains(text, "4107") {
				t.Errorf("%s: the diagnostic shows the value of the override: %q", name, text)
			}
		}
	}
}
