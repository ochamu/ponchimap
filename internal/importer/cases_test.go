package importer_test

import (
	"bytes"
	"cmp"
	"encoding/json"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/ochamu/ponchimap/internal/diag"
	"github.com/ochamu/ponchimap/internal/importer"
	"github.com/ochamu/ponchimap/internal/input"
	"github.com/ochamu/ponchimap/internal/model"
	"github.com/ochamu/ponchimap/internal/view"
)

// casesFolder is testdata/import, whose README.md says what a case holds and
// how it is run. The path is worked out before any test changes the working
// directory.
var casesFolder = func() string {
	folder, err := filepath.Abs("../../testdata/import")
	if err != nil {
		panic(err)
	}
	return folder
}()

func request() importer.Request {
	return importer.Request{Model: "model.json", Workspace: ".",
		Target: &model.Target{Repository: ".", Environment: "dev"}}
}

func TestCases(t *testing.T) {
	entries, err := os.ReadDir(casesFolder)
	if err != nil {
		t.Fatal(err)
	}
	cases := 0
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		cases++
		t.Run(entry.Name(), func(t *testing.T) {
			last, _ := runCase(t, entry.Name(), true)
			other, err := os.ReadFile(filepath.Join(casesFolder, entry.Name(), "same-as"))
			if err != nil {
				return
			}
			want, _ := runCase(t, strings.TrimSpace(string(other)), false)
			if diff := differences(sorted(t, last), sorted(t, want)); len(diff) > 0 {
				t.Errorf("the final model differs from the one of %s:\n%s", strings.TrimSpace(string(other)), strings.Join(diff, "\n"))
			}
		})
	}
	if cases == 0 {
		t.Fatal("testdata/import holds no case")
	}
}

type expected struct {
	Status      string          `json:"status"`
	Model       json.RawMessage `json:"model"`
	Diagnostics []struct {
		Severity string  `json:"severity"`
		Code     string  `json:"code"`
		Path     string  `json:"path"`
		Pointer  *string `json:"pointer"`
		TargetID *string `json:"target_id"`
	} `json:"diagnostics"`
	Added        *[]string         `json:"added"`
	Removed      *[]string         `json:"removed"`
	Mapping      map[string]string `json:"mapping"`
	Acquisitions *[]acquisition    `json:"acquisitions"`
	View         *[]viewDiagnostic `json:"view"`
}

type acquisition struct {
	ID       string `json:"id"`
	Source   string `json:"source"`
	Route    string `json:"route"`
	Complete bool   `json:"complete"`
}

type viewDiagnostic struct {
	Code     string `json:"code"`
	TargetID string `json:"target_id"`
}

type files struct{ model, records []byte }

func saved(t *testing.T) files {
	t.Helper()
	read := func(name string) []byte {
		text, err := os.ReadFile(name)
		if err != nil && !os.IsNotExist(err) {
			t.Fatal(err)
		}
		return text
	}
	return files{read("model.json"), read("acquisitions.json")}
}

func (f files) equal(other files) bool {
	same := func(a, b []byte) bool { return (a == nil) == (b == nil) && bytes.Equal(a, b) }
	return same(f.model, other.model) && same(f.records, other.records)
}

