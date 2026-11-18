package view

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/ochamu/ponchimap/internal/model"
)

// lines writes an assessment in the form of testdata/marks/README.md, sorted.
func lines(a *Assessment) []string {
	field := func(items ...string) string {
		if joined := strings.Join(items, ","); joined != "" {
			return joined
		}
		return "-"
	}
	out := []string{}
	for _, f := range a.Findings {
		out = append(out, strings.Join([]string{string(f.Code), field(f.About...), field(f.Key), field(f.Slot), field(f.Evidence...), field(f.Coverage...)}, "\t"))
	}
	for _, d := range a.Defects {
		out = append(out, strings.Join([]string{string(d.Code), d.ID, "-", "-", field(d.Evidence...), "-"}, "\t"))
	}
	slices.Sort(out)
	return out
}

// states writes the findings without the coverage they list, so that two
// assessments can be compared when one model has more coverage records.
func states(a *Assessment) []string {
	out := []string{}
	for _, f := range a.Findings {
		f.Coverage = nil
		out = append(out, strings.Join(lines(&Assessment{Findings: []Finding{f}}), ""))
	}
	slices.Sort(out)
	return out
}

// A listing of an account names no element. When it stops half way, denied or
// failed, it has said nothing about what a read of an element itself found, so
// no element becomes unverified, and no mismatch goes away.
func TestAReadThatNamesNoElementAndStoppedChangesNoState(t *testing.T) {
	models, err := filepath.Glob(filepath.Join("..", "..", "testdata", "marks", "*.json"))
	if err != nil || len(models) == 0 {
		t.Fatalf("found no cases: %v", err)
	}
	samples, err := filepath.Glob(filepath.Join("..", "..", "testdata", "samples", "*", "model.json"))
	if err != nil || len(samples) == 0 {
		t.Fatalf("found no samples: %v", err)
	}
	for _, path := range append(models, samples...) {
		m := loadModel(t, path)
		// Every node gets an account, so that a listing of the account is
		// about it and about the settings it is the source of.
		listed := map[[2]string]bool{}
		for i := range m.Nodes {
			n := &m.Nodes[i]
			if n.Identity == nil {
				n.Identity = map[string]any{}
			}
			if _, ok := n.Identity["account_id"]; !ok {
				n.Identity["account_id"] = "example-listed-account"
			}
			listed[[2]string{n.Provider, fmt.Sprint(n.Identity["account_id"])}] = true
		}
		want := states(Assess(m))
		for _, status := range []model.CoverageStatus{model.CovPartial, model.CovDenied, model.CovUnsupported, model.CovFailed} {
			with := *m
			with.Coverage = slices.Clone(m.Coverage)
			for account := range listed {
				with.Coverage = append(with.Coverage, model.Coverage{
					ID:        "cv-listing-" + account[0],
					Scope:     model.Scope{"provider": account[0], "account_id": account[1]},
					Status:    status,
					CheckedAt: "2027-01-01T00:00:00Z",
				})
			}
			if got := states(Assess(&with)); !slices.Equal(got, want) {
				t.Errorf("%s: a %s listing of the account changed the states\n got  %q\n want %q", path, status, got, want)
			}
		}
	}
}

func TestAssessmentsMatchTheHandDerivedCases(t *testing.T) {
	models, err := filepath.Glob(filepath.Join("..", "..", "testdata", "marks", "*.json"))
	if err != nil || len(models) == 0 {
		t.Fatalf("found no cases: %v", err)
	}
	for _, path := range models {
		t.Run(strings.TrimSuffix(filepath.Base(path), ".json"), func(t *testing.T) {
			raw, err := os.ReadFile(strings.TrimSuffix(path, ".json") + ".want")
			if err != nil {
				t.Fatalf("the case has no expected lines: %v", err)
			}
			want := []string{}
			for _, line := range strings.Split(string(raw), "\n") {
				if line != "" {
					want = append(want, line)
				}
			}
			slices.Sort(want)
			m := loadModel(t, path)
			got := lines(Assess(m))
			if !slices.Equal(got, want) {
				t.Errorf("assessment\n got  %q\n want %q", got, want)
			}
			if again := Assess(m); !reflect.DeepEqual(again, Assess(m)) || !slices.Equal(lines(again), got) {
				t.Error("a second assessment of the same model differs")
			}
		})
	}
}
