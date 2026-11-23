package cli

import (
	"maps"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"
)

func checkPathRefusal(t *testing.T, a answer, before map[string]string) {
	t.Helper()
	out := machine(t, a.stdout)
	if a.code != 2 || out.Status != "error" || len(out.Artifacts) != 0 ||
		!slices.Equal(codes(out.Diagnostics), []string{"usage.invalid"}) {
		t.Errorf("exit code %d, output %+v; want 2, error, no artifacts, and usage.invalid", a.code, out)
	}
	if !maps.Equal(snapshot(t, "."), before) {
		t.Error("the command changed, added, or removed a file")
	}
}

func TestRenderRejectsOverlappingPaths(t *testing.T) {
	for _, tc := range []struct{ name, report string }{
		{"report is the model", "model.json"},
		{"report is the SVG", "out/overview.svg"},
		{"report is the temporary SVG", "out/.overview.svg.tmp"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Chdir(t.TempDir())
			writeFile(t, "model.json", generatedModel(1))
			writeFile(t, "out/overview.svg", string(marked([]byte("<svg/>"))))
			before := snapshot(t, ".")
			a := run("render", "--model", "model.json", "--output", "out", "--report", tc.report, "--json")
			checkPathRefusal(t, a, before)
		})
	}
}

func symlink(t *testing.T, leadsTo, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(leadsTo, path); err != nil {
		t.Skipf("this system makes no symbolic link: %v", err)
	}
}

func TestRenderRejectsPathsThatOverlapThroughALink(t *testing.T) {
	for _, tc := range []struct {
		name          string
		link, leadsTo func(dir string) string
		model, report func(link string) string
	}{
		{
			name:    "report reaches the SVG through a linked directory",
			link:    func(string) string { return filepath.Join(t.TempDir(), "alias") },
			leadsTo: func(dir string) string { return filepath.Join(dir, "out") },
			model:   func(string) string { return "model.json" },
			report:  func(link string) string { return filepath.Join(link, "overview.svg") },
		},
		{
			name:    "model is a link to the file at the report",
			link:    func(string) string { return "linked.json" },
			leadsTo: func(string) string { return "model.json" },
			model:   func(link string) string { return link },
			report:  func(string) string { return "model.json" },
		},
		{
			name:    "model is a link at the temporary SVG",
			link:    func(string) string { return filepath.Join("out", ".overview.svg.tmp") },
			leadsTo: func(string) string { return filepath.Join("..", "model.json") },
			model:   func(link string) string { return link },
			report:  func(string) string { return "report.md" },
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			t.Chdir(dir)
			writeFile(t, "model.json", generatedModel(1))
			writeFile(t, "out/overview.svg", string(marked([]byte("<svg/>"))))
			link := tc.link(dir)
			symlink(t, tc.leadsTo(dir), link)
			before := snapshot(t, ".")
			a := run("render", "--model", tc.model(link), "--output", "out", "--report", tc.report(link), "--json")
			checkPathRefusal(t, a, before)
		})
	}
}

// Save removes whatever stands at a temporary name and makes the file anew,
// so a link there stops no render and nothing is written through it.
func TestRenderReplacesALinkAtATemporaryName(t *testing.T) {
	for _, tc := range []struct{ name, leadsTo string }{
		{"to nothing", "nowhere"},
		{"to the model", filepath.Join("..", "model.json")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Chdir(t.TempDir())
			model := generatedModel(1)
			writeFile(t, "model.json", model)
			symlink(t, tc.leadsTo, filepath.Join("out", ".overview.svg.tmp"))
			a := run("render", "--model", "model.json", "--output", "out", "--report", "report.md")
			if a.code != 0 || !exists("out/overview.svg") {
				t.Fatalf("exit code %d, want 0 and the SVG saved\n%s", a.code, a.stderr)
			}
			if exists("out/nowhere") || readFile(t, "model.json") != model {
				t.Error("render wrote through the link at the temporary name")
			}
		})
	}
}

func TestRenderRefusesALinkToNothingAtTheReportAsItRefusesAnyLink(t *testing.T) {
	t.Chdir(t.TempDir())
	writeFile(t, "model.json", generatedModel(1))
	symlink(t, "nowhere", "report.md")
	a := run("render", "--model", "model.json", "--output", "out", "--report", "report.md", "--json")
	out := machine(t, a.stdout)
	if d := only(t, out.Diagnostics, "output.unwritable"); a.code != 4 || d.Location.Path != "report.md" {
		t.Errorf("exit code %d, diagnostics %v; want 4 and output.unwritable at report.md", a.code, codes(out.Diagnostics))
	}
	if exists("nowhere") || exists("out") {
		t.Error("render made a file although it refused the report")
	}
}

