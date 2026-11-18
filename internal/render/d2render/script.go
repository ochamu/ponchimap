package d2render

import (
	"context"
	"encoding/base64"
	"fmt"
	"strings"

	"github.com/d2lang/d2/d2ast"

	"github.com/ochamu/ponchimap/internal/assets"
	"github.com/ochamu/ponchimap/internal/view"
)

const (
	ownerClassPrefix = "m-"
	legendHeading    = "凡例"
)

// Root keys that are not scene IDs. Scene IDs are quoted the same way, so a
// scene element would have to be named exactly like one of these to clash.
const (
	titleKey  = "__title__"
	legendKey = "__legend__"
)

func ownerClass(sceneID string) string { return ownerClassPrefix + sceneID }

// quote writes s as a D2 double-quoted string. The dollar sign is escaped
// because D2 expands ${...} inside double quotes.
func quote(s string) string {
	return `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`, `$`, `\$`, "\n", `\n`).Replace(s) + `"`
}

// sceneKey is the D2 key of a group or a card. D2 takes a container named
// like one of its board keywords for a set of boards even when the key is
// quoted: it refuses a relation into it and repeats the rest of the figure
// inside it. Such an ID gets a trailing underscore, which no scene ID ends
// with.
func sceneKey(sceneID string) string {
	if _, board := d2ast.BoardKeywords[strings.ToLower(sceneID)]; board {
		sceneID += "_"
	}
	return quote(sceneID)
}

func classList(names ...string) string {
	quoted := make([]string, len(names))
	for i, n := range names {
		quoted[i] = quote(n)
	}
	return "class: [" + strings.Join(quoted, "; ") + "]"
}

type scriptWriter struct {
	ctx   context.Context
	b     strings.Builder
	scene *view.Scene
	m     *measurer
	set   assets.Set
	depth int
	err   error

	childGroups map[string][]view.Group
	groupNodes  map[string][]view.Node
	paths       map[string]string
}

// writeScript translates a scene to D2. The same scene always yields the
// same text.
func writeScript(ctx context.Context, s *view.Scene, m *measurer, set assets.Set) (string, error) {
	w := &scriptWriter{ctx: ctx, scene: s, m: m, set: set, childGroups: map[string][]view.Group{}, groupNodes: map[string][]view.Node{}, paths: map[string]string{}}
	for _, g := range s.Groups {
		w.childGroups[g.ParentID] = append(w.childGroups[g.ParentID], g)
	}
	for _, n := range s.Nodes {
		w.groupNodes[n.GroupID] = append(w.groupNodes[n.GroupID], n)
	}

	direction := s.Direction
	if direction == "" {
		direction = view.DirRight
	}
	w.line("direction: %s", direction)
	writeClasses(&w.b)
	if s.Title != "" {
		w.line("%s: %s {class: %s}", quote(titleKey), quote(s.Title), classDiagTitle)
	}
	w.members("", "", 0)
	for _, e := range s.Edges {
		w.edge(e)
	}
	w.legend()
	return w.b.String(), w.err
}

func (w *scriptWriter) line(format string, args ...any) {
	w.b.WriteString(strings.Repeat("  ", w.depth))
	fmt.Fprintf(&w.b, format, args...)
	w.b.WriteByte('\n')
}

func (w *scriptWriter) open(format string, args ...any) {
	w.line(format+" {", args...)
	w.depth++
}

func (w *scriptWriter) close() {
	w.depth--
	w.line("}")
}

// members writes the groups and nodes directly inside one group, or at the
// root when groupID is empty, and records each node's absolute D2 path.
// boundaries counts the boundaries around them: D2 colors have no alpha, so
// a boundary nested in another alternates between two tints to stay visible.
func (w *scriptWriter) members(groupID, path string, boundaries int) {
	for _, g := range w.childGroups[groupID] {
		names, inside := []string{groupClass[g.Kind]}, boundaries
		if g.Kind == view.GroupBoundary {
			if boundaries%2 == 1 {
				names = append(names, classInnerBound)
			}
			inside++
		}
		w.open("%s: %s", sceneKey(g.ID), quote(g.Label))
		w.line("%s", classList(append(names, ownerClass(g.ID))...))
		w.members(g.ID, path+sceneKey(g.ID)+".", inside)
		w.close()
	}
	for _, n := range w.groupNodes[groupID] {
		w.paths[n.ID] = path + sceneKey(n.ID)
		w.card(n)
	}
}

