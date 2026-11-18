package view

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"
	"unicode"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"gopkg.in/yaml.v3"

	"github.com/ochamu/ponchimap/internal/diag"
	"github.com/ochamu/ponchimap/internal/model"
	"github.com/ochamu/ponchimap/schema"
)

const (
	cases = "../../testdata/view"
	// The cases are fitted to this model: nodes n101 to n103, edges e101 to
	// e103.
	casesModel = "../../testdata/small-models/worker-d1-and-mail.json"
)

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

// fit fits a view config that was read to the model, as a command does, and
// returns every diagnostic of the two steps.
func fit(t *testing.T, f *ConfigFile, diags diag.List, m *model.Model) (*Config, diag.List) {
	t.Helper()
	if (f == nil) != diags.HasErrors() {
		t.Fatalf("config returned = %v with errors = %v", f != nil, diags.HasErrors())
	}
	if f == nil {
		return nil, diags
	}
	c, more := f.Resolve(m)
	if (c == nil) != more.HasErrors() {
		t.Fatalf("fitted config returned = %v with errors = %v", c != nil, more.HasErrors())
	}
	return c, append(diags, more...)
}

// read loads the view config at path and fits it to the model.
func read(t *testing.T, path string, m *model.Model) (*Config, diag.List) {
	t.Helper()
	f, diags := ParseConfig(path, readFile(t, path))
	return fit(t, f, diags, m)
}

// parse reads the text and fits it to the model of the cases.
func parse(t *testing.T, text string) (*Config, diag.List) {
	t.Helper()
	f, diags := ParseConfig("view.yaml", []byte(text))
	return fit(t, f, diags, loadModel(t, casesModel))
}

func only(t *testing.T, diags diag.List, code string) diag.Diagnostic {
	t.Helper()
	if len(diags) != 1 || diags[0].Code != code {
		t.Fatalf("got %v, want one %s", diags, code)
	}
	return diags[0]
}

func TestTheViewConfigOfASmallModelLoadsAndIsApplied(t *testing.T) {
	m := loadModel(t, "../../testdata/small-models/worker-and-d1.json")
	c, diags := read(t, "../../testdata/small-models/worker-and-d1.view.yaml", m)
	if c == nil || len(diags) > 0 {
		t.Fatal(diags)
	}
	want := &Config{
		SchemaVersion: "0.1", View: "overview",
		Labels: map[string]string{"n001": "API処理", "n002": "アプリ用DB"},
		Groups: []GroupConfig{{ID: "g-app", Kind: "logical", Label: "アプリのバックエンド", Members: []string{"n001", "n002"}, at: "/groups/0"}},
		Layout: LayoutConfig{Direction: DirRight, Engine: EngineTALA, Seed: 1},
	}
	if !reflect.DeepEqual(c, want) {
		t.Errorf("config = %+v\nwant %+v", c, want)
	}
	s := scene(t, m, c)
	if n := node(t, s, "n001"); n.Role != "API処理" || n.GroupID != "g-app" {
		t.Errorf("node n001 = %+v, want the label and the group of the view config", n)
	}
}

// wantLine is a diagnostic in the first five fields of a line of a .want
// file. The sixth field is a text the message contains.
func wantLine(d diag.Diagnostic) []string {
	dash := func(s string) string {
		if s == "" {
			return "-"
		}
		return s
	}
	return []string{string(d.Severity), fmt.Sprintf("%d:%d", d.Location.Line, d.Location.Column), d.Code, dash(d.Location.Pointer), dash(d.TargetID)}
}

func caseNames(t *testing.T) []string {
	t.Helper()
	paths, _ := filepath.Glob(filepath.Join(cases, "*.yaml"))
	if len(paths) == 0 {
		t.Fatal("no cases")
	}
	names := make([]string, len(paths))
	for i, path := range paths {
		names[i] = strings.TrimSuffix(filepath.Base(path), ".yaml")
	}
	return names
}

