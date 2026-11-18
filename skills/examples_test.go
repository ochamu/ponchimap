// The test package is skills_test because internal/cli reaches this package
// through init, and an in-package test that imports internal/cli would be an
// import cycle.
package skills_test

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/ochamu/ponchimap/internal/cli"
	"github.com/ochamu/ponchimap/skills"
)

const reference = "ponchimap/references/findings.md"

// fence opens a findings example in the reference. Other JSON blocks there
// are fragments.
const fence = "```json findings"

type example struct {
	source string
	text   string
}

func examples(t *testing.T) []example {
	t.Helper()
	text, err := fs.ReadFile(skills.Files, reference)
	if err != nil {
		t.Fatal(err)
	}
	var found []example
	var block *strings.Builder
	lines := bufio.NewScanner(bytes.NewReader(text))
	for lines.Scan() {
		line := lines.Text()
		switch {
		case block == nil && line == fence:
			block = &strings.Builder{}
		case block != nil && line == "```":
			var file struct {
				Acquisition struct{ Source, Route string }
			}
			if err := json.Unmarshal([]byte(block.String()), &file); err != nil {
				t.Fatalf("an example of %s is not JSON: %v", reference, err)
			}
			found = append(found, example{file.Acquisition.Source, block.String()})
			block = nil
		case block != nil:
			block.WriteString(line + "\n")
		}
	}
	if block != nil {
		t.Fatalf("an example of %s has no closing fence", reference)
	}
	return found
}

// inWorkspace puts the examples where the Skill tells the agent to put
// them, one file per source, in a new folder that the test runs in.
func inWorkspace(t *testing.T, list []example) {
	t.Helper()
	t.Chdir(t.TempDir())
	folder := filepath.Join(".ponchimap", "findings")
	if err := os.MkdirAll(folder, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, ex := range list {
		path := filepath.Join(folder, ex.source+".json")
		if _, err := os.Stat(path); err == nil {
			t.Fatalf("two examples have the source %q; the Skill keeps one file per source", ex.source)
		}
		if err := os.WriteFile(path, []byte(ex.text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

type result struct {
	Status      string
	Diagnostics []struct{ Severity, Code, Message string }
	Artifacts   []struct {
		Kind           string
		Added, Removed []string
		IDs            []struct{ Input, Kind, Candidate, ID string }
	}
}

func run(t *testing.T, args ...string) result {
	t.Helper()
	var stdout, stderr bytes.Buffer
	code := cli.Run(context.Background(), append(args, "--json"), &stdout, &stderr)
	var out result
	if err := json.Unmarshal(stdout.Bytes(), &out); err != nil {
		t.Fatalf("%s: the output is not JSON: %v\n%s", args[0], err, stderr.String())
	}
	for _, d := range out.Diagnostics {
		if d.Severity == "error" {
			t.Errorf("%s: %s: %s", args[0], d.Code, d.Message)
		}
	}
	if code != 0 {
		t.Fatalf("%s: exit code %d, want 0\n%s", args[0], code, stderr.String())
	}
	return out
}

// modelIDs maps each node and edge candidate of each file to its model ID.
func modelIDs(out result) map[string]string {
	ids := map[string]string{}
	for _, a := range out.Artifacts {
		for _, m := range a.IDs {
			if m.Kind == "node" || m.Kind == "edge" {
				ids[m.Input+" "+m.Kind+" "+m.Candidate] = m.ID
			}
		}
	}
	return ids
}

func TestTheFindingsExamplesPassImportAndLintTogether(t *testing.T) {
	list := examples(t)
	var routes []string
	for _, ex := range list {
		var file struct{ Acquisition struct{ Route string } }
		_ = json.Unmarshal([]byte(ex.text), &file)
		routes = append(routes, file.Acquisition.Route)
	}
	for _, route := range []string{"repository", "cli", "document"} {
		if !slices.Contains(routes, route) {
			t.Errorf("%s has no findings example with the route %s", reference, route)
		}
	}
	inWorkspace(t, list)
	run(t, "import", "--environment", "production")
	run(t, "lint")
}

func TestRewritingTheExamplesAsTheSkillSaysKeepsEveryID(t *testing.T) {
	list := examples(t)
	inWorkspace(t, list)
	first := modelIDs(run(t, "import", "--environment", "production"))

	for i, ex := range list {
		text := strings.Replace(ex.text, `"checked_at": "2026-11-17T`, `"checked_at": "2026-11-18T`, 1)
		text = strings.Replace(text, `-20261117-`, `-20261118-`, 1)
		if text == ex.text {
			t.Fatalf("the example of %s does not have the acquisition form the test rewrites", ex.source)
		}
		list[i].text = text
	}
	for _, ex := range list {
		if err := os.WriteFile(filepath.Join(".ponchimap", "findings", ex.source+".json"), []byte(ex.text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	out := run(t, "import")
	for _, a := range out.Artifacts {
		if a.Kind == "model" && (len(a.Added) != 0 || len(a.Removed) != 0) {
			t.Errorf("the second import added %v and removed %v; want neither", a.Added, a.Removed)
		}
	}
	second := modelIDs(out)
	if len(first) == 0 || len(first) != len(second) {
		t.Fatalf("%d candidates mapped on the first import and %d on the second", len(first), len(second))
	}
	for key, id := range first {
		if second[key] != id {
			t.Errorf("%s: model ID %s, then %s", key, id, second[key])
		}
	}
}