func TestRenderingASampleTwiceGivesTheSameBytes(t *testing.T) {
	model, config := sample("small-order-app")
	first, second := newTarget(t), newTarget(t)
	for _, g := range []target{first, second} {
		a := g.render(model, config)
		if a.code != 0 || a.stdout != "" {
			t.Fatalf("render: exit code %d and standard output %q, want 0 and nothing\n%s", a.code, a.stdout, a.stderr)
		}
		wantEnd := "saved " + g.svg() + "\nsaved " + g.png() + "\nsaved " + g.report + "\ndrawn with tala, right, seed 1\n"
		if !strings.HasSuffix(a.stderr, wantEnd) {
			t.Errorf("the standard error ends with\n%s\nwant\n%s", a.stderr, wantEnd)
		}
	}
	files := snapshot(t, first.dir)
	if names := slices.Sorted(maps.Keys(files)); !slices.Equal(names, []string{"figures/overview.png", "figures/overview.svg", "report.md"}) {
		t.Fatalf("the render left the files %v, want the SVG, the PNG, and the report", names)
	}
	for name, content := range files {
		if content == "" {
			t.Errorf("%s is empty", name)
		}
	}
	if again := snapshot(t, second.dir); !reflect.DeepEqual(again, files) {
		for name := range files {
			if again[name] != files[name] {
				t.Errorf("%s differs between two renders of the same inputs", name)
			}
		}
	}
}

// The report a render saves states the layout the figure was drawn with.
func TestTheSavedReportStatesTheLayoutOfTheSameRender(t *testing.T) {
	model, config := sample("small-order-app")
	g := newTarget(t)
	if a := g.render(model, config, "--seed", "4242"); a.code != 0 {
		t.Fatalf("render: exit code %d\n%s", a.code, a.stderr)
	}
	_, redraw, found := strings.Cut(readFile(t, g.report), "\n## 再描画\n")
	if !found {
		t.Fatal("the saved report has no section 再描画")
	}
	for _, said := range []string{"tala", "right", "4242"} {
		if !strings.Contains(redraw, said) {
			t.Errorf("the section does not state %s", said)
		}
	}
}

func TestARenderWithELKHasNoSeedInTheOutput(t *testing.T) {
	model, config := sample("small-order-app")
	g := newTarget(t)
	a := g.render(model, config, "--engine", "elk", "--direction", "down", "--json")
	if a.code != 0 {
		t.Fatalf("render: exit code %d\n%s", a.code, a.stderr)
	}
	svg := machine(t, a.stdout).Artifacts[0]
	if svg.Layout == nil || svg.Layout.Engine != "elk" || svg.Layout.Direction != "down" || svg.Layout.Seed != nil {
		t.Errorf("the layout of the SVG artifact is %+v, want elk, down, and no seed", svg.Layout)
	}
	if a.lastLine() != "drawn with elk, down" {
		t.Errorf("the last line is %q, want %q", a.lastLine(), "drawn with elk, down")
	}
}

func TestTheJSONOfARenderListsTheSavedFiles(t *testing.T) {
	model, config := sample("small-order-app")
	g := newTarget(t)
	plain := g.render(model, config)
	a := g.render(model, config, "--json")
	if a.code != 0 {
		t.Fatalf("render --json: exit code %d\n%s", a.code, a.stderr)
	}
	if a.stderr != plain.stderr {
		t.Errorf("--json changed the text for a person:\n%s\nwithout it:\n%s", a.stderr, plain.stderr)
	}
	out := machine(t, a.stdout)
	if out.Status != "ok" {
		t.Errorf("status %q, want ok", out.Status)
	}
	// Which warnings the sample gets is for lint to say.
	for _, d := range out.Diagnostics {
		if d.Severity != "warning" {
			t.Errorf("the render that ended ok has the diagnostic %s of the severity %s, want warnings alone", d.Code, d.Severity)
		}
		if d.Hint == "" || !strings.Contains(a.stderr, d.String()+"\n  hint: "+d.Hint+"\n") {
			t.Errorf("the standard error does not show the hint of %s under it:\n%s", d.Code, a.stderr)
		}
	}

	want := []struct {
		kind, path string
		keys       []string
	}{
		{"svg", g.svg(), []string{"kind", "layout", "path"}},
		{"png", g.png(), []string{"kind", "path", "scale"}},
		{"report", g.report, []string{"kind", "path"}},
	}
	if len(out.Artifacts) != len(want) {
		t.Fatalf("the JSON lists %d artifacts, want %d", len(out.Artifacts), len(want))
	}
	for i, w := range want {
		if got := out.Artifacts[i]; got.Kind != w.kind || got.Path != w.path || !slices.Equal(got.keys, w.keys) {
			t.Errorf("artifact %d is %s at %s with the members %v, want %s at %s with %v", i, got.Kind, got.Path, got.keys, w.kind, w.path, w.keys)
		}
	}
	svg, png := out.Artifacts[0], out.Artifacts[1]
	if svg.Layout.Engine != "tala" || svg.Layout.Direction != "right" || svg.Layout.Seed == nil || *svg.Layout.Seed != 1 {
		t.Errorf("the layout of the SVG artifact is %+v, want tala, right, seed 1", svg.Layout)
	}
	if *png.Scale != 2 {
		t.Errorf("the scale of the PNG is %d, want 2", *png.Scale)
	}
}

