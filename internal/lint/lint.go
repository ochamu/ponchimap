// Package lint checks a model and a view config before anything is drawn.
// Check is the one way from input files to a scene: render draws what Check
// hands out.
package lint

import (
	"github.com/ochamu/ponchimap/internal/diag"
	"github.com/ochamu/ponchimap/internal/input"
	"github.com/ochamu/ponchimap/internal/model"
	"github.com/ochamu/ponchimap/internal/view"
)

// Source is one input text. Path names it in diagnostics. The text is read
// from Path when Text is nil. The zero Source is no input.
type Source struct {
	Path string
	Text []byte
}

func (s Source) given() bool { return s.Path != "" || s.Text != nil }

func (s Source) read() ([]byte, diag.List) {
	if s.Text != nil {
		return s.Text, nil
	}
	return input.ReadFile(s.Path)
}

// Input is what one check reads.
type Input struct {
	Model Source
	// View is optional. Without it the model is drawn with the default config.
	View Source
	// Layout overrides the layout of the view config for this run, field by
	// field: a zero field keeps what the config says. It changes where things
	// are drawn, and no diagnostic about the model or the view config. A
	// field that names no layout is the error lint.invalid_layout.
	Layout view.LayoutConfig
}

// Checked is an input without an error. Its scene was built from its model
// and the view config fitted to that model, with the layout override applied.
type Checked struct {
	model  *model.Model
	scene  *view.Scene
	layout view.LayoutConfig
}

// Model is the model that was checked.
func (c *Checked) Model() *model.Model { return c.model }

// Scene is the figure to draw: the one way a command comes by a scene.
func (c *Checked) Scene() *view.Scene { return c.scene }

// Layout is the engine, the direction, and the seed the scene is to be drawn
// with, after the defaults and the override.
func (c *Checked) Layout() view.LayoutConfig { return c.layout }

const defaultViewConfig = "schema_version: \"0.1\"\n"

// The diagnostics about a check as a whole, which no place in a file stands
// for. Each is made from an error whose text may name values of the input,
// so that text is never passed on.
var (
	toolDefect = diag.Diagnostic{Severity: diag.Error, Code: "tool.defect",
		Message: "the inputs passed every check but the figure could not be built; this is a defect of the tool",
		Hint:    "Report the defect with inputs that cause it."}
	invalidLayout = diag.Diagnostic{Severity: diag.Error, Code: "lint.invalid_layout",
		Message: "the layout override names no layout",
		Hint:    "Use tala or elk for the engine, right or down for the direction, and a seed of 1 or more."}
)

// Check runs every check. The list is in its fixed order, and the same input
// gives the same list. Checked is nil exactly when the list has an error.
//
// The diagnostics about the model do not depend on the view config, and
// those about the text and the form of the view config do not depend on the
// model, so one broken file does not hide what is wrong with
// the other. The warnings about the figure are the exception: they are made
// when the figure is built, from both files, and only without an error.
func Check(in Input) (*Checked, diag.List) {
	config := in.View
	if !config.given() {
		config = Source{Text: []byte(defaultViewConfig)}
	}
	m, modelDoc, list := readModel(in.Model)
	file, configDoc, onConfig := readViewConfig(config)

	var fitted *view.Config
	if m != nil {
		list = append(list, modelProblems(m, func(pointer string) diag.Location {
			at := modelDoc.Locate(pointer, false)
			at.Path = in.Model.Path
			return at
		})...)
		if file != nil {
			var unfit diag.List
			fitted, unfit = file.Resolve(m)
			onConfig = append(onConfig, unfit...)
		}
	}
	list = append(list, onConfig...)

	var checked *Checked
	// Without an error the model was read and the view config was fitted to
	// it.
	if !list.HasErrors() {
		var onFigure diag.List
		checked, onFigure = build(fitted, built{in: in, m: m, modelDoc: modelDoc, configDoc: configDoc})
		list = append(list, onFigure...)
	}
	list.Sort()
	return checked, list
}

// build returns the scene with the warnings of view.Build about it, or the
// one error that says why there is none.
func build(fitted *view.Config, b built) (*Checked, diag.List) {
	fitted.Layout = overridden(fitted.Layout, b.in.Layout)
	// The layout of the config was held to its values when the config was
	// read, so only the override can name no layout.
	if fitted.Layout.Normalize() != nil {
		return nil, diag.List{invalidLayout}
	}
	scene, err := view.Build(b.m, fitted)
	if err != nil {
		return nil, diag.List{b.failure()}
	}
	return &Checked{model: b.m, scene: scene, layout: fitted.Layout}, b.warnings(scene)
}

func readModel(source Source) (*model.Model, *input.Document, diag.List) {
	text, unreadable := source.read()
	if unreadable != nil {
		return nil, nil, unreadable
	}
	m, list := model.Parse(source.Path, text)
	// The model keeps no places, so the text is read once more for them.
	// model.Parse has reported what this reading finds.
	doc, _ := input.DecodeJSON(text)
	return m, doc, list
}

func readViewConfig(source Source) (*view.ConfigFile, *input.Document, diag.List) {
	text, unreadable := source.read()
	if unreadable != nil {
		return nil, nil, unreadable
	}
	file, list := view.ParseConfig(source.Path, text)
	doc, _ := input.DecodeYAML(text)
	return file, doc, list
}

func overridden(layout, override view.LayoutConfig) view.LayoutConfig {
	if override.Engine != "" {
		layout.Engine = override.Engine
	}
	if override.Direction != "" {
		layout.Direction = override.Direction
	}
	if override.Seed != 0 {
		layout.Seed = override.Seed
	}
	return layout
}
