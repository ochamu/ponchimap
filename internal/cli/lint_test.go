package cli

import (
	"fmt"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
)

// wanted reads the want file of a lint case: the codes of the diagnostics
// the input gives, in order, and how many of them are errors and warnings.
// The files were derived by hand from the rules (testdata/lint/README.md).
func wanted(t *testing.T, name string) (codes []string, errors, warnings int) {
	t.Helper()
	codes = []string{}
	for _, line := range strings.Split(strings.TrimSpace(readFile(t, filepath.Join(lintCases, name, "want"))), "\n") {
		if line == "" {
			continue
		}
		fields := strings.Split(line, "\t")
		codes = append(codes, fields[3])
		if fields[0] == "error" {
			errors++
		} else {
			warnings++
		}
	}
	return codes, errors, warnings
}

func TestLintEndsWithCode2ExactlyWhenTheInputHasAnError(t *testing.T) {
	cases, _ := filepath.Glob(filepath.Join(lintCases, "*", "want"))
	if len(cases) < 12 {
		t.Fatalf("found %d lint cases, want twelve at least", len(cases))
	}
	plural := func(n int, noun string) string {
		if n != 1 {
			noun += "s"
		}
		return fmt.Sprintf("%d %s", n, noun)
	}
	for _, path := range cases {
		name := filepath.Base(filepath.Dir(path))
		wantCodes, errors, warnings := wanted(t, name)
		wantCode, wantStatus := 0, "ok"
		if errors > 0 {
			wantCode, wantStatus = 2, "error"
		}
		model, config := lintCase(name)
		a := run("lint", "--model", model, "--view", config, "--json")
		if a.code != wantCode {
			t.Errorf("%s: exit code %d, want %d", name, a.code, wantCode)
		}
		if want := plural(errors, "error") + ", " + plural(warnings, "warning"); a.lastLine() != want {
			t.Errorf("%s: the last line is %q, want %q", name, a.lastLine(), want)
		}
		out := machine(t, a.stdout)
		if out.Status != wantStatus || len(out.Artifacts) != 0 {
			t.Errorf("%s: status %q with %d artifacts, want %q with none", name, out.Status, len(out.Artifacts), wantStatus)
		}
		if got := codes(out.Diagnostics); !slices.Equal(got, wantCodes) {
			t.Errorf("%s: the diagnostics are %v, want %v", name, got, wantCodes)
		}
		for _, d := range out.Diagnostics {
			if !strings.Contains(a.stderr, d.String()+"\n  hint: "+d.Hint+"\n") {
				t.Errorf("%s: the standard error does not show %s in two lines, the second its hint:\n%s", name, d.Code, a.stderr)
			}
		}
	}
}

// The diagnostics go to the standard error, which also holds the count.
func TestLintWithoutJSONWritesNothingToTheStandardOutput(t *testing.T) {
	for _, name := range []string{"broken-view", "unknown-support-key", "no-evidence"} {
		model, config := lintCase(name)
		if a := run("lint", "--model", model, "--view", config); a.stdout != "" || !strings.Contains(a.stderr, "\n  hint: ") {
			t.Errorf("%s: the standard output holds %q and the standard error %q, want nothing and the diagnostics", name, a.stdout, a.stderr)
		}
	}
}

func TestLintWithoutFlagsReadsTheDefaultPathsAndWritesNothing(t *testing.T) {
	model, _ := lintCase("no-view")
	modelText := readFile(t, model)
	_, brokenView := lintCase("broken-view")
	brokenViewText := readFile(t, brokenView)

	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, ".ponchimap", "model.json"), modelText)
	t.Chdir(dir)
	before := snapshot(t, dir)

	if a, want := run("lint"), (answer{code: 0, stderr: "0 errors, 0 warnings\n"}); a != want {
		t.Errorf("lint without a view config answers %+v, want %+v", a, want)
	}
	if after := snapshot(t, dir); !reflect.DeepEqual(after, before) {
		t.Errorf("lint changed the files of the directory: %v, were %v", after, before)
	}

	writeFile(t, "architecture.view.yaml", brokenViewText)
	a := run("lint")
	if a.code != 2 || !strings.Contains(a.stderr, "architecture.view.yaml:") {
		t.Errorf("lint with a broken default view config: exit code %d, want 2 and a diagnostic at the file\n%s", a.code, a.stderr)
	}
}

func TestLintReportsAFileThatCannotBeRead(t *testing.T) {
	model, _ := lintCase("no-view")
	missing := filepath.Join(t.TempDir(), "missing")
	for name, args := range map[string][]string{
		"the model":             {"--model", missing, "--view", ""},
		"the named view config": {"--model", model, "--view", missing},
	} {
		a := run(append([]string{"lint", "--json"}, args...)...)
		out := machine(t, a.stdout)
		if d := only(t, out.Diagnostics, "input.unreadable"); a.code != 2 || d.Location.Path != missing {
			t.Errorf("%s is missing: exit code %d and input.unreadable at %q, want 2 and %q", name, a.code, d.Location.Path, missing)
		}
	}
}

func TestLintRefusesAnEmptyModelPath(t *testing.T) {
	a := run("lint", "--model", "", "--view", "", "--json")
	assertRefused(t, "an empty --model", a)
	out := machine(t, a.stdout)
	if d := only(t, out.Diagnostics, "usage.invalid"); !strings.Contains(d.Message, "--model") || len(out.Diagnostics) != 1 {
		t.Errorf("the diagnostics are %v with the message %q, want one usage.invalid that names --model", codes(out.Diagnostics), d.Message)
	}
}