func TestARenderWithABrokenModelKeepsTheEarlierFiles(t *testing.T) {
	model, config := sample("small-order-app")
	g := newTarget(t)
	if a := g.render(model, config); a.code != 0 {
		t.Fatalf("render: exit code %d\n%s", a.code, a.stderr)
	}
	earlier := snapshot(t, g.dir)

	broken, _ := lintCase("schema-violation")
	a := g.render(broken, "", "--json")
	out := machine(t, a.stdout)
	if a.code != 2 || out.Status != "error" || len(out.Artifacts) != 0 {
		t.Errorf("render of a broken model: exit code %d, status %q, %d artifacts, want 2, error, none", a.code, out.Status, len(out.Artifacts))
	}
	if want := "nothing is saved; the files of an earlier render are as they were"; a.lastLine() != want {
		t.Errorf("the last line is %q, want %q", a.lastLine(), want)
	}
	if after := snapshot(t, g.dir); !reflect.DeepEqual(after, earlier) {
		t.Error("the files of the earlier render changed")
	}
}

// With ELK, a tree of 200 generated services is too large for a PNG at scale
// 2 and one of 600 for a PNG at scale 1 (100 still get the full PNG, and 400
// get none).
func TestAFigureTooLargeForAFullPNGIsSavedWithAPNGAtScale1(t *testing.T) {
	g := newTarget(t)
	model := writeFile(t, filepath.Join(g.dir, "model.json"), generatedModel(200))
	a := g.render(model, "", "--engine", "elk", "--json")
	out := machine(t, a.stdout)
	if a.code != 0 || out.Status != "ok" {
		t.Fatalf("render: exit code %d and status %q, want 0 and ok\n%s", a.code, out.Status, a.stderr)
	}
	warning := only(t, out.Diagnostics, "render.png_reduced")
	if warning.Severity != "warning" || warning.Location.Path != g.png() || warning.Hint == "" {
		t.Errorf("render.png_reduced is %+v, want a warning at the PNG with a hint", warning)
	}
	if !regexp.MustCompile(` \([a-z ]+ exceeds? limit [0-9]+\)$`).MatchString(warning.Message) {
		t.Errorf("the message %q does not end with the limit of the engine in parentheses", warning.Message)
	}
	png := out.Artifacts[1]
	if png.Kind != "png" || *png.Scale != 1 {
		t.Errorf("the second artifact is %s at scale %v, want the PNG at scale 1", png.Kind, png.Scale)
	}
}