func runCase(t *testing.T, name string, verify bool) ([]byte, importer.Request) {
	t.Helper()
	source := filepath.Join(casesFolder, name)
	workspace := t.TempDir()
	t.Chdir(workspace)
	for _, start := range []string{"model.json", "acquisitions.json"} {
		if _, err := os.Stat(filepath.Join(source, start)); err == nil {
			copyFile(t, filepath.Join(source, start), start, nil)
		}
	}

	names := names{}
	var last importer.Request
	for n := 1; ; n++ {
		step := filepath.Join(source, strconv.Itoa(n))
		if _, err := os.Stat(step); err != nil {
			if n == 1 {
				t.Fatalf("the case %s has no step", name)
			}
			break
		}
		fail := func(format string, args ...any) {
			t.Helper()
			t.Errorf("step %d: %s", n, fmt.Sprintf(format, args...))
		}

		req := request()
		if err := os.RemoveAll("findings"); err != nil {
			t.Fatal(err)
		}
		os.Remove("supplements.yaml")
		var inputs []string
		if given, err := os.ReadDir(filepath.Join(step, "findings")); err == nil {
			if err := os.Mkdir("findings", 0o755); err != nil {
				t.Fatal(err)
			}
			for _, file := range given {
				inputs = append(inputs, filepath.Join("findings", file.Name()))
				copyFile(t, filepath.Join(step, "findings", file.Name()), inputs[len(inputs)-1], names.resolve(t))
			}
			req.Findings = []string{"findings"}
		}
		if _, err := os.Stat(filepath.Join(step, "supplements.yaml")); err == nil {
			copyFile(t, filepath.Join(step, "supplements.yaml"), "supplements.yaml", names.resolve(t))
			req.Supplements = "supplements.yaml"
		}

		before := saved(t)
		elsewhere := t.TempDir()
		if verify {
			if err := os.CopyFS(elsewhere, os.DirFS(workspace)); err != nil {
				t.Fatal(err)
			}
		}
		res := importer.Run(req)
		after := saved(t)
		names[n], last = res, req
		if !verify {
			continue
		}

		var want expected
		if err := json.Unmarshal(names.resolve(t)(mustRead(t, filepath.Join(step, "want.json"))), &want); err != nil {
			t.Fatalf("step %d: want.json: %v", n, err)
		}
		if string(res.Status) != want.Status {
			fail("status %q, want %q\n%s", res.Status, want.Status, listed(res.Diagnostics))
		}
		switch string(want.Model) {
		case `"any"`:
		case `"unchanged"`:
			if !after.equal(files{before.model, after.records}) {
				fail("the model file changed")
			}
		default:
			if after.model == nil {
				fail("no model was saved\n%s", listed(res.Diagnostics))
			} else if diff := differences(sorted(t, after.model), sorted(t, want.Model)); len(diff) > 0 {
				fail("the saved model differs from the wanted one:\n%s", strings.Join(diff, "\n"))
			}
		}

		var wanted, got []string
		for _, d := range want.Diagnostics {
			pointer, target := "*", "*"
			if d.Pointer != nil {
				pointer = *d.Pointer
			}
			if d.TargetID != nil {
				target = *d.TargetID
			}
			wanted = append(wanted, fmt.Sprintf("%s %s %s %s %s", d.Severity, d.Code, d.Path, pointer, target))
		}
		for _, d := range res.Diagnostics {
			got = append(got, fmt.Sprintf("%s %s %s %s %s", d.Severity, d.Code, d.Location.Path, d.Location.Pointer, d.TargetID))
		}
		if missing, extra := unmatched(wanted, got); len(missing)+len(extra) > 0 {
			fail("diagnostics (severity code path pointer target):\n  wanted and not given: %q\n  given and not wanted: %q", missing, extra)
		}
		if want.Added != nil && !sameSet(*want.Added, res.Added) {
			fail("added %q, want %q", res.Added, *want.Added)
		}
		if want.Removed != nil && !sameSet(*want.Removed, res.Removed) {
			fail("removed %q, want %q", res.Removed, *want.Removed)
		}
		for candidate, id := range want.Mapping {
			input, local, _ := strings.Cut(candidate, "#")
			at := slices.IndexFunc(res.IDs, func(m importer.Mapping) bool { return m.Input == input && m.Candidate == local })
			switch {
			case at < 0:
				fail("the result has no mapping of %s", candidate)
			case res.IDs[at].ID != id:
				fail("%s became %q, want %q", candidate, res.IDs[at].ID, id)
			}
		}
		if want.Acquisitions != nil {
			var file struct {
				Acquisitions []acquisition `json:"acquisitions"`
			}
			if err := json.Unmarshal(after.records, &file); err != nil {
				fail("acquisitions.json: %v", err)
			}
			byID := func(a, b acquisition) int { return cmp.Compare(a.ID, b.ID) }
			slices.SortFunc(file.Acquisitions, byID)
			slices.SortFunc(*want.Acquisitions, byID)
			if !slices.Equal(file.Acquisitions, *want.Acquisitions) {
				fail("acquisitions.json holds %+v, want %+v", file.Acquisitions, *want.Acquisitions)
			}
		}
		if text, err := os.ReadFile(filepath.Join(step, "view.yaml")); err == nil {
			if want.View == nil {
				t.Fatalf("step %d: the step has a view.yaml and want.json has no view", n)
			}
			config, found := view.ParseConfig("view.yaml", names.resolve(t)(text))
			m, loaded := model.Parse("model.json", mustRead(t, "model.json"))
			if config == nil || m == nil || len(found) > 0 {
				t.Fatalf("step %d: view.yaml or the saved model cannot be read: %v %v", n, found, loaded)
			}
			_, found = config.Resolve(m)
			var got []viewDiagnostic
			for _, d := range found {
				got = append(got, viewDiagnostic{d.Code, d.TargetID})
			}
			byTarget := func(a, b viewDiagnostic) int {
				return cmp.Or(cmp.Compare(a.Code, b.Code), cmp.Compare(a.TargetID, b.TargetID))
			}
			slices.SortFunc(got, byTarget)
			slices.SortFunc(*want.View, byTarget)
			if !slices.Equal(got, *want.View) {
				fail("resolving view.yaml gives %+v, want %+v", got, *want.View)
			}
		}

		var written struct {
			Marks map[string][]string `json:"marks"`
		}
		if err := json.Unmarshal(mustRead(t, filepath.Join(step, "want.json")), &written); err != nil {
			t.Fatalf("step %d: want.json: %v", n, err)
		}
		if written.Marks != nil {
			if problems := markProblems(written.Marks, names.resolve(t)); len(problems) > 0 {
				fail("the marks of the saved model:\n%s", strings.Join(problems, "\n"))
			}
		}

		if res.Status == "error" && !after.equal(before) {
			fail("the step ended in an error and changed the model or the records")
		}
		importer.Run(req)
		if again := saved(t); !again.equal(after) {
			fail("running the step again changed the model or the records:\n%s",
				strings.Join(differences(sorted(t, again.model), sorted(t, after.model)), "\n"))
		}
		t.Chdir(elsewhere)
		slices.Reverse(inputs)
		req.Findings = inputs
		reversed := importer.Run(req)
		if got := saved(t); !got.equal(after) {
			fail("giving the files in the reverse order saves another model or other records:\n%s\n%s",
				strings.Join(differences(sorted(t, got.model), sorted(t, after.model)), "\n"), got.records)
		}
		if !reflect.DeepEqual(reversed.Diagnostics, res.Diagnostics) {
			fail("giving the files in the reverse order gives other diagnostics:\n%s\nand in the order of the names:\n%s",
				listed(reversed.Diagnostics), listed(res.Diagnostics))
		}
		t.Chdir(workspace)
	}
	return saved(t).model, last
}

