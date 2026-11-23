package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math/rand/v2"
	"path/filepath"

	"github.com/ochamu/ponchimap/internal/assets"
	"github.com/ochamu/ponchimap/internal/diag"
	"github.com/ochamu/ponchimap/internal/lint"
	"github.com/ochamu/ponchimap/internal/render"
	"github.com/ochamu/ponchimap/internal/render/d2render"
	"github.com/ochamu/ponchimap/internal/report"
	"github.com/ochamu/ponchimap/internal/view"
	"github.com/ochamu/ponchimap/internal/workspace"
)

// A seed that --reroll picks is short enough to copy by hand into the view
// config.
const largestRerolledSeed = 9999

// newRenderer makes the Renderer of a render. A test puts its own in its
// place, to see what a render does with a failure that no quick input causes.
var newRenderer = func(set assets.Set) (render.Renderer, error) { return d2render.New(set) }

// drawing is one render held in memory, before any file is touched.
type drawing struct {
	// files are in the order they are saved.
	files []workspace.File
	// saved is the outcome once the files are in place. Its diagnostics are
	// the ones about the saved files alone.
	saved outcome
}

func runRender(ctx context.Context, args []string, stdout, stderr io.Writer) outcome {
	d, o := draw(ctx, args, stdout)
	if d == nil {
		// Without a drawing, only a command line that asks for the flags
		// ends well.
		if o.result != ok {
			o.lines = append(o.lines, "nothing is saved; the files of an earlier render are as they were")
		}
		return o
	}
	if err := workspace.Save(d.files); err != nil {
		return outcome{result: runError, diagnostics: append(o.diagnostics, unwritable(err))}
	}
	d.saved.diagnostics = append(o.diagnostics, d.saved.diagnostics...)
	return d.saved
}

// draw checks the inputs and makes every file of the render in memory. It
// gives no drawing when the command ends before that, with the outcome to end
// with. With a drawing, the outcome holds the diagnostics so far.
func draw(ctx context.Context, args []string, stdout io.Writer) (*drawing, outcome) {
	flags := newFlagSet("render")
	in := addInputFlags(flags)
	svgName, pngName := workspace.FigureBaseName+".svg", workspace.FigureBaseName+".png"
	output := flags.String("output", workspace.DefaultFigureDir, "the `directory` that gets "+svgName+" and "+pngName+" (default "+workspace.DefaultFigureDir+")")
	reportPath := flags.String("report", workspace.DefaultReport, "the report `file` (default "+workspace.DefaultReport+")")
	var override view.LayoutConfig
	addLayoutFlags(flags, &override)
	reroll := flags.Bool("reroll", false, fmt.Sprintf("draw with a newly picked TALA seed from 1 to %d and print it; the view config stays as it is", largestRerolledSeed))
	if o, stop := parseFlags(flags, args, stdout); stop {
		return nil, o
	}
	if *reroll && override.Seed != 0 {
		return nil, refused("--reroll picks the seed, so --seed cannot be given with it")
	}
	if o, stop := unnamed(in.modelPath(), flagPath{"--output", "directory", *output},
		flagPath{"--report", "file", *reportPath}); stop {
		return nil, o
	}
	svgPath, pngPath := filepath.Join(*output, svgName), filepath.Join(*output, pngName)
	model, config := in.read()
	outputs := []string{svgPath, pngPath, *reportPath}
	for _, path := range outputs {
		outputs = append(outputs, workspace.TemporaryPath(path))
	}
	if err := workspace.CheckPaths([]string{model.Path, config.Path}, outputs); err != nil {
		return nil, refused("%s", err)
	}
	if path := notOurs(svgPath, pngPath); path != "" {
		return nil, outcome{result: inputError, diagnostics: diag.List{notReplaced(path)}}
	}
	if *reroll {
		override.Seed = view.Seed(1 + rand.Int64N(largestRerolledSeed))
	}

	checked, list := lint.Check(lint.Input{Model: model, View: config, Layout: override})
	if checked == nil {
		return nil, outcome{result: inputError, diagnostics: list}
	}
	layout := checked.Layout()
	// The engine can come from the view config, so this is known only now.
	if *reroll && layout.Engine == view.EngineELK {
		return nil, outcome{result: inputError, diagnostics: append(list,
			usageProblem("--reroll picks a TALA seed, and this figure is drawn with elk, which has no seed"))}
	}

	set := assets.Bundled()
	var res *render.Result
	renderer, err := newRenderer(set)
	if err == nil {
		res, err = renderer.Render(ctx, checked.Scene(), render.Options{Engine: layout.Engine, Seed: int64(layout.Seed)})
	}
	if err != nil {
		problem, result := failure(err)
		return nil, outcome{result: result, diagnostics: append(list, problem)}
	}

	drawnWith := drawnLayout{Engine: string(layout.Engine), Direction: string(layout.Direction)}
	if layout.Engine == view.EngineTALA {
		drawnWith.Seed = int64(layout.Seed)
	}
	evidence := report.Build(checked.Model(), checked.Scene(), report.Drawing{Layout: layout, View: config.Path != ""})

	// One list gives the files, the artifacts, and the lines, so that the
	// artifacts are in the order the files are saved.
	d := &drawing{saved: outcome{diagnostics: pngProblem(res, pngPath)}}
	for _, f := range []struct {
		artifact
		data []byte
	}{
		{artifact{Kind: "svg", Path: svgPath, Layout: &drawnWith}, marked(res.SVG)},
		// Without a PNG the nil data has the PNG of an earlier render
		// removed: it would show another figure than the new SVG.
		{artifact{Kind: "png", Path: pngPath, Scale: res.PNGScale}, res.PNG},
		{artifact{Kind: "report", Path: *reportPath}, evidence.Markdown()},
	} {
		d.files = append(d.files, workspace.File{Path: f.Path, Data: f.data})
		if f.data != nil {
			d.saved.artifacts = append(d.saved.artifacts, f.artifact)
			d.saved.lines = append(d.saved.lines, "saved "+f.Path)
		}
	}
	if res.PNG == nil {
		d.saved.result = partial
	}
	drawnLine := fmt.Sprintf("drawn with %s, %s", drawnWith.Engine, drawnWith.Direction)
	if layout.Engine == view.EngineTALA {
		drawnLine += fmt.Sprintf(", seed %d", drawnWith.Seed)
	}
	d.saved.lines = append(d.saved.lines, drawnLine)
	if *reroll {
		where := in.view
		if where == "" {
			where = "a view config"
		}
		d.saved.lines = append(d.saved.lines, fmt.Sprintf("the new seed is %d; to keep this figure, set \"seed\" under \"layout\" to %d in %s (change the seed line that is there, or add one)", drawnWith.Seed, drawnWith.Seed, where))
	}

	// A Renderer does not look at the context in every step. Once the saving
	// starts it is not stopped, so this is the last place to stop.
	if err := ctx.Err(); err != nil {
		problem, result := failure(err)
		return nil, outcome{result: result, diagnostics: append(list, problem)}
	}
	return d, outcome{diagnostics: list}
}

