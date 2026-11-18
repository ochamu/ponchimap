package diag_test

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"
	"unicode"

	"github.com/ochamu/ponchimap/internal/diag"
)

func TestOnlyAnErrorMakesAListFail(t *testing.T) {
	warning := diag.Diagnostic{Severity: diag.Warning, Code: "x.warning", Message: "look at this"}
	failing := diag.Diagnostic{Severity: diag.Error, Code: "x.error", Message: "this is wrong"}

	if l := (diag.List{}); l.HasErrors() {
		t.Error("an empty list fails")
	}
	if l := (diag.List{warning}); l.HasErrors() {
		t.Error("a list of warnings fails")
	}
	l := diag.List{warning, failing}
	if !l.HasErrors() {
		t.Error("a list with an error does not fail")
	}
}

func TestAListSortsByPlaceThenByWhatIsSaid(t *testing.T) {
	at := func(path string, line, column int, pointer, code string) diag.Diagnostic {
		return diag.Diagnostic{Severity: diag.Error, Code: code, Message: code,
			Location: diag.Location{Path: path, Pointer: pointer, Line: line, Column: column}}
	}
	want := diag.List{
		at("a.json", 0, 0, "/nodes/2", "x.one"),
		at("a.json", 0, 0, "/nodes/10", "x.one"),
		at("a.json", 3, 9, "/nodes/1/kind", "x.one"),
		at("a.json", 3, 20, "/nodes/1/evidence", "x.one"),
		at("a.json", 3, 20, "/nodes/1/id", "x.one"),
		at("a.json", 3, 20, "/nodes/1/id", "x.two"),
		at("b.json", 1, 1, "", "x.one"),
	}
	got := slices.Clone(want)
	slices.Reverse(got)
	got.Sort()
	if !slices.Equal(got, want) {
		t.Errorf("sorted to\n%v\nwant\n%v", got, want)
	}
}

func TestTheJSONFormNamesTheWholeDocumentWithAnEmptyPointer(t *testing.T) {
	raw, err := json.Marshal(diag.Diagnostic{Severity: diag.Error, Code: "input.syntax", Message: "m",
		Location: diag.Location{Path: "model.json", Line: 2, Column: 5}})
	if err != nil {
		t.Fatal(err)
	}
	const want = `{"severity":"error","code":"input.syntax","message":"m","location":{"path":"model.json","pointer":"","line":2,"column":5}}`
	if string(raw) != want {
		t.Errorf("JSON form\n%s\nwant\n%s", raw, want)
	}
}

func TestTheLineForAPersonStaysOneLineWhateverTheKeysOfTheInputHold(t *testing.T) {
	forged := diag.Diagnostic{Severity: diag.Error, Code: "schema.unknown_property", Message: "the property is not defined by the schema",
		Location: diag.Location{Path: "dir\nname/model.json", Pointer: "/x\nmodel.json:1:1: error forged.code: a forged line/y\x1b[2K\rhidden/z\u202egnp.exe", Line: 1, Column: 9}}
	text := forged.String()
	if lines := strings.Split(text, "\n"); len(lines) != 1 {
		t.Errorf("a diagnostic is written on %d lines:\n%q", len(lines), text)
	}
	for _, r := range text {
		if !unicode.IsPrint(r) {
			t.Errorf("the text holds the character %U as it is: %q", r, text)
		}
	}
	if !strings.Contains(forged.String(), "a forged line") || !strings.Contains(forged.String(), "hidden") {
		t.Errorf("the keys are no longer told apart: %q", forged.String())
	}

	plain := diag.Diagnostic{Severity: diag.Error, Code: "x.error", Message: "m",
		Location: diag.Location{Path: "docs/モデル.json", Pointer: "/nodes/0/attributes/役割", Line: 2, Column: 3}}
	if got, want := plain.String(), "docs/モデル.json:2:3: error x.error: m (/nodes/0/attributes/役割)"; got != want {
		t.Errorf("an ordinary path and pointer are rewritten:\n%s\nwant\n%s", got, want)
	}
}

// A syntax error of YAML has a line and no column. The column is left out,
// in the line for a person and in JSON, and never written as 0.
func TestAPlaceWithoutAColumnIsWrittenWithTheLineAlone(t *testing.T) {
	d := diag.Diagnostic{Severity: diag.Error, Code: "input.syntax", Message: "m", Location: diag.Location{Path: "view.yaml", Line: 4}}
	if got, want := d.String(), "view.yaml:4: error input.syntax: m"; got != want {
		t.Errorf("the line is %q, want %q", got, want)
	}
	out, err := json.Marshal(d)
	if err != nil {
		t.Fatal(err)
	}
	if want := `{"severity":"error","code":"input.syntax","message":"m","location":{"path":"view.yaml","pointer":"","line":4}}`; string(out) != want {
		t.Errorf("JSON form %s, want %s", out, want)
	}
}

func TestAListWithoutDiagnosticsIsAnEmptyArrayInJSON(t *testing.T) {
	var none diag.List
	out, err := json.Marshal(struct {
		Nil   diag.List `json:"nil"`
		Empty diag.List `json:"empty"`
	}{none, diag.List{}})
	if err != nil {
		t.Fatal(err)
	}
	if got, want := string(out), `{"nil":[],"empty":[]}`; got != want {
		t.Errorf("JSON form %s, want %s", got, want)
	}
	one, err := json.Marshal(diag.List{{Severity: diag.Warning, Code: "x.warning", Message: "m"}})
	if err != nil {
		t.Fatal(err)
	}
	if got, want := string(one), `[{"severity":"warning","code":"x.warning","message":"m","location":{"pointer":""}}]`; got != want {
		t.Errorf("JSON form %s, want %s", got, want)
	}
}
