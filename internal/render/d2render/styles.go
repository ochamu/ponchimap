package d2render

import (
	"fmt"
	"strings"

	"github.com/ochamu/ponchimap/internal/view"
)

type class struct {
	name string
	part part
	d2   string
}

type part int

const (
	partOther part = iota
	partGroup
	partCard
)

const (
	classCard        = "card"
	classInnerBound  = "boundary-inner"
	classIcon        = "icon"
	classCardText    = "card-text"
	classTitle       = "title"
	classRole        = "role"
	classMarkRow     = "mark-row"
	classMarkGap     = "mark-gap"
	classDiagTitle   = "diagram-title"
	classLegend      = "legend"
	classLegendRows  = "legend-rows"
	classLegendText  = "legend-text"
	classLegendNote  = "legend-note"
	classSwatch      = "swatch"
	classSwatchLine  = "swatch-line"
	classSwatchPoint = "swatch-point"
	classSwatchGap   = "swatch-gap"
)

var (
	frameClass = map[view.Frame]string{view.FrameSolid: "frame-solid", view.FrameDashed: "frame-dashed"}
	lineClass  = map[view.Line]string{view.LineSolid: "line-solid", view.LineDashed: "line-dashed"}
	groupClass = map[view.GroupKind]string{view.GroupBoundary: "group-boundary", view.GroupLogical: "group-logical"}
	markClass  = map[view.MarkKind]string{view.MarkUnverified: "mark-unverified", view.MarkMismatch: "mark-mismatch"}
)

// Sizes are in layout units. Both engines keep fixed gaps between nodes
// whatever the nodes hold, so the type is set large against those gaps: a
// figure fitted to a page then spends more of the page on text. Lines and
// icons grow with the type. Edge labels stay smaller than card text because
// they have to fit into those gaps.
const (
	titleFontSize      = 23
	roleFontSize       = 20
	markFontSize       = 17
	edgeFontSize       = 15
	boundaryFontSize   = 21
	logicalFontSize    = 20
	headingFontSize    = 30
	legendFontSize     = 18
	legendNoteFontSize = 17

	iconSize     = 60
	cardGap      = 15
	markHeight   = 27
	markPadX     = 11
	swatchWidth  = 45
	swatchHeight = 24
	strokeWidth  = 3

	roleColumns      = 28
	edgeLabelColumns = 26
	noteColumns      = 84

	legendPairsPerRow = 2
)

