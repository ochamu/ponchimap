package input

import (
	"bytes"
	"cmp"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"github.com/santhosh-tekuri/jsonschema/v6/kind"

	"github.com/ochamu/ponchimap/internal/diag"
)

// UnsupportedVersion is the code of a document whose schema_version the
// schema does not accept. Check reports nothing else about such a document.
const UnsupportedVersion = "schema.unsupported_version"

// schemaURL names the main schema of every Schema this package compiles, and
// a schema it refers to stands beside it in schemaFolder under its own name.
// The schemas carry no $id, and nothing is ever loaded from these URLs or any
// other.
const (
	schemaFolder = "input:///"
	schemaURL    = schemaFolder + "schema.json"
)

// Schema is a compiled JSON Schema that reports problems as diagnostics.
type Schema struct {
	compiled *jsonschema.Schema
	docs     map[string]any
}

// Resource is a schema that the main schema refers to by a relative $ref.
type Resource struct {
	Name string // as the $ref names it, such as model.schema.json
	JSON []byte
}

type refuseLoading struct{}

func (refuseLoading) Load(string) (any, error) {
	return nil, errors.New("a schema is never loaded from a URL")
}

// MustCompile compiles schema bytes, with the schemas that they refer to. It
// panics on a broken schema, which is a defect of the build, and never loads
// anything but the bytes it was given.
func MustCompile(schemaJSON []byte, others ...Resource) *Schema {
	c := jsonschema.NewCompiler()
	c.UseLoader(refuseLoading{})
	c.AssertFormat()
	c.RegisterFormat(&jsonschema.Format{Name: "date-time-or-period", Validate: validateInstantOrPeriod})
	docs := map[string]any{}
	add := func(at string, raw []byte) {
		doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(raw))
		if err != nil {
			panic(fmt.Sprintf("input: the schema %s is not JSON: %v", at, err))
		}
		// The library hands out the location of a refused property name in a
		// slice that it goes on writing to, so the location names whatever was
		// validated last. patternProperties with additionalProperties says the
		// same, and its violations are located.
		if hasKey(doc, "propertyNames") {
			panic(fmt.Sprintf("input: the schema %s uses propertyNames, whose violations cannot be located; use patternProperties with additionalProperties", at))
		}
		if err := c.AddResource(at, doc); err != nil {
			panic(fmt.Sprintf("input: %v", err))
		}
		docs[at] = doc
	}
	add(schemaURL, schemaJSON)
	for _, other := range others {
		add(schemaFolder+other.Name, other.JSON)
	}
	compiled, err := c.Compile(schemaURL)
	if err != nil {
		panic(fmt.Sprintf("input: the schema does not compile: %v", err))
	}
	return &Schema{compiled: compiled, docs: docs}
}

// hasKey reports whether an object anywhere in v has a member named key.
func hasKey(v any, key string) bool {
	switch x := v.(type) {
	case map[string]any:
		if _, has := x[key]; has {
			return true
		}
		for _, inner := range x {
			if hasKey(inner, key) {
				return true
			}
		}
	case []any:
		for _, inner := range x {
			if hasKey(inner, key) {
				return true
			}
		}
	}
	return false
}

var errNotTheForm = errors.New("the value does not have the form")

// validateInstantOrPeriod is the format date-time-or-period: one moment, or
// two joined by a slash with the first not after the second. The pattern
// beside the format in a schema gives the form of a moment. What is left to
// check here is that each date exists and that the period runs forward. The
// pattern allows at most nine digits of a second, so time.Time holds every
// moment exactly.
func validateInstantOrPeriod(v any) error {
	s, ok := v.(string)
	if !ok {
		return nil
	}
	parts := strings.Split(s, "/")
	if len(parts) > 2 {
		return errNotTheForm
	}
	var times []time.Time
	for _, p := range parts {
		t, err := time.Parse(time.RFC3339, p)
		if err != nil {
			return errNotTheForm
		}
		times = append(times, t)
	}
	if len(times) == 2 && times[0].After(times[1]) {
		return errNotTheForm
	}
	return nil
}

// valueCodes are the codes about a value itself, in the order in which one
// is kept when several land on the same value.
var valueCodes = []string{"schema.type", "schema.enum", "schema.format", "schema.range", "schema.duplicate_item", "schema.invalid"}

