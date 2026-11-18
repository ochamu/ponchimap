package input

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"

	"gopkg.in/yaml.v3"

	"github.com/ochamu/ponchimap/internal/diag"
)

// DecodeYAML reads YAML text that holds one document, and gives the JSON
// value it stands for. The document is nil when nothing could be read.
//
// The YAML library only parses. What a value is, is decided here by the core
// schema of YAML 1.2, the reading that editors apply: a scalar in quotes or
// in block form is a text, and a plain scalar is null, a boolean, or a number
// when it has that form, and a text otherwise. So a date stays the text that
// was written. Aliases, tags, merge keys, and directives are not read.
func DecodeYAML(raw []byte) (*Document, diag.List) {
	text := bytes.TrimPrefix(raw, bom)
	if bad := firstInvalidUTF8(text); bad >= 0 {
		return nil, diag.List{invalidUTF8(yamlMark(text, bad))}
	}
	if bad := firstNotInYAML(text); bad >= 0 {
		return nil, diag.List{problem("input.syntax", yamlMark(text, bad).location(""),
			"the text holds a character that YAML does not allow",
			"Remove the control character. Inside double quotes it can be written as an escape such as \\t or \\x1b.")}
	}
	// The library reads a %YAML 1.1 line and refuses a %YAML 1.2 line without
	// saying where, and a %TAG line only serves tags, which are not read.
	if at := firstDirective(text); at >= 0 {
		return nil, diag.List{problem("input.unsupported", yamlMark(text, at).location(""),
			"a directive is not read", "Remove the line that starts with %.")}
	}
	dec := yaml.NewDecoder(bytes.NewReader(text))
	first, err := nextDocument(dec)
	switch {
	case errors.Is(err, io.EOF):
		return nil, diag.List{problem("input.syntax", yamlMark(text, len(text)).location(""),
			"the text holds no YAML document", "Write the YAML document into the file.")}
	case err != nil:
		return nil, diag.List{yamlProblem(err)}
	}
	r := &yamlReader{doc: &Document{null: "null, which is also what a key without a value gives"}, text: newLineIndex(text)}
	// A document without content, such as a lone line of three hyphens, is
	// given by the library as a null.
	root := &yaml.Node{Kind: yaml.ScalarNode, Line: first.Line, Column: first.Column}
	if len(first.Content) > 0 {
		root = first.Content[0]
	}
	value, at, err := r.value(root)
	var limit beyondLimit
	if errors.As(err, &limit) {
		return nil, diag.List{limit.problem}
	}
	r.doc.Value, r.doc.root = value, at
	list := r.found
	switch second, err := nextDocument(dec); {
	case errors.Is(err, io.EOF):
	case err != nil:
		return nil, diag.List{yamlProblem(err)}
	default:
		list = append(list, problem("input.trailing_data", mark{second.Line, second.Column}.location(""),
			"the text goes on with another YAML document",
			"A file holds one YAML document. Remove the line of three hyphens and what follows it."))
	}
	return r.doc, list
}

var errLibraryFailed = errors.New("the YAML library failed")

// nextDocument reads the next document of the stream as a tree. The library
// recovers from the errors it raises itself. Any other panic is a defect in
// it, and is returned so that no text can stop the tool.
func nextDocument(dec *yaml.Decoder) (n *yaml.Node, err error) {
	defer func() {
		if recover() != nil {
			err = errLibraryFailed
		}
	}()
	n = new(yaml.Node)
	return n, dec.Decode(n)
}

// yamlError is the form of an error of the library: a line, when the library
// names one, and what the problem is.
var yamlError = regexp.MustCompile(`^yaml: (?:line ([0-9]+): )?(.*)$`)

// syntaxReason is what the tool says about one problem that the library
// names in a fixed text.
type syntaxReason struct {
	message, hint string
	// fromParser says that the parser of the library raises the problem, and
	// not its scanner. The parser counts lines from 0 and the scanner from 1.
	fromParser bool
}

const (
	tabHint    = "Look at this line and the lines after it. Indent with spaces, not with tabs."
	escapeHint = "The string starts on this line, or on the first line of the file. Inside double quotes a backslash starts an escape: write two backslashes for one, or put the string in single quotes."
)

