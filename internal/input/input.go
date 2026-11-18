// Package input reads one input document, JSON or YAML, and reports every
// problem with it as diagnostics that say where the problem is. No diagnostic
// carries a value of the input: the texts of encoding/json, of the YAML
// library, and of the schema library quote the input, so none of them is ever
// passed on.
package input

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/ochamu/ponchimap/internal/diag"
)

// Document is one input after it was read: the JSON value, and where each
// part of it stands in the text.
type Document struct {
	// Value is nil, bool, string, json.Number, []any or map[string]any.
	Value any

	// root is where the value stands in the text, and where every value that
	// it holds stands.
	root place
	// null is how a diagnostic says that a value of the document is null. A
	// person who writes YAML gets a null by writing nothing after a key.
	null string
	// incomplete says that a part of the text is not in Value: a number
	// beyond the limits of a number, or a construct that is not read. Such a
	// document is located and searched, and never checked against a schema,
	// which would report the gap as a problem of its own.
	incomplete bool
}

// place is where a value stands in the text, and where the values that it
// holds stand. The places of a document form a tree of the shape of its
// value, so that a key is held once however many values stand under it: a
// table from every JSON Pointer to a place would repeat the keys above a
// value for each value.
type place struct {
	// value is where the value starts, and key where its key starts when it
	// is a member of an object.
	value, key mark
	// items are the places of the items of an array, or of the members of an
	// object, in the order of the text.
	items []place
	// member says which of the items a key of an object names: the last one
	// that was written with the key.
	member map[string]int
}

// add notes where the member that key names stands.
func (p *place) add(key string, member place) {
	if p.member == nil {
		p.member = map[string]int{}
	}
	p.member[key] = len(p.items)
	p.items = append(p.items, member)
}

// mark is a place in the text. Line and column count from 1, the column in
// characters.
type mark struct{ line, column int }

func (m mark) location(pointer string) diag.Location {
	return diag.Location{Pointer: pointer, Line: m.line, Column: m.column}
}

// The limits of a number. Within them stand every identifier and every 64-bit
// float, whose exponent stays inside 324 either way. To check a number the
// schema library works it out, and for a few characters such as 1e1000000
// that is a computation of millions of digits, so a number beyond the limits
// is refused before the library sees it.
const (
	maxNumberLength   = 400
	maxNumberExponent = 400
)

var numberHint = fmt.Sprintf("A number is read up to %d characters, with an exponent from -%d to %d. Write a longer identifier as a string.",
	maxNumberLength, maxNumberExponent, maxNumberExponent)

// beyondNumberLimits says which limit the number text breaks, or nothing.
func beyondNumberLimits(text string) string {
	if len(text) > maxNumberLength {
		return fmt.Sprintf("the number is written with more than %d characters", maxNumberLength)
	}
	if at := strings.IndexAny(text, "eE"); at >= 0 {
		exponent, err := strconv.Atoi(text[at+1:])
		if err != nil || exponent > maxNumberExponent || exponent < -maxNumberExponent {
			return fmt.Sprintf("the exponent of the number is outside the range from -%d to %d", maxNumberExponent, maxNumberExponent)
		}
	}
	return ""
}

// The limits of a JSON Pointer. Every diagnostic carries the pointer of its
// value, so the pointers of many diagnostics under long keys, or under many
// levels, take the length of the keys times the number of diagnostics: a
// text of 264 KB with 40000 refused values under 60 KB of keys took 3.9 GB.
// maxDepth is how deep objects and arrays may nest, and maxPointerLength how
// many bytes the pointer of a value may take.
const (
	maxDepth         = 64
	maxPointerLength = 1024
)

// beyondLimit ends the reading of a document at a value that is beyond a
// limit of a JSON Pointer.
type beyondLimit struct{ problem diag.Diagnostic }

func (beyondLimit) Error() string { return "beyond a limit" }

// trail is the way from the document to the value being read: the tokens of
// the JSON Pointer of the value. The pointer is written out for a diagnostic
// only.
type trail struct {
	tokens []string
	// length is the length of the pointer.
	length int
}

func (t *trail) pointer() string {
	var b strings.Builder
	b.Grow(t.length)
	for _, token := range t.tokens {
		b.WriteByte('/')
		b.WriteString(token)
	}
	return b.String()
}

// enter steps to the member or the item that token names, which starts at
// at. It fails when the pointer of that value would be beyond the limit.
func (t *trail) enter(token string, at mark) error {
	if t.length+1+len(token) > maxPointerLength {
		return beyondLimit{problem("input.pointer_limit", at.location(t.pointer()),
			fmt.Sprintf("the keys that lead to the value are too long: its JSON Pointer takes more than %d bytes", maxPointerLength),
			fmt.Sprintf("A value is read when its JSON Pointer, the keys and indexes on the way to it joined by slashes, takes at most %d bytes. Shorten the keys.", maxPointerLength))}
	}
	t.tokens = append(t.tokens, token)
	t.length += 1 + len(token)
	return nil
}

