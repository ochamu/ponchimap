package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/ochamu/ponchimap/internal/diag"
)

const (
	samples   = "../../testdata/samples"
	lintCases = "../../testdata/lint"
)

// answer is everything a caller of the command can observe besides files.
type answer struct {
	code           int
	stdout, stderr string
}

func run(args ...string) answer {
	return runWith(context.Background(), args...)
}

func runWith(ctx context.Context, args ...string) answer {
	var stdout, stderr bytes.Buffer
	code := Run(ctx, args, &stdout, &stderr)
	return answer{code, stdout.String(), stderr.String()}
}

// lastLine is the last line the command wrote for a person.
func (a answer) lastLine() string {
	lines := strings.Split(strings.TrimSuffix(a.stderr, "\n"), "\n")
	return lines[len(lines)-1]
}

// target is where one test render saves, all under dir.
type target struct {
	dir, output, report string
}

func newTarget(t *testing.T) target {
	dir := t.TempDir()
	return target{dir: dir, output: filepath.Join(dir, "figures"), report: filepath.Join(dir, "report.md")}
}

func (g target) flags() []string {
	return []string{"--output", g.output, "--report", g.report}
}

func (g target) svg() string { return filepath.Join(g.output, "overview.svg") }

func (g target) png() string { return filepath.Join(g.output, "overview.png") }

// render runs render with the model, the view config (none for an empty
// path), the paths of the target, and more arguments.
func (g target) render(model, config string, more ...string) answer {
	args := append([]string{"render", "--model", model, "--view", config}, g.flags()...)
	return run(append(args, more...)...)
}

func sample(name string) (model, config string) {
	return filepath.Join(samples, name, "model.json"), filepath.Join(samples, name, "view.yaml")
}

func lintCase(name string) (model, config string) {
	model, config = filepath.Join(lintCases, name, "model.json"), filepath.Join(lintCases, name, "view.yaml")
	if _, err := os.Stat(config); err != nil {
		config = ""
	}
	return model, config
}