func TestEveryCaseGivesTheDiagnosticsItsWantFileLists(t *testing.T) {
	m := loadModel(t, casesModel)
	for _, name := range caseNames(t) {
		t.Run(name, func(t *testing.T) {
			raw, err := os.ReadFile(filepath.Join(cases, name+".want"))
			if err != nil {
				t.Fatal(err)
			}
			var want [][]string
			for line := range strings.Lines(string(raw)) {
				if line = strings.TrimRight(line, "\n"); line != "" {
					want = append(want, strings.Split(line, "\t"))
				}
			}
			path := filepath.Join(cases, name+".yaml")
			c, diags := read(t, path, m)
			var got []string
			for _, d := range diags {
				got = append(got, strings.Join(wantLine(d), "\t")+"\t"+d.Message)
			}
			report := fmt.Sprintf("got:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.TrimSpace(string(raw)))
			if len(diags) != len(want) {
				t.Fatalf("%d diagnostics, want %d\n%s", len(diags), len(want), report)
			}
			for i, d := range diags {
				if len(want[i]) != 6 {
					t.Fatalf("line %d of the want file does not have six fields", i+1)
				}
				if !slices.Equal(wantLine(d), want[i][:5]) || (want[i][5] != "-" && !strings.Contains(d.Message, want[i][5])) {
					t.Errorf("diagnostic %d differs\n%s", i+1, report)
				}
				if d.Location.Path != path {
					t.Errorf("diagnostic %d names the file %q, want %q", i+1, d.Location.Path, path)
				}
				if d.Hint == "" {
					t.Errorf("diagnostic %d has no hint: %v", i+1, d)
				}
			}
			if strings.HasPrefix(name, "valid-") && c == nil {
				t.Error("a valid view config was not returned")
			}
		})
	}
}

// A view config holds labels and titles that a person wrote. A value that is
// refused may be a secret pasted into the wrong place, so no diagnostic shows
// it. A key is part of the location and is shown.
func TestNoDiagnosticShowsAValueOfTheInput(t *testing.T) {
	shows := func(diags diag.List) string {
		fields := []string{}
		for _, d := range diags {
			fields = append(fields, d.String(), d.Message, d.Hint, d.TargetID, d.Location.Pointer, d.Location.Path)
		}
		for _, f := range fields {
			lower := strings.ToLower(f)
			for _, secret := range []string{"secret-token", "secret token", "secret_token"} {
				if strings.Contains(lower, secret) {
					return f
				}
			}
		}
		return ""
	}
	path := filepath.Join(cases, "secret-values.yaml")
	_, diags := ParseConfig(path, readFile(t, path))
	if len(diags) == 0 {
		t.Fatal("the case gave no diagnostics")
	}
	if shown := shows(diags); shown != "" {
		t.Errorf("a diagnostic shows a value of the input: %q", shown)
	}
	for name, text := range map[string]string{
		"the version":                  "schema_version: secret-token\n",
		"a number in another form":     "schema_version: \"0.1\"\nlayout:\n  seed: 0x5ec7e7\ntitle: &secret-token x\n",
		"an anchor and an alias":       "schema_version: \"0.1\"\ntitle: &secret-token x\nlabels:\n  n101: *secret-token\n",
		"an alias without an anchor":   "schema_version: \"0.1\"\ntitle: *secret-token\n",
		"a tag":                        "schema_version: \"0.1\"\ntitle: !secret-token x\n",
		"a value after a syntax error": "schema_version: \"0.1\"\ntitle: secret-token: x\n",
		"a duplicate key's value":      "schema_version: \"0.1\"\ntitle: secret-token\ntitle: secret-token\nview: 1\n",
		"a second document":            "schema_version: \"0.1\"\n---\nsecret-token\n",
	} {
		_, diags := parse(t, text)
		if len(diags) == 0 {
			t.Errorf("%s: no diagnostics", name)
		}
		if shown := shows(diags); shown != "" {
			t.Errorf("%s: a diagnostic shows a value of the input: %q", name, shown)
		}
	}
	c, diags := parse(t, "schema_version: \"0.1\"\nhidden: [n999]\ngroups:\n  - id: g-a\n    label: secret-token\n    members: [n998]\n")
	if shown := shows(diags); c == nil || len(diags) != 3 || shown != "" {
		t.Errorf("the warnings %v show a value of the input: %q", diags, shown)
	}
}

// personLines is the list as a person reads it, one diagnostic a line.
func personLines(diags diag.List) string {
	lines := make([]string, len(diags))
	for i, d := range diags {
		lines[i] = d.String()
	}
	return strings.Join(lines, "\n")
}

// A key is written by whoever wrote the file. A line break or an escape
// character in it must not turn one diagnostic into two lines or reach the
// terminal as it is.
func TestAKeyOfTheInputCannotBreakTheLineOfADiagnostic(t *testing.T) {
	_, diags := parse(t, "schema_version: \"0.1\"\n\"x\\nview.yaml:1:1: error forged.code: a forged line\": 1\n\"y\\e[2K\\rhidden\": 2\nlabels:\n  \"z\\u202Egnp\": a\n")
	if len(diags) != 3 {
		t.Fatalf("want the two unknown properties and the refused name, got %v", diags)
	}
	out := personLines(diags)
	if lines := strings.Split(out, "\n"); len(lines) != len(diags) {
		t.Errorf("%d diagnostics are written on %d lines:\n%q", len(diags), len(lines), out)
	}
	for _, r := range out {
		if r != '\n' && !unicode.IsPrint(r) {
			t.Errorf("the text holds the character %U as it is: %q", r, out)
		}
	}
}

func TestTheSameViewConfigAlwaysGivesTheSameDiagnostics(t *testing.T) {
	m := loadModel(t, casesModel)
	for _, name := range caseNames(t) {
		path := filepath.Join(cases, name+".yaml")
		_, first := read(t, path, m)
		for range 20 {
			if _, again := read(t, path, m); !reflect.DeepEqual(first, again) {
				t.Fatalf("%s: the diagnostics differ between two readings:\n%v\n%v", name, first, again)
			}
		}
	}
}

func TestASeedIsAWholeNumberOfOneOrMore(t *testing.T) {
	for seed, want := range map[string]Seed{"1": 1, "7": 7, "2.0": 2, "1e3": 1000, "2147483647": 2147483647} {
		c, diags := parse(t, "schema_version: \"0.1\"\nlayout:\n  seed: "+seed+"\n")
		if c == nil || c.Layout.Seed != want {
			t.Errorf("seed %s gives %+v with %v, want %d", seed, c, diags, want)
		}
	}
	for seed, code := range map[string]string{
		"2.5": "schema.type", "\"7\"": "schema.type", "true": "schema.type", "~": "schema.type", "[1]": "schema.type",
		"0": "schema.range", "-1": "schema.range", "2147483648": "schema.range", "1e30": "schema.range",
	} {
		c, diags := parse(t, "schema_version: \"0.1\"\nlayout:\n  seed: "+seed+"\n")
		d := only(t, diags, code)
		if c != nil || d.Location.Pointer != "/layout/seed" || d.Location.Line != 3 || d.Location.Column != 9 {
			t.Errorf("seed %s: config = %v, diagnostic %v; want no config and /layout/seed at 3:9", seed, c != nil, d)
		}
		if !strings.Contains(d.Hint, "whole number from 1") {
			t.Errorf("seed %s: the hint %q does not say what a seed is", seed, d.Hint)
		}
	}
}

func TestAConfigGetsTheDefaultsItDoesNotWrite(t *testing.T) {
	want := LayoutConfig{Direction: DirRight, Engine: EngineTALA, Seed: 1}
	for _, layout := range []string{"", "layout: {}\n", "layout:\n  direction: right\n"} {
		c, diags := parse(t, "schema_version: \"0.1\"\n"+layout)
		if c == nil || c.Layout != want || c.View != "overview" {
			t.Errorf("%q gives %+v with %v, want %+v and the overview", layout, c, diags, want)
		}
	}
	c, diags := parse(t, "schema_version: \"0.1\"\nlayout:\n  direction: down\n  engine: elk\n  seed: 7\nboundaries:\n  - {member: n101, label: A}\ngroups:\n  - {id: g, label: G, members: [n101]}\n")
	if c == nil {
		t.Fatal(diags)
	}
	if want := (LayoutConfig{Direction: DirDown, Engine: EngineELK, Seed: 7}); c.Layout != want {
		t.Errorf("layout = %+v, want %+v", c.Layout, want)
	}
	if c.Boundaries[0].Level != "owner" || c.Groups[0].Kind != "logical" {
		t.Errorf("level = %q and kind = %q, want owner and logical", c.Boundaries[0].Level, c.Groups[0].Kind)
	}
}

// A title or a label that reads as a date, or as a word such as yes, is the
// string that was written. The YAML library alone would read a date as a time
// and write it back in another form.
func TestAStringThatReadsAsADateIsKeptAsWritten(t *testing.T) {
	m := loadModel(t, casesModel)
	c, diags := read(t, filepath.Join(cases, "valid-strings-that-read-as-other-types.yaml"), m)
	if c == nil {
		t.Fatal(diags)
	}
	want := map[string]string{"n101": "2026-11-15T10:00:00Z", "n102": "yes", "n103": "off", "e101": "12:30", "e102": "2026-11-15 10:00:00"}
	if c.Title != "2026-11-15" || !reflect.DeepEqual(c.Labels, want) {
		t.Errorf("title = %q, labels = %v; want 2026-11-15 and %v", c.Title, c.Labels, want)
	}
	old, diags := read(t, filepath.Join(cases, "valid-strings-that-yaml-1-1-reads-as-numbers.yaml"), m)
	if want := map[string]string{"n101": "1_000", "n102": "0b11"}; old == nil || !reflect.DeepEqual(old.Labels, want) {
		t.Errorf("labels = %+v with %v; want %v", old, diags, want)
	}
	s := scene(t, m, c)
	if s.Title != "2026-11-15" {
		t.Errorf("the figure has the title %q", s.Title)
	}
}

func scene(t *testing.T, m *model.Model, c *Config) *Scene {
	t.Helper()
	s, err := Build(m, c)
	if err != nil {
		t.Fatalf("build scene: %v", err)
	}
	return s
}

// A view config is kept while the model changes. An entry for an ID that the
// model no longer has is left out with a warning that names the ID and says
// what to do, and every other entry still applies.
func TestAnEntryForAMissingIDIsLeftOutAndTheRestIsDrawn(t *testing.T) {
	m := loadModel(t, casesModel)
	c, diags := read(t, filepath.Join(cases, "references-to-ids-the-model-lacks.yaml"), m)
	if c == nil {
		t.Fatal(diags)
	}
	for _, d := range diags {
		if d.Severity != diag.Warning || !model.ValidID(d.TargetID) || !strings.Contains(d.Hint, "Remove") && !strings.Contains(d.Hint, "remove") {
			t.Errorf("%v with the hint %q; want a warning that names the ID and says what to do", d, d.Hint)
		}
	}
	want := &Config{
		SchemaVersion: "0.1", View: "overview", Title: "モデルから消えたIDを指す表示設定",
		Labels:     map[string]string{"n101": "入口", "e101": "呼び出し"},
		Hidden:     []string{"e103"},
		Boundaries: []BoundaryAlias{{Member: "n101", Level: "owner", Label: "配信アカウント", at: "/boundaries/1"}},
		Groups:     []GroupConfig{{ID: "g-a", Kind: "logical", Label: "残るグループ", Members: []string{"n101"}, at: "/groups/0"}},
		Layout:     LayoutConfig{Direction: DirRight, Engine: EngineTALA, Seed: 1},
	}
	if !reflect.DeepEqual(c, want) {
		t.Errorf("the fitted config is\n%+v\nwant\n%+v", c, want)
	}
	s := scene(t, m, c)
	if n := node(t, s, "n101"); n.Role != "入口" || n.GroupID != "g-a" {
		t.Errorf("node n101 = %+v, want the label and the group that the model still has", n)
	}
	var boundaries, groups []string
	for _, g := range s.Groups {
		if g.Kind == GroupBoundary {
			boundaries = append(boundaries, g.Label)
		} else {
			groups = append(groups, g.ID)
		}
	}
	if !slices.Equal(boundaries, []string{"配信アカウント"}) || !slices.Equal(groups, []string{"g-a"}) {
		t.Errorf("boundaries = %v, logical groups = %v; want the named boundary and the group with a member left", boundaries, groups)
	}
}

// The hint of a diagnostic leads to a config that is read. A group whose
// members are all gone is removed as the hint says, and with the last group
// the key goes too: a key without a value is refused, in words that a person
// who writes YAML knows.
func TestFollowingTheHintForAGroupWithoutMembersGivesAConfigThatIsRead(t *testing.T) {
	_, diags := parse(t, "schema_version: \"0.1\"\ngroups:\n  - id: g-old\n    label: 前の部分\n    members: [n999]\naggregates:\n  - id: agg-old\n    label: 前のまとめ\n    members: [n998]\n")
	var hints []string
	for _, d := range diags {
		if d.Code == "view.no_members" {
			hints = append(hints, d.Hint)
		}
	}
	if len(hints) != 2 || !strings.Contains(hints[0], "remove the groups key") || !strings.Contains(hints[1], "remove the aggregates key") {
		t.Errorf("the hints are %q; want each to say that the key goes with the last entry", hints)
	}
	if c, diags := parse(t, "schema_version: \"0.1\"\n"); c == nil || len(diags) > 0 {
		t.Errorf("the config without the two keys gives %v", diags)
	}
	_, diags = parse(t, "schema_version: \"0.1\"\ngroups:\n")
	if d := only(t, diags, "schema.type"); !strings.Contains(d.Message, "key without a value") || !strings.Contains(d.Message, "an array") {
		t.Errorf("a groups key without a value: %q", d.Message)
	}
}

// A boundary is named through one of its nodes. The boundaries are derived
// from the model, so a node that the config hides still says which boundary
// the label is for, and hiding the node does not cost the name.
func TestABoundaryNamedThroughAHiddenNodeIsKept(t *testing.T) {
	c, diags := parse(t, "schema_version: \"0.1\"\nhidden: [n102]\nboundaries:\n  - {member: n101, label: A}\n  - {member: n102, label: B}\n")
	if c == nil || len(diags) != 0 {
		t.Fatalf("config = %v with %v; want a config and no diagnostic", c != nil, diags)
	}
	if len(c.Boundaries) != 2 || c.Boundaries[1].Member != "n102" || c.Boundaries[1].Label != "B" {
		t.Errorf("the boundaries are %+v; want the entry of the hidden node kept", c.Boundaries)
	}
}

func TestMembersAreJudgedOnTheEntriesThatAreLeft(t *testing.T) {
	c, diags := parse(t, "schema_version: \"0.1\"\nhidden: [n999]\ngroups:\n"+
		"  - {id: g-gone, label: G, members: [n999]}\n"+
		"  - {id: g-a, label: A, members: [n999, n101]}\n"+
		"  - {id: g-b, label: B, members: [n999, n102, n101]}\n")
	var errs []string
	for _, d := range diags {
		if d.Severity == diag.Error {
			errs = append(errs, d.Code+" "+d.Location.Pointer+" "+d.TargetID)
		}
	}
	if want := []string{"view.listed_twice /groups/2/members/2 g-b"}; c != nil || !slices.Equal(errs, want) {
		t.Errorf("config = %v, errors %q; want no config and %q", c != nil, errs, want)
	}
}

// A small text must not take long, whatever it holds: aliases to aliases,
// thousands of levels, huge numbers, or the same problem many times over.
func TestASmallViewConfigIsReadQuickly(t *testing.T) {
	var laughs strings.Builder
	laughs.WriteString("schema_version: \"0.1\"\na0: &a0 [x, x, x, x, x, x, x, x, x, x]\n")
	for i := 1; i <= 40; i++ {
		fmt.Fprintf(&laughs, "a%d: &a%d [%s]\n", i, i, strings.TrimSuffix(strings.Repeat(fmt.Sprintf("*a%d, ", i-1), 10), ", "))
	}
	many := func(n int, item func(i int) string) string {
		var b strings.Builder
		for i := range n {
			b.WriteString(item(i))
		}
		return b.String()
	}
	m := loadModel(t, casesModel)
	for name, text := range map[string]string{
		"aliases to aliases":         laughs.String(),
		"ten thousand levels":        "schema_version: \"0.1\"\ntitle: " + strings.Repeat("[", 9990) + strings.Repeat("]", 9990) + "\n",
		"huge exponents":             "schema_version: \"0.1\"\nhidden: [" + many(5000, func(int) string { return "1e999999999, " }) + "x]\n",
		"long numbers":               "schema_version: \"0.1\"\nhidden: [" + many(200, func(int) string { return strings.Repeat("9", 5000) + ", " }) + "x]\n",
		"labels for missing IDs":     "schema_version: \"0.1\"\nlabels:\n" + many(50000, func(i int) string { return fmt.Sprintf("  n%d: x\n", i+1000) }),
		"the same key again":         "schema_version: \"0.1\"\nlabels:\n" + many(50000, func(int) string { return "  n101: x\n" }),
		"labels with refused names":  "schema_version: \"0.1\"\nlabels:\n" + many(50000, func(i int) string { return fmt.Sprintf("  N%d: x\n", i) }),
		"hidden IDs, all different":  "schema_version: \"0.1\"\nhidden: [" + many(50000, func(i int) string { return fmt.Sprintf("n%d, ", i+1000) }) + "n1]\n",
		"members of one group":       "schema_version: \"0.1\"\ngroups:\n  - id: g\n    label: G\n    members: [" + many(50000, func(i int) string { return fmt.Sprintf("n%d, ", i+1000) }) + "n1]\n",
		"groups with the same ID":    "schema_version: \"0.1\"\ngroups:\n" + many(20000, func(int) string { return "  - {id: g, label: G, members: [n101]}\n" }),
		"one long line":              "{schema_version: \"0.1\", labels: {" + many(50000, func(i int) string { return fmt.Sprintf("n%d: \"注文\", ", i+1000) }) + "n1: x}}\n",
		"unknown properties":         "schema_version: \"0.1\"\n" + many(50000, func(i int) string { return fmt.Sprintf("k%d: 1\n", i) }),
		"a megabyte of one label":    "schema_version: \"0.1\"\ntitle: " + strings.Repeat("9", 1<<20) + "\n",
		"a megabyte of plain digits": "schema_version: \"0.1\"\ntitle: \"" + strings.Repeat("9", 1<<20) + "\"\n",
	} {
		start := time.Now()
		f, diags := ParseConfig("view.yaml", []byte(text))
		if f != nil {
			_, more := f.Resolve(m)
			diags = append(diags, more...)
		}
		if took := time.Since(start); took > 5*time.Second {
			t.Errorf("%s (%d KB): read in %v with %d diagnostics", name, len(text)/1024, took.Round(time.Millisecond), len(diags))
		}
	}
}

// The schema is published for editors, so what it accepts and what the tool
// accepts must be the same wherever the schema can say it. Here a second
// reader, the YAML library with its own reading of values, and the schema
// library alone, with no code of the tool, judge every case that the tool
// either reads or refuses for a reason the schema states.
func TestTheToolAndThePublishedSchemaAloneAcceptTheSameConfigs(t *testing.T) {
	compiler := jsonschema.NewCompiler()
	compiler.AssertFormat()
	doc, err := jsonschema.UnmarshalJSON(strings.NewReader(string(schema.View)))
	if err != nil {
		t.Fatal(err)
	}
	if err := compiler.AddResource("view.schema.json", doc); err != nil {
		t.Fatal(err)
	}
	published, err := compiler.Compile("view.schema.json")
	if err != nil {
		t.Fatal(err)
	}
	// The second reader reads values as YAML 1.1 does. 1_000 and 0b11 are
	// numbers to it and strings to YAML 1.2, which the tool and editors
	// follow, so the two readers differ on this one case by design.
	const readDifferentlyByYAML11 = "valid-strings-that-yaml-1-1-reads-as-numbers"
	compared := 0
	for _, name := range caseNames(t) {
		if name == readDifferentlyByYAML11 {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(cases, name+".yaml"))
		if err != nil {
			t.Fatal(err)
		}
		c, diags := ParseConfig(name, raw)
		// What only the tool can see is outside the comparison: problems of
		// the text, and IDs declared twice.
		if slices.ContainsFunc(diags, func(d diag.Diagnostic) bool { return !strings.HasPrefix(d.Code, "schema.") }) {
			continue
		}
		var plain any
		if err := yaml.Unmarshal(raw, &plain); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		// encoding/json writes a time as an RFC 3339 string, which is a string
		// to the schema as the written date is.
		asJSON, err := json.Marshal(plain)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		value, err := jsonschema.UnmarshalJSON(strings.NewReader(string(asJSON)))
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		accepted := published.Validate(value) == nil
		if accepted != (c != nil) {
			t.Errorf("%s: the tool accepts = %v, the published schema alone accepts = %v", name, c != nil, accepted)
		}
		compared++
	}
	if compared < 12 {
		t.Errorf("only %d cases were compared", compared)
	}
}

// The Go types, the defaults, and the references that Resolve follows are
// written apart from the schema. These checks fail when one side is changed
// and the other is forgotten.
func TestTheSchemaAndTheGoTypesAgree(t *testing.T) {
	var s struct {
		Properties map[string]struct {
			Default any `json:"default"`
		} `json:"properties"`
		Defs map[string]struct {
			Pattern    string `json:"pattern"`
			Properties map[string]struct {
				Default any `json:"default"`
			} `json:"properties"`
		} `json:"$defs"`
	}
	if err := json.Unmarshal(schema.View, &s); err != nil {
		t.Fatal(err)
	}
	tags := func(v any) []string {
		var names []string
		for i := range reflect.TypeOf(v).NumField() {
			if tag := reflect.TypeOf(v).Field(i).Tag.Get("json"); tag != "" {
				names = append(names, strings.Split(tag, ",")[0])
			}
		}
		slices.Sort(names)
		return names
	}
	keys := func(properties map[string]struct {
		Default any `json:"default"`
	}) []string {
		var names []string
		for name := range properties {
			names = append(names, name)
		}
		slices.Sort(names)
		return names
	}
	for name, c := range map[string]struct{ schema, tags []string }{
		"the config":       {keys(s.Properties), tags(Config{})},
		"a boundary alias": {keys(s.Defs["boundaryAlias"].Properties), tags(BoundaryAlias{})},
		"a group":          {keys(s.Defs["group"].Properties), tags(GroupConfig{})},
		"an aggregate":     {keys(s.Defs["aggregate"].Properties), tags(AggregateConfig{})},
		"the layout":       {keys(s.Defs["layout"].Properties), tags(LayoutConfig{})},
	} {
		if !slices.Equal(c.schema, c.tags) {
			t.Errorf("%s: the schema defines %v, the Go type reads %v", name, c.schema, c.tags)
		}
	}

	c, diags := parse(t, "schema_version: \"0.1\"\nboundaries:\n  - {member: n101, label: A}\ngroups:\n  - {id: g, label: G, members: [n101]}\n")
	if c == nil {
		t.Fatal(diags)
	}
	layout := s.Defs["layout"].Properties
	for name, d := range map[string]struct{ schema, filled any }{
		"view":             {s.Properties["view"].Default, c.View},
		"boundary level":   {s.Defs["boundaryAlias"].Properties["level"].Default, c.Boundaries[0].Level},
		"group kind":       {s.Defs["group"].Properties["kind"].Default, c.Groups[0].Kind},
		"layout engine":    {layout["engine"].Default, string(c.Layout.Engine)},
		"layout direction": {layout["direction"].Default, string(c.Layout.Direction)},
		"layout seed":      {layout["seed"].Default, float64(c.Layout.Seed)},
	} {
		if d.schema != d.filled {
			t.Errorf("%s: the schema states the default %v, a config is given %v", name, d.schema, d.filled)
		}
	}

	var modelSchema struct {
		Defs map[string]struct {
			Pattern string `json:"pattern"`
		} `json:"$defs"`
	}
	if err := json.Unmarshal(schema.Model, &modelSchema); err != nil {
		t.Fatal(err)
	}
	if got, want := s.Defs["id"].Pattern, modelSchema.Defs["id"].Pattern; got != want || want == "" {
		t.Errorf("the ID form of the view config is %q, of the model %q", got, want)
	}

	// Where the schema says that a value is the ID of an element of the
	// model. Resolve follows each of these; a new one needs code there.
	var raw any
	if err := json.Unmarshal(schema.View, &raw); err != nil {
		t.Fatal(err)
	}
	var references []string
	var walk func(v any, at string)
	walk = func(v any, at string) {
		switch x := v.(type) {
		case map[string]any:
			if ref, _ := x["$ref"].(string); ref == "#/$defs/nodeId" || ref == "#/$defs/elementId" {
				references = append(references, at+" "+strings.TrimPrefix(ref, "#/$defs/"))
			}
			for k, inner := range x {
				walk(inner, at+"/"+k)
			}
		case []any:
			for i, inner := range x {
				walk(inner, fmt.Sprintf("%s/%d", at, i))
			}
		}
	}
	walk(raw, "")
	slices.Sort(references)
	want := []string{
		"/$defs/boundaryAlias/properties/member nodeId",
		"/$defs/members/items nodeId",
		"/properties/hidden/items elementId",
	}
	if !slices.Equal(references, want) {
		t.Errorf("the schema names elements of the model at\n%v\nResolve follows\n%v", references, want)
	}
	// The names under labels are IDs of elements too. A schema cannot refer
	// to a form for names, so the form is written there a second time.
	var labels struct {
		Properties struct {
			Labels struct {
				PatternProperties map[string]any `json:"patternProperties"`
			} `json:"labels"`
		} `json:"properties"`
	}
	if err := json.Unmarshal(schema.View, &labels); err != nil {
		t.Fatal(err)
	}
	if forms := labels.Properties.Labels.PatternProperties; len(forms) != 1 || forms[s.Defs["id"].Pattern] == nil {
		t.Errorf("the names under labels have the forms %v, want the ID form %q alone", forms, s.Defs["id"].Pattern)
	}
}