func (t *trail) leave() {
	last := len(t.tokens) - 1
	t.length -= 1 + len(t.tokens[last])
	t.tokens = t.tokens[:last]
}

// open says that a collection starts at at, and fails when it is nested
// beyond the limit.
func (t *trail) open(at mark) error {
	if len(t.tokens) == maxDepth {
		return beyondLimit{problem("input.depth_limit", at.location(t.pointer()),
			fmt.Sprintf("the value is nested more than %d levels deep", maxDepth),
			fmt.Sprintf("Objects and arrays are read up to %d levels deep. Flatten the structure.", maxDepth))}
	}
	return nil
}

var bom = []byte("\xEF\xBB\xBF")

// ReadFile reads the file at path, or says in a diagnostic why it cannot be
// read.
func ReadFile(path string) ([]byte, diag.List) {
	raw, err := os.ReadFile(path)
	if err != nil {
		// The path is in the location already, so the reason leaves it out.
		reason := err
		if inner := errors.Unwrap(err); inner != nil {
			reason = inner
		}
		return nil, diag.List{{Severity: diag.Error, Code: "input.unreadable",
			Message: "the file cannot be read: " + reason.Error(), Location: diag.Location{Path: path},
			Hint: "Check that the path names a file that can be read."}}
	}
	return raw, nil
}

// DecodeJSON reads JSON text. The document is nil when nothing could be read.
func DecodeJSON(raw []byte) (*Document, diag.List) {
	text := bytes.TrimPrefix(raw, bom)
	c := &cursor{text: text, at: mark{1, 1}}
	if bad := firstInvalidUTF8(text); bad >= 0 {
		return nil, diag.List{invalidUTF8(c.mark(bad))}
	}
	r := &jsonReader{doc: &Document{null: "null"}, dec: json.NewDecoder(bytes.NewReader(text)), cursor: c}
	r.dec.UseNumber()
	value, root, err := r.value()
	var limit beyondLimit
	switch {
	case errors.As(err, &limit):
		return nil, diag.List{limit.problem}
	case err != nil:
		return nil, diag.List{r.syntaxProblem(err)}
	}
	r.doc.Value, r.doc.root = value, root
	list := r.found
	if rest := skipSpace(text, int(r.dec.InputOffset())); rest < len(text) {
		list = append(list, problem("input.trailing_data", c.mark(rest).location(""),
			"the text goes on after the JSON value", "A file holds one JSON value. Remove what follows it."))
	}
	return r.doc, list
}

// Locate returns the location of the value at pointer, or of the member's key
// when key is true. Line and Column are 0 for a pointer the document lacks.
func (d *Document) Locate(pointer string, key bool) diag.Location {
	p, found := d.root, true
	if pointer != "" {
		for _, token := range strings.Split(pointer[1:], "/") {
			if p, found = p.child(unescape(token)); !found {
				break
			}
		}
	}
	at := p.value
	if key {
		at = p.key
	}
	if !found || at == (mark{}) {
		return diag.Location{Pointer: pointer}
	}
	return at.location(pointer)
}

// child is the place of the member or the item that name names.
func (p place) child(name string) (place, bool) {
	if p.member != nil {
		i, ok := p.member[name]
		if !ok {
			return place{}, false
		}
		return p.items[i], true
	}
	if i, err := strconv.Atoi(name); err == nil && i >= 0 && i < len(p.items) {
		return p.items[i], true
	}
	return place{}, false
}

// cursor counts the lines and the characters of a JSON text up to an offset.
// A reader asks for places in the order of the text, so every byte is counted
// once, however long a line is: a machine writes a whole document on one line.
type cursor struct {
	text   []byte
	offset int
	at     mark
}

func (c *cursor) mark(offset int) mark {
	if offset < c.offset {
		c.offset, c.at = 0, mark{1, 1}
	}
	for _, b := range c.text[c.offset:offset] {
		switch {
		case b == '\n':
			c.at = mark{c.at.line + 1, 1}
		case startsChar(b):
			c.at.column++
		}
	}
	c.offset = offset
	return c.at
}

// startsChar reports whether b is the first byte of a character in UTF-8.
func startsChar(b byte) bool { return b&0xC0 != 0x80 }

func problem(code string, at diag.Location, message, hint string) diag.Diagnostic {
	return diag.Diagnostic{Severity: diag.Error, Code: code, Message: message, Location: at, Hint: hint}
}

