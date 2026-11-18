package importer

import (
	"bytes"
	"cmp"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"slices"
	"strconv"

	"github.com/ochamu/ponchimap/internal/model"
)

// local is an ID that a findings file gives a candidate. It means something
// only inside that file, so it never goes into the model or into a target ID.
type local string

// idKey is the key under which every new ID is an HMAC. It is the same for
// every workspace, so that the same findings give the same IDs wherever they
// are imported.
const idKey = "\x3f\x9c\x1a\x7e\x5b\x2d\x48\x60\x9a\x1c\xe4\x7f\x0b\x6d\x83\x52\xc4\xe1\x9f\x7a\x2b\x5d\x6c\x80\x91\xe3\xf4\xa5\xb6\xc7\xd8\xe9"

// The kinds of ID, which are also the first field of what is hashed.
const (
	nodeID        = "node"
	edgeID        = "edge"
	evidenceID    = "evidence"
	coverageID    = "coverage"
	supplementsID = "supplements"
)

var idPrefix = map[string]string{nodeID: "n-", edgeID: "e-", evidenceID: "ev-", coverageID: "cv-", supplementsID: "sp-"}

// idAlphabet writes 5 bits a character and leaves out i, l, o, and u, which
// are easily misread.
const idAlphabet = "0123456789abcdefghjkmnpqrstvwxyz"

// Changing how an ID is made, or idKey, changes the IDs that users' models
// get from then on.
func newID(kind string, fields ...string) string {
	mac := hmac.New(sha256.New, []byte(idKey))
	for _, field := range append([]string{kind}, fields...) {
		var length [4]byte
		binary.BigEndian.PutUint32(length[:], uint32(len(field)))
		mac.Write(length[:])
		mac.Write([]byte(field))
	}
	sum := mac.Sum(nil)
	bits := binary.BigEndian.Uint64(sum[:8]) >> 24
	text := make([]byte, 8)
	for i := range text {
		text[i] = idAlphabet[bits>>(35-5*i)&31]
	}
	return idPrefix[kind] + string(text)
}

type issuer struct {
	inModel, issued map[string]bool
}

func (s *issuer) issue(kind string, fields ...string) string {
	for try := 0; ; try++ {
		numbered := fields
		if try > 0 {
			numbered = append(slices.Clip(fields), strconv.Itoa(try))
		}
		id := newID(kind, numbered...)
		if !s.inModel[id] && !s.issued[id] {
			s.issued[id] = true
			return id
		}
	}
}

func canonical(v any) string {
	var text bytes.Buffer
	enc := json.NewEncoder(&text)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		// The values come from documents that were read as JSON, so the code
		// that built this one has a defect. The text of the error may quote
		// the value, so only its type is shown.
		panic(fmt.Sprintf("importer: a value cannot be written as JSON (%T)", err))
	}
	return string(bytes.TrimSuffix(text.Bytes(), []byte("\n")))
}

func digest(v any) string { return digestOf([]byte(canonical(v))) }

func digestOf(text []byte) string {
	sum := sha256.Sum256(text)
	return "sha256:" + hex.EncodeToString(sum[:])
}

// stance is what an evidence item says and where, without its lines and its
// time of checking: what its ID is made from, apart from its element. A newer
// read of the same place gives the same stance.
type stance struct {
	kind, supports, refType, refPath, validTime string
}

func stanceOf(ev model.Evidence) stance {
	supports := slices.Clone(ev.Supports)
	slices.SortFunc(supports, func(a, b model.Support) int {
		return cmp.Or(cmp.Compare(a.Key, b.Key), cmp.Compare(canonical(a.Value), canonical(b.Value)))
	})
	return stance{string(ev.Kind), canonical(supports), ev.Ref.Type, ev.Ref.Path, ev.ValidTime}
}

// extent is what a coverage record is about, which is what its ID is made
// from.
type extent struct{ source, scope string }

func takeOver[T comparable](removed, added []T, same ...func(T) any) map[T]T {
	from := map[T]T{}
	for _, key := range same {
		free := map[any][]T{}
		for _, old := range removed {
			free[key(old)] = append(free[key(old)], old)
		}
		var unpaired []T
		for _, entry := range added {
			left := free[key(entry)]
			if len(left) == 0 {
				unpaired = append(unpaired, entry)
				continue
			}
			from[entry], free[key(entry)] = left[0], left[1:]
		}
		taken := map[T]bool{}
		for _, old := range from {
			taken[old] = true
		}
		removed = slices.DeleteFunc(slices.Clone(removed), func(old T) bool { return taken[old] })
		added = unpaired
	}
	return from
}

// issue gives every new evidence item and coverage record its ID. A new one
// first takes over the ID of one that this run took out and that says the
// same, so that a newer read of the same place keeps the ID.
//
// Among several that say the same, the one that says it at the same lines
// and with the same time is paired first: that is the same read come again,
// and pairing it by position would swap the IDs of two such items whenever
// their IDs sort the other way round than their candidates.
func (w *work) issue() {
	w.ids.inModel = map[string]bool{}
	byOrigin := func(a, b *origin) int { return a.compare(b) }

	type owned struct {
		it *item
		el *element
	}
	var fresh []owned
	for _, el := range w.elements() {
		if el.gone {
			continue
		}
		w.ids.inModel[el.id()] = true
		var removed, added []*item
		for _, it := range el.items {
			switch {
			case it.out && it.at >= 0:
				removed = append(removed, it)
			case it.out:
			case it.at >= 0:
				w.ids.inModel[it.ev.ID] = true
			default:
				added = append(added, it)
			}
		}
		slices.SortFunc(added, func(a, b *item) int { return byOrigin(a.from, b.from) })
		whole := func(it *item) any {
			ev := it.ev
			ev.ID = ""
			return canonical(ev)
		}
		from := takeOver(removed, added, whole, func(it *item) any { return stanceOf(it.ev) })
		for _, it := range added {
			if old, paired := from[it]; paired {
				it.ev.ID, it.at = old.ev.ID, old.at
				w.ids.issued[it.ev.ID] = true
			} else {
				fresh = append(fresh, owned{it, el})
			}
		}
	}

	extentOf := func(c *cover) any {
		return extent{w.history.source(c.c.AcquisitionID), canonical(c.c.Scope)}
	}
	var removed, added, freshCover []*cover
	for _, c := range w.coverage {
		switch {
		case c.out && c.at >= 0:
			removed = append(removed, c)
		case c.out:
		case c.at >= 0:
			w.ids.inModel[c.c.ID] = true
		default:
			added = append(added, c)
		}
	}
	slices.SortFunc(added, func(a, b *cover) int { return byOrigin(a.from, b.from) })
	from := takeOver(removed, added, func(c *cover) any {
		record := c.c
		record.ID = ""
		return canonical(record)
	}, extentOf)
	for _, c := range added {
		if old, paired := from[c]; paired {
			c.c.ID, c.at = old.c.ID, old.at
			w.ids.issued[c.c.ID] = true
		} else {
			freshCover = append(freshCover, c)
		}
	}

	slices.SortStableFunc(fresh, func(a, b owned) int { return byOrigin(a.it.from, b.it.from) })
	for _, f := range fresh {
		st := stanceOf(f.it.ev)
		f.it.ev.ID = w.ids.issue(evidenceID, f.el.id(), st.kind, st.supports, st.refType, st.refPath, st.validTime)
	}
	for _, c := range freshCover {
		ext := extentOf(c).(extent)
		c.c.ID = w.ids.issue(coverageID, ext.source, ext.scope)
	}
}
