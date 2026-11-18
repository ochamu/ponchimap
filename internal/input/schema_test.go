package input_test

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/ochamu/ponchimap/internal/diag"
	"github.com/ochamu/ponchimap/internal/input"
)

const namesAndLists = `{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "type": "object",
  "properties": {
    "labels": {
      "description": "Texts by name. A name is written in lowercase letters.",
      "type": "object",
      "patternProperties": { "^[a-z]+$": { "type": "string" } },
      "additionalProperties": false
    },
    "listed": {
      "description": "Each name once.",
      "type": "array",
      "items": { "type": "string" },
      "uniqueItems": true
    }
  }
}`

func checkJSON(t *testing.T, text string) diag.List {
	t.Helper()
	doc, diags := input.DecodeJSON([]byte(text))
	if doc == nil || len(diags) > 0 {
		t.Fatalf("the text was not read: %v", diags)
	}
	return input.MustCompile([]byte(namesAndLists)).Check(doc)
}

// line is a diagnostic as LINE:COLUMN code pointer.
func line(d diag.Diagnostic) string {
	return fmt.Sprintf("%d:%d %s %s", d.Location.Line, d.Location.Column, d.Code, d.Location.Pointer)
}

func lines(diags diag.List) []string {
	out := make([]string, len(diags))
	for i, d := range diags {
		out[i] = line(d)
	}
	return out
}

func wantLines(t *testing.T, diags diag.List, want ...string) {
	t.Helper()
	if got := lines(diags); strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("diagnostics:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

// Where the author names the properties, a name that lacks the form the
// schema gives is a problem of that member, at its key. The value of another
// member can be wrong as well, and each is reported where it stands.
func TestARefusedPropertyNameIsReportedAtItsKey(t *testing.T) {
	diags := checkJSON(t, `{"labels": {"Bad": "x", "ok": "y",
  "Worse": 3, "wrong": 4}, "listed": []}`)
	wantLines(t, diags,
		"1:13 schema.unknown_property /labels/Bad",
		"2:3 schema.unknown_property /labels/Worse",
		"2:24 schema.type /labels/wrong")
	for _, d := range diags[:2] {
		if !strings.Contains(d.Message, "name of the property") || d.Hint != "Texts by name. A name is written in lowercase letters." {
			t.Errorf("the diagnostic says %q with the hint %q; want it to speak of the name and say what a name is", d.Message, d.Hint)
		}
	}
}

// The schema library hands out the location of a name that propertyNames
// refuses in a slice it goes on writing to, so the location is that of
// whatever was validated last. A schema that uses the keyword is refused when
// the tool is built, before it can misplace a diagnostic.
func TestASchemaThatCannotLocateItsViolationsIsRefused(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Error("a schema with propertyNames was compiled")
		}
	}()
	input.MustCompile([]byte(`{"type": "object", "propertyNames": {"pattern": "^[a-z]+$"}}`))
}

func TestARepeatedItemIsReportedAtTheLaterItem(t *testing.T) {
	diags := checkJSON(t, `{"listed": ["a", "b", "a", "b"]}`)
	wantLines(t, diags, "1:23 schema.duplicate_item /listed/2")
	if d := diags[0]; !strings.Contains(d.Message, "index 0") || d.Hint != "Each name once." {
		t.Errorf("the diagnostic says %q with the hint %q; want the index of the first item and the description of the list", d.Message, d.Hint)
	}
}

// reportSchema defines the names of sharedSchema itself with other meanings,
// which nothing refers to, so that a hint taken from the wrong document shows.
const reportSchema = `{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "type": "object",
  "required": ["code"],
  "additionalProperties": false,
  "properties": {
    "code": { "$ref": "shared.schema.json#/$defs/code" },
    "title": {
      "description": "The title of the report.",
      "$ref": "shared.schema.json#/$defs/name"
    },
    "author": { "$ref": "shared.schema.json#/$defs/person" }
  },
  "$defs": {
    "name": { "description": "Not the name that the report refers to.", "type": "string" },
    "code": { "description": "Not the code that the report refers to.", "type": "string" },
    "person": {
      "description": "Not the person that the report refers to.",
      "type": "object",
      "properties": {
        "name": { "description": "Not the name of the person." },
        "alias": { "type": "string" }
      }
    }
  }
}`