var (
	tabReason    = syntaxReason{message: "a tab indents a line", hint: tabHint}
	escapeReason = syntaxReason{message: "a string in double quotes holds an escape that YAML does not know", hint: escapeHint}
)

// syntaxReasons says, for the problems that a person who writes YAML by hand
// meets, what is wrong and where to look. The line that the library names is
// where the block, the brackets, or the string with the problem starts. When
// that is the first line of the file, the library names the line at which it
// stopped reading instead. The hints say both.
var syntaxReasons = map[string]syntaxReason{
	"mapping values are not allowed in this context": {
		message: "a colon followed by a space stands where no key can be",
		hint:    "The colon is on this line. Often the line before it lacks the space after its colon or its hyphen, as in key:value or -item, so that the two lines read as one value. Otherwise this line holds a value with a colon followed by a space, which needs quotes, or it is indented more than the key above it.",
	},
	"did not find expected key": {
		fromParser: true,
		message:    "a line does not fit the mapping that it stands in",
		hint:       "The mapping starts on this line. When it starts on the first line of the file, this is the line that does not fit. Look from this line down for a key that is indented less or more than the keys before it, or for a closing bracket without an opening one.",
	},
	"did not find expected '-' indicator": {
		fromParser: true,
		message:    "a line does not fit the list that it stands in",
		hint:       "The list starts on this line. When it starts on the first line of the file, this is the line that does not fit. Look from this line down for a line that is indented less than the items before it and does not start with a hyphen.",
	},
	"did not find expected ',' or ']'": {
		fromParser: true,
		message:    "a list in brackets is not closed, or a comma is missing between two of its items",
		hint:       "The bracket [ opens on this line. When it opens on the first line of the file, this is the line where the list breaks off. Close the list with ], and separate its items with commas.",
	},
	"did not find expected ',' or '}'": {
		fromParser: true,
		message:    "a mapping in braces is not closed, or a comma is missing between two of its members",
		hint:       "The brace { opens on this line. When it opens on the first line of the file, this is the line where the mapping breaks off. Close the mapping with }, and separate its members with commas.",
	},
	"found character that cannot start any token": {
		message: "a key or a value starts with a character that YAML reserves, or a tab indents the line",
		hint:    "The character is on this line. Put a value in quotes when it starts with @, %, or `, and indent with spaces, not with tabs.",
	},
	"found a tab character that violates indentation":              tabReason,
	"found a tab character where an indentation space is expected": tabReason,
	"found unexpected end of stream": {
		message: "a quoted string is not closed",
		hint:    "The quote opens on this line, or on the first line of the file. Close the string with the quote character that opens it.",
	},
	"found unknown escape character":              escapeReason,
	"did not find expected hexdecimal number":     escapeReason,
	"found invalid Unicode character escape code": escapeReason,
	"could not find expected ':'": {
		message: "a key is not followed by a colon and a space",
		hint:    "The key is on this line, or on the first line of the file. Write the member as key: value, with a space after the colon.",
	},
	"block sequence entries are not allowed in this context": {
		message: "a list item starts where a list cannot start",
		hint:    "A hyphen followed by a space starts a list item on this line. Write the items on the lines below the key, or write the list in brackets.",
	},
	"exceeded max depth of 10000": {
		message: "the text nests thousands of levels deep",
		hint:    fmt.Sprintf("Objects and arrays are read up to %d levels deep. Flatten the structure.", maxDepth),
	},
}

