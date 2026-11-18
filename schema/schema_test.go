package schema_test

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/ochamu/ponchimap/internal/diag"
	"github.com/ochamu/ponchimap/internal/input"
	"github.com/ochamu/ponchimap/schema"
)

var modelSchema = input.Resource{Name: "model.schema.json", JSON: schema.Model}

// The examples are the files that the README and docs/reference.md give to
// the reader, so that they and the schemas cannot drift apart.
func example(t *testing.T, name string) string {
	t.Helper()
	raw, err := os.ReadFile("../examples/first-run/" + name)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

func read(t *testing.T, decode func([]byte) (*input.Document, diag.List), text string) *input.Document {
	t.Helper()
	doc, diags := decode([]byte(text))
	if doc == nil || len(diags) > 0 {
		t.Fatalf("the text was not read: %v", diags)
	}
	return doc
}

func wantProblems(t *testing.T, s *input.Schema, doc *input.Document, want ...string) {
	t.Helper()
	var got []string
	for _, d := range s.Check(doc) {
		got = append(got, d.Code+" "+d.Location.Pointer)
	}
	if !slices.Equal(got, want) {
		t.Errorf("the schema reports %q, want %q", got, want)
	}
}

// The ID form is defined in model.schema.json, so an ID out of that form
// shows that the schema reads the definitions it refers to.
func TestTheFindingsExamplesPassTheirSchema(t *testing.T) {
	findings := input.MustCompile(schema.Findings, modelSchema)
	names, err := filepath.Glob("../examples/first-run/findings/*.json")
	if err != nil || len(names) < 2 {
		t.Fatalf("the findings examples are %q (%v), want the repository read and the queries", names, err)
	}
	for _, name := range names {
		t.Run(filepath.Base(name), func(t *testing.T) {
			wantProblems(t, findings, read(t, input.DecodeJSON, example(t, "findings/"+filepath.Base(name))))
		})
	}

	text := example(t, "findings/memo-app.json")
	broken := strings.Replace(text, `"id": "aq-hand-20261116"`, `"id": "AQ scan"`, 1)
	wantProblems(t, findings, read(t, input.DecodeJSON, broken), "schema.format /acquisition/id")
}

func TestTheSupplementsExamplePassesItsSchema(t *testing.T) {
	supplements := input.MustCompile(schema.Supplements, modelSchema)
	text := example(t, "supplements.yaml")
	wantProblems(t, supplements, read(t, input.DecodeYAML, text))

	broken := strings.Replace(text, "target_id: n-1p6t4dhx", "target_id: N-1", 1)
	wantProblems(t, supplements, read(t, input.DecodeYAML, broken), "schema.format /statements/0/target_id")
}
