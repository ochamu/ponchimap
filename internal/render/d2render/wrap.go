package d2render

import (
	"context"
	"strings"
	"unicode"

	"golang.org/x/text/width"
)

const (
	noLineStart = "、。，．・：；！？）］｝」』】〕〉》ー,.)]}!?:;"
	noLineEnd   = "（［｛「『【〔〈《([{"
)

// wrap breaks text into lines of at most limit display columns,
// where a full-width rune counts 2. D2 does not wrap, so this decides every
// line break in a diagram.
//
// It never breaks inside an ASCII word or a katakana word, never starts a
// line with closing punctuation, and never ends one with an opening bracket.
// A single unbreakable run longer than limit overflows rather than being cut.
// Among the breaks that remain it avoids the ones that read badly in
// Japanese (see breakCost), spending one extra line on that if it must, then
// keeps the lines as even as it can.
//
// The work grows steeply with the length of the text, so it stops with the
// context's error once ctx is done.
func wrap(ctx context.Context, text string, limit int) ([]string, error) {
	var lines []string
	for _, para := range strings.Split(text, "\n") {
		wrapped, err := wrapUnits(ctx, splitUnits(para), limit)
		if err != nil {
			return nil, err
		}
		lines = append(lines, wrapped...)
	}
	return lines, nil
}

// unit is a run that must stay on one line. gap records that whitespace
// separated it from the unit before.
type unit struct {
	text string
	gap  bool
}

func (u unit) first() rune {
	for _, r := range u.text {
		return r
	}
	return 0
}

func (u unit) last() rune {
	var last rune
	for _, r := range u.text {
		last = r
	}
	return last
}

func splitUnits(s string) []unit {
	var units []unit
	gap := false
	var last rune
	for _, r := range s {
		if unicode.IsSpace(r) {
			gap = true
			continue
		}
		glued := strings.ContainsRune(noLineStart, r) ||
			strings.ContainsRune(noLineEnd, last) ||
			(isASCII(r) && isASCII(last)) ||
			(isKatakana(r) && isKatakana(last))
		if len(units) > 0 && !gap && glued {
			units[len(units)-1].text += string(r)
		} else {
			units = append(units, unit{text: string(r), gap: gap && len(units) > 0})
		}
		gap, last = false, r
	}
	return units
}

// breakCost rates a line break between two units; lower reads better. With
// no dictionary, script changes stand in for word boundaries: a line that
// starts with hiragana usually starts with a particle or the tail of a verb,
// and a break between two kanji, or after a digit, usually splits a compound.
func breakCost(before, after unit) int {
	a, b := before.last(), after.first()
	switch {
	case after.gap, strings.ContainsRune(noLineStart, a), strings.ContainsRune(noLineEnd, b):
		return 0
	case unicode.Is(unicode.Hiragana, b):
		return 2
	case unicode.Is(unicode.Han, b) && (unicode.Is(unicode.Han, a) || unicode.IsDigit(a)):
		return 1
	}
	return 0
}

// layoutCost orders candidate layouts: worse breaks first, then more lines,
// then the widest line, so equally clean layouts of the same height are
// compared on how even they are.
type layoutCost struct{ breaks, lines, widest int }

func (c layoutCost) less(o layoutCost) bool {
	if c.breaks != o.breaks {
		return c.breaks < o.breaks
	}
	if c.lines != o.lines {
		return c.lines < o.lines
	}
	return c.widest < o.widest
}

// spareLines is how many lines beyond the minimum a layout may use when
// that buys cleaner breaks.
const spareLines = 1

func wrapUnits(ctx context.Context, units []unit, limit int) ([]string, error) {
	n := len(units)
	if n == 0 {
		return nil, nil
	}
	lineWidth := func(i, j int) int {
		w := displayWidth(units[i].text)
		for _, u := range units[i+1 : j] {
			if u.gap {
				w++
			}
			w += displayWidth(u.text)
		}
		return w
	}
	fits := func(i, j int) bool { return j-i == 1 || lineWidth(i, j) <= limit }

	type cell struct {
		cost layoutCost
		from int
		ok   bool
	}
	// rows[k][j] is the best way to set units 0..j-1 in k+1 lines.
	var rows [][]cell
	chosen := -1
	for k := 0; k < n && (chosen < 0 || k <= chosen+spareLines); k++ {
		row := make([]cell, n+1)
		for j := 1; j <= n; j++ {
			// The table costs about lines x units squared line widths, hours
			// for a very long label, so a cancelled render must not wait for
			// it.
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			if k == 0 {
				if fits(0, j) {
					row[j] = cell{cost: layoutCost{lines: 1, widest: lineWidth(0, j)}, ok: true}
				}
				continue
			}
			for i := 1; i < j; i++ {
				if !rows[k-1][i].ok || !fits(i, j) {
					continue
				}
				prev := rows[k-1][i].cost
				cost := layoutCost{breaks: prev.breaks + breakCost(units[i-1], units[i]), lines: k + 1, widest: max(prev.widest, lineWidth(i, j))}
				if !row[j].ok || cost.less(row[j].cost) {
					row[j] = cell{cost: cost, from: i, ok: true}
				}
			}
		}
		rows = append(rows, row)
		if row[n].ok && (chosen < 0 || row[n].cost.less(rows[chosen][n].cost)) {
			chosen = k
		}
	}

	lines := make([]string, chosen+1)
	for k, j := chosen, n; k >= 0; k-- {
		i := rows[k][j].from
		var b strings.Builder
		for at, u := range units[i:j] {
			if at > 0 && u.gap {
				b.WriteByte(' ')
			}
			b.WriteString(u.text)
		}
		lines[k] = b.String()
		j = i
	}
	return lines, nil
}

func isASCII(r rune) bool { return r != 0 && r < unicode.MaxASCII }

// isKatakana counts the long vowel mark, which Unicode files under Common.
func isKatakana(r rune) bool { return unicode.Is(unicode.Katakana, r) || r == 'ー' }

func displayWidth(s string) int {
	n := 0
	for _, r := range s {
		switch width.LookupRune(r).Kind() {
		case width.EastAsianWide, width.EastAsianFullwidth:
			n += 2
		default:
			n++
		}
	}
	return n
}
