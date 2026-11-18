package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// importCases is the folder of the import cases. It is worked out before a
// test changes the directory.
var importCases = func() string {
	path, err := filepath.Abs("../../testdata/import")
	if err != nil {
		panic(err)
	}
	return path
}()

// inRepository is the directory of a user who ran init and put the findings
// of one step of an import case into the default folder. The test runs
// there, so the command line can leave every path to its default.
func inRepository(t *testing.T, step string) {
	t.Helper()
	t.Chdir(t.TempDir())
	if err := os.CopyFS(".ponchimap/findings", os.DirFS(filepath.Join(importCases, step, "findings"))); err != nil {
		t.Fatal(err)
	}
}

const scanAndProbe = "scan-and-probe-in-one-run/1"

// changeFindings rewrites one findings file of the default folder.
func changeFindings(t *testing.T, name string, change func(file map[string]any)) {
	t.Helper()
	path := filepath.Join(".ponchimap/findings", name)
	var file map[string]any
	if err := json.Unmarshal([]byte(readFile(t, path)), &file); err != nil {
		t.Fatal(err)
	}
	change(file)
	text, err := json.MarshalIndent(file, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, path, string(text))
}

type savedModel struct {
	Target struct{ Environment string }
	Nodes  []struct {
		ID       string
		Evidence []struct {
			Kind string
			Ref  struct{ Path string }
		}
	}
}

func readModel(t *testing.T, path string) savedModel {
	t.Helper()
	var m savedModel
	if err := json.Unmarshal([]byte(readFile(t, path)), &m); err != nil {
		t.Fatal(err)
	}
	return m
}

const defaultModel = ".ponchimap/model.json"

func TestImportMakesTheModelWithNoFlagAndLintAcceptsIt(t *testing.T) {
	inRepository(t, scanAndProbe)
	a := run("import", "--json")
	out := machine(t, a.stdout)
	if a.code != 0 || out.Status != "ok" || len(out.Diagnostics) != 0 {
		t.Fatalf("exit code %d, status %q, diagnostics %v; want 0, ok, and none\n%s", a.code, out.Status, codes(out.Diagnostics), a.stderr)
	}
	if len(out.Artifacts) != 2 || out.Artifacts[0].Kind != "model" || out.Artifacts[0].Path != defaultModel ||
		out.Artifacts[1].Kind != "acquisitions" || out.Artifacts[1].Path != ".ponchimap/acquisitions.json" {
		t.Fatalf("the artifacts are %+v, want the model and the acquisition records at their default places", out.Artifacts)
	}
	saved := out.Artifacts[0]
	m := readModel(t, defaultModel)
	if m.Target.Environment != "dev" || len(m.Nodes) != 2 || len(saved.Added) != 3 || saved.Removed == nil || len(saved.Removed) != 0 {
		t.Errorf("the model is about %q and has %d nodes; added %v, removed %v; want dev, 2 nodes, 3 added, and an empty list of removed",
			m.Target.Environment, len(m.Nodes), saved.Added, saved.Removed)
	}
	// A program finds the element of the model that a candidate became.
	became := map[string]string{}
	for _, row := range saved.IDs {
		became[row.Input+"#"+row.Kind+"#"+row.Candidate] = row.ID
	}
	if id := became[".ponchimap/findings/scan.json#node#worker"]; id == "" || !slices.Contains(saved.Added, id) ||
		id != became[".ponchimap/findings/probe.json#node#w"] {
		t.Errorf("the candidates worker of the scan and w of the probe became %q and %q, want one added node; ids: %v",
			id, became[".ponchimap/findings/probe.json#node#w"], became)
	}
	if lint := run("lint"); lint.code != 0 {
		t.Errorf("lint ends with %d on the imported model:\n%s", lint.code, lint.stderr)
	}

	// The same input again is the same model.
	before := snapshot(t, ".ponchimap")
	again := machine(t, run("import", "--json").stdout)
	if after := snapshot(t, ".ponchimap"); !mapsEqual(after, before) || len(again.Artifacts[0].Added) != 0 {
		t.Errorf("the same import again changed the files or added %v", again.Artifacts[0].Added)
	}
}

