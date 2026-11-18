package d2render

import (
	"fmt"
	"math"
	"strings"
	"sync"

	"golang.org/x/image/font"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/font/sfnt"
	"golang.org/x/image/math/fixed"

	"github.com/ochamu/ponchimap/internal/render"
)

// lineHeight is the height of one text line in ems. D2 puts the first
// baseline one font size below the top of the label box and spaces further
// lines by box height / line count, so with the font's 0.88 em ascent and
// 0.12 em descent this factor leaves the same margin above and below the ink.
const lineHeight = 1.24

type faceKey struct {
	bold bool
	px   int
}

// measurer sizes text from the advance widths of the same font bytes D2
// draws with, so a label box and its glyphs agree.
type measurer struct {
	regular, bold *opentype.Font

	mu    sync.Mutex
	faces map[faceKey]font.Face
}

func newMeasurer(regular, bold []byte) (*measurer, error) {
	r, err := opentype.Parse(regular)
	if err != nil {
		return nil, fmt.Errorf("d2render: parse regular font: %w", err)
	}
	b, err := opentype.Parse(bold)
	if err != nil {
		return nil, fmt.Errorf("d2render: parse bold font: %w", err)
	}
	return &measurer{regular: r, bold: b, faces: map[faceKey]font.Face{}}, nil
}

func (m *measurer) font(bold bool) *opentype.Font {
	if bold {
		return m.bold
	}
	return m.regular
}

func (m *measurer) face(px int, bold bool) (font.Face, error) {
	key := faceKey{bold, px}
	if f, ok := m.faces[key]; ok {
		return f, nil
	}
	f, err := opentype.NewFace(m.font(bold), &opentype.FaceOptions{Size: float64(px), DPI: 72, Hinting: font.HintingNone})
	if err != nil {
		return nil, fmt.Errorf("d2render: font face at %dpx: %w", px, err)
	}
	m.faces[key] = f
	return f, nil
}

// size returns the pixel box of text drawn at px. The width is the widest
// line; the height is lineHeight per line.
func (m *measurer) size(text string, px int, bold bool) (w, h int, err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	f, err := m.face(px, bold)
	if err != nil {
		return 0, 0, err
	}
	lines := strings.Split(text, "\n")
	var widest fixed.Int26_6
	for _, line := range lines {
		widest = max(widest, font.MeasureString(f, line))
	}
	return widest.Ceil(), int(math.Ceil(float64(len(lines)*px) * lineHeight)), nil
}

// checkGlyphs fails on the first rune of text the font cannot draw. D2 has no
// fallback font here, so such a rune would come out as an empty box.
func (m *measurer) checkGlyphs(text string, bold bool) error {
	var buf sfnt.Buffer
	for _, r := range text {
		if r == '\n' {
			continue
		}
		idx, err := m.font(bold).GlyphIndex(&buf, r)
		if err != nil {
			return fmt.Errorf("d2render: look up U+%04X in the font: %w", r, err)
		}
		if idx == 0 {
			return &render.MissingGlyphError{Rune: r}
		}
	}
	return nil
}
