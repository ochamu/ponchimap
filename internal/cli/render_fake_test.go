package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/ochamu/ponchimap/internal/assets"
	"github.com/ochamu/ponchimap/internal/render"
	"github.com/ochamu/ponchimap/internal/view"
)

// fakeRenderer stands in for the drawing engine in the tests of what a render
// does around the drawing.
type fakeRenderer func(ctx context.Context, opts render.Options) (*render.Result, error)

func (f fakeRenderer) Render(ctx context.Context, _ *view.Scene, opts render.Options) (*render.Result, error) {
	return f(ctx, opts)
}

// drawWith has every render of the test drawn by f.
func drawWith(t *testing.T, f fakeRenderer) {
	t.Helper()
	real := newRenderer
	newRenderer = func(assets.Set) (render.Renderer, error) { return f, nil }
	t.Cleanup(func() { newRenderer = real })
}

// quickFigure is a figure whose SVG states the layout it was asked for.
func quickFigure(_ context.Context, opts render.Options) (*render.Result, error) {
	return &render.Result{
		SVG: fmt.Appendf(nil, "<svg><!-- %s, seed %d --></svg>\n", opts.Engine, opts.Seed),
		PNG: []byte("a PNG"), PNGScale: render.FullPNGScale,
	}, nil
}

// marker stands for a text of the user's figure in an error of the engine.
const marker = "LABEL-OF-THE-USER"

func TestAFailureOfTheDrawingEngineEndsWithCode4AndNeverShowsItsText(t *testing.T) {
	model, _ := lintCase("no-view")
	for name, c := range map[string]struct {
		draw fakeRenderer
		code string
	}{
		"the layout engine cannot place the figure": {func(context.Context, render.Options) (*render.Result, error) {
			return nil, &render.LayoutError{Engine: view.EngineTALA, Err: errors.New("no room for " + marker)}
		}, "render.layout_failed"},
		"the layout failure is wrapped": {func(context.Context, render.Options) (*render.Result, error) {
			return nil, fmt.Errorf("d2render: %w", &render.LayoutError{Engine: view.EngineELK, Err: errors.New(marker)})
		}, "render.layout_failed"},
		"the engine refuses the figure": {func(context.Context, render.Options) (*render.Result, error) {
			return nil, errors.New("d2render: compile: " + marker)
		}, "tool.defect"},
		"the engine panics": {func(context.Context, render.Options) (*render.Result, error) {
			panic("unexpected " + marker)
		}, "tool.defect"},
		"the engine runs out of time": {func(context.Context, render.Options) (*render.Result, error) {
			return nil, fmt.Errorf("layout of %s: %w", marker, context.DeadlineExceeded)
		}, "run.canceled"},
	} {
		drawWith(t, c.draw)
		g := newTarget(t)
		a := g.render(model, "", "--json")
		out := machine(t, a.stdout)
		if a.code != 4 || out.Status != "error" || len(out.Artifacts) != 0 {
			t.Errorf("%s: exit code %d, status %q, %d artifacts, want 4, error, none", name, a.code, out.Status, len(out.Artifacts))
		}
		if d := only(t, out.Diagnostics, c.code); d.Severity != "error" || d.Hint == "" {
			t.Errorf("%s: %s is %+v, want an error with a hint", name, c.code, d)
		}
		if strings.Contains(a.stdout+a.stderr, marker) {
			t.Errorf("%s: the output shows the text of the engine's error:\n%s%s", name, a.stdout, a.stderr)
		}
		if exists(g.output) || exists(g.report) {
			t.Errorf("%s: the render saved a file", name)
		}
	}
}

func TestAPanicLeavesAStackTraceOnTheStandardError(t *testing.T) {
	model, _ := lintCase("no-view")
	drawWith(t, func(context.Context, render.Options) (*render.Result, error) {
		var classes map[string][]string
		classes["n001"] = nil
		return nil, nil
	})
	a := newTarget(t).render(model, "")
	// The trace names the function that panicked, and the Go runtime's own
	// words for the panic hold nothing of the input.
	for _, want := range []string{"panic: assignment to entry in nil map", "goroutine ", "TestAPanicLeavesAStackTraceOnTheStandardError", "error tool.defect: "} {
		if !strings.Contains(a.stderr, want) {
			t.Errorf("the standard error has no %q:\n%s", want, a.stderr)
		}
	}
	if a.code != 4 {
		t.Errorf("exit code %d, want 4", a.code)
	}
}