// yamlProblem turns an error of the library into a diagnostic. The text of
// the error can quote the input, so it is never passed on: it picks one of
// the reasons above, or none.
func yamlProblem(err error) diag.Diagnostic {
	if errors.Is(err, errLibraryFailed) {
		return problem("tool.defect", diag.Location{},
			"the tool could not read the YAML text; this is a defect of the tool, please report it",
			"Report the defect with a text that causes it.")
	}
	const causes = "Check the indentation, the quotes, and the brackets. Put a value in quotes when it holds a colon followed by a space, or starts with a character such as *, &, !, %, @, or `."
	m := yamlError.FindStringSubmatch(err.Error())
	if m == nil {
		m = []string{"", "", ""}
	}
	line, _ := strconv.Atoi(m[1])
	if reason, known := syntaxReasons[m[2]]; known {
		switch {
		case line == 0:
			// The library leaves the line out exactly when the problem and
			// what holds it are both on the first line.
			line = 1
		case reason.fromParser:
			line++
		}
		return problem("input.syntax", diag.Location{Line: line}, "the text is not valid YAML: "+reason.message, reason.hint)
	}
	if strings.HasPrefix(m[2], "unknown anchor ") {
		return problem("input.syntax", diag.Location{}, "the text is not valid YAML: an alias names an anchor that is not defined",
			"An asterisk at the start of a value makes the value an alias. To write a string that starts with an asterisk, put it in quotes. An alias is not read: write the value out where it is used.")
	}
	if line > 0 {
		return problem("input.syntax", diag.Location{Line: line}, "the text is not valid YAML near this line",
			"The YAML reader names the line only roughly. Look at this line, the line before it, and the line after it, then further down in the block, the quotes, or the brackets that start there. "+causes)
	}
	return problem("input.syntax", diag.Location{}, "the text is not valid YAML",
		"The YAML reader did not say where. The problem is most likely on the first line. "+causes)
}

// yamlMark is the place of offset as the YAML library counts places: a line
// ends at a line feed, at a carriage return, at the two together, and at the
// line breaks U+0085, U+2028, and U+2029.
func yamlMark(text []byte, offset int) mark {
	at := mark{1, 1}
	for i := 0; i < offset; {
		r, size := utf8.DecodeRune(text[i:])
		switch {
		case r == '\r' && i+1 < len(text) && text[i+1] == '\n':
		case isBreak(r):
			at = mark{at.line + 1, 1}
		default:
			at.column++
		}
		i += size
	}
	return at
}

// firstDirective is the offset of the first directive of the text, or -1. A
// directive is a line that starts with % and stands before the document,
// where only empty lines and comments may come first.
func firstDirective(text []byte) int {
	for i := 0; i < len(text); {
		if text[i] == '%' {
			return i
		}
		content, next := firstLine(text[i:])
		if content = bytes.TrimLeft(content, " \t"); len(content) > 0 && content[0] != '#' {
			return -1
		}
		i += next
	}
	return -1
}

// firstLine returns the first line of text without its line break, and the
// offset of the line after it.
func firstLine(text []byte) (content []byte, next int) {
	for i := 0; i < len(text); {
		r, size := utf8.DecodeRune(text[i:])
		switch {
		case r == '\r' && i+1 < len(text) && text[i+1] == '\n':
			return text[:i], i + 2
		case isBreak(r):
			return text[:i], i + size
		}
		i += size
	}
	return text, len(text)
}

// isBreak reports whether the YAML library ends a line at r.
func isBreak(r rune) bool {
	return r == '\n' || r == '\r' || r == '\u0085' || r == '\u2028' || r == '\u2029'
}

// firstNotInYAML is the offset of the first character that a YAML text may
// not hold, or -1. The library refuses such a character without saying where
// it is.
func firstNotInYAML(text []byte) int {
	for i := 0; i < len(text); {
		r, size := utf8.DecodeRune(text[i:])
		allowed := r == '\t' || r == '\n' || r == '\r' || (r >= 0x20 && r <= 0x7E) || r == 0x85 ||
			(r >= 0xA0 && r <= 0xD7FF) || (r >= 0xE000 && r <= 0xFFFD) || r >= 0x10000
		if !allowed {
			return i
		}
		i += size
	}
	return -1
}

