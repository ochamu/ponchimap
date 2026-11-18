package d2render

import (
	"strings"

	"github.com/d2lang/d2/d2target"
)

// settleCards moves only what is inside a card. The cards, the groups and
// the lines stay where D2 put them.
//
// ELK grows a node that has several edges on one side to make room for
// them, and TALA also returns cards larger than their contents. The
// contents stay in the top-left corner of the larger frame. They are moved
// back to the center.
func settleCards(shapes []d2target.Shape) {
	cards := map[string]int{}
	parts := map[string][]int{}
	for i, s := range shapes {
		p, owner := partOf(s.Classes)
		switch {
		case owner == "" || p == partGroup:
		case p == partCard:
			cards[owner] = i
		default:
			parts[owner] = append(parts[owner], i)
		}
	}
	for owner, at := range cards {
		card := shapes[at]
		first := shapes[parts[owner][0]]
		left, top, right, bottom := first.Pos.X, first.Pos.Y, first.Pos.X+first.Width, first.Pos.Y+first.Height
		for _, i := range parts[owner] {
			s := shapes[i]
			left, top = min(left, s.Pos.X), min(top, s.Pos.Y)
			right, bottom = max(right, s.Pos.X+s.Width), max(bottom, s.Pos.Y+s.Height)
		}
		dx := card.Pos.X + (card.Width-(right-left))/2 - left
		dy := card.Pos.Y + (card.Height-(bottom-top))/2 - top
		for _, i := range parts[owner] {
			shapes[i].Pos.X += dx
			shapes[i].Pos.Y += dy
		}
	}
}

func partOf(names []string) (p part, owner string) {
	for _, name := range names {
		if id, ok := strings.CutPrefix(name, ownerClassPrefix); ok {
			owner = id
			continue
		}
		if c := classByName[name]; c.part != partOther {
			p = c.part
		}
	}
	return p, owner
}