// failure is the diagnostic for a render that gave no figure, and how the run
// ends. The user can replace a character the fonts cannot draw, so that is an
// error of the input. No message carries the text of an engine's error, which
// may quote a label.
func failure(err error) (diag.Diagnostic, result) {
	var glyph *render.MissingGlyphError
	var layout *render.LayoutError
	switch {
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		return diag.Diagnostic{Severity: diag.Error, Code: "run.canceled",
			Message: "the run was canceled before anything was saved",
			Hint:    "Run the command again."}, runError
	case errors.As(err, &glyph):
		return diag.Diagnostic{Severity: diag.Error, Code: "render.missing_glyph", TargetID: glyph.SceneID,
			Message: fmt.Sprintf("the bundled font has no glyph for the character U+%04X", glyph.Rune),
			Hint:    "Replace the character in the label, the title, or the role of the element, or in the title of the figure."}, inputError
	case errors.As(err, &layout):
		return diag.Diagnostic{Severity: diag.Error, Code: "render.layout_failed",
			Message: "the layout engine could not place the figure",
			Hint:    "Draw again with --engine elk or with another --seed. A large figure can be made smaller with hidden or aggregates in the view config."}, runError
	}
	return defect("the drawing engine could not draw a figure that passed every check; this is a defect of the tool, please report it"), runError
}

// pngProblem is the warning about a PNG below the full scale, or nothing. It
// ends with the engine's limit when the engine named one.
func pngProblem(res *render.Result, path string) diag.List {
	d := diag.Diagnostic{Severity: diag.Warning, Location: diag.Location{Path: path}}
	switch {
	case res.PNG == nil:
		d.Code = "render.png_missing"
		d.Message = "the drawing engine could not draw a PNG of this figure, so none is saved and the PNG of an earlier render is removed"
		d.Hint = "Use the SVG. To get a PNG, make the figure smaller with hidden or aggregates in the view config."
	case res.PNGScale != render.FullPNGScale:
		d.Code = "render.png_reduced"
		d.Message = fmt.Sprintf("the drawing engine could not draw the PNG at scale %d, so it is saved at scale %d", render.FullPNGScale, res.PNGScale)
		d.Hint = fmt.Sprintf("The PNG can be used as it is. To get one at scale %d, make the figure smaller with hidden or aggregates in the view config.", render.FullPNGScale)
	default:
		return nil
	}
	if res.PNGLimit != "" {
		d.Message += " (" + res.PNGLimit + ")"
	}
	return diag.List{d}
}

func unwritable(err error) diag.Diagnostic {
	// Save gives no other error. Its cause comes without the path, which is
	// in the location.
	failed := err.(*workspace.Error)
	return diag.Diagnostic{Severity: diag.Error, Code: "output.unwritable", Location: diag.Location{Path: failed.Path},
		Message: "the file cannot be written: " + failed.Err.Error(),
		Hint:    "Check that the directory of the path can be written to and that the path names no directory and no symbolic link."}
}