func (w *scriptWriter) wrap(text string, limit int) []string {
	lines, err := wrap(w.ctx, text, limit)
	if err != nil && w.err == nil {
		w.err = err
	}
	return lines
}

func (w *scriptWriter) card(n view.Node) {
	owner := ownerClass(n.ID)
	icon := "data:image/svg+xml;base64," + base64.StdEncoding.EncodeToString(w.set.Icon(n.IconKey))

	w.open("%s", sceneKey(n.ID))
	w.line("%s", classList(classCard, frameClass[n.Frame], owner))
	w.line("i: {%s; icon: %s}", classList(classIcon, owner), quote(icon))
	w.open("t")
	w.line("%s", classList(classCardText, owner))
	if n.Title != "" {
		w.line("s: %s {%s}", quote(n.Title), classList(classTitle, owner))
	}
	for i, role := range w.wrap(n.Role, roleColumns) {
		w.line("r%d: %s {%s}", i, quote(role), classList(classRole, owner))
	}
	if len(n.Marks) > 0 {
		w.open("k")
		w.line("%s", classList(classMarkRow, owner))
		for i, mark := range n.Marks {
			if i > 0 {
				w.line("g%d: {%s}", i, classList(classMarkGap, owner))
			}
			w.mark(fmt.Sprintf("m%d", i), mark.Kind, mark.Text(), owner)
		}
		w.close()
	}
	w.close()
	w.close()
}

// mark writes one pill. Its width is set from the measured text because a
// grid cell would otherwise stretch it to the column.
func (w *scriptWriter) mark(key string, kind view.MarkKind, text string, extraClasses ...string) {
	textWidth, _, err := w.m.size(text, markFontSize, false)
	if err != nil && w.err == nil {
		w.err = err
	}
	names := append([]string{markClass[kind]}, extraClasses...)
	w.line("%s: %s {%s; width: %d; height: %d}", key, quote(text), classList(names...), textWidth+2*markPadX, markHeight)
}

func (w *scriptWriter) edge(e view.Edge) {
	label := strings.Join(w.wrap(e.Label, edgeLabelColumns), "\n")
	w.line("%s -> %s: %s {%s}", w.paths[e.From], w.paths[e.To], quote(label), classList(lineClass[e.Line], ownerClass(e.ID)))
}

func (w *scriptWriter) legend() {
	w.open("%s: %s", quote(legendKey), quote(legendHeading))
	w.line("class: %s", classLegend)
	if len(w.scene.Legend) > 0 {
		w.open("rows")
		w.line("class: %s", classLegendRows)
		// D2 fills a grid row by row only when both counts are given on
		// the shape itself, rows first.
		w.line("grid-rows: %d", (len(w.scene.Legend)+legendPairsPerRow-1)/legendPairsPerRow)
		w.line("grid-columns: %d", 2*legendPairsPerRow)
		for i, item := range w.scene.Legend {
			w.swatch(fmt.Sprintf("s%d", i), swatches[item])
			w.line("t%d: %s {class: %s}", i, quote(item.Text()), classLegendText)
		}
		w.close()
	}
	i := 0
	for _, note := range append([]string{view.LegendNote}, w.scene.Notes...) {
		for _, text := range w.wrap(note, noteColumns) {
			w.line("n%d: %s {class: %s}", i, quote(text), classLegendNote)
			i++
		}
	}
	w.close()
}

func (w *scriptWriter) swatch(key string, s swatch) {
	switch {
	case s.line != "":
		w.open("%s", key)
		w.line("class: %s", classSwatchLine)
		w.line("a: {class: %s}", classSwatchPoint)
		w.line("g: {class: %s}", classSwatchGap)
		w.line("b: {class: %s}", classSwatchPoint)
		w.line("a -> b: {class: %s}", s.line)
		w.close()
	case s.mark != "":
		w.mark(key, s.mark, s.mark.Label())
	default:
		w.line("%s: {%s}", key, classList(append(append([]string{}, s.box...), classSwatch)...))
	}
}