// The wanted marks are keyed by the names as want.json writes them, so that a
// problem can say the name beside the issued ID.
func markProblems(wanted map[string][]string, resolve func([]byte) []byte) []string {
	raw, unreadable := input.ReadFile("model.json")
	m, invalid := model.Parse("model.json", raw)
	if m == nil {
		return []string{"  the saved model cannot be read:\n" + listed(slices.Concat(unreadable, invalid))}
	}
	assessment := view.Assess(m)
	var problems []string
	for _, d := range assessment.Defects {
		problems = append(problems, fmt.Sprintf("  defect %s: %v", d.Code, d))
	}

	nameOf := map[string]string{}
	for _, name := range slices.Sorted(maps.Keys(wanted)) {
		id := string(resolve([]byte(name)))
		if other, taken := nameOf[id]; taken {
			problems = append(problems, fmt.Sprintf("  %s and %s both name %s", other, name, id))
			continue
		}
		nameOf[id] = name
	}
	elements := map[string]string{}
	for _, node := range m.Nodes {
		elements[node.ID] = "node"
	}
	for _, edge := range m.Edges {
		elements[edge.ID] = "edge"
	}

	findings := func(id string) string {
		var lines []string
		for _, f := range assessment.For(id) {
			lines = append(lines, fmt.Sprintf("\n      %s key=%q slot=%q about=%q evidence=%q coverage=%q",
				f.Code, f.Key, f.Slot, f.About, f.Evidence, f.Coverage))
		}
		if lines == nil {
			return " none"
		}
		return strings.Join(lines, "")
	}
	kinds := func(id string) []string {
		out := []string{}
		for _, kind := range assessment.Kinds(id) {
			out = append(out, string(kind))
		}
		return out
	}
	for _, id := range slices.Sorted(maps.Keys(elements)) {
		name, listed := nameOf[id]
		given := kinds(id)
		switch {
		case !listed:
			problems = append(problems, fmt.Sprintf("  the %s %s is in the model and marks does not list it; given %q\n    findings:%s",
				elements[id], id, given, findings(id)))
		case !sameSet(wanted[name], given):
			problems = append(problems, fmt.Sprintf("  the %s %s (%s): wanted %q, given %q\n    findings:%s",
				elements[id], name, id, slices.Sorted(slices.Values(wanted[name])), given, findings(id)))
		}
	}
	for _, id := range slices.Sorted(maps.Keys(nameOf)) {
		if _, found := elements[id]; !found {
			problems = append(problems, fmt.Sprintf("  marks lists %s (%s), which is no node or edge of the model", nameOf[id], id))
		}
	}
	return problems
}

