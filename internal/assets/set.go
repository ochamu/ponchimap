package assets

// Set is the fonts and the icons a figure is drawn with.
type Set struct {
	Fonts FontSet
	icons map[string][]byte // by icon key; always holds GenericIcon
}

// Bundled is the set embedded in the tool.
func Bundled() Set {
	return Set{
		Fonts: FontSet{
			Family:  "BIZ UDPGothic",
			Regular: fontRegular,
			Bold:    fontBold,
		},
		icons: bundledIcons(),
	}
}

// Icon is the SVG of an icon key. An unknown key gives the generic icon, so
// a service without an icon is still drawn.
func (s Set) Icon(key string) []byte {
	if svg, ok := s.icons[key]; ok {
		return svg
	}
	return s.icons[GenericIcon]
}