// Check returns every place where the document departs from the schema. An
// incomplete document is not checked: the reader has reported what it left
// out, and the check would report the gap again, or work a huge number out.
func (s *Schema) Check(d *Document) diag.List {
	if d.incomplete {
		return nil
	}
	if version, ok := s.unsupportedVersion(d); ok {
		return diag.List{version}
	}
	err := s.compiled.Validate(d.Value)
	if err == nil {
		return nil
	}
	var findings []finding
	var walk func(e *jsonschema.ValidationError, up []*jsonschema.ValidationError)
	walk = func(e *jsonschema.ValidationError, up []*jsonschema.ValidationError) {
		if len(e.Causes) == 0 {
			findings = append(findings, s.problems(e, up, d.null)...)
			return
		}
		for _, c := range e.Causes {
			walk(c, append(up, e))
		}
	}
	walk(err.(*jsonschema.ValidationError), nil)

	rank := func(code string) int { return slices.Index(valueCodes, code) + 1 }
	slices.SortFunc(findings, func(a, b finding) int {
		return cmp.Or(strings.Compare(a.pointer, b.pointer), cmp.Compare(rank(a.code), rank(b.code)),
			strings.Compare(a.code, b.code), strings.Compare(a.message, b.message), strings.Compare(a.hint, b.hint))
	})
	var list diag.List
	valueAt := map[string]bool{}
	seen := map[[3]string]bool{}
	for _, f := range findings {
		if rank(f.code) > 0 {
			if valueAt[f.pointer] {
				continue
			}
			valueAt[f.pointer] = true
		}
		if k := [3]string{f.code, f.pointer, f.message}; !seen[k] {
			seen[k] = true
			at := d.Locate(f.place, f.key)
			at.Pointer = f.pointer
			list = append(list, problem(f.code, at, f.message, f.hint))
		}
	}
	list.Sort()
	return list
}

// finding is one problem the schema library reported, before it is placed in
// the text.
type finding struct {
	code, pointer, message, hint string
	// place is the pointer whose position in the text the problem takes. It
	// differs from pointer for a missing member, which has no text of its own
	// and takes the position of the object that lacks it.
	place string
	// key says that the position is the member's key, not its value.
	key bool
}

// problems turns one leaf of the library's error tree into problems. Only
// names and limits that the schema defines go into a message.
func (s *Schema) problems(e *jsonschema.ValidationError, up []*jsonschema.ValidationError, null string) []finding {
	at := ""
	for _, tok := range e.InstanceLocation {
		at += "/" + escape(tok)
	}
	missing := func(names []string) []finding {
		var out []finding
		for _, name := range names {
			out = append(out, finding{code: "schema.required", pointer: at + "/" + escape(name), place: at,
				message: fmt.Sprintf("required property %q is missing", name), hint: s.propertyDescription(e.SchemaURL, name)})
		}
		return out
	}
	switch k := e.ErrorKind.(type) {
	case *kind.Required:
		return missing(k.Missing)
	case *kind.DependentRequired:
		return missing(k.Missing)
	case *kind.AdditionalProperties:
		message, hint := "the property is not defined by the schema", "No properties are defined here."
		if names := s.propertyNames(e.SchemaURL); len(names) > 0 {
			hint = "The properties defined here are: " + strings.Join(names, ", ") + "."
		} else if _, byPattern := s.object(e.SchemaURL)["patternProperties"]; byPattern {
			// The author names the properties here, in a form the schema
			// gives, and the description of the object says what the form is.
			message, hint = "the name of the property does not have the form that the schema allows here", s.description(e.SchemaURL)
		}
		var out []finding
		for _, name := range k.Properties {
			out = append(out, finding{code: "schema.unknown_property", pointer: at + "/" + escape(name), place: at + "/" + escape(name), key: true,
				message: message, hint: hint})
		}
		return out
	case *kind.UniqueItems:
		later := at + "/" + strconv.Itoa(k.Duplicates[1])
		return []finding{{code: "schema.duplicate_item", pointer: later, place: later,
			message: fmt.Sprintf("the item is already in the list, at index %d", k.Duplicates[0]), hint: s.valueHint(e, up)}}
	}
	code, message := valueProblem(e.ErrorKind, null)
	return []finding{{code: code, pointer: at, place: at, message: message, hint: s.valueHint(e, up)}}
}

// valueProblem is the code and the message for a value that the schema
// refuses. null is how the document says that a value is null.
func valueProblem(k jsonschema.ErrorKind, null string) (code, message string) {
	switch k := k.(type) {
	case *kind.Type:
		want := make([]string, len(k.Want))
		for i, w := range k.Want {
			want[i] = typeName(w)
		}
		got := typeName(k.Got)
		if k.Got == "null" {
			got = null
		}
		return "schema.type", fmt.Sprintf("the value is %s; it must be %s", got, strings.Join(want, " or "))
	case *kind.Enum:
		return "schema.enum", allowed(k.Want...)
	case *kind.Const:
		return "schema.enum", allowed(k.Want)
	case *kind.Pattern, *kind.Format:
		return "schema.format", "the value does not have the required form"
	case *kind.MinLength:
		if k.Want == 1 {
			return "schema.range", "the value must not be empty"
		}
		return "schema.range", fmt.Sprintf("the value must be %d characters or longer", k.Want)
	case *kind.MaxLength:
		return "schema.range", fmt.Sprintf("the value must be %d characters or shorter", k.Want)
	case *kind.MinItems:
		if k.Want == 1 {
			return "schema.range", "the value must not be empty"
		}
		return "schema.range", fmt.Sprintf("the value must hold %d items or more", k.Want)
	case *kind.MaxItems:
		return "schema.range", fmt.Sprintf("the value must hold %d items or fewer", k.Want)
	case *kind.MinProperties:
		return "schema.range", fmt.Sprintf("the value must hold %d properties or more", k.Want)
	case *kind.MaxProperties:
		return "schema.range", fmt.Sprintf("the value must hold %d properties or fewer", k.Want)
	case *kind.Minimum:
		return "schema.range", fmt.Sprintf("the value must be %s or more", number(k.Want))
	case *kind.Maximum:
		return "schema.range", fmt.Sprintf("the value must be %s or less", number(k.Want))
	case *kind.ExclusiveMinimum:
		return "schema.range", fmt.Sprintf("the value must be more than %s", number(k.Want))
	case *kind.ExclusiveMaximum:
		return "schema.range", fmt.Sprintf("the value must be less than %s", number(k.Want))
	}
	return "schema.invalid", "the value is not allowed here"
}