func TestACanceledRenderEndsWithCode4AndWritesNothing(t *testing.T) {
	model, _ := lintCase("no-view")
	check := func(name string, g target, a answer) {
		t.Helper()
		out := machine(t, a.stdout)
		if a.code != 4 || out.Status != "error" || !slices.Equal(codes(out.Diagnostics), []string{"run.canceled"}) {
			t.Errorf("%s: exit code %d, status %q, diagnostics %v, want 4, error, run.canceled", name, a.code, out.Status, codes(out.Diagnostics))
		}
		if want := "nothing is saved; the files of an earlier render are as they were"; a.lastLine() != want {
			t.Errorf("%s: the last line is %q, want %q", name, a.lastLine(), want)
		}
		if files := snapshot(t, g.dir); len(files) != 0 {
			t.Errorf("%s: the canceled render left the files %v", name, files)
		}
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	g := newTarget(t)
	check("canceled before the drawing, with the real engine", g, runWith(ctx, slices.Concat([]string{"render", "--model", model, "--view", "", "--json"}, g.flags())...))

	// An engine that does not look at the context draws to the end. The
	// figure it gives is not saved.
	ctx, cancel = context.WithCancel(context.Background())
	drawWith(t, func(ctx context.Context, opts render.Options) (*render.Result, error) {
		cancel()
		return quickFigure(ctx, opts)
	})
	g = newTarget(t)
	check("canceled during the drawing", g, runWith(ctx, slices.Concat([]string{"render", "--model", model, "--view", "", "--json"}, g.flags())...))
}

func TestATargetThatCannotBeWrittenEndsWithCode4AndKeepsTheEarlierFiles(t *testing.T) {
	model, _ := lintCase("no-view")
	drawWith(t, quickFigure)
	g := newTarget(t)
	if a := g.render(model, ""); a.code != 0 {
		t.Fatalf("render: exit code %d\n%s", a.code, a.stderr)
	}
	earlierReport := readFile(t, g.report)

	elsewhere := t.TempDir()
	directoryAtSVG := filepath.Join(elsewhere, "taken")
	if err := os.MkdirAll(filepath.Join(directoryAtSVG, "overview.svg"), 0o755); err != nil {
		t.Fatal(err)
	}
	fileAtOutput := writeFile(t, filepath.Join(elsewhere, "a-file"), "not a directory")
	for name, c := range map[string]struct{ output, unwritable string }{
		"a directory stands where the SVG goes": {directoryAtSVG, filepath.Join(directoryAtSVG, "overview.svg")},
		"a file stands where --output goes":     {fileAtOutput, filepath.Join(fileAtOutput, "overview.svg")},
	} {
		// Another model, so that the report of this render would differ from
		// the earlier one.
		other := writeFile(t, filepath.Join(elsewhere, "model.json"), generatedModel(3))
		a := run("render", "--model", other, "--view", "", "--output", c.output, "--report", g.report, "--json")
		out := machine(t, a.stdout)
		d := only(t, out.Diagnostics, "output.unwritable")
		if a.code != 4 || out.Status != "error" || len(out.Artifacts) != 0 || d.Location.Path != c.unwritable || d.Hint == "" {
			t.Errorf("%s: exit code %d, status %q, %d artifacts, and %+v, want 4, error, none, and the diagnostic at %s", name, a.code, out.Status, len(out.Artifacts), d, c.unwritable)
		}
		if strings.Contains(d.Message, elsewhere) {
			t.Errorf("%s: the message %q repeats the path", name, d.Message)
		}
		if readFile(t, g.report) != earlierReport {
			t.Errorf("%s: the report of the earlier render changed", name)
		}
	}
}

// changed are the files of after that before has not, or has with another
// content, by name.
func changed(before, after map[string]string) []string {
	names := []string{}
	for name, content := range after {
		if was, had := before[name]; !had || was != content {
			names = append(names, name)
		}
	}
	slices.Sort(names)
	return names
}

func TestAnEmptyPathIsRefusedAndTheEarlierFilesStayAsTheyWere(t *testing.T) {
	sampleModel, _ := lintCase("no-view")
	modelText := readFile(t, sampleModel)
	drawWith(t, quickFigure)
	for _, flag := range []string{"--model", "--output", "--report"} {
		t.Run(flag, func(t *testing.T) {
			// An empty --output is the current directory, so each case runs
			// in a directory of its own.
			dir := t.TempDir()
			t.Chdir(dir)
			writeFile(t, "model.json", modelText)
			render := []string{"render", "--model", "model.json", "--view", "", "--output", "figures", "--report", "report.md"}
			if a := run(render...); a.code != 0 {
				t.Fatalf("render: exit code %d\n%s", a.code, a.stderr)
			}
			// Another model and another seed, so that every file of this
			// render would differ from the earlier one.
			writeFile(t, "model.json", generatedModel(3))
			earlier := snapshot(t, dir)

			a := run(append(render, "--seed", "2", "--json", flag, "")...)
			if after := snapshot(t, dir); !reflect.DeepEqual(after, earlier) {
				t.Errorf("the render changed or added the files %v", changed(earlier, after))
			}
			assertRefused(t, flag, a)
			out := machine(t, a.stdout)
			if d := only(t, out.Diagnostics, "usage.invalid"); !strings.Contains(d.Message, flag) || len(out.Diagnostics) != 1 {
				t.Errorf("the diagnostics are %v with the message %q, want one usage.invalid that names %s", codes(out.Diagnostics), d.Message, flag)
			}
		})
	}
}

func TestARenderDoesNotReplaceAFigureThatItDidNotWrite(t *testing.T) {
	model, _ := lintCase("no-view")
	drawWith(t, quickFigure)
	for name, c := range map[string]struct {
		svg, png string
		refused  func(target) string
	}{
		"an SVG the user drew": {svg: "<svg xmlns=\"http://www.w3.org/2000/svg\"><title>Our system</title></svg>\n",
			refused: target.svg},
		"an SVG the user drew with D2": {svg: `<?xml version="1.0" encoding="utf-8"?><svg xmlns="http://www.w3.org/2000/svg"><svg class="d2-1 d2-svg"></svg></svg>` + "\n",
			refused: target.svg},
		"a PNG with no SVG beside it": {png: "a PNG the user exported", refused: target.png},
	} {
		g := newTarget(t)
		if c.svg != "" {
			writeFile(t, g.svg(), c.svg)
		}
		if c.png != "" {
			writeFile(t, g.png(), c.png)
		}
		earlier := snapshot(t, g.dir)
		a := g.render(model, "", "--json")
		out := machine(t, a.stdout)
		if a.code != 2 || out.Status != "error" || len(out.Artifacts) != 0 {
			t.Errorf("%s: exit code %d, status %q, %d artifacts, want 2, error, none", name, a.code, out.Status, len(out.Artifacts))
		}
		if d := only(t, out.Diagnostics, "output.not_ours"); d.Location.Path != c.refused(g) || d.Hint == "" {
			t.Errorf("%s: output.not_ours is %+v, want it at %s with a hint", name, d, c.refused(g))
		}
		if after := snapshot(t, g.dir); !reflect.DeepEqual(after, earlier) {
			t.Errorf("%s: the render changed or added the files %v", name, changed(earlier, after))
		}
	}

	g := newTarget(t)
	if a := g.render(model, ""); a.code != 0 {
		t.Fatalf("render: exit code %d\n%s", a.code, a.stderr)
	}
	if a := g.render(model, "", "--seed", "2"); a.code != 0 || !strings.Contains(readFile(t, g.svg()), "seed 2") {
		t.Errorf("a second render: exit code %d, want 0 and the figure of the earlier render replaced\n%s", a.code, a.stderr)
	}
}

func TestRerollPicksTheSeedAnewAndTheEngineDrawsWithIt(t *testing.T) {
	model, _ := lintCase("no-view")
	drawWith(t, quickFigure)
	// Without a view config the seed is 1, so a seed other than 1 was picked
	// by --reroll. A pick is 1 once in 9999 times, and four picks in a row
	// are all 1 once in 10 to the 16th.
	picked := map[string]bool{}
	for range 4 {
		g := newTarget(t)
		a := g.render(model, "", "--reroll")
		line := regexp.MustCompile(`^the new seed is ([0-9]+); `).FindStringSubmatch(a.lastLine())
		if a.code != 0 || line == nil {
			t.Fatalf("render --reroll: exit code %d and the last line %q, want 0 and the new seed", a.code, a.lastLine())
		}
		if n, err := strconv.Atoi(line[1]); err != nil || n < 1 || n > 9999 {
			t.Errorf("the new seed is %s, want one from 1 to 9999", line[1])
		}
		if svg, want := readFile(t, g.svg()), string(figureMark)+"<svg><!-- tala, seed "+line[1]+" --></svg>\n"; svg != want {
			t.Errorf("the engine was asked for %q, want %q", svg, want)
		}
		picked[line[1]] = true
	}
	if len(picked) == 1 && picked["1"] {
		t.Error("four renders with --reroll all drew with the seed 1, the one a render without --reroll draws with")
	}
}