func mapsEqual(a, b map[string]string) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if other, has := b[k]; !has || other != v {
			return false
		}
	}
	return true
}

func TestImportReadsTheSupplementsAtTheDefaultPlaceOnlyWhenTheyAreThere(t *testing.T) {
	inRepository(t, scanAndProbe)
	first := run("import", "--json")
	if first.code != 0 || strings.Contains(first.stderr, "supplements") {
		t.Fatalf("without supplements: exit code %d, want 0 and no word about supplements:\n%s", first.code, first.stderr)
	}
	// A file that the command line names has to be there.
	before := snapshot(t, ".ponchimap")
	a := run("import", "--supplements", ".ponchimap/supplements.yaml", "--json")
	if out := machine(t, a.stdout); a.code != 2 || !slices.Equal(codes(out.Diagnostics), []string{"input.unreadable"}) || !mapsEqual(snapshot(t, ".ponchimap"), before) {
		t.Errorf("a named file that is missing: exit code %d, diagnostics %v; want 2, input.unreadable alone, and the files as they were", a.code, codes(out.Diagnostics))
	}
	if a := run("import", "--supplements", ""); a.code != 0 || strings.Contains(a.stderr, "supplements") {
		t.Errorf("an empty name: exit code %d, want 0 and no word about supplements:\n%s", a.code, a.stderr)
	}

	// The supplements at the default place are read, and the model names the
	// file by one path however the command line spells it.
	worker := ""
	for _, row := range machine(t, first.stdout).Artifacts[0].IDs {
		if row.Kind == "node" && row.Candidate == "worker" {
			worker = row.ID
		}
	}
	writeFile(t, ".ponchimap/supplements.yaml", "schema_version: \"0.1\"\nstatements:\n  - target_id: "+worker+"\n    supports:\n      - key: role\n        value: serves the API\n    source: user\n    checked_at: \"2026-11-16T10:00:00Z\"\n")
	stated := func() []string {
		var paths []string
		for _, n := range readModel(t, defaultModel).Nodes {
			for _, ev := range n.Evidence {
				if n.ID == worker && ev.Kind == "stated" {
					paths = append(paths, ev.Ref.Path)
				}
			}
		}
		return paths
	}
	if a := run("import"); a.code != 0 || strings.Contains(a.stderr, "does not exist") || !slices.Equal(stated(), []string{".ponchimap/supplements.yaml"}) {
		t.Fatalf("with supplements at the default place: exit code %d, the statement is from %v; want 0 and the default path\n%s", a.code, stated(), a.stderr)
	}
	here, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	saved := snapshot(t, ".ponchimap")
	for _, spelled := range []string{filepath.Join(here, ".ponchimap/supplements.yaml"), "./.ponchimap/../.ponchimap/supplements.yaml"} {
		if a := run("import", "--supplements", spelled, "--input", filepath.Join(here, ".ponchimap", "findings")+string(filepath.Separator)); a.code != 0 || !mapsEqual(snapshot(t, ".ponchimap"), saved) {
			t.Errorf("the same files spelled as %s: exit code %d, and the files changed: %t; want 0 and no change\n%s", spelled, a.code, !mapsEqual(snapshot(t, ".ponchimap"), saved), a.stderr)
		}
	}
	if records := readFile(t, ".ponchimap/acquisitions.json"); strings.Contains(records, here) || !strings.Contains(records, "\"input\": \".ponchimap/findings/scan.json\"") {
		t.Errorf("the records do not name the findings from the directory of the command:\n%s", records)
	}
}

