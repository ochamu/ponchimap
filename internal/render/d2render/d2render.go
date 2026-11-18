// Package d2render draws a Scene with D2. It is the only package that
// imports D2; nothing of D2 crosses the render.Renderer boundary.
//
// The adapter prepares what D2 is given (the script and the measured size of
// every label) and settles the contents of each card. Where D2 puts the
// cards, the lines, and the labels of the lines is drawn as D2 gives it.
package d2render

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"runtime/debug"
	"strings"
	"sync"

	"github.com/d2lang/d2/d2compiler"
	"github.com/d2lang/d2/d2graph"
	"github.com/d2lang/d2/d2layouts/d2elklayout"
	"github.com/d2lang/d2/d2layouts/d2talalayout"
	"github.com/d2lang/d2/d2lib"
	"github.com/d2lang/d2/d2renderers/d2fonts"
	"github.com/d2lang/d2/d2renderers/d2svg"
	"github.com/d2lang/d2/d2target"
	"github.com/d2lang/d2/lib/log"

	"github.com/ochamu/ponchimap/internal/assets"
	"github.com/ochamu/ponchimap/internal/render"
	"github.com/ochamu/ponchimap/internal/view"
)

const padding = 24

// ELK's gaps, in layout units. D2's defaults (70 between layers, 40 between
// an edge and a node, 50 inside a container) leave more room than the cards
// and inline labels need and make a layered figure about a tenth wider.
const (
	elkLayerGap     = 40
	elkEdgeNodeGap  = 24
	elkGroupPadding = 30
)

var elkOptions = func() d2elklayout.ConfigurableOpts {
	o := d2elklayout.DefaultOpts
	o.NodeSpacing = elkLayerGap
	o.EdgeNodeSpacing = elkEdgeNodeGap
	o.Padding = fmt.Sprintf("[top=%d,left=%d,bottom=%d,right=%d]", elkGroupPadding, elkGroupPadding, elkGroupPadding, elkGroupPadding)
	return o
}()

func layoutFor(opts render.Options) (d2graph.LayoutGraph, error) {
	switch opts.Engine {
	case view.EngineTALA:
		// TALA lays out its default seeds 1 to 3 and keeps one of them when
		// given none, so a seed that is not named would hide which layout
		// the figure shows.
		if opts.Seed < 1 {
			return nil, fmt.Errorf("d2render: TALA needs a seed of 1 or more, got %d", opts.Seed)
		}
		return placing(opts.Engine, func(ctx context.Context, g *d2graph.Graph) error {
			return d2talalayout.Layout(ctx, g, &d2talalayout.Options{Seeds: []int64{opts.Seed}})
		}), nil
	case view.EngineELK:
		return placing(opts.Engine, func(ctx context.Context, g *d2graph.Graph) error {
			return d2elklayout.Layout(ctx, g, &elkOptions)
		}), nil
	}
	return nil, fmt.Errorf("d2render: unsupported layout engine %q", opts.Engine)
}

// placing gives the error of a layout function as a render.LayoutError. An
// engine that stops because its context is done fails as well. That is not a
// figure the engine could not place, so it comes back as the context's error.
func placing(engine view.Engine, layout d2graph.LayoutGraph) d2graph.LayoutGraph {
	return func(ctx context.Context, g *d2graph.Graph) error {
		err := layout(ctx, g)
		if err == nil {
			return nil
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return &render.LayoutError{Engine: engine, Err: err}
	}
}

type Renderer struct {
	family  *d2fonts.FontFamily
	measure *measurer
	set     assets.Set
}

var _ render.Renderer = (*Renderer)(nil)

// D2 keeps the fonts of a family under its name for the whole process, so
// fonts registered a second time would change what every earlier Renderer
// draws with. The first Renderer registers its fonts.
var registered struct {
	sync.Mutex
	family *d2fonts.FontFamily
}

// New gives a Renderer that draws with the icons of set and with the fonts of
// the first set a Renderer of the process was made with. The tool has one set.
func New(set assets.Set) (*Renderer, error) {
	registered.Lock()
	defer registered.Unlock()
	// Parsed before D2 gets them, so that fonts D2 cannot draw with are never
	// the fonts of the process.
	m, err := newMeasurer(set.Fonts.Regular, set.Fonts.Bold)
	if err != nil {
		return nil, err
	}
	if registered.family == nil {
		// D2 writes a checksum into the font bytes it is given, so it gets
		// copies and the set keeps the bytes of the font files.
		regular, bold := bytes.Clone(set.Fonts.Regular), bytes.Clone(set.Fonts.Bold)
		// D2 falls back to its bundled Latin font for a style passed as nil, so
		// the italic and semibold slots get the regular and bold faces.
		family, err := d2fonts.AddFontFamily(set.Fonts.Family, regular, regular, bold, bold)
		if err != nil {
			return nil, fmt.Errorf("d2render: register font: %w", err)
		}
		registered.family = family
	}
	return &Renderer{family: registered.family, measure: m, set: set}, nil
}

const d2Module = "github.com/d2lang/d2"

// Version is the version of the D2 module linked into the binary, or unknown
// for a binary that carries no module information.
func Version() string {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return "unknown"
	}
	for _, dep := range info.Deps {
		if dep.Path != d2Module {
			continue
		}
		if dep.Replace != nil {
			dep = dep.Replace
		}
		if dep.Version != "" {
			return dep.Version
		}
	}
	return "unknown"
}