func TestAFigureTooLargeForAnyPNGIsSavedWithoutOneAndTheEarlierPNGIsRemoved(t *testing.T) {
	g := newTarget(t)
	earlier := writeFile(t, filepath.Join(g.dir, "earlier.json"), generatedModel(3))
	if a := g.render(earlier, "", "--engine", "elk"); a.code != 0 || !exists(g.png()) {
		t.Fatalf("the earlier render: exit code %d, want 0 and a PNG\n%s", a.code, a.stderr)
	}
	model := writeFile(t, filepath.Join(g.dir, "model.json"), generatedModel(600))
	a := g.render(model, "", "--engine", "elk", "--json")
	out := machine(t, a.stdout)
	if a.code != 0 || out.Status != "partial" {
		t.Fatalf("render: exit code %d and status %q, want 0 and partial\n%s", a.code, out.Status, a.stderr)
	}
	warning := only(t, out.Diagnostics, "render.png_missing")
	if warning.Severity != "warning" || warning.Location.Path != g.png() || warning.Hint == "" {
		t.Errorf("render.png_missing is %+v, want a warning at the PNG with a hint", warning)
	}
	kinds := []string{}
	for _, artifact := range out.Artifacts {
		kinds = append(kinds, artifact.Kind)
	}
	if !slices.Equal(kinds, []string{"svg", "report"}) {
		t.Errorf("the artifacts are %v, want the SVG and the report", kinds)
	}
	if exists(g.png()) {
		t.Error("the PNG of the earlier render is still there")
	}
	if strings.Contains(a.stderr, "saved "+g.png()) || !strings.Contains(a.stderr, "saved "+g.svg()+"\n") {
		t.Errorf("the standard error must say that the SVG is saved and not that the PNG is:\n%s", a.stderr)
	}
}

func TestRerollDrawsWithANewSeedAndLeavesTheViewConfigAsItIs(t *testing.T) {
	model, sampleConfig := sample("small-order-app")
	g := newTarget(t)
	config := writeFile(t, filepath.Join(g.dir, "view.yaml"), readFile(t, sampleConfig))
	a := g.render(model, config, "--reroll", "--json")
	if a.code != 0 {
		t.Fatalf("render --reroll: exit code %d\n%s", a.code, a.stderr)
	}
	line := regexp.MustCompile(`^the new seed is ([0-9]+); to keep this figure, set "seed" under "layout" to ([0-9]+) in (.+) \(change the seed line that is there, or add one\)$`).FindStringSubmatch(a.lastLine())
	if line == nil || line[1] != line[2] || line[3] != config {
		t.Fatalf("the last line is %q, want the new seed, twice, and the path of the view config", a.lastLine())
	}
	seed, _ := strconv.ParseInt(line[1], 10, 64)
	if seed < 1 || seed > 9999 {
		t.Errorf("the new seed is %d, want one from 1 to 9999", seed)
	}
	if got := machine(t, a.stdout).Artifacts[0].Layout.Seed; got == nil || *got != seed {
		t.Errorf("the layout of the SVG artifact has the seed %v, want %d", got, seed)
	}
	if readFile(t, config) != readFile(t, sampleConfig) {
		t.Error("--reroll changed the view config")
	}

	kept := newTarget(t)
	if a := kept.render(model, config, "--seed", line[1]); a.code != 0 {
		t.Fatalf("render --seed %s: exit code %d\n%s", line[1], a.code, a.stderr)
	}
	if readFile(t, kept.svg()) != readFile(t, g.svg()) {
		t.Errorf("--seed %s drew another figure than --reroll with the seed %s", line[1], line[1])
	}
}

func TestRerollIsRefusedWithASeedAndForAFigureDrawnWithELK(t *testing.T) {
	model, _ := lintCase("no-view")
	g := newTarget(t)
	withELK := writeFile(t, filepath.Join(g.dir, "elk.yaml"), "schema_version: \"0.1\"\nlayout:\n  engine: elk\n")
	for name, args := range map[string][]string{
		"--reroll with --seed":                     {"--view", "", "--reroll", "--seed", "4"},
		"--reroll with --engine elk":               {"--view", "", "--reroll", "--engine", "elk"},
		"--reroll with elk in the view config":     {"--view", withELK, "--reroll"},
		"--reroll with elk twice, flag and config": {"--view", withELK, "--reroll", "--engine", "elk"},
	} {
		a := run(slices.Concat([]string{"render", "--model", model}, g.flags(), args)...)
		assertRefused(t, name, a)
		if exists(g.output) || exists(g.report) {
			t.Errorf("%s: the refused render saved a file", name)
		}
	}
	// The flag replaces the engine of the view config, so TALA draws.
	a := run(slices.Concat([]string{"render", "--model", model, "--view", withELK, "--reroll", "--engine", "tala"}, g.flags())...)
	if a.code != 0 || !strings.HasPrefix(a.lastLine(), "the new seed is ") {
		t.Errorf("--reroll with --engine tala over elk in the view config: exit code %d, want 0 and the new seed\n%s", a.code, a.stderr)
	}
}