func TestImportEndsByWhatWentWrong(t *testing.T) {
	// A read that did not finish gives a result the user can use.
	inRepository(t, scanAndProbe)
	changeFindings(t, "probe.json", func(file map[string]any) { file["acquisition"].(map[string]any)["complete"] = false })
	a := run("import", "--json")
	if out := machine(t, a.stdout); a.code != 0 || out.Status != "partial" || len(out.Artifacts) != 2 {
		t.Errorf("an unfinished read: exit code %d, status %q, %d artifacts; want 0, partial, and the two files", a.code, out.Status, len(out.Artifacts))
	}

	// An input that breaks the schema is for the user to correct.
	inRepository(t, scanAndProbe)
	changeFindings(t, "scan.json", func(file map[string]any) { file["surprise"] = true })
	a = run("import", "--json")
	if out := machine(t, a.stdout); a.code != 2 || out.Status != "error" || !slices.Equal(codes(out.Diagnostics), []string{"schema.unknown_property"}) || exists(defaultModel) {
		t.Errorf("a broken input: exit code %d, status %q, diagnostics %v; want 2, error, schema.unknown_property alone, and no model", a.code, out.Status, codes(out.Diagnostics))
	}

	// A model that cannot be written is no fault of the input.
	inRepository(t, scanAndProbe)
	a = run("import", "--model", "no-such-directory/model.json", "--json")
	if out := machine(t, a.stdout); a.code != 4 || out.Status != "error" || !slices.Contains(codes(out.Diagnostics), "import.write_failed") {
		t.Errorf("a model in a directory that does not exist: exit code %d, status %q, diagnostics %v; want 4, error, and import.write_failed", a.code, out.Status, codes(out.Diagnostics))
	}

	assertRefused(t, "an empty model path", run("import", "--model", ""))
	assertRefused(t, "an empty input path", run("import", "--input", ""))
}

// A repository can hold a symbolic link under any name, also under the name
// that import gives a file while it writes it. What the link points to is
// outside what import may change.
func TestImportDoesNotWriteThroughASymbolicLinkOfTheRepository(t *testing.T) {
	outside := writeFile(t, filepath.Join(t.TempDir(), "outside.txt"), "kept\n")
	inRepository(t, scanAndProbe)
	if err := os.Symlink(outside, ".ponchimap/acquisitions.json.next.tmp"); err != nil {
		t.Fatal(err)
	}
	a := run("import")
	if a.code != 0 {
		t.Errorf("import beside a symbolic link: exit code %d, want 0\n%s", a.code, a.stderr)
	}
	if got := readFile(t, outside); got != "kept\n" {
		t.Errorf("import wrote through the symbolic link: the file outside the repository now starts with %.40q", got)
	}
	info, err := os.Lstat(".ponchimap/acquisitions.json")
	if err != nil {
		t.Fatal(err)
	}
	if !info.Mode().IsRegular() {
		t.Errorf("the records are saved as %v, want a regular file", info.Mode())
	}
}

// The model that import is about to save passes the checks of lint first, so
// a model that lint refuses is never the result of an import.
func TestImportSavesNoModelThatLintRefuses(t *testing.T) {
	inRepository(t, scanAndProbe)
	// A model written by hand whose only evidence denies its node.
	writeFile(t, defaultModel, `{"schema_version": "0.1", "target": {"repository": ".", "environment": "dev"},
  "nodes": [{"id": "n-example-account-01", "kind": "resource", "provider": "cloudflare", "service": "cloudflare.kv",
    "identity": {"account_id": "example-account-01", "native_id": "example-namespace"},
    "evidence": [{"id": "ev-by-hand", "kind": "declared", "supports": [{"key": "existence", "value": false}],
      "ref": {"type": "file", "path": "wrangler.toml"}, "checked_at": "2026-11-16T08:00:00Z"}]}],
  "edges": [], "coverage": []}
`)
	before := snapshot(t, ".ponchimap")
	a := run("import", "--json")
	out := machine(t, a.stdout)
	if a.code != 2 || !slices.Contains(codes(out.Diagnostics), "model.no_evidence") || len(out.Artifacts) != 0 || !mapsEqual(snapshot(t, ".ponchimap"), before) {
		t.Errorf("exit code %d, diagnostics %v, %d artifacts; want 2, model.no_evidence, none, and the files as they were\n%s",
			a.code, codes(out.Diagnostics), len(out.Artifacts), a.stderr)
	}
}