const sharedSchema = `{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "$defs": {
    "name": { "type": "string", "minLength": 1 },
    "code": {
      "description": "A code is three capital letters.",
      "type": "string",
      "pattern": "^[A-Z]{3}$"
    },
    "person": {
      "type": "object",
      "required": ["name", "team"],
      "additionalProperties": false,
      "properties": {
        "name": { "description": "The name of the person.", "$ref": "#/$defs/name" },
        "team": { "$ref": "#/$defs/code" }
      }
    }
  }
}`

func checkReport(t *testing.T, text string) diag.List {
	t.Helper()
	doc, diags := input.DecodeJSON([]byte(text))
	if doc == nil || len(diags) > 0 {
		t.Fatalf("the text was not read: %v", diags)
	}
	shared := input.Resource{Name: "shared.schema.json", JSON: []byte(sharedSchema)}
	return input.MustCompile([]byte(reportSchema), shared).Check(doc)
}

func wantHints(t *testing.T, diags diag.List, want ...string) {
	t.Helper()
	got := make([]string, len(diags))
	for i, d := range diags {
		got[i] = d.Hint
	}
	if !slices.Equal(got, want) {
		t.Errorf("hints:\n%q\nwant:\n%q", got, want)
	}
}

// The definition of a name in the other document says nothing about what the
// name is for. The place that refers to it does.
func TestAValueRefusedByAnotherDocumentTakesTheHintOfThePlaceThatRefersToIt(t *testing.T) {
	diags := checkReport(t, `{"code": "ABC", "title": ""}`)
	wantLines(t, diags, "1:26 schema.range /title")
	wantHints(t, diags, "The title of the report.")
}

func TestAValueRefusedByAnotherDocumentTakesTheHintWrittenThere(t *testing.T) {
	diags := checkReport(t, `{"code": "abc"}`)
	wantLines(t, diags, "1:10 schema.format /code")
	wantHints(t, diags, "A code is three capital letters.")
}

func TestAnUnknownPropertyOfAnObjectOfAnotherDocumentListsThePropertiesDefinedThere(t *testing.T) {
	diags := checkReport(t, `{"code": "ABC",
 "author": {"name": "Ada", "team": "OPS",
  "nickname": "A"}}`)
	wantLines(t, diags, "3:3 schema.unknown_property /author/nickname")
	wantHints(t, diags, "The properties defined here are: name, team.")
}

func TestAMissingPropertyOfAnObjectOfAnotherDocumentTakesItsDescriptionThere(t *testing.T) {
	diags := checkReport(t, `{"code": "ABC",
 "author": {"team": "OPS"}}`)
	wantLines(t, diags, "2:12 schema.required /author/name")
	wantHints(t, diags, "The name of the person.")
}

// The $ref is read from the document that holds it: the code of the report is
// defined in the other document, and so is the team of a person, which the
// other document names with a $ref inside itself.
func TestAMissingPropertyIsDescribedByTheDefinitionItRefersTo(t *testing.T) {
	diags := checkReport(t, `{"author": {"name": "Ada"}}`)
	wantLines(t, diags,
		"1:1 schema.required /code",
		"1:12 schema.required /author/team")
	wantHints(t, diags, "A code is three capital letters.", "A code is three capital letters.")
}

func TestAReferredSchemaThatCannotLocateItsViolationsIsRefused(t *testing.T) {
	main := []byte(`{"$ref": "names.schema.json"}`)
	names := func(schema string) input.Resource {
		return input.Resource{Name: "names.schema.json", JSON: []byte(schema)}
	}
	input.MustCompile(main, names(`{"type": "object", "patternProperties": {"^[a-z]+$": {}}, "additionalProperties": false}`))
	defer func() {
		if recover() == nil {
			t.Error("a schema that refers to one with propertyNames was compiled")
		}
	}()
	input.MustCompile(main, names(`{"type": "object", "propertyNames": {"pattern": "^[a-z]+$"}}`))
}