var classes = []class{
	{name: classCard, part: partCard, d2: fmt.Sprintf(`label: ""; grid-columns: 2; grid-gap: %d; style: {border-radius: 8}`, cardGap)},
	{name: frameClass[view.FrameSolid], d2: fmt.Sprintf(`style: {fill: "#ffffff"; stroke: "#334155"; stroke-width: %d}`, strokeWidth)},
	{name: frameClass[view.FrameDashed], d2: fmt.Sprintf(`style: {fill: "#ffffff"; stroke: "#64748b"; stroke-width: %d; stroke-dash: 4}`, strokeWidth)},
	{name: classIcon, d2: fmt.Sprintf(`shape: image; label: ""; width: %d; height: %d`, iconSize, iconSize)},
	{name: classCardText, d2: `label: ""; grid-columns: 1; horizontal-gap: 0; vertical-gap: 2; style: {fill: transparent; stroke-width: 0}`},
	{name: classTitle, d2: fmt.Sprintf(`shape: text; style: {font-size: %d; bold: true; font-color: "#0f172a"}`, titleFontSize)},
	{name: classRole, d2: fmt.Sprintf(`shape: text; style: {font-size: %d; font-color: "#475569"}`, roleFontSize)},
	{name: classMarkRow, d2: `label: ""; grid-rows: 1; horizontal-gap: 0; vertical-gap: 3; style: {fill: transparent; stroke-width: 0}`},
	{name: classMarkGap, d2: fmt.Sprintf(`label: ""; width: 5; height: %d; style: {fill: transparent; stroke-width: 0}`, markHeight)},
	{name: markClass[view.MarkUnverified], d2: markLook(`fill: "#fef3c7"; stroke: "#b45309"; font-color: "#92400e"`)},
	{name: markClass[view.MarkMismatch], d2: markLook(`fill: "#fee2e2"; stroke: "#b91c1c"; font-color: "#991b1b"`)},
	{name: groupClass[view.GroupBoundary], part: partGroup, d2: fmt.Sprintf(`label.near: top-left; style: {fill: "#f1f5f9"; stroke: "#64748b"; stroke-width: %d; border-radius: 6; font-size: %d; bold: true; font-color: "#1e293b"}`, strokeWidth, boundaryFontSize)},
	{name: classInnerBound, d2: `style: {fill: "#e2e8f0"}`},
	{name: groupClass[view.GroupLogical], part: partGroup, d2: fmt.Sprintf(`label.near: top-left; style: {fill: transparent; stroke: "#94a3b8"; stroke-width: %d; stroke-dash: 2; border-radius: 6; font-size: %d; bold: false; font-color: "#475569"}`, strokeWidth, logicalFontSize)},
	// On a connection, fill is the background D2 paints behind the label.
	// It keeps a group border from running through the label's text.
	{name: lineClass[view.LineSolid], d2: fmt.Sprintf(`style: {stroke: "#334155"; stroke-width: %d; fill: "#ffffff"; font-size: %d; italic: false; font-color: "#1e293b"}`, strokeWidth, edgeFontSize)},
	{name: lineClass[view.LineDashed], d2: fmt.Sprintf(`style: {stroke: "#64748b"; stroke-width: %d; stroke-dash: 4; fill: "#ffffff"; font-size: %d; italic: false; font-color: "#1e293b"}`, strokeWidth, edgeFontSize)},
	{name: classDiagTitle, d2: fmt.Sprintf(`shape: text; near: top-center; style: {font-size: %d; bold: true; font-color: "#0f172a"}`, headingFontSize)},
	{name: classLegend, d2: fmt.Sprintf(`near: bottom-center; grid-columns: 1; grid-gap: 6; style: {fill: "#ffffff"; stroke: "#cbd5e1"; stroke-width: 1; border-radius: 6; font-size: %d; bold: true; font-color: "#334155"}`, legendFontSize)},
	{name: classLegendRows, d2: `label: ""; horizontal-gap: 12; vertical-gap: 5; style: {fill: transparent; stroke-width: 0}`},
	{name: classLegendText, d2: fmt.Sprintf(`shape: text; style: {font-size: %d; font-color: "#334155"}`, legendFontSize)},
	{name: classLegendNote, d2: fmt.Sprintf(`shape: text; style: {font-size: %d; font-color: "#64748b"}`, legendNoteFontSize)},
	{name: classSwatch, d2: fmt.Sprintf(`label: ""; width: %d; height: %d; style: {border-radius: 3}`, swatchWidth, swatchHeight)},
	{name: classSwatchLine, d2: `label: ""; grid-rows: 1; grid-gap: 0; style: {fill: transparent; stroke-width: 0}`},
	{name: classSwatchPoint, d2: fmt.Sprintf(`label: ""; width: 1; height: %d; style: {fill: transparent; stroke-width: 0}`, swatchHeight)},
	{name: classSwatchGap, d2: fmt.Sprintf(`label: ""; width: %d; height: %d; style: {fill: transparent; stroke-width: 0}`, swatchWidth-2, swatchHeight)},
}

// markLook is the D2 body of a mark pill in the given colors. The font size
// comes from the constant the writer measures pill text with.
func markLook(colors string) string {
	return fmt.Sprintf(`style: {font-size: %d; bold: false; %s; border-radius: %d; stroke-width: 1}`, markFontSize, colors, markHeight/2)
}

var classByName = func() map[string]class {
	m := make(map[string]class, len(classes))
	for _, c := range classes {
		m[c.name] = c
	}
	return m
}()

func writeClasses(b *strings.Builder) {
	b.WriteString("classes: {\n")
	for _, c := range classes {
		b.WriteString("  " + c.name + ": {" + c.d2 + "}\n")
	}
	b.WriteString("}\n")
}

// swatch says how a legend row samples its item: a short line of the given
// class, a mark pill, or a small box composed from the listed classes.
type swatch struct {
	line string
	mark view.MarkKind
	box  []string
}

var swatches = map[view.LegendItem]swatch{
	view.LegendSolidLine:   {line: lineClass[view.LineSolid]},
	view.LegendDashedLine:  {line: lineClass[view.LineDashed]},
	view.LegendSolidFrame:  {box: []string{frameClass[view.FrameSolid]}},
	view.LegendDashedFrame: {box: []string{frameClass[view.FrameDashed]}},
	view.LegendUnverified:  {mark: view.MarkUnverified},
	view.LegendMismatch:    {mark: view.MarkMismatch},
	view.LegendBoundary:    {box: []string{groupClass[view.GroupBoundary]}},
	view.LegendLogical:     {box: []string{groupClass[view.GroupLogical]}},
}