func (r *Renderer) Render(ctx context.Context, scene *view.Scene, opts render.Options) (*render.Result, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	layout, err := layoutFor(opts)
	if err != nil {
		return nil, err
	}
	script, err := writeScript(ctx, scene, r.measure, r.set)
	if err != nil {
		return nil, err
	}
	texts, err := r.measureTexts(script)
	if err != nil {
		var missing *render.MissingGlyphError
		if errors.As(err, &missing) {
			missing.SceneID = sceneIDWith(scene, missing.Rune)
		}
		return nil, err
	}
	// D2 writes a width and height on the SVG root only when given a scale.
	// Without them a browser fits the figure to its window and page zoom
	// does not enlarge it.
	pad, omitVersion, fullSize := int64(padding), true, 1.0
	renderOpts := &d2svg.RenderOpts{Pad: &pad, OmitVersion: &omitVersion, Scale: &fullSize}

	ctx = log.WithDefault(ctx)
	diagram, err := r.compile(ctx, script, texts, opts.Engine, layout, renderOpts)
	if err != nil {
		return nil, err
	}
	settleCards(diagram.Shapes)

	svg, err := d2svg.Render(diagram, renderOpts)
	if err != nil {
		return nil, fmt.Errorf("d2render: svg: %w", err)
	}
	png, pngScale, pngErr, err := rasterize(ctx, diagram, pad)
	if err != nil {
		return nil, fmt.Errorf("d2render: png: %w", err)
	}
	return &render.Result{SVG: svg, PNG: png, PNGScale: pngScale, PNGLimit: pngLimit(pngErr)}, nil
}

// compile lays the script out with one layout function and fails unless D2
// asked for exactly the requested engine.
func (r *Renderer) compile(ctx context.Context, script string, texts []*d2target.MText, engine view.Engine, layout d2graph.LayoutGraph, renderOpts *d2svg.RenderOpts) (*d2target.Diagram, error) {
	requested := string(engine)
	var ran []string
	diagram, _, err := d2lib.Compile(ctx, script, &d2lib.CompileOptions{
		// No Ruler: a label missing from MeasuredTexts must fail instead of
		// being sized by D2's own measurer, which over-measures Japanese.
		MeasuredTexts: texts,
		FontFamily:    r.family,
		Layout:        &requested,
		LayoutResolver: func(name string) (d2graph.LayoutGraph, error) {
			ran = append(ran, name)
			return layout, nil
		},
	}, renderOpts)
	if err != nil {
		return nil, fmt.Errorf("d2render: compile: %w", err)
	}
	if len(ran) != 1 || ran[0] != requested {
		return nil, fmt.Errorf("d2render: asked for layout engine %q but D2 resolved %q", requested, ran)
	}
	return diagram, nil
}

// measureTexts asks D2 for the label list instead of rebuilding it, so the
// entries match D2's lookup key by construction.
func (r *Renderer) measureTexts(script string) ([]*d2target.MText, error) {
	g, _, err := d2compiler.Compile("", strings.NewReader(script), nil)
	if err != nil {
		return nil, fmt.Errorf("d2render: compile: %w", err)
	}
	texts := g.Texts()
	for _, t := range texts {
		if err := r.measure.checkGlyphs(t.Text, t.IsBold); err != nil {
			return nil, err
		}
		w, h, err := r.measure.size(t.Text, t.FontSize, t.IsBold)
		if err != nil {
			return nil, err
		}
		t.Dimensions = d2target.TextDimensions{Width: w, Height: h}
	}
	return texts, nil
}

// sceneIDWith is the ID of the first group, node, or relation of the scene
// with a text that holds r. D2's label list, which the glyphs are checked
// over, does not say which element a text belongs to.
func sceneIDWith(s *view.Scene, r rune) string {
	for _, g := range s.Groups {
		if strings.ContainsRune(g.Label, r) {
			return g.ID
		}
	}
	for _, n := range s.Nodes {
		if strings.ContainsRune(n.Title, r) || strings.ContainsRune(n.Role, r) {
			return n.ID
		}
	}
	for _, e := range s.Edges {
		if strings.ContainsRune(e.Label, r) {
			return e.ID
		}
	}
	return ""
}