// The model is saved as render saves its files: nothing is written through a
// symbolic link.
func TestImportDoesNotWriteTheModelThroughALink(t *testing.T) {
	inRepository(t, scanAndProbe)
	if a := run("import"); a.code != 0 {
		t.Fatalf("the first import ends with %d:\n%s", a.code, a.stderr)
	}
	if err := os.Rename(defaultModel, "elsewhere.json"); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join("..", "elsewhere.json"), defaultModel); err != nil {
		t.Skipf("this system makes no symbolic link: %v", err)
	}
	kept := readFile(t, "elsewhere.json")
	// A newer read of the scan, so that the model has to change.
	changeFindings(t, "scan.json", func(file map[string]any) {
		acquisition := file["acquisition"].(map[string]any)
		acquisition["id"], acquisition["checked_at"] = "aq-scan-2", "2026-11-17T09:00:00Z"
	})
	a := run("import", "--json")
	out := machine(t, a.stdout)
	if a.code != 4 || !slices.Contains(codes(out.Diagnostics), "import.write_failed") || readFile(t, "elsewhere.json") != kept {
		t.Errorf("exit code %d, diagnostics %v, and the file behind the link changed: %t; want 4, import.write_failed, and the file as it was\n%s",
			a.code, codes(out.Diagnostics), readFile(t, "elsewhere.json") != kept, a.stderr)
	}
}

func TestANewModelIsAboutTheEnvironmentOfTheFindingsOrTheOneThatIsGiven(t *testing.T) {
	inRepository(t, scanAndProbe)
	changeFindings(t, "probe.json", func(file map[string]any) {
		file["acquisition"].(map[string]any)["target"].(map[string]any)["environment"] = "prod"
	})
	a := run("import", "--json")
	if out := machine(t, a.stdout); a.code != 2 || !slices.Equal(codes(out.Diagnostics), []string{"import.no_target"}) || exists(defaultModel) {
		t.Fatalf("findings of two environments: exit code %d, diagnostics %v; want 2, import.no_target alone, and no model", a.code, codes(out.Diagnostics))
	}
	if a := run("import", "--environment", "prod"); a.code != 0 || readModel(t, defaultModel).Target.Environment != "prod" {
		t.Fatalf("with --environment prod: exit code %d, and the model is about %q; want 0 and prod\n%s", a.code, readModel(t, defaultModel).Target.Environment, a.stderr)
	}
	// The model is there now, and what it is about does not change.
	a = run("import", "--environment", "staging")
	if a.code != 0 || readModel(t, defaultModel).Target.Environment != "prod" || !strings.Contains(a.stderr, "--environment is for a new model and is not used") {
		t.Errorf("--environment with a model that exists: exit code %d, the model is about %q; want 0, prod, and a note that the flag is not used\n%s",
			a.code, readModel(t, defaultModel).Target.Environment, a.stderr)
	}
}

func TestImportShowsItsFlagsAndTheirDefaults(t *testing.T) {
	a := run("import", "-h")
	if a.code != 0 || a.stderr != "" {
		t.Fatalf("import -h: exit code %d and standard error %q, want 0 and nothing", a.code, a.stderr)
	}
	for _, want := range []string{"--input", "--supplements", "--model", "--environment", "--json",
		".ponchimap/findings", ".ponchimap/supplements.yaml", ".ponchimap/model.json"} {
		if !strings.Contains(a.stdout, want) {
			t.Errorf("import -h does not show %s:\n%s", want, a.stdout)
		}
	}
}