type names map[int]*importer.Result

var issuedName = regexp.MustCompile(`@(\d+)/([A-Za-z0-9._/-]+)(?:#([A-Za-z0-9-]+))?`)

func (n names) resolve(t *testing.T) func([]byte) []byte {
	return func(text []byte) []byte {
		t.Helper()
		return issuedName.ReplaceAllFunc(text, func(name []byte) []byte {
			part := issuedName.FindSubmatch(name)
			step, _ := strconv.Atoi(string(part[1]))
			res, id := n[step], ""
			switch {
			case res == nil:
			case part[3] == nil && string(part[2]) == "supplements.yaml":
				id = supplementsAcquisition(res)
			case part[3] != nil:
				for _, m := range res.IDs {
					if m.Input == string(part[2]) && m.Candidate == string(part[3]) {
						id = m.ID
					}
				}
			}
			if id == "" {
				t.Fatalf("the name %s cannot be resolved: step %d gave it no ID", name, step)
			}
			return []byte(id)
		})
	}
}

// supplementsAcquisition is the acquisition ID of the evidence that the saved
// model holds for a statement of the supplements.
func supplementsAcquisition(res *importer.Result) string {
	if res.Model == nil {
		return ""
	}
	var evidence []model.Evidence
	for _, n := range res.Model.Nodes {
		evidence = append(evidence, n.Evidence...)
	}
	for _, e := range res.Model.Edges {
		evidence = append(evidence, e.Evidence...)
	}
	for _, m := range res.IDs {
		if at := slices.IndexFunc(evidence, func(ev model.Evidence) bool { return ev.ID == m.ID }); m.Input == "supplements.yaml" && at >= 0 {
			return evidence[at].AcquisitionID
		}
	}
	return ""
}

func mustRead(t *testing.T, path string) []byte {
	t.Helper()
	text, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return text
}