// The forms of a plain scalar in the core schema of YAML 1.2. numberForm
// holds every form that the schema reads as a number, and jsonNumberForm the
// plain decimal form, which is all that is read as one.
var (
	nullForm       = regexp.MustCompile(`^(~|null|Null|NULL)?$`)
	trueForm       = regexp.MustCompile(`^(true|True|TRUE)$`)
	falseForm      = regexp.MustCompile(`^(false|False|FALSE)$`)
	numberForm     = regexp.MustCompile(`^([-+]?[0-9]+|0o[0-7]+|0x[0-9a-fA-F]+|[-+]?(\.[0-9]+|[0-9]+(\.[0-9]*)?)([eE][-+]?[0-9]+)?|[-+]?\.(inf|Inf|INF)|\.(nan|NaN|NAN))$`)
	jsonNumberForm = regexp.MustCompile(`^-?(0|[1-9][0-9]*)(\.[0-9]+)?([eE][-+]?[0-9]+)?$`)
)

const (
	aliasHint = "Write the value out where it is used."
	tagHint   = "Remove the tag. To write a string, put the value in quotes."
)

type yamlReader struct {
	doc   *Document
	text  *lineIndex
	trail trail
	found diag.List
}

// lineIndex finds the bytes that stand at a place of the text. The reader
// asks for places in the order of the text, so a line is walked once however
// long it is.
type lineIndex struct {
	text   []byte
	starts []int
	// at and offset are the last place that was found.
	at     mark
	offset int
}

func newLineIndex(text []byte) *lineIndex {
	x := &lineIndex{text: text, starts: []int{0}}
	for i := 0; i < len(text); {
		_, next := firstLine(text[i:])
		i += next
		x.starts = append(x.starts, i)
	}
	return x
}

// from returns the text from the place on, or nothing for a place the text
// does not have.
func (x *lineIndex) from(at mark) []byte {
	if at.line < 1 || at.line > len(x.starts) || at.column < 1 {
		return nil
	}
	if at.line != x.at.line || at.column < x.at.column {
		x.at, x.offset = mark{at.line, 1}, x.starts[at.line-1]
	}
	for ; x.at.column < at.column && x.offset < len(x.text); x.at.column++ {
		_, size := utf8.DecodeRune(x.text[x.offset:])
		x.offset += size
	}
	return x.text[x.offset:]
}

// tagged reports whether the node is written with a tag. The library marks a
// tag such as !!str or !mine. It reads the tag "!" alone, which makes a
// scalar a string, without a mark, so the text at the place of a scalar is
// looked at: a scalar starts at its anchor or its tag, and lines and comments
// may stand between the anchor and the tag. On a collection the tag "!"
// changes nothing, and a block collection stands at its first key.
//
// After the anchor of a scalar without content, a "!" on a later line can be
// the tag of the key that follows. That key is refused for its tag as well.
func (r *yamlReader) tagged(n *yaml.Node) bool {
	if n.Style&yaml.TaggedStyle != 0 {
		return true
	}
	if n.Kind != yaml.ScalarNode {
		return false
	}
	written := r.text.from(markOf(n))
	if anchor := "&" + n.Anchor; n.Anchor != "" && bytes.HasPrefix(written, []byte(anchor)) {
		written = afterSeparation(written[len(anchor):])
	}
	return bytes.HasPrefix(written, []byte("!"))
}

// afterSeparation skips what may stand between the anchor, the tag, and the
// content of a node: spaces, tabs, line breaks, and comments.
func afterSeparation(text []byte) []byte {
	for len(text) > 0 {
		switch r, size := utf8.DecodeRune(text); {
		case r == ' ', r == '\t', isBreak(r):
			text = text[size:]
		case r == '#':
			_, next := firstLine(text)
			text = text[next:]
		default:
			return text
		}
	}
	return text
}

func markOf(n *yaml.Node) mark { return mark{n.Line, n.Column} }

// at is the location of the node, with the pointer that the trail leads to.
func (r *yamlReader) at(n *yaml.Node) diag.Location {
	return markOf(n).location(r.trail.pointer())
}

// unread reports a construct at the node that is not read. It leaves a gap
// in the value.
func (r *yamlReader) unread(n *yaml.Node, message, hint string) {
	r.doc.incomplete = true
	r.found = append(r.found, problem("input.unsupported", r.at(n), message, hint))
}

func isText(n *yaml.Node) bool {
	return n.Style&(yaml.SingleQuotedStyle|yaml.DoubleQuotedStyle|yaml.LiteralStyle|yaml.FoldedStyle) != 0
}

