package assets

import _ "embed"

//go:embed fonts/BIZUDPGothic-Regular.ttf
var fontRegular []byte

//go:embed fonts/BIZUDPGothic-Bold.ttf
var fontBold []byte

// FontSet is the one typeface used to measure and to draw. Using the same
// bytes for both is what keeps boxes and glyphs in agreement.
type FontSet struct {
	Family  string
	Regular []byte
	Bold    []byte
}