func invalidUTF8(at mark) diag.Diagnostic {
	return problem("input.invalid_utf8", at.location(""), "the text is not valid UTF-8", "Save the file in UTF-8.")
}

func duplicateKey(at diag.Location) diag.Diagnostic {
	return problem("input.duplicate_key", at,
		"the object already has a member with this key", "Remove one of the two members.")
}

func numberLimit(at diag.Location, message string) diag.Diagnostic {
	return problem("input.number_limit", at, message, numberHint)
}

// jsonReader reads one value token by token, so that the place of every key
// and value is known.
type jsonReader struct {
	doc    *Document
	dec    *json.Decoder
	cursor *cursor
	trail  trail
	found  diag.List
}

func (r *jsonReader) syntaxProblem(err error) diag.Diagnostic {
	const hint = "Correct the JSON text at this place."
	text := r.cursor.text
	end := func() diag.Location { return r.cursor.mark(len(text)).location("") }
	var syntax *json.SyntaxError
	switch {
	case errors.As(err, &syntax):
		// Offset counts the bytes read up to and including the one that broke
		// the syntax.
		at := min(max(int(syntax.Offset)-1, 0), len(text))
		return problem("input.syntax", r.cursor.mark(at).location(""), "the text is not valid JSON at this place", hint)
	case len(bytes.TrimSpace(text)) == 0:
		return problem("input.syntax", end(), "the text holds no JSON value", "Write the JSON value into the file.")
	case errors.Is(err, io.EOF), errors.Is(err, io.ErrUnexpectedEOF):
		return problem("input.syntax", end(), "the text ends before the JSON value is complete", hint)
	}
	return problem("input.syntax", end(), "the text is not valid JSON", hint)
}

func (r *jsonReader) here() mark {
	return r.cursor.mark(skipSeparators(r.cursor.text, int(r.dec.InputOffset())))
}

// value reads the value that the trail leads to, and where it stands.
func (r *jsonReader) value() (any, place, error) {
	at := r.here()
	here := place{value: at}
	tok, err := r.dec.Token()
	if err != nil {
		return nil, here, err
	}
	if _, opens := tok.(json.Delim); opens {
		if err := r.trail.open(at); err != nil {
			return nil, here, err
		}
	}
	switch tok {
	case json.Delim('{'):
		obj := map[string]any{}
		for r.dec.More() {
			keyAt := r.here()
			keyTok, err := r.dec.Token()
			if err != nil {
				return nil, here, err
			}
			key := keyTok.(string)
			if err := r.trail.enter(escape(key), keyAt); err != nil {
				return nil, here, err
			}
			if _, dup := obj[key]; dup {
				r.found = append(r.found, duplicateKey(keyAt.location(r.trail.pointer())))
			}
			v, member, err := r.value()
			if err != nil {
				return nil, here, err
			}
			r.trail.leave()
			member.key = keyAt
			obj[key] = v
			here.add(key, member)
		}
		_, err := r.dec.Token()
		return obj, here, err
	case json.Delim('['):
		arr := []any{}
		for r.dec.More() {
			if err := r.trail.enter(strconv.Itoa(len(arr)), r.here()); err != nil {
				return nil, here, err
			}
			v, item, err := r.value()
			if err != nil {
				return nil, here, err
			}
			r.trail.leave()
			arr, here.items = append(arr, v), append(here.items, item)
		}
		_, err := r.dec.Token()
		return arr, here, err
	}
	if number, isNumber := tok.(json.Number); isNumber {
		if message := beyondNumberLimits(string(number)); message != "" {
			r.doc.incomplete = true
			r.found = append(r.found, numberLimit(at.location(r.trail.pointer()), message))
		}
	}
	return tok, here, nil
}

// escape makes key one token of a JSON Pointer (RFC 6901), and unescape
// gives the key back.
func escape(key string) string {
	return strings.ReplaceAll(strings.ReplaceAll(key, "~", "~0"), "/", "~1")
}

func unescape(token string) string {
	return strings.ReplaceAll(strings.ReplaceAll(token, "~1", "/"), "~0", "~")
}

func firstInvalidUTF8(text []byte) int {
	for i := 0; i < len(text); {
		r, size := utf8.DecodeRune(text[i:])
		if r == utf8.RuneError && size == 1 {
			return i
		}
		i += size
	}
	return -1
}

func skipSpace(text []byte, offset int) int {
	for offset < len(text) && strings.IndexByte(" \t\r\n", text[offset]) >= 0 {
		offset++
	}
	return offset
}

// skipSeparators moves past the whitespace, commas, and colons that the
// decoder's offset stands before when it is about to read a token.
func skipSeparators(text []byte, offset int) int {
	for offset < len(text) && strings.IndexByte(" \t\r\n,:", text[offset]) >= 0 {
		offset++
	}
	return offset
}