// value reads the value that the trail leads to, and where it stands.
func (r *yamlReader) value(n *yaml.Node) (any, place, error) {
	here := place{value: markOf(n)}
	switch {
	case n.Kind == yaml.AliasNode:
		r.unread(n, "an alias is not read", aliasHint)
		return nil, here, nil
	case r.tagged(n):
		r.unread(n, "a tag is not read", tagHint)
		return nil, here, nil
	case n.Kind == yaml.ScalarNode:
		return r.scalar(n), here, nil
	}
	if err := r.trail.open(here.value); err != nil {
		return nil, here, err
	}
	if n.Kind == yaml.SequenceNode {
		arr := make([]any, 0, len(n.Content))
		here.items = make([]place, 0, len(n.Content))
		for i, item := range n.Content {
			if err := r.trail.enter(strconv.Itoa(i), markOf(item)); err != nil {
				return nil, here, err
			}
			v, at, err := r.value(item)
			if err != nil {
				return nil, here, err
			}
			r.trail.leave()
			arr, here.items = append(arr, v), append(here.items, at)
		}
		return arr, here, nil
	}
	obj := map[string]any{}
	here.items = make([]place, 0, len(n.Content)/2)
	for i := 0; i+1 < len(n.Content); i += 2 {
		k := n.Content[i]
		key, ok := r.key(k)
		if !ok {
			continue
		}
		if err := r.trail.enter(escape(key), markOf(k)); err != nil {
			return nil, here, err
		}
		if _, dup := obj[key]; dup {
			r.found = append(r.found, duplicateKey(r.at(k)))
		}
		v, member, err := r.value(n.Content[i+1])
		if err != nil {
			return nil, here, err
		}
		r.trail.leave()
		member.key = markOf(k)
		obj[key] = v
		here.add(key, member)
	}
	return obj, here, nil
}

// key returns the text of a key of the mapping that the trail leads to. A
// key that is not a text is reported, and its member is left out of the
// value.
func (r *yamlReader) key(n *yaml.Node) (string, bool) {
	switch {
	case n.Kind == yaml.AliasNode:
		r.unread(n, "an alias is not read", aliasHint)
	case r.tagged(n):
		r.unread(n, "a tag is not read", tagHint)
	case n.Kind == yaml.ScalarNode && isText(n):
		return n.Value, true
	case n.Kind == yaml.ScalarNode && n.Value == "<<":
		r.unread(n, "a merge key is not read", "Write the members out in this mapping.")
	case n.Kind == yaml.ScalarNode && readsAsText(n.Value):
		return n.Value, true
	default:
		// No required property has such a key, so the value is still whole
		// enough to check against a schema.
		r.found = append(r.found, problem("input.non_string_key", r.at(n), "the key is not a string",
			"Write the key as a string. Put it in quotes when it reads as a number, true, false, or null."))
	}
	return "", false
}

// readsAsText reports whether the core schema reads the plain scalar as a
// text.
func readsAsText(plain string) bool {
	return !nullForm.MatchString(plain) && !trueForm.MatchString(plain) && !falseForm.MatchString(plain) && !numberForm.MatchString(plain)
}

func (r *yamlReader) scalar(n *yaml.Node) any {
	switch {
	case isText(n) || readsAsText(n.Value):
		return n.Value
	case nullForm.MatchString(n.Value):
		return nil
	case trueForm.MatchString(n.Value):
		return true
	case falseForm.MatchString(n.Value):
		return false
	case !jsonNumberForm.MatchString(n.Value):
		// 017 is 15 to one YAML reader and 17 to another, and an identifier
		// written as a number loses its leading zeros, so no reading is
		// picked.
		r.doc.incomplete = true
		r.found = append(r.found, problem("input.number_form", r.at(n), "the number is not written in plain decimal form",
			"Write a number in plain decimal form, such as 12, -3, 1.5, or 1e3. To write a string, such as an identifier that starts with 0, put the value in quotes."))
		return nil
	}
	if message := beyondNumberLimits(n.Value); message != "" {
		r.doc.incomplete = true
		r.found = append(r.found, numberLimit(r.at(n), message))
	}
	return json.Number(n.Value)
}