// drawSample draws the 22-node sample with the given layout block in its view
// config and returns the SVG.
func drawSample(t *testing.T, layout string, flags ...string) string {
	t.Helper()
	model, config := sample("order-service")
	head, _, found := strings.Cut(readFile(t, config), "\nlayout:")
	if !found {
		t.Fatal("the view config of the sample has no layout block")
	}
	g := newTarget(t)
	own := writeFile(t, filepath.Join(g.dir, "view.yaml"), head+"\nlayout:\n"+layout)
	if a := g.render(model, own, flags...); a.code != 0 {
		t.Fatalf("render: exit code %d\n%s", a.code, a.stderr)
	}
	return readFile(t, g.svg())
}

func TestTheViewConfigDecidesTheFigure(t *testing.T) {
	base := drawSample(t, "  engine: tala\n  direction: right\n  seed: 1\n")
	for key, layout := range map[string]string{
		"engine":    "  engine: elk\n  direction: right\n  seed: 1\n",
		"direction": "  engine: tala\n  direction: down\n  seed: 1\n",
		"seed":      "  engine: tala\n  direction: right\n  seed: 2\n",
	} {
		if drawSample(t, layout) == base {
			t.Errorf("changing layout.%s in the view config left the figure as it was", key)
		}
	}
}

func TestAFlagReplacesTheValueInTheViewConfig(t *testing.T) {
	fromConfig := drawSample(t, "  engine: tala\n  direction: right\n  seed: 2\n")
	fromFlag := drawSample(t, "  engine: tala\n  direction: right\n  seed: 1\n", "--seed", "2")
	if fromConfig != fromFlag {
		t.Error("--seed 2 drew a figure other than the one layout.seed: 2 gives")
	}
}

func TestFlagsThatNameNoLayoutAreRefused(t *testing.T) {
	model, _ := lintCase("no-view")
	g := newTarget(t)
	for name, c := range map[string]struct{ flag, value string }{
		"an unknown engine":          {"-engine", "dagre"},
		"an empty engine":            {"-engine", ""},
		"an unknown direction":       {"-direction", "left"},
		"the seed 0":                 {"-seed", "0"},
		"a negative seed":            {"-seed", "-3"},
		"a seed with a fraction":     {"-seed", "1.5"},
		"a seed that is no number":   {"-seed", "lucky"},
		"a seed above the largest":   {"-seed", "2147483648"},
		"a seed too long for 64 bit": {"-seed", "99999999999999999999"},
	} {
		a := run(slices.Concat([]string{"render", "--model", model, "--view", ""}, g.flags(), []string{"-" + c.flag, c.value})...)
		assertRefused(t, name, a)
		if !strings.Contains(a.stderr, c.flag) {
			t.Errorf("%s: the message does not name the flag %s:\n%s", name, c.flag, a.stderr)
		}
		if exists(g.output) {
			t.Errorf("%s: the refused render saved a figure", name)
		}
	}
	// ELK ignores the seed, which keeps this render quick.
	a := run(slices.Concat([]string{"render", "--model", model, "--view", "", "--engine", "elk", "--direction", "down", "--seed", "2147483647"}, g.flags())...)
	if a.code != 0 {
		t.Errorf("the largest seed, elk, and down: exit code %d, want 0\n%s", a.code, a.stderr)
	}
}

func TestWithoutTheDefaultViewConfigTheModelIsDrawnAsWithAnEmptyOne(t *testing.T) {
	model, _ := lintCase("no-view")
	modelText := readFile(t, model)
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, ".ponchimap", "model.json"), modelText)
	t.Chdir(dir)

	defaults := target{dir: dir, output: "docs/architecture", report: ".ponchimap/report.md"}
	a := run("render")
	want := "saved " + defaults.svg() + "\nsaved " + defaults.png() + "\nsaved " + defaults.report + "\ndrawn with tala, right, seed 1\n"
	if a.code != 0 || a.stderr != want {
		t.Fatalf("render without flags: exit code %d, want 0 and the default paths alone\n%s", a.code, a.stderr)
	}
	if strings.Contains(readFile(t, defaults.report), "モデルと表示設定") {
		t.Error("the report names a view config among the files to redraw with, and none was given")
	}
	withoutConfig := readFile(t, defaults.svg())

	// A view config that sets nothing.
	writeFile(t, "empty.view.yaml", "schema_version: \"0.1\"\n")
	for name, config := range map[string]string{"a view config that sets nothing": "empty.view.yaml", "an empty --view": ""} {
		g := newTarget(t)
		a := g.render(".ponchimap/model.json", config)
		if a.code != 0 {
			t.Fatalf("%s: exit code %d, want 0\n%s", name, a.code, a.stderr)
		}
		if readFile(t, g.svg()) != withoutConfig {
			t.Errorf("%s: the figure differs from the one drawn without the default view config", name)
		}
		if named := strings.Contains(readFile(t, g.report), "モデルと表示設定"); named != (config != "") {
			t.Errorf("%s: the report names the view config among the files to redraw with: %t, want %t", name, named, config != "")
		}
	}

	g := newTarget(t)
	a = g.render(".ponchimap/model.json", "missing.view.yaml", "--json")
	out := machine(t, a.stdout)
	if d := only(t, out.Diagnostics, "input.unreadable"); a.code != 2 || d.Location.Path != "missing.view.yaml" {
		t.Errorf("a named view config that is missing: exit code %d and input.unreadable at %q, want 2 and the named path", a.code, d.Location.Path)
	}
	if exists(g.output) {
		t.Error("the render with a missing view config saved a figure")
	}
}