// snapshot is every file under dir, by its path from dir, with its content.
func snapshot(t *testing.T, dir string) map[string]string {
	t.Helper()
	files := map[string]string{}
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel, _ := filepath.Rel(dir, path)
		files[filepath.ToSlash(rel)] = readFile(t, path)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return files
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func writeFile(t *testing.T, path, content string) string {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func exists(path string) bool {
	_, err := os.Lstat(path)
	return err == nil
}

// machineOutput is the JSON that a command writes with --json, with every
// member it may have and no other.
type machineOutput struct {
	Status      string            `json:"status"`
	Diagnostics []diag.Diagnostic `json:"diagnostics"`
	Artifacts   []machineArtifact `json:"artifacts"`
}

type machineArtifact struct {
	Kind   string `json:"kind"`
	Path   string `json:"path"`
	Layout *struct {
		Engine    string `json:"engine"`
		Direction string `json:"direction"`
		Seed      *int64 `json:"seed"`
	} `json:"layout"`
	Scale   *int     `json:"scale"`
	Added   []string `json:"added"`
	Removed []string `json:"removed"`
	IDs     []struct {
		Input, Kind, Candidate, ID string
	} `json:"ids"`
	// keys are the members the artifact has, in ascending order.
	keys []string
}

// machine reads stdout as the machine output and fails the test unless it is
// one JSON value, indented by two spaces, with one newline after it, whose
// members are the documented ones, the two lists written as arrays.
func machine(t *testing.T, stdout string) machineOutput {
	t.Helper()
	dec := json.NewDecoder(strings.NewReader(stdout))
	var root map[string]json.RawMessage
	if err := dec.Decode(&root); err != nil {
		t.Fatalf("the standard output is not a JSON object: %v\n%s", err, stdout)
	}
	if _, err := dec.Token(); err != io.EOF {
		t.Fatalf("the standard output holds more than one JSON value:\n%s", stdout)
	}
	if !strings.HasPrefix(stdout, "{\n  \"status\": \"") || !strings.HasSuffix(stdout, "\n}\n") || strings.HasSuffix(stdout, "\n\n") {
		t.Errorf("the JSON is not indented by two spaces with one newline at its end:\n%s", stdout)
	}
	if keys := slices.Sorted(maps.Keys(root)); !slices.Equal(keys, []string{"artifacts", "diagnostics", "status"}) {
		t.Fatalf("the JSON has the root members %v, want status, diagnostics, and artifacts", keys)
	}
	for _, list := range []string{"diagnostics", "artifacts"} {
		if !bytes.HasPrefix(root[list], []byte("[")) {
			t.Errorf("%s is %s, want an array", list, root[list])
		}
	}
	strict := json.NewDecoder(strings.NewReader(stdout))
	strict.DisallowUnknownFields()
	var out machineOutput
	if err := strict.Decode(&out); err != nil {
		t.Fatalf("the JSON has a member the document does not name: %v\n%s", err, stdout)
	}
	var artifacts []map[string]json.RawMessage
	if err := json.Unmarshal(root["artifacts"], &artifacts); err != nil {
		t.Fatal(err)
	}
	for i, members := range artifacts {
		out.Artifacts[i].keys = slices.Sorted(maps.Keys(members))
	}
	return out
}

func codes(list []diag.Diagnostic) []string {
	codes := make([]string, len(list))
	for i, d := range list {
		codes[i] = d.Code
	}
	return codes
}

// only is the one diagnostic with the code, and fails the test unless the
// list holds exactly one.
func only(t *testing.T, list []diag.Diagnostic, code string) diag.Diagnostic {
	t.Helper()
	var found []diag.Diagnostic
	for _, d := range list {
		if d.Code == code {
			found = append(found, d)
		}
	}
	if len(found) != 1 {
		t.Fatalf("the diagnostics hold %d of %s, want one: %v", len(found), code, codes(list))
	}
	return found[0]
}

// generatedModel is a valid model of n services, each called by an earlier
// one, so that the figure is one tree.
func generatedModel(n int) string {
	evidence := func(id string) []any {
		return []any{map[string]any{
			"id": id, "kind": "code",
			"supports":   []any{map[string]any{"key": "call.implemented", "value": true}},
			"ref":        map[string]any{"type": "file", "path": "src/app.ts"},
			"checked_at": "2026-11-13T00:00:00Z",
		}}
	}
	nodes, edges := []any{}, []any{}
	for i := range n {
		nodes = append(nodes, map[string]any{
			"id": fmt.Sprintf("n%d", i), "kind": "service", "provider": "example", "service": "example.api",
			"evidence": evidence(fmt.Sprintf("evn%d", i)),
		})
		if i > 0 {
			edges = append(edges, map[string]any{
				"id": fmt.Sprintf("e%d", i), "from": fmt.Sprintf("n%d", (i-1)/2), "to": fmt.Sprintf("n%d", i), "relation": "implementation",
				"evidence": evidence(fmt.Sprintf("eve%d", i)),
			})
		}
	}
	text, err := json.Marshal(map[string]any{
		"schema_version": "0.1",
		"target":         map[string]any{"repository": ".", "environment": "dev"},
		"nodes":          nodes, "edges": edges, "coverage": []any{},
	})
	if err != nil {
		panic(err)
	}
	return string(text)
}

// assertRefused fails the test unless the command line was refused: exit
// code 2, and one usage.invalid diagnostic for a person with its hint.
func assertRefused(t *testing.T, name string, a answer) {
	t.Helper()
	if a.code != 2 {
		t.Errorf("%s: exit code %d, want 2\n%s", name, a.code, a.stderr)
	}
	if !strings.Contains(a.stderr, "error usage.invalid: ") || !strings.Contains(a.stderr, "\n  hint: Run \"ponchimap help\"") {
		t.Errorf("%s: the standard error has no usage.invalid diagnostic with the hint to run help:\n%s", name, a.stderr)
	}
}

func TestACommandLineThatCannotBeRunIsRefusedAsText(t *testing.T) {
	for name, args := range map[string][]string{
		"no command":                            {},
		"an unknown command":                    {"draw"},
		"an unknown flag":                       {"lint", "--bogus"},
		"an argument that is no flag":           {"lint", "model.json"},
		"a flag without its value":              {"render", "--model"},
		"a flag of render with lint":            {"lint", "--output", "figures"},
		"a flag with version":                   {"version", "--verbose"},
		"a value for --json that is no boolean": {"lint", "--json=perhaps"},
	} {
		a := run(args...)
		assertRefused(t, name, a)
		if a.stdout != "" {
			t.Errorf("%s: the standard output holds %q, want nothing", name, a.stdout)
		}
	}
}

func TestACommandLineThatCannotBeRunIsRefusedAsJSONUnderJSON(t *testing.T) {
	for name, args := range map[string][]string{
		"no command":                  {"--json"},
		"an unknown command":          {"draw", "--json"},
		"an unknown flag":             {"lint", "--bogus", "--json"},
		"an argument that is no flag": {"lint", "--json", "model.json"},
		"a flag with version":         {"version", "--json"},
		"-json with one hyphen":       {"render", "-json", "--bogus"},
		"--json=true":                 {"render", "--bogus", "--json=true"},
	} {
		a := run(args...)
		assertRefused(t, name, a)
		out := machine(t, a.stdout)
		if out.Status != "error" || !slices.Equal(codes(out.Diagnostics), []string{"usage.invalid"}) || len(out.Artifacts) != 0 {
			t.Errorf("%s: the JSON is %+v, want status error, one usage.invalid, and no artifact", name, out)
		}
	}
	if a := run("lint", "--bogus", "--json=false"); a.stdout != "" {
		t.Errorf("--json=false: the standard output holds %q, want nothing", a.stdout)
	}
}

func TestHelpShowsEveryCommandOfTheTable(t *testing.T) {
	help := run("help")
	if help.code != 0 || help.stderr != "" {
		t.Fatalf("help: exit code %d and standard error %q, want 0 and nothing", help.code, help.stderr)
	}
	for _, c := range commands {
		if !strings.Contains(help.stdout, "\n  "+c.name+" ") || !strings.Contains(help.stdout, c.summary) {
			t.Errorf("help does not show the command %s with its summary:\n%s", c.name, help.stdout)
		}
	}
	if !strings.Contains(help.stdout, "\n  help ") {
		t.Errorf("help does not show help:\n%s", help.stdout)
	}
	for _, alias := range []string{"-h", "--help"} {
		if got := run(alias); got != help {
			t.Errorf("%s answers %+v, want the answer of help", alias, got)
		}
	}
}

func TestACommandWithHShowsItsFlags(t *testing.T) {
	inputFlags := []string{"--model", "--view", "--json"}
	renderFlags := []string{"--output", "--report", "--engine", "--direction", "--seed", "--reroll"}
	defaults := []string{".ponchimap/model.json", "architecture.view.yaml", "docs/architecture", ".ponchimap/report.md"}

	render := run("render", "-h")
	if render.code != 0 || render.stderr != "" {
		t.Fatalf("render -h: exit code %d and standard error %q, want 0 and nothing", render.code, render.stderr)
	}
	for _, want := range slices.Concat(inputFlags, renderFlags, defaults) {
		if !strings.Contains(render.stdout, want) {
			t.Errorf("render -h does not show %s:\n%s", want, render.stdout)
		}
	}
	if got := run("render", "--help"); got != render {
		t.Errorf("render --help answers %+v, want the answer of render -h", got)
	}

	lint := run("lint", "-h")
	for _, want := range inputFlags {
		if !strings.Contains(lint.stdout, want) {
			t.Errorf("lint -h does not show %s:\n%s", want, lint.stdout)
		}
	}
	for _, other := range renderFlags {
		if strings.Contains(lint.stdout, other) {
			t.Errorf("lint -h shows %s, a flag of render:\n%s", other, lint.stdout)
		}
	}
}

// A program that passes --json reads the standard output as JSON whatever
// else the command line says.
func TestUnderJSONTheFlagsOfACommandAreShownOnTheStandardError(t *testing.T) {
	a := run("render", "--json", "-h")
	if out := machine(t, a.stdout); a.code != 0 || out.Status != "ok" || len(out.Diagnostics) != 0 || len(out.Artifacts) != 0 {
		t.Errorf("render --json -h: exit code %d and JSON %+v, want 0 and an ok without diagnostics and artifacts", a.code, out)
	}
	if a.stderr != run("render", "-h").stdout {
		t.Errorf("render --json -h writes to the standard error\n%s\nwant the flags as render -h shows them", a.stderr)
	}
}

func TestVersionPrintsTheVersionsOfTheToolD2AndGoOnOneLine(t *testing.T) {
	required := regexp.MustCompile(`(?m)^\s*github\.com/d2lang/d2 (\S+)$`).FindStringSubmatch(readFile(t, "../../go.mod"))
	if required == nil {
		t.Fatal("go.mod requires no D2")
	}
	want := answer{code: 0, stdout: fmt.Sprintf("ponchimap (devel), d2 %s, %s\n", required[1], runtime.Version())}
	if got := run("version"); got != want {
		t.Errorf("version answers %+v, want %+v", got, want)
	}
}

func TestToolVersionPrefersTheReleaseTag(t *testing.T) {
	original := version
	t.Cleanup(func() { version = original })
	version = "  v1.2.3-rc.1\n"
	if got := toolVersion(); got != "v1.2.3-rc.1" {
		t.Errorf("toolVersion() = %q, want v1.2.3-rc.1", got)
	}
}