var typeNames = map[string]string{
	"null": "null", "boolean": "a boolean", "number": "a number", "integer": "an integer",
	"string": "a string", "array": "an array", "object": "an object",
}

func typeName(t string) string { return cmp.Or(typeNames[t], t) }

// allowed lists values that the schema gives, never the value of the input.
func allowed(values ...any) string {
	texts := make([]string, len(values))
	for i, v := range values {
		raw, _ := json.Marshal(v)
		texts[i] = string(raw)
	}
	return "the value is not one of the allowed values: " + strings.Join(texts, ", ")
}

func number(r *big.Rat) string {
	if r.IsInt() {
		return r.Num().String()
	}
	return strings.TrimRight(r.FloatString(20), "0")
}

func (s *Schema) unsupportedVersion(d *Document) (diag.Diagnostic, bool) {
	root, _ := d.Value.(map[string]any)
	got, isText := root["schema_version"].(string)
	version := s.object("#/properties/schema_version")
	want, pinned := version["const"]
	// A version that is not a text is a problem of its type, which the schema
	// reports with the rest of the document.
	if !isText || !pinned || got == want {
		return diag.Diagnostic{}, false
	}
	supported, _ := json.Marshal(want)
	hint, _ := version["description"].(string)
	return problem(UnsupportedVersion, d.Locate("/schema_version", false),
		fmt.Sprintf("this schema version is not supported; the supported version is %s", supported), hint), true
}

// valueHint is the description of the schema object that holds the failing
// keyword. When that object has none, as a shared definition often does, the
// object that refers to it with $ref for the same value is asked instead.
func (s *Schema) valueHint(e *jsonschema.ValidationError, up []*jsonschema.ValidationError) string {
	at := e.SchemaURL
	hint := s.description(at)
	for i := len(up) - 1; i >= 0 && hint == ""; i-- {
		ref, isRef := up[i].ErrorKind.(*kind.Reference)
		if isRef && ref.URL == at && slices.Equal(up[i].InstanceLocation, e.InstanceLocation) {
			at = up[i].SchemaURL
			hint = s.description(at)
		}
	}
	return hint
}

// propertyDescription is the description of the property name in the object
// schema at objectURL, or of the schema its $ref names.
func (s *Schema) propertyDescription(objectURL, name string) string {
	props, _ := s.object(objectURL)["properties"].(map[string]any)
	prop, _ := props[name].(map[string]any)
	if d, ok := prop["description"].(string); ok {
		return d
	}
	ref, _ := prop["$ref"].(string)
	base, err := url.Parse(objectURL)
	if ref == "" || err != nil {
		return ""
	}
	// A $ref is written relative to the document that holds it.
	target, err := base.Parse(ref)
	if err != nil {
		return ""
	}
	return s.description(target.String())
}

func (s *Schema) propertyNames(objectURL string) []string {
	props, _ := s.object(objectURL)["properties"].(map[string]any)
	names := make([]string, 0, len(props))
	for name := range props {
		names = append(names, name)
	}
	slices.Sort(names)
	return names
}

func (s *Schema) description(ref string) string {
	d, _ := s.object(ref)["description"].(string)
	return d
}

func (s *Schema) object(ref string) map[string]any {
	doc, fragment, found := strings.Cut(ref, "#")
	pointer, err := url.PathUnescape(fragment)
	if !found || err != nil || (pointer != "" && !strings.HasPrefix(pointer, "/")) {
		return nil
	}
	v := s.docs[cmp.Or(doc, schemaURL)]
	for _, tok := range strings.Split(pointer, "/")[1:] {
		tok = strings.ReplaceAll(strings.ReplaceAll(tok, "~1", "/"), "~0", "~")
		switch x := v.(type) {
		case map[string]any:
			v = x[tok]
		case []any:
			i, err := strconv.Atoi(tok)
			if err != nil || i < 0 || i >= len(x) {
				return nil
			}
			v = x[i]
		default:
			return nil
		}
	}
	obj, _ := v.(map[string]any)
	return obj
}