func TestALabelTheFontCannotDrawIsAnErrorThatDoesNotShowTheLabel(t *testing.T) {
	model, _ := lintCase("no-view")
	for name, c := range map[string]struct{ config, targetID string }{
		"in the label of a node":  {"schema_version: \"0.1\"\nlabels:\n  n002: \"Rocket \U0001F680 store\"\n", "n002"},
		"in the title of a group": {"schema_version: \"0.1\"\ngroups:\n  - id: g-app\n    label: \"Rocket \U0001F680 group\"\n    members: [n001]\n", "g-app"},
		"in the title":            {"schema_version: \"0.1\"\ntitle: \"Rocket \U0001F680 figure\"\n", ""},
	} {
		g := newTarget(t)
		a := g.render(model, writeFile(t, filepath.Join(g.dir, "view.yaml"), c.config), "--json")
		out := machine(t, a.stdout)
		d := only(t, out.Diagnostics, "render.missing_glyph")
		if a.code != 2 || out.Status != "error" || d.TargetID != c.targetID || !strings.Contains(d.Message, "U+1F680") || d.Hint == "" {
			t.Errorf("%s: exit code %d, status %q, and %+v, want 2, error, and the character U+1F680 at the ID %q", name, a.code, out.Status, d, c.targetID)
		}
		for _, shown := range []string{"Rocket", "\U0001F680"} {
			if strings.Contains(a.stdout+a.stderr, shown) {
				t.Errorf("%s: the output shows %q of the label", name, shown)
			}
		}
		if exists(g.output) || exists(g.report) {
			t.Errorf("%s: the render saved a file", name)
		}
	}
}

// A boundary that is not drawn as the model or the view config says it is
// told with the figure too, not by lint alone: the figure is saved, and the
// warning stands at a line of the file that can be corrected.
func TestARenderShowsTheWarningsAboutBoundariesAndSavesTheFigure(t *testing.T) {
	for name, want := range map[string]struct{ code, file string }{
		"network-without-region": {"view.network_without_region", "model.json"},
		"boundary-contested":     {"view.boundary_contested", "model.json"},
		"boundary-unbacked":      {"view.boundary_unbacked", "view.yaml"},
		"group-in-parts":         {"view.split", "view.yaml"},
	} {
		wantCodes, errors, _ := wanted(t, name)
		if errors != 0 {
			t.Fatalf("%s: the case has an error, so nothing is drawn", name)
		}
		model, config := lintCase(name)
		g := newTarget(t)
		a := g.render(model, config, "--json")
		out := machine(t, a.stdout)
		if a.code != 0 || out.Status != "ok" {
			t.Errorf("%s: exit code %d and status %q, want 0 and ok\n%s", name, a.code, out.Status, a.stderr)
		}
		if got := codes(out.Diagnostics); !slices.Equal(got, wantCodes) {
			t.Errorf("%s: the diagnostics are %v, want %v", name, got, wantCodes)
		}
		d := only(t, out.Diagnostics, want.code)
		if filepath.Base(d.Location.Path) != want.file || d.Location.Line == 0 || d.TargetID == "" {
			t.Errorf("%s: %s stands at %+v about %q, want a line of %s and the ID of what it is about", name, d.Code, d.Location, d.TargetID, want.file)
		}
		if !strings.Contains(a.stderr, d.String()+"\n  hint: "+d.Hint+"\n") {
			t.Errorf("%s: the standard error does not show %s in two lines, the second its hint:\n%s", name, d.Code, a.stderr)
		}
		if !strings.Contains(readFile(t, g.svg()), "<svg") {
			t.Errorf("%s: the saved figure is no SVG", name)
		}
	}
}
