// Package render defines the boundary between the display model and whatever
// engine lays it out and draws it. Engine types stay behind this boundary.
package render

import (
	"context"
	"fmt"

	"github.com/ochamu/ponchimap/internal/view"
)

// Options name the one layout a render runs. A render never tries a second
// engine or a second seed.
type Options struct {
	Engine view.Engine
	// Seed picks one of the layouts TALA can give for the same scene. The
	// caller names it and keeps it, so that the same scene and options give
	// the same bytes. ELK has no seed and ignores it.
	Seed int64
}

type Renderer interface {
	Render(ctx context.Context, scene *view.Scene, opts Options) (*Result, error)
}

// FullPNGScale is the scale of the PNG unless the engine cannot draw the
// figure that large.
const FullPNGScale = 2

type Result struct {
	SVG []byte
	// PNG is the figure at PNGScale times the width and height of the SVG.
	// An engine limits how large a PNG it draws. A figure it cannot draw at
	// FullPNGScale gets a PNG at scale 1, and a figure it cannot draw at
	// scale 1 either gets none: PNG is nil and PNGScale is 0. The SVG is there
	// in every case.
	PNG      []byte
	PNGScale int
	// PNGLimit says which limit of the engine kept the PNG from the full scale,
	// in the engine's words, such as "frame pixels exceed limit 67108864". It
	// holds numbers and fixed words only, so it is safe to show. It is empty for
	// a PNG at FullPNGScale, and when the engine's reason is not one of its
	// limits: that text may quote the figure.
	PNGLimit string
}

// MissingGlyphError says the fonts have no glyph for a character of a text
// in the scene. It names the character and never the text, which may be a
// label of the user.
type MissingGlyphError struct {
	Rune rune
	// SceneID is the ID of the group, node, or relation of the scene whose
	// text holds the character. It is empty when the text is the title, the
	// legend, or a note.
	SceneID string
}

func (e *MissingGlyphError) Error() string {
	return fmt.Sprintf("the font has no glyph for U+%04X", e.Rune)
}

// LayoutError says the layout engine could not place the figure. Err is the
// engine's own error.
type LayoutError struct {
	Engine view.Engine
	Err    error
}

func (e *LayoutError) Error() string {
	return fmt.Sprintf("the layout engine %s could not place the figure: %v", e.Engine, e.Err)
}

func (e *LayoutError) Unwrap() error { return e.Err }
