// Package diag holds the diagnostics the tool returns: what is wrong with an
// input, where, and how to put it right.
//
// A code is a fixed identifier. Scripts and agents branch on it, so a code
// keeps its name when its message is reworded. A message never shows a value
// of the input: it may name what the schema defines, the location may name
// the keys of the input, and the target ID is the ID of a model element.
package diag

import (
	"cmp"
	"encoding/json"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"unicode"
)

type Severity string

const (
	// Error means the input cannot be used as it is.
	Error Severity = "error"
	// Warning means the input can be used, and something in it needs a look.
	Warning Severity = "warning"
)

type Diagnostic struct {
	Severity Severity `json:"severity"`
	Code     string   `json:"code"`
	// Message is the cause, in English.
	Message string `json:"message"`
	// TargetID is the ID of the element the diagnostic is about: an element of
	// the model, or a group or an aggregate of the view config. It is empty
	// when no element with a well-formed ID holds the location.
	TargetID string   `json:"target_id,omitempty"`
	Location Location `json:"location"`
	// Hint says how to confirm or correct the cause.
	Hint string `json:"hint,omitempty"`
}

// Location says where in an input the cause is.
type Location struct {
	// Path is the file, as the caller named it.
	Path string `json:"path,omitempty"`
	// Pointer is the JSON Pointer (RFC 6901) of the value or the member the
	// diagnostic is about. A member the input lacks is named by the pointer
	// it would have, so that a program need not read the message. The empty
	// pointer is the whole document.
	Pointer string `json:"pointer"`
	// Line and Column count from 1, the column in characters. Each is 0 when
	// it is not known: a syntax error of YAML has a line and no column.
	Line   int `json:"line,omitempty"`
	Column int `json:"column,omitempty"`
}

// String is the one-line form for a person:
//
//	model.json:14:9: error schema.enum: the value is not one of ... (/nodes/0/kind, n001)
func (d Diagnostic) String() string {
	var b strings.Builder
	if d.Location.Path != "" {
		b.WriteString(onOneLine(d.Location.Path) + ":")
	}
	if d.Location.Line > 0 {
		fmt.Fprintf(&b, "%d:", d.Location.Line)
	}
	if d.Location.Line > 0 && d.Location.Column > 0 {
		fmt.Fprintf(&b, "%d:", d.Location.Column)
	}
	if b.Len() > 0 {
		b.WriteString(" ")
	}
	fmt.Fprintf(&b, "%s %s: %s", d.Severity, d.Code, d.Message)
	where := []string{}
	if d.Location.Pointer != "" {
		where = append(where, onOneLine(d.Location.Pointer))
	}
	if d.TargetID != "" {
		where = append(where, d.TargetID)
	}
	if len(where) > 0 {
		b.WriteString(" (" + strings.Join(where, ", ") + ")")
	}
	return b.String()
}

// onOneLine writes text that comes from the input, a path or the keys in a
// pointer, so that it cannot break the line or steer the terminal: a line
// break, an escape character, or a character that reorders the text around
// it is written as a Go escape such as \n, \x1b, or \u202e. The tool reads
// files of other people's repositories, so a key may be written to mislead.
func onOneLine(text string) string {
	if !strings.ContainsFunc(text, func(r rune) bool { return !unicode.IsPrint(r) }) {
		return text
	}
	quoted := strconv.Quote(text)
	return quoted[1 : len(quoted)-1]
}

// List is the diagnostics of one operation.
type List []Diagnostic

// MarshalJSON writes a list without diagnostics as an empty array, never as
// null, so that a program reading the output always finds an array.
func (l List) MarshalJSON() ([]byte, error) {
	if len(l) == 0 {
		return []byte("[]"), nil
	}
	return json.Marshal([]Diagnostic(l))
}

func (l List) HasErrors() bool {
	return slices.ContainsFunc(l, func(d Diagnostic) bool { return d.Severity == Error })
}

// Sort puts the list in its fixed order: by file, then by position in the
// text, then by pointer, code, and message. The same input always gives the
// same list.
func (l List) Sort() {
	slices.SortStableFunc(l, func(a, b Diagnostic) int {
		return cmp.Or(
			cmp.Compare(a.Location.Path, b.Location.Path),
			cmp.Compare(a.Location.Line, b.Location.Line),
			cmp.Compare(a.Location.Column, b.Location.Column),
			comparePointers(a.Location.Pointer, b.Location.Pointer),
			cmp.Compare(a.Code, b.Code),
			cmp.Compare(a.Message, b.Message),
		)
	})
}

// comparePointers orders JSON Pointers token by token, array indices as
// numbers, so that /nodes/2 comes before /nodes/10.
func comparePointers(a, b string) int {
	as, bs := strings.Split(a, "/"), strings.Split(b, "/")
	for i := 0; i < len(as) && i < len(bs); i++ {
		an, aerr := strconv.Atoi(as[i])
		bn, berr := strconv.Atoi(bs[i])
		if aerr == nil && berr == nil {
			if c := cmp.Compare(an, bn); c != 0 {
				return c
			}
			continue
		}
		if c := cmp.Compare(as[i], bs[i]); c != 0 {
			return c
		}
	}
	return cmp.Compare(len(as), len(bs))
}