func copyFile(t *testing.T, from, to string, change func([]byte) []byte) {
	t.Helper()
	text := mustRead(t, from)
	if change != nil {
		text = change(text)
	}
	if err := os.WriteFile(to, text, 0o644); err != nil {
		t.Fatal(err)
	}
}

func listed(list diag.List) string {
	lines := make([]string, len(list))
	for i, d := range list {
		lines[i] = "  " + d.String()
	}
	return strings.Join(lines, "\n")
}

func sameSet(a, b []string) bool {
	a, b = slices.Sorted(slices.Values(a)), slices.Sorted(slices.Values(b))
	return slices.Equal(a, b)
}

// The wanted lines with the fewest * are paired first, so that a line that
// says less does not take the partner of one that says more.
func unmatched(wanted, given []string) (missing, extra []string) {
	slices.SortStableFunc(wanted, func(a, b string) int { return cmp.Compare(strings.Count(a, "*"), strings.Count(b, "*")) })
	extra = slices.Clone(given)
	for _, w := range wanted {
		fields := strings.Fields(w)
		at := slices.IndexFunc(extra, func(g string) bool {
			got := strings.Split(g, " ")
			for i, field := range fields {
				if field != "*" && field != got[i] {
					return false
				}
			}
			return true
		})
		if at < 0 {
			missing = append(missing, w)
			continue
		}
		extra = slices.Delete(extra, at, at+1)
	}
	return missing, extra
}

func sorted(t *testing.T, text []byte) any {
	t.Helper()
	dec := json.NewDecoder(bytes.NewReader(text))
	dec.UseNumber()
	var m map[string]any
	if err := dec.Decode(&m); err != nil {
		t.Fatalf("the model is not JSON: %v", err)
	}
	byID := func(list any) {
		items, _ := list.([]any)
		slices.SortStableFunc(items, func(a, b any) int {
			idA, _ := a.(map[string]any)["id"].(string)
			idB, _ := b.(map[string]any)["id"].(string)
			return cmp.Compare(idA, idB)
		})
	}
	for _, list := range []string{"nodes", "edges", "coverage"} {
		byID(m[list])
		items, _ := m[list].([]any)
		for _, item := range items {
			byID(item.(map[string]any)["evidence"])
		}
	}
	return m
}

func differences(got, want any) []string {
	var out []string
	var walk func(pointer string, got, want any)
	show := func(v any) string {
		text, _ := json.Marshal(v)
		if len(text) > 300 {
			return string(text[:300]) + "…"
		}
		return string(text)
	}
	walk = func(pointer string, got, want any) {
		g, isObject := got.(map[string]any)
		w, wantObject := want.(map[string]any)
		gl, isList := got.([]any)
		wl, wantList := want.([]any)
		switch {
		case isObject && wantObject:
			keys := map[string]bool{}
			for key := range g {
				keys[key] = true
			}
			for key := range w {
				keys[key] = true
			}
			for _, key := range slices.Sorted(func(yield func(string) bool) {
				for key := range keys {
					if !yield(key) {
						return
					}
				}
			}) {
				gv, has := g[key]
				wv, wanted := w[key]
				switch {
				case !has:
					out = append(out, fmt.Sprintf("  %s/%s is missing, want %s", pointer, key, show(wv)))
				case !wanted:
					out = append(out, fmt.Sprintf("  %s/%s is %s, and is not wanted", pointer, key, show(gv)))
				default:
					walk(pointer+"/"+key, gv, wv)
				}
			}
		case isList && wantList && len(gl) == len(wl):
			for i := range gl {
				walk(pointer+"/"+strconv.Itoa(i), gl[i], wl[i])
			}
		case !reflect.DeepEqual(got, want):
			out = append(out, fmt.Sprintf("  %s is %s, want %s", pointer, show(got), show(want)))
		}
	}
	walk("", got, want)
	return out
}
