package model_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/santhosh-tekuri/jsonschema/v6"

	"github.com/ochamu/ponchimap/internal/diag"
	"github.com/ochamu/ponchimap/internal/input"
	"github.com/ochamu/ponchimap/internal/model"
	"github.com/ochamu/ponchimap/schema"
)

const cases = "../../testdata/model"

// gotLine is a diagnostic in the first four fields of a line of a .want
// file: LINE:COLUMN, code, pointer, and target ID, with - for an empty field.
// The fifth field is a text the message contains.
func gotLine(d diag.Diagnostic) []string {
	dash := func(s string) string {
		if s == "" {
			return "-"
		}
		return s
	}
	return []string{fmt.Sprintf("%d:%d", d.Location.Line, d.Location.Column), d.Code, dash(d.Location.Pointer), dash(d.TargetID)}
}

func TestEveryCaseGivesTheDiagnosticsItsWantFileLists(t *testing.T) {
	paths, _ := filepath.Glob(filepath.Join(cases, "*.json"))
	if len(paths) == 0 {
		t.Fatal("no cases")
	}
	for _, path := range paths {
		name := strings.TrimSuffix(filepath.Base(path), ".json")
		t.Run(name, func(t *testing.T) {
			raw, err := os.ReadFile(filepath.Join(cases, name+".want"))
			if err != nil {
				t.Fatal(err)
			}
			var want [][]string
			for line := range strings.Lines(string(raw)) {
				if line = strings.TrimRight(line, "\n"); line != "" {
					want = append(want, strings.Split(line, "\t"))
				}
			}
			m, diags := load(t, path)
			var got []string
			for _, d := range diags {
				got = append(got, strings.Join(gotLine(d), "\t")+"\t"+d.Message)
			}
			report := fmt.Sprintf("got:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.TrimSpace(string(raw)))
			if len(diags) != len(want) {
				t.Fatalf("%d diagnostics, want %d\n%s", len(diags), len(want), report)
			}
			for i, d := range diags {
				if len(want[i]) != 5 {
					t.Fatalf("line %d of the want file does not have five fields", i+1)
				}
				if !slices.Equal(gotLine(d), want[i][:4]) || (want[i][4] != "-" && !strings.Contains(d.Message, want[i][4])) {
					t.Errorf("diagnostic %d differs\n%s", i+1, report)
				}
				if d.Location.Path != path {
					t.Errorf("diagnostic %d names the file %q, want %q", i+1, d.Location.Path, path)
				}
			}
			if strings.HasPrefix(name, "valid-") && m == nil {
				t.Error("a valid model was not returned")
			}
			if (m == nil) != diags.HasErrors() {
				t.Errorf("model returned = %v with errors = %v", m != nil, diags.HasErrors())
			}
		})
	}
}

func TestNoDiagnosticShowsAValueOfTheInput(t *testing.T) {
	path := filepath.Join(cases, "secret-values.json")
	_, diags := load(t, path)
	if len(diags) == 0 {
		t.Fatal("the case gave no diagnostics")
	}
	fields := []string{}
	for _, d := range diags {
		fields = append(fields, d.String(), d.Message, d.Hint, d.TargetID, d.Location.Pointer, d.Location.Path)
	}
	for _, f := range fields {
		if strings.Contains(strings.ToLower(f), "secret-token") {
			t.Errorf("a diagnostic shows a value of the input: %q", f)
		}
	}
}

// byValue makes JSON numbers comparable by their value, so that 1e1 equals 10.
func byValue(v any) any {
	switch x := v.(type) {
	case json.Number:
		r, _ := new(big.Rat).SetString(string(x))
		return r.RatString()
	case []any:
		for i := range x {
			x[i] = byValue(x[i])
		}
	case map[string]any:
		for k := range x {
			x[k] = byValue(x[k])
		}
	}
	return v
}

func decodeByValue(t *testing.T, raw []byte) any {
	t.Helper()
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil {
		t.Fatal(err)
	}
	return byValue(v)
}

func TestALoadedModelKeepsWhatItWasGiven(t *testing.T) {
	path := filepath.Join(cases, "valid-kept-values.json")
	m, diags := load(t, path)
	if m == nil || len(diags) > 0 {
		t.Fatalf("the model did not load: %v", diags)
	}
	n := m.Nodes[0]
	ev := n.Evidence[0]
	i := slices.IndexFunc(ev.Supports, func(s model.Support) bool { return s.Key == "vendor.uptime_days" })
	if i < 0 || !reflect.DeepEqual(ev.Supports[i].Value, map[string]any{"days": json.Number("12"), "exact": false}) {
		t.Errorf("the item with an undefined key is not kept with its value: %v", ev.Supports)
	}
	if got := fmt.Sprint(n.Identity["native_id"]); got != "12345678901234567890" {
		t.Errorf("identity.native_id prints as %s", got)
	}
	kept := map[string]map[string]any{
		"identity":       n.Identity,
		"attributes":     n.Attributes,
		"coverage scope": m.Coverage[0].Scope,
		"filter":         m.Coverage[0].Scope["filter"].(map[string]any),
		"target scope":   m.Target.Scope[0],
	}
	for where, key := range map[string]string{"identity": "labels", "attributes": "machine_type", "coverage scope": "zone", "filter": "labels", "target scope": "folder"} {
		if _, ok := kept[where][key]; !ok {
			t.Errorf("the undefined key %s under %s is not kept", key, where)
		}
	}
	if m.Schema != "../../schema/model.schema.json" {
		t.Errorf("$schema = %q", m.Schema)
	}
	if ev.Ref.StartLine != 10 || ev.Ref.EndLine != 12 {
		t.Errorf("the lines 1e1 and 12.0 read as %d and %d", ev.Ref.StartLine, ev.Ref.EndLine)
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	out, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := decodeByValue(t, out), decodeByValue(t, raw); !reflect.DeepEqual(got, want) {
		t.Errorf("the model written back differs from the input\ngot:  %v\nwant: %v", got, want)
	}
}

func parse(t *testing.T, text string) (*model.Model, diag.List) {
	t.Helper()
	return model.Parse("inline.json", []byte(text))
}

// load reads the model file at path.
func load(t *testing.T, path string) (*model.Model, diag.List) {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return model.Parse(path, raw)
}

func only(t *testing.T, diags diag.List, code string) diag.Diagnostic {
	t.Helper()
	if len(diags) != 1 || diags[0].Code != code {
		t.Fatalf("want one %s, got %v", code, diags)
	}
	return diags[0]
}

const minimal = `{"schema_version": "0.1", "target": {"repository": ".", "environment": "dev"}, "nodes": [], "edges": [], "coverage": []}`

func TestTheTextItselfIsChecked(t *testing.T) {
	d := only(t, second(parse(t, "{\n  \"schema_version\": \"0.1\",\n  \"target\" {}\n}")), "input.syntax")
	if d.Location.Line != 3 || d.Location.Column != 12 {
		t.Errorf("a syntax error at 3:12 is reported at %d:%d", d.Location.Line, d.Location.Column)
	}

	if m, diags := parse(t, "\xEF\xBB\xBF"+minimal); m == nil || len(diags) > 0 {
		t.Errorf("a model after a byte order mark was refused: %v", diags)
	}
	d = only(t, second(parse(t, "\xEF\xBB\xBF[]")), "schema.type")
	if d.Location.Line != 1 || d.Location.Column != 1 {
		t.Errorf("the byte order mark is counted in the position %d:%d", d.Location.Line, d.Location.Column)
	}

	shiftJIS := "{\"schema_version\": \"0.1\", \"target\": {\"repository\": \"\x83\x65\x83\x58\x83\x67\", \"environment\": \"dev\"}, \"nodes\": [], \"edges\": [], \"coverage\": []}"
	only(t, second(parse(t, shiftJIS)), "input.invalid_utf8")

	only(t, second(parse(t, "")), "input.syntax")

	japanese := "{\"schema_version\": \"0.1\", \"nodes\": [], \"edges\": [], \"coverage\": [],\n \"target\": {\"repository\": \"日本語のリポジトリ\", \"environment\": 1}}"
	d = only(t, second(parse(t, japanese)), "schema.type")
	if d.Location.Line != 2 || d.Location.Column != 55 {
		t.Errorf("a value after Japanese text is reported at %d:%d, want 2:55 counted in characters", d.Location.Line, d.Location.Column)
	}
}

// A number is read up to 400 characters and an exponent of 400 either way,
// which holds every identifier and every 64-bit float. Past that the tool
// refuses the number where it stands, before anything works it out: a few
// characters such as 1e1000000 would otherwise cost a large computation each.
func TestANumberBeyondTheLimitsIsRefusedWhereItStands(t *testing.T) {
	numbered := func(number string) string {
		return `{"schema_version": "0.1", "target": {"repository": ".", "environment": "dev"}, "nodes": [
 {"id": "a", "kind": "service", "provider": "p", "service": "s", "identity": {"native_id": ` + number + `}, "evidence": []}], "edges": [], "coverage": []}`
	}
	for number, read := range map[string]bool{
		"1e400":                         true,
		"1e-400":                        true,
		"1E+0400":                       true,
		"-1.5e400":                      true,
		strings.Repeat("9", 400):        true,
		"1e401":                         false,
		"1e-401":                        false,
		"1E+00401":                      false,
		"-1.5e401":                      false,
		strings.Repeat("9", 401):        false,
		"0." + strings.Repeat("7", 399): false,
		"1e1000000":                     false,
		"1e-1000000":                    false,
	} {
		m, diags := parse(t, numbered(number))
		if read {
			if m == nil || len(diags) > 0 {
				t.Errorf("%.20s (%d characters) is refused: %v", number, len(number), diags)
			} else if got := fmt.Sprint(m.Nodes[0].Identity["native_id"]); got != number {
				t.Errorf("%.20s is kept as %.20s", number, got)
			}
			continue
		}
		d := only(t, diags, "input.number_limit")
		_, secondLine, _ := strings.Cut(numbered(number), "\n")
		column := strings.Index(secondLine, number) + 1
		if m != nil || d.Location.Pointer != "/nodes/0/identity/native_id" || d.Location.Line != 2 || d.Location.Column != column {
			t.Errorf("%.20s: model = %v, diagnostic %.120v; want no model and the number at 2:%d", number, m != nil, d, column)
		}
		if said := d.Message + d.Hint; !strings.Contains(said, "400") || strings.Contains(said, number) {
			t.Errorf("%.20s: the diagnostic says %q; want the limit of 400 and not the number", number, said)
		}
	}
}

// Objects and arrays are read 64 levels deep. Every value is recorded under
// its JSON Pointer, so without a limit a chain of ten thousand brackets, 20 KB
// of text, takes 100 MB. The identity value below sits four levels deep: in
// the document, the nodes, the node, and the identity.
func TestNestingIsReadSixtyFourLevelsDeep(t *testing.T) {
	nested := func(arrays int) string {
		return `{"schema_version": "0.1", "target": {"repository": ".", "environment": "dev"}, "nodes": [
 {"id": "a", "kind": "service", "provider": "p", "service": "s", "identity": {"k": ` +
			strings.Repeat("[", arrays) + strings.Repeat("]", arrays) + `}, "evidence": []}], "edges": [], "coverage": []}`
	}
	if m, diags := parse(t, nested(60)); m == nil || len(diags) > 0 {
		t.Errorf("64 levels are refused: %v", diags)
	}
	for _, arrays := range []int{61, 9000} {
		text := nested(arrays)
		m, diags := parse(t, text)
		d := only(t, diags, "input.depth_limit")
		_, secondLine, _ := strings.Cut(text, "\n")
		column := strings.Index(secondLine, "[[") + 60 + 1
		pointer := "/nodes/0/identity/k" + strings.Repeat("/0", 60)
		if m != nil || d.Location.Pointer != pointer || d.Location.Line != 2 || d.Location.Column != column {
			t.Errorf("%d arrays: model = %v, diagnostic %.80v at %d:%d; want no model and the 65th level at 2:%d",
				arrays, m != nil, d.Code, d.Location.Line, d.Location.Column, column)
		}
		if !strings.Contains(d.Message+d.Hint, "64") {
			t.Errorf("%d arrays: the diagnostic says %q; want the limit of 64", arrays, d.Message+d.Hint)
		}
	}
}

// The JSON Pointer of a value takes 1024 bytes at most. Every diagnostic
// carries the pointer of its value, so without a limit the pointers of many
// diagnostics under a long key take many times the text. The pointer of the
// value below is /nodes/0/identity/ and the key.
func TestTheKeysThatLeadToAValueTake1024BytesAtMost(t *testing.T) {
	under := func(keyLength int) string {
		return `{"schema_version": "0.1", "target": {"repository": ".", "environment": "dev"}, "nodes": [
 {"id": "a", "kind": "service", "provider": "p", "service": "s", "identity": {"` + strings.Repeat("k", keyLength) + `": 1}, "evidence": []}], "edges": [], "coverage": []}`
	}
	longest := 1024 - len("/nodes/0/identity/")
	if m, diags := parse(t, under(longest)); m == nil || len(diags) > 0 {
		t.Errorf("a pointer of 1024 bytes is refused: %.200v", diags)
	}
	text := under(longest + 1)
	m, diags := parse(t, text)
	d := only(t, diags, "input.pointer_limit")
	_, secondLine, _ := strings.Cut(text, "\n")
	column := strings.Index(secondLine, `{"k`) + 2
	if m != nil || d.Location.Pointer != "/nodes/0/identity" || d.Location.Line != 2 || d.Location.Column != column {
		t.Errorf("model = %v, diagnostic at %d:%d with the pointer %.40q; want no model and the key at 2:%d under /nodes/0/identity",
			m != nil, d.Location.Line, d.Location.Column, d.Location.Pointer, column)
	}
	if !strings.Contains(d.Message+d.Hint, "1024") || d.Hint == "" {
		t.Errorf("the diagnostic says %.200q; want the limit of 1024", d.Message+d.Hint)
	}
}

// A machine writes a whole document on one line, so a column is found far
// from the start of its line and after characters of several bytes.
func TestAColumnOnALongLineCountsCharacters(t *testing.T) {
	long := strings.Repeat("長い名前のリポジトリ", 2000)
	text := `{"schema_version": "0.1", "nodes": [], "edges": [], "coverage": [], "target": {"repository": "` + long + `", "environment": 1}}`
	d := only(t, second(parse(t, text)), "schema.type")
	want := utf8.RuneCountInString(text[:strings.LastIndex(text, "1}}")]) + 1
	if d.Location.Line != 1 || d.Location.Column != want {
		t.Errorf("the value is reported at %d:%d, want 1:%d", d.Location.Line, d.Location.Column, want)
	}
}

func second(_ *model.Model, diags diag.List) diag.List { return diags }

// personLines is the list as a person reads it, one diagnostic a line.
func personLines(diags diag.List) string {
	lines := make([]string, len(diags))
	for i, d := range diags {
		lines[i] = d.String()
	}
	return strings.Join(lines, "\n")
}

// A key is written by whoever wrote the file, which may be another person's
// repository. A line break or an escape character in it must not turn one
// diagnostic into two lines or reach the terminal as it is.
func TestAKeyOfTheInputCannotBreakTheLineOfADiagnostic(t *testing.T) {
	escape := "\\" + "u001b"
	text := strings.Replace(minimal, `"coverage": []`,
		`"coverage": [], "x\nmodel.json:1:1: error forged.code: a forged line": 1, "y`+escape+`[2K\rhidden": 2`, 1)
	_, diags := parse(t, text)
	if len(diags) != 2 {
		t.Fatalf("want the two unknown properties, got %v", diags)
	}
	out := personLines(diags)
	if lines := strings.Split(out, "\n"); len(lines) != len(diags) {
		t.Errorf("%d diagnostics are written on %d lines:\n%q", len(diags), len(lines), out)
	}
	for _, r := range out {
		if r != '\n' && !unicode.IsPrint(r) {
			t.Errorf("the text holds the character %U as it is: %q", r, out)
		}
	}
}

func TestTheSameInputGivesTheSameList(t *testing.T) {
	path := filepath.Join(cases, "several-problems.json")
	_, first := load(t, path)
	for range 20 {
		if _, again := load(t, path); !reflect.DeepEqual(again, first) {
			t.Fatalf("the list changed between runs\nfirst:\n%v\nagain:\n%v", first, again)
		}
	}
}

func TestTheSchemaCompilesFromItsOwnBytesOnly(t *testing.T) {
	var doc any
	if err := json.Unmarshal(schema.Model, &doc); err != nil {
		t.Fatal(err)
	}
	walk(doc, "", func(pointer string, obj map[string]any) {
		if ref, ok := obj["$ref"].(string); ok && !strings.HasPrefix(ref, "#") {
			t.Errorf("%s refers outside the schema: %s", pointer, ref)
		}
	})
	input.MustCompile(schema.Model)

	for _, ref := range []string{"https://example.invalid/other.json", "other.json"} {
		func() {
			defer func() {
				if recover() == nil {
					t.Errorf("a schema that refers to %s compiled; the reference was loaded from somewhere", ref)
				}
			}()
			input.MustCompile([]byte(`{"$ref": "` + ref + `"}`))
		}()
	}
}

// walk calls fn for every object in v with its JSON Pointer.
func walk(v any, pointer string, fn func(pointer string, obj map[string]any)) {
	switch x := v.(type) {
	case map[string]any:
		fn(pointer, x)
		for k, inner := range x {
			walk(inner, pointer+"/"+strings.ReplaceAll(strings.ReplaceAll(k, "~", "~0"), "/", "~1"), fn)
		}
	case []any:
		for i, inner := range x {
			walk(inner, fmt.Sprintf("%s/%d", pointer, i), fn)
		}
	}
}

func schemaDoc(t *testing.T) map[string]any {
	t.Helper()
	var doc map[string]any
	if err := json.Unmarshal(schema.Model, &doc); err != nil {
		t.Fatal(err)
	}
	return doc
}

func TestTheSchemaAndTheGoTypesNameTheSameProperties(t *testing.T) {
	doc := schemaDoc(t)
	defs := doc["$defs"].(map[string]any)
	objects := map[string]struct {
		schema map[string]any
		goType any
	}{
		"root":     {doc, model.Model{}},
		"target":   {defs["target"].(map[string]any), model.Target{}},
		"node":     {defs["node"].(map[string]any), model.Node{}},
		"edge":     {defs["edge"].(map[string]any), model.Edge{}},
		"evidence": {defs["evidence"].(map[string]any), model.Evidence{}},
		"ref":      {defs["ref"].(map[string]any), model.Ref{}},
		"support":  {defs["support"].(map[string]any), model.Support{}},
		"coverage": {defs["coverage"].(map[string]any), model.Coverage{}},
	}
	for name, o := range objects {
		var inSchema []string
		for p := range o.schema["properties"].(map[string]any) {
			inSchema = append(inSchema, p)
		}
		var inGo []string
		typ := reflect.TypeOf(o.goType)
		for i := range typ.NumField() {
			tag, _, _ := strings.Cut(typ.Field(i).Tag.Get("json"), ",")
			inGo = append(inGo, tag)
		}
		slices.Sort(inSchema)
		slices.Sort(inGo)
		if !slices.Equal(inSchema, inGo) {
			t.Errorf("%s: the schema defines %v, the Go type %s has the tags %v", name, inSchema, typ.Name(), inGo)
		}
	}
}

func TestTheReferenceCheckCoversEveryPlaceTheSchemaNamesAnElement(t *testing.T) {
	want := []string{
		"/$defs/edge/properties/from -> #/$defs/nodeId",
		"/$defs/edge/properties/to -> #/$defs/nodeId",
		"/$defs/scope/properties/target_ids/items -> #/$defs/elementId",
		"/$defs/support/allOf/3/then/properties/value -> #/$defs/nodeId",
	}
	var got []string
	walk(schemaDoc(t), "", func(pointer string, obj map[string]any) {
		if ref, _ := obj["$ref"].(string); ref == "#/$defs/nodeId" || ref == "#/$defs/elementId" {
			got = append(got, pointer+" -> "+ref)
		}
	})
	slices.Sort(got)
	if !slices.Equal(got, want) {
		t.Errorf("the schema names a node or an element at other places than the reference check knows.\n"+
			"Extend the reference check in internal/model to them, then this list.\ngot:\n%s\nwant:\n%s",
			strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

// supportDescriptions are the descriptions the schema gives the value of each
// supports key it defines.
func supportDescriptions(t *testing.T) map[string]string {
	t.Helper()
	support := schemaDoc(t)["$defs"].(map[string]any)["support"].(map[string]any)
	out := map[string]string{}
	for _, clause := range support["allOf"].([]any) {
		c := clause.(map[string]any)
		key := c["if"].(map[string]any)["properties"].(map[string]any)["key"].(map[string]any)["const"].(string)
		value := c["then"].(map[string]any)["properties"].(map[string]any)["value"].(map[string]any)
		out[key], _ = value["description"].(string)
	}
	return out
}

func TestAWrongValueOfADefinedSupportsKeySaysWhatTheKeyMeans(t *testing.T) {
	descriptions := supportDescriptions(t)
	wrong := map[string]string{
		"existence": `"yes"`, "role": `""`, "connection.configured": `1`, "call.implemented": `"true"`,
		"traffic.observed": `null`, "region": `3`, "exposure": `true`, "lifecycle.requires": `"b"`,
	}
	if len(descriptions) < len(wrong) {
		t.Fatalf("the schema describes %d supports keys, fewer than the %d tried here", len(descriptions), len(wrong))
	}
	for key, value := range wrong {
		text := strings.Replace(withTimes("2026-11-13T09:00:00Z", ""),
			`{"key": "call.implemented", "value": true}`, `{"key": "`+key+`", "value": `+value+`}`, 1)
		d := only(t, second(parse(t, text)), map[bool]string{true: "schema.range", false: "schema.type"}[key == "role"])
		if descriptions[key] == "" || d.Hint != descriptions[key] {
			t.Errorf("%s: hint = %q, want the description of the key in the schema, %q", key, d.Hint, descriptions[key])
		}
	}
}

func TestTheHintOfAnAcquisitionIDDoesNotSpeakOfModelElements(t *testing.T) {
	text := strings.Replace(withTimes("2026-11-13T09:00:00Z", ""), `"checked_at"`, `"acquisition_id": "AQ 101", "checked_at"`, 1)
	d := only(t, second(parse(t, text)), "schema.format")
	if d.Hint == "" || strings.Contains(d.Hint, "node") || strings.Contains(d.Hint, "edge") {
		t.Errorf("hint = %q, want the ID form without a word about nodes or edges", d.Hint)
	}
}

func TestAVersionThatIsNotAStringIsATypeProblem(t *testing.T) {
	for _, version := range []string{`0.1`, `null`, `["0.1"]`} {
		text := strings.Replace(minimal, `"schema_version": "0.1"`, `"schema_version": `+version, 1)
		d := only(t, second(parse(t, text)), "schema.type")
		if d.Location.Pointer != "/schema_version" || !strings.Contains(d.Message, "string") || !strings.Contains(d.Hint, `"0.1"`) {
			t.Errorf("schema_version %s: %v, hint %q; want a type problem that names a string and shows \"0.1\"", version, d, d.Hint)
		}
	}
	d := only(t, second(parse(t, strings.Replace(minimal, `"0.1"`, `"0.2"`, 1))), "schema.unsupported_version")
	if !strings.Contains(d.Message, `"0.1"`) {
		t.Errorf("an unsupported version does not name the supported one: %v", d)
	}
}

// publicSchema checks a value the way a tool that has only the published
// schema does: the stock validator, its standard formats asserted, and none
// of the formats this tool adds.
func publicSchema(t *testing.T) func(text string) bool {
	t.Helper()
	doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(schema.Model))
	if err != nil {
		t.Fatal(err)
	}
	c := jsonschema.NewCompiler()
	c.AssertFormat()
	if err := c.AddResource("public:///model.schema.json", doc); err != nil {
		t.Fatal(err)
	}
	compiled, err := c.Compile("public:///model.schema.json")
	if err != nil {
		t.Fatal(err)
	}
	return func(text string) bool {
		value, err := jsonschema.UnmarshalJSON(strings.NewReader(text))
		if err != nil {
			t.Fatal(err)
		}
		return compiled.Validate(value) == nil
	}
}

func withTimes(checkedAt, validTime string) string {
	valid := ""
	if validTime != "" {
		valid = `, "valid_time": "` + validTime + `"`
	}
	return `{"schema_version": "0.1", "target": {"repository": ".", "environment": "dev"}, "nodes": [
	 {"id": "a", "kind": "service", "provider": "p", "service": "s", "evidence": [
	  {"id": "ev", "kind": "code", "supports": [{"key": "call.implemented", "value": true}],
	   "ref": {"type": "file", "path": "a.ts"}, "checked_at": "` + checkedAt + `"` + valid + `}]}], "edges": [], "coverage": []}`
}

func TestCheckedAtIsReadTheSameWayAsThePublishedSchemaAloneReadsIt(t *testing.T) {
	public := publicSchema(t)
	for moment, want := range map[string]bool{
		"2026-11-13T09:00:00Z":            true,
		"2026-11-13T18:00:00+09:00":       true,
		"2026-11-13T09:00:00-00:00":       true,
		"2026-11-13T09:00:00.123456789Z":  true,
		"2024-02-29T00:00:00Z":            true,
		"2026-11-13T9:00:00Z":             false,
		"2026-11-13T09:00:00+24:00":       false,
		"2026-11-13T09:00:00+23:60":       false,
		"2026-11-13t09:00:00z":            false,
		"2026-12-31T23:59:60Z":            false,
		"2026-11-13T24:00:00Z":            false,
		"2026-02-30T00:00:00Z":            false,
		"2026-11-13T09:00:00":             false,
		"2026-11-13T09:00:00+0900":        false,
		"2026-11-13T09:00:00,5Z":          false,
		"2026-11-13 09:00:00Z":            false,
		"2026-11-13":                      false,
		"2026-11-13T09:00:00.1234567890Z": false,
	} {
		text := withTimes(moment, "")
		m, _ := parse(t, text)
		if got := m != nil; got != want {
			t.Errorf("checked_at %s: loaded = %v, want %v", moment, got, want)
		}
		if got := public(text); got != want {
			t.Errorf("checked_at %s: the published schema alone accepts = %v, want %v", moment, got, want)
		}
		if _, err := time.Parse(time.RFC3339, moment); m != nil && err != nil {
			t.Errorf("checked_at %s was read, and time.Parse refuses it: %v", moment, err)
		}
	}
}

func TestValidTimeIsAMomentOrAPeriodThatDoesNotEndBeforeItStarts(t *testing.T) {
	public := publicSchema(t)
	for validTime, want := range map[string]bool{
		"2026-11-13T09:00:00Z":                                            true,
		"2026-11-13T09:00:00Z/2026-11-13T10:00:00Z":                       true,
		"2026-11-13T09:00:00Z/2026-11-13T09:00:00Z":                       true,
		"2026-11-13T00:00:00Z/2026-11-13T09:00:00+09:00":                  true,
		"2026-11-13T09:00:00.000000001Z/2026-11-13T09:00:00.000000002Z":   true,
		"2026-11-13T09:00:00.000000002Z/2026-11-13T09:00:00.000000001Z":   false,
		"2026-11-13T09:00:00.0000000002Z/2026-11-13T09:00:00.0000000001Z": false,
		"2026-11-13T10:00:00Z/2026-11-13T09:00:00Z":                       false,
		"2026-11-13T9:00:00Z":                                             false,
		"2026-11-13T09:00:00+24:00":                                       false,
		"2026-02-30T00:00:00Z":                                            false,
		"2025-01-01/2025-06-30":                                           false,
		"2026-11-13T09:00:00Z/P7D":                                        false,
		"2026-11-13T09:00:00Z/2026-11-13T10:00:00Z/2026-11-13T11:00:00Z":  false,
	} {
		text := withTimes("2026-11-13T09:00:00Z", validTime)
		m, _ := parse(t, text)
		if got := m != nil; got != want {
			t.Errorf("valid_time %s: loaded = %v, want %v", validTime, got, want)
		}
		// A tool without the format of this tool may accept more, never less.
		if want && !public(text) {
			t.Errorf("valid_time %s: the published schema alone refuses what the tool reads", validTime)
		}
	}
}

func TestValidID(t *testing.T) {
	for id, want := range map[string]bool{"n101": true, "agg-kv": true, "a": true, "": false, "bd_1": false, "N1": false, "9a": false, "a.b": false} {
		if got := model.ValidID(id); got != want {
			t.Errorf("ValidID(%q) = %v, want %v", id, got, want)
		}
	}
}
