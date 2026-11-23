package importer_test

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/ochamu/ponchimap/internal/diag"
	"github.com/ochamu/ponchimap/internal/importer"
	"github.com/ochamu/ponchimap/internal/model"
	"github.com/ochamu/ponchimap/internal/view"
)

// The expected values of these tests were worked out by hand from the rules
// of the package comment, as those of testdata/import were.

type object = map[string]any

func inWorkspace(t *testing.T) {
	t.Helper()
	t.Chdir(t.TempDir())
}

func write(t *testing.T, path string, v any) {
	t.Helper()
	text, isText := v.([]byte)
	if !isText {
		var err error
		if text, err = json.MarshalIndent(v, "", "  "); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, text, 0o644); err != nil {
		t.Fatal(err)
	}
}

func fromCase(t *testing.T, name, step string) {
	t.Helper()
	if err := os.RemoveAll("findings"); err != nil {
		t.Fatal(err)
	}
	if err := os.CopyFS("findings", os.DirFS(filepath.Join(casesFolder, name, step, "findings"))); err != nil {
		t.Fatal(err)
	}
}

func take(findings ...string) *importer.Result {
	req := request()
	req.Findings = findings
	return importer.Run(req)
}

func findings(id, checkedAt string, complete bool, nodes ...object) object {
	return object{"schema_version": "0.1", "nodes": append([]object{}, nodes...), "acquisition": object{
		"id": id, "source": "repository-scan", "route": "repository", "operation": "scan",
		"target": object{"repository": ".", "environment": "dev"}, "checked_at": checkedAt, "complete": complete}}
}

func from(source string, file object) object {
	file["acquisition"].(object)["source"] = source
	return file
}

func resource(id, service, nativeID string, evidence ...object) object {
	return object{"id": id, "kind": "resource", "provider": "cloudflare", "service": service,
		"identity": object{"account_id": "example-account-01", "native_id": nativeID}, "evidence": append([]object{}, evidence...)}
}

func evidence(id, kind string, pairs ...any) object {
	supports := []object{}
	for i := 0; i < len(pairs); i += 2 {
		supports = append(supports, object{"key": pairs[i], "value": pairs[i+1]})
	}
	return object{"id": id, "kind": kind, "supports": supports,
		"ref": object{"type": "file", "path": "wrangler.toml", "start_line": 1, "end_line": 2}, "checked_at": "2026-11-16T09:00:00Z"}
}

func codes(list diag.List) []string {
	out := []string{}
	for _, d := range list {
		out = append(out, d.Code)
	}
	return out
}

func idOf(t *testing.T, res *importer.Result, input, candidate string) string {
	t.Helper()
	for _, m := range res.IDs {
		if m.Input == input && m.Candidate == candidate {
			return m.ID
		}
	}
	t.Fatalf("the result has no mapping of %s#%s\n%s", input, candidate, listed(res.Diagnostics))
	return ""
}

func loadModel(t *testing.T) *model.Model {
	t.Helper()
	m, found := model.Parse("model.json", mustRead(t, "model.json"))
	if m == nil {
		t.Fatalf("the saved model cannot be read: %v", found)
	}
	return m
}

func node(t *testing.T, m *model.Model, id string) model.Node {
	t.Helper()
	at := slices.IndexFunc(m.Nodes, func(n model.Node) bool { return n.ID == id })
	if at < 0 {
		t.Fatalf("the model has no node %s", id)
	}
	return m.Nodes[at]
}

func evidenceIDs(items []model.Evidence) []string {
	out := []string{}
	for _, ev := range items {
		out = append(out, ev.ID)
	}
	return out
}

func folder(t *testing.T) []string {
	t.Helper()
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	names := []string{}
	for _, entry := range entries {
		names = append(names, entry.Name())
	}
	return names
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func TestWithoutAModelAndWithoutATargetNothingIsMade(t *testing.T) {
	inWorkspace(t)
	fromCase(t, "first-import", "1")
	before := folder(t)
	req := request()
	req.Target, req.Findings = nil, []string{"findings"}
	res := importer.Run(req)
	if res.Status != "error" || !slices.Equal(codes(res.Diagnostics), []string{"import.no_target"}) || res.Diagnostics[0].Location.Path != "model.json" {
		t.Errorf("status %q, diagnostics:\n%s\nwant an error import.no_target at model.json", res.Status, listed(res.Diagnostics))
	}
	if after := folder(t); !slices.Equal(after, before) {
		t.Errorf("the workspace holds %q, want what it held: %q", after, before)
	}
}

func TestAFolderWithoutFindingsIsAWarning(t *testing.T) {
	inWorkspace(t)
	if err := os.Mkdir("findings", 0o755); err != nil {
		t.Fatal(err)
	}
	write(t, "findings/notes.txt", []byte("not findings"))
	res := take("findings")
	if res.Status != "ok" || !slices.Equal(codes(res.Diagnostics), []string{"import.no_findings"}) ||
		res.Diagnostics[0].Severity != diag.Warning || res.Diagnostics[0].Location.Path != "findings" {
		t.Errorf("status %q, diagnostics:\n%s\nwant ok with the warning import.no_findings at findings", res.Status, listed(res.Diagnostics))
	}
}

func TestAFindingsPathThatDoesNotExistIsAnError(t *testing.T) {
	inWorkspace(t)
	res := take("missing.json")
	if res.Status != "error" || !slices.Equal(codes(res.Diagnostics), []string{"input.unreadable"}) || res.Diagnostics[0].Location.Path != "missing.json" {
		t.Errorf("status %q, diagnostics:\n%s\nwant an error input.unreadable at missing.json", res.Status, listed(res.Diagnostics))
	}
	if exists("model.json") {
		t.Error("a model was written")
	}
}

func TestTheTargetOfAnAcquisitionTakesNoKeyThatTheSchemaDoesNotDefine(t *testing.T) {
	const value = "kept-nowhere-41c7"
	for pointer, put := range map[string]func(target object){
		"/acquisition/target/session_token":  func(target object) { target["session_token"] = value },
		"/acquisition/target/filter/api_key": func(target object) { target["filter"] = object{"slot": "DB", "api_key": value} },
	} {
		t.Run(pointer, func(t *testing.T) {
			inWorkspace(t)
			file := findings("aq-scan-1", "2026-11-16T09:20:00Z", true,
				resource("api", "cloudflare.workers", "example-worker", evidence("c1", "code", "existence", true)))
			put(file["acquisition"].(object)["target"].(object))
			write(t, "findings/scan.json", file)
			before := folder(t)

			res := take("findings")
			if res.Status != "error" || !slices.Equal(codes(res.Diagnostics), []string{"schema.unknown_property"}) {
				t.Fatalf("status %q, diagnostics:\n%s\nwant an error schema.unknown_property alone", res.Status, listed(res.Diagnostics))
			}
			if at := res.Diagnostics[0].Location; at.Path != "findings/scan.json" || at.Pointer != pointer {
				t.Errorf("the diagnostic stands at %+v, want %s in findings/scan.json", at, pointer)
			}
			if shown, _ := json.Marshal(res); bytes.Contains(shown, []byte(value)) {
				t.Errorf("the result shows the value: %s", shown)
			}
			if after := folder(t); !slices.Equal(after, before) {
				t.Errorf("the workspace holds %q, want what it held: %q", after, before)
			}
		})
	}
}

func TestTheCheckOfTheModelDecidesWhetherItIsSaved(t *testing.T) {
	refusal := diag.Diagnostic{Severity: diag.Error, Code: "test.refused", Message: "the model is refused", Location: diag.Location{Path: "model.json"}}
	remark := diag.Diagnostic{Severity: diag.Warning, Code: "test.remark", Message: "the model has a flaw", Location: diag.Location{Path: "model.json"}}

	t.Run("an error", func(t *testing.T) {
		inWorkspace(t)
		fromCase(t, "first-import", "1")
		before := folder(t)
		req := request()
		req.Findings = []string{"findings"}
		var checked []byte
		req.Check = func(path string, text []byte) diag.List {
			if path != "model.json" {
				t.Errorf("the check was given the path %q, want model.json", path)
			}
			checked = text
			return diag.List{remark, refusal}
		}
		res := importer.Run(req)
		if res.Status != "error" || !slices.Equal(codes(res.Diagnostics), []string{"test.refused"}) || res.Model != nil {
			t.Errorf("status %q, diagnostics:\n%s\nwant an error with the refusal of the check alone, and no model", res.Status, listed(res.Diagnostics))
		}
		if m, found := model.Parse("model.json", checked); m == nil || len(m.Nodes) != 2 || len(m.Edges) != 1 {
			t.Errorf("the check was not given the model with its two nodes and one edge: %v", found)
		}
		if after := folder(t); !slices.Equal(after, before) {
			t.Errorf("the workspace holds %q, want what it held: %q", after, before)
		}
	})
	t.Run("only a warning", func(t *testing.T) {
		inWorkspace(t)
		fromCase(t, "first-import", "1")
		req := request()
		req.Findings = []string{"findings"}
		var checked []byte
		req.Check = func(_ string, text []byte) diag.List {
			checked = text
			return diag.List{remark}
		}
		res := importer.Run(req)
		if res.Status != "ok" || len(res.Diagnostics) != 0 {
			t.Errorf("status %q, diagnostics:\n%s\nwant ok and no diagnostic", res.Status, listed(res.Diagnostics))
		}
		if written := saved(t).model; written == nil || !bytes.Equal(written, checked) {
			t.Error("the saved model is not the text that the check was given")
		}
	})
}

func TestImportKeepsTheModelWhenItCannotStageTheRecords(t *testing.T) {
	inputs := t.TempDir()
	first, second := filepath.Join(inputs, "first.json"), filepath.Join(inputs, "second.json")
	copyFile(t, filepath.Join(casesFolder, "declared-value-changed/1/findings/scan.json"), first, nil)
	copyFile(t, filepath.Join(casesFolder, "declared-value-changed/2/findings/scan.json"), second, nil)
	inWorkspace(t)
	if res := take(first); res.Status != "ok" {
		t.Fatalf("the first import: %s", listed(res.Diagnostics))
	}
	before := saved(t)
	write(t, filepath.Join("acquisitions.json.next.tmp", "keep"), []byte("keep"))

	if res := take(first); res.Status != "ok" || len(res.Diagnostics) != 0 {
		t.Errorf("the same input again, which changes nothing, gives status %q:\n%s\nwant ok: a run that changes nothing writes nothing",
			res.Status, listed(res.Diagnostics))
	}
	res := take(second)
	if res.Status != "error" || !slices.Equal(codes(res.Diagnostics), []string{"import.write_failed"}) ||
		res.Diagnostics[0].Location.Path != "acquisitions.json.next" {
		t.Errorf("status %q, diagnostics:\n%s\nwant an error import.write_failed at acquisitions.json.next, the first file a run writes",
			res.Status, listed(res.Diagnostics))
	}
	if !saved(t).equal(before) {
		t.Error("the model or the records changed")
	}
}

func TestTheModelKeepsItsOrderAndNewEntriesFollowByID(t *testing.T) {
	inWorkspace(t)
	stated := func(id string) object {
		return object{"id": id, "kind": "stated", "supports": []object{{"key": "existence", "value": true}},
			"ref": object{"type": "document", "path": "docs/a.md"}, "checked_at": "2026-11-16T08:00:00Z"}
	}
	notTried := func(id, service string) object {
		return object{"id": id, "scope": object{"provider": "cloudflare", "service": service}, "status": "not_attempted", "checked_at": "2026-11-16T08:00:00Z"}
	}
	write(t, "model.json", object{"schema_version": "0.1", "target": object{"repository": ".", "environment": "dev"},
		"nodes": []object{
			{"id": "zeta", "kind": "resource", "provider": "cloudflare", "service": "cloudflare.workers",
				"identity": object{"account_id": "example-account-01", "native_id": "example-worker"},
				"evidence": []object{stated("ev-z"), stated("ev-a")}},
			{"id": "alpha", "kind": "service", "provider": "resend", "service": "resend", "evidence": []object{stated("ev-m")}},
		},
		"edges":    []object{},
		"coverage": []object{notTried("cv-z", "cloudflare.workers"), notTried("cv-a", "cloudflare.d1")},
	})
	file := findings("aq-scan-1", "2026-11-16T09:00:00Z", true,
		resource("worker", "cloudflare.workers", "example-worker",
			evidence("w1", "declared", "existence", true), evidence("w2", "declared", "region", "apac"), evidence("w3", "code", "existence", true)),
		resource("one", "cloudflare.d1", "database-1", evidence("d1", "declared", "existence", true)),
		resource("two", "cloudflare.d1", "database-2", evidence("d2", "declared", "existence", true)),
		resource("three", "cloudflare.d1", "database-3", evidence("d3", "declared", "existence", true)))
	file["coverage"] = []object{notTried("c1", "cloudflare.kv"), notTried("c2", "cloudflare.r2"), notTried("c3", "cloudflare.queues")}
	write(t, "findings/scan.json", file)
	if res := take("findings"); res.Status != "ok" {
		t.Fatalf("the import: %s", listed(res.Diagnostics))
	}

	follows := func(what string, got, had []string, added int) {
		t.Helper()
		if len(got) != len(had)+added || !slices.Equal(got[:len(had)], had) || !slices.IsSorted(got[len(had):]) {
			t.Errorf("%s stand in the order %q, want %q and then %d new ones in the order of their IDs", what, got, had, added)
		}
	}
	m := loadModel(t)
	var nodes, coverage []string
	for _, n := range m.Nodes {
		nodes = append(nodes, n.ID)
	}
	for _, c := range m.Coverage {
		coverage = append(coverage, c.ID)
	}
	follows("the nodes", nodes, []string{"zeta", "alpha"}, 3)
	follows("the evidence of the node zeta", evidenceIDs(m.Nodes[0].Evidence), []string{"ev-z", "ev-a"}, 3)
	follows("the coverage records", coverage, []string{"cv-z", "cv-a"}, 3)
}

// The IDs that the key of the importer gives the candidates of
// first-import. They were worked out apart from the importer, from the rule
// alone: HMAC-SHA256 under the key over the kind of ID and the values that
// IDs in the package comment lists, each as a length of four bytes and its
// text, and the first 40 bits written with 5 bits a character. Changing how
// IDs are made changes the IDs that users' models get from then on, and with
// them the entries of their view configs, so a change here is a change for
// every user.
func TestTheSameInputGivesTheseIDs(t *testing.T) {
	inWorkspace(t)
	fromCase(t, "first-import", "1")
	res := take("findings")
	for candidate, want := range map[string]string{
		"worker": "n-46vchgf0", "db": "n-mepr7fa9", "binding": "e-2an2qjz8",
		"w1": "ev-fq3cry6d", "d1": "ev-gn1aw9c2", "b1": "ev-16f1e2kb",
	} {
		if got := idOf(t, res, "findings/scan.json", candidate); got != want {
			t.Errorf("the candidate %s got the ID %s, want %s", candidate, got, want)
		}
	}
}

func TestAReadOfTheSamePlaceKeepsTheEvidenceIDAndAnotherValueGetsANewOne(t *testing.T) {
	inWorkspace(t)
	fromCase(t, "declared-value-changed", "1")
	first := take("findings")
	fromCase(t, "declared-value-changed", "2")
	second := take("findings")
	if a, b := idOf(t, first, "findings/scan.json", "w1"), idOf(t, second, "findings/scan.json", "w1"); a != b {
		t.Errorf("the evidence that says the same on other lines got the ID %s, want the ID it had, %s", b, a)
	}
	if a, b := idOf(t, first, "findings/scan.json", "d1"), idOf(t, second, "findings/scan.json", "d1"); a == b {
		t.Errorf("the evidence that states another region kept the ID %s, want a new one", a)
	}
}

func laterScan(id, checkedAt string) object {
	return findings(id, checkedAt, true,
		resource("worker", "cloudflare.workers", "example-worker", evidence("w1", "declared", "existence", true)),
		resource("queue-"+id, "cloudflare.queues", "queue-of-"+id, evidence("q1", "declared", "existence", true)))
}

func TestARunThatStoppedAfterItWroteTheModelLeftTheRecordsOfThatModel(t *testing.T) {
	inWorkspace(t)
	write(t, "first/scan.json", laterScan("aq-scan-1", "2026-11-16T09:00:00Z"))
	write(t, "second/scan.json", laterScan("aq-scan-2", "2026-11-16T10:00:00Z"))
	write(t, "third/scan.json", laterScan("aq-scan-3", "2026-11-16T11:00:00Z"))
	take("first")
	recordsBefore := saved(t).records
	if res := take("third"); res.Status != "ok" {
		t.Fatalf("the import that is stopped: %s", listed(res.Diagnostics))
	}
	stopped := saved(t)
	write(t, "acquisitions.json.next", stopped.records)
	write(t, "acquisitions.json", recordsBefore)

	res := take("second")
	if res.Status != "ok" || len(res.Diagnostics) != 0 {
		t.Fatalf("status %q, diagnostics:\n%s\nwant ok and no diagnostic", res.Status, listed(res.Diagnostics))
	}
	if got := idOf(t, res, "second/scan.json", "queue-aq-scan-2"); got != "" {
		t.Errorf("the read older than the newest complete one of the stopped run made the node %s, want nothing of it", got)
	}
	if after := saved(t); !after.equal(stopped) {
		t.Errorf("the model or the records are not those of the stopped run:\n%s", after.records)
	}
	if exists("acquisitions.json.next") {
		t.Error("acquisitions.json.next is still there")
	}
}

func TestARunThatStoppedBeforeItWroteTheModelLeftRecordsThatAreIgnored(t *testing.T) {
	inWorkspace(t)
	write(t, "first/scan.json", laterScan("aq-scan-1", "2026-11-16T09:00:00Z"))
	take("first")
	before := saved(t)

	var records object
	if err := json.Unmarshal(before.records, &records); err != nil {
		t.Fatal(err)
	}
	other := sha256.Sum256([]byte("a model that was never written"))
	records["model"] = "sha256:" + hex.EncodeToString(other[:])
	records["acquisitions"] = append(records["acquisitions"].([]any), object{
		"id": "aq-scan-9", "source": "repository-scan", "route": "repository", "operation": "scan",
		"checked_at": "2026-11-16T12:00:00Z", "complete": true, "input": "ninth/scan.json", "digest": "sha256:00", "reported": 0})
	write(t, "acquisitions.json.next", records)

	// Had the run read these records, the read of 09:00 would be older than
	// a complete one, and its evidence and its nodes would leave the model.
	res := take()
	if res.Status != "ok" || len(res.Diagnostics) != 0 {
		t.Fatalf("status %q, diagnostics:\n%s\nwant ok and no diagnostic", res.Status, listed(res.Diagnostics))
	}
	if after := saved(t); !after.equal(before) {
		t.Errorf("the model or the records changed:\n%s", after.records)
	}
	if exists("acquisitions.json.next") {
		t.Error("acquisitions.json.next is still there")
	}
}

func TestImportPreservesPermissionsWhileStagingAndPublishingRecords(t *testing.T) {
	const records = "acquisitions.json"
	inWorkspace(t)
	fromCase(t, "first-import", "1")
	if err := os.WriteFile(records, []byte(`{"schema_version":"0.1","acquisitions":[]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	permissions := func(path string) os.FileMode {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		return info.Mode().Perm()
	}
	want := permissions(records)
	write(t, ".model.json.tmp/keep", []byte("block the model write after staging"))
	if res := take("findings"); !slices.Contains(codes(res.Diagnostics), "import.write_failed") {
		t.Fatal(listed(res.Diagnostics))
	}
	staged := mustRead(t, records+".next")
	if got := permissions(records + ".next"); got != want {
		t.Errorf("staged records permissions = %v, want %v", got, want)
	}
	if err := os.RemoveAll(".model.json.tmp"); err != nil {
		t.Fatal(err)
	}
	if res := take("findings"); res.Status != "ok" {
		t.Fatal(listed(res.Diagnostics))
	}
	if got := permissions(records); got != want || !bytes.Equal(mustRead(t, records), staged) {
		t.Errorf("published records have mode %v, want %v and the staged content", got, want)
	}
}

func TestARunThatEndsInAnErrorLeavesThePendingRecordsAsTheyAre(t *testing.T) {
	inWorkspace(t)
	write(t, "first/scan.json", laterScan("aq-scan-1", "2026-11-16T09:00:00Z"))
	take("first")
	before := saved(t)
	pending := []byte(`{"schema_version": "0.1", "model": "sha256:00", "acquisitions": []}`)
	write(t, "acquisitions.json.next", pending)
	write(t, "broken/scan.json", object{"schema_version": "0.1"})

	if res := take("broken"); res.Status != "error" {
		t.Fatalf("status %q, want an error for the findings without an acquisition", res.Status)
	}
	if after := mustRead(t, "acquisitions.json.next"); !bytes.Equal(after, pending) {
		t.Errorf("acquisitions.json.next changed to %s", after)
	}
	if !saved(t).equal(before) {
		t.Error("the model or the records changed")
	}
}

func TestRecordsThatCannotBeReadStopTheImport(t *testing.T) {
	inWorkspace(t)
	fromCase(t, "first-import", "1")
	take("findings")
	before := saved(t).model
	write(t, "acquisitions.json", []byte("not what the file holds\n"))
	fromCase(t, "declared-value-changed", "2")
	res := take("findings")
	if res.Status != "error" || !slices.Equal(codes(res.Diagnostics), []string{"import.records_unreadable"}) || res.Diagnostics[0].Location.Path != "acquisitions.json" {
		t.Errorf("status %q, diagnostics:\n%s\nwant an error import.records_unreadable at acquisitions.json", res.Status, listed(res.Diagnostics))
	}
	if !bytes.Equal(saved(t).model, before) {
		t.Error("the model changed")
	}
}

func TestAStatementDoesNotKeepANodeInARunWithoutTheSupplements(t *testing.T) {
	inWorkspace(t)
	fromCase(t, "statement-does-not-keep-a-node", "1")
	first := take("findings")
	worker, db, binding := idOf(t, first, "findings/scan.json", "worker"), idOf(t, first, "findings/scan.json", "db"), idOf(t, first, "findings/scan.json", "binding")
	write(t, "supplements.yaml", []byte("schema_version: \"0.1\"\nstatements:\n  - target_id: "+db+
		"\n    supports:\n      - key: role\n        value: keeps the orders\n    source: the user\n    checked_at: \"2026-11-16T10:00:00Z\"\n"))
	req := request()
	req.Supplements = "supplements.yaml"
	if res := importer.Run(req); res.Status != "ok" || len(node(t, loadModel(t), db).Evidence) != 2 {
		t.Fatalf("the statement was not taken in: %s", listed(res.Diagnostics))
	}

	fromCase(t, "statement-does-not-keep-a-node", "3")
	res := take("findings")
	if !sameSet(res.Removed, []string{db, binding}) || !slices.Equal(codes(res.Diagnostics), []string{"import.removed", "import.removed"}) {
		t.Errorf("removed %q, diagnostics:\n%s\nwant the database and its binding removed, with a warning each", res.Removed, listed(res.Diagnostics))
	}
	if m := loadModel(t); len(m.Nodes) != 1 || m.Nodes[0].ID != worker || len(m.Edges) != 0 {
		t.Errorf("the model holds %d nodes and %d edges, want the worker alone", len(m.Nodes), len(m.Edges))
	}
}

func connectsTo(target, node string) []byte {
	return []byte("schema_version: \"0.1\"\nstatements:\n  - target_id: " + target +
		"\n    supports:\n      - key: connection.target\n        value: " + node +
		"\n    source: the user\n    checked_at: \"2026-11-16T10:00:00Z\"\n")
}

func TestAStatementAboutAConnectionToANodeThatTheModelLacksIsLeftOut(t *testing.T) {
	inWorkspace(t)
	fromCase(t, "first-import", "1")
	first := take("findings")
	worker := idOf(t, first, "findings/scan.json", "worker")
	write(t, "supplements.yaml", connectsTo(worker, "n-nothing"))
	req := request()
	req.Supplements = "supplements.yaml"
	res := importer.Run(req)
	if res.Status != "ok" || !slices.Equal(codes(res.Diagnostics), []string{"import.unknown_target"}) ||
		res.Diagnostics[0].Location.Path != "supplements.yaml" || res.Diagnostics[0].Location.Pointer != "/statements/0/supports/0/value" ||
		res.Diagnostics[0].TargetID != "n-nothing" {
		t.Errorf("status %q, diagnostics:\n%s\nwant ok with the warning import.unknown_target at the value of the statement, about n-nothing",
			res.Status, listed(res.Diagnostics))
	}
	if got := idOf(t, res, "supplements.yaml", "0"); got != "" || len(node(t, loadModel(t), worker).Evidence) != 1 {
		t.Errorf("the statement became the evidence %q, want it left out", got)
	}
}

func TestAStatementAboutAConnectionGoesWhenItsTargetLeaves(t *testing.T) {
	inWorkspace(t)
	fromCase(t, "binding-removed", "1")
	first := take("findings")
	worker, db := idOf(t, first, "findings/scan.json", "worker"), idOf(t, first, "findings/scan.json", "db")
	write(t, "supplements.yaml", connectsTo(worker, db))
	req := request()
	req.Supplements = "supplements.yaml"
	if res := importer.Run(req); res.Status != "ok" || len(node(t, loadModel(t), worker).Evidence) != 2 {
		t.Fatalf("the statement was not taken in: %s", listed(res.Diagnostics))
	}

	// The statement keeps no element (R1 to R3), so the database leaves,
	// and evidence of the model cannot name a node that the model lacks.
	fromCase(t, "binding-removed", "2")
	res := take("findings")
	if res.Status != "ok" || !slices.Contains(res.Removed, db) {
		t.Fatalf("status %q, removed %q, want ok and the database removed\n%s", res.Status, res.Removed, listed(res.Diagnostics))
	}
	if items := node(t, loadModel(t), worker).Evidence; len(items) != 1 || items[0].Kind != model.EvDeclared {
		t.Errorf("the worker has the evidence %q, want its declaration alone", evidenceIDs(items))
	}
}

func TestAnAttributeThatTheEvidenceComesToDisputeIsTakenOut(t *testing.T) {
	inWorkspace(t)
	write(t, "scan/scan.json", findings("aq-scan-1", "2026-11-16T09:00:00Z", true,
		resource("db", "cloudflare.d1", "example-database-id", evidence("d1", "declared", "existence", true, "region", "apac"))))
	first := take("scan")
	db := idOf(t, first, "scan/scan.json", "db")
	if region := node(t, loadModel(t), db).Attributes["region"]; region != "apac" {
		t.Fatalf("attributes.region is %v after the scan, want apac", region)
	}
	write(t, "doc/doc.json", from("docs-architecture", findings("aq-doc-1", "2026-11-16T09:30:00Z", true,
		resource("database", "cloudflare.d1", "example-database-id", evidence("s1", "stated", "region", "weur")))))
	res := take("doc")
	if !slices.Equal(codes(res.Diagnostics), []string{"import.attribute_disputed"}) || res.Diagnostics[0].TargetID != db {
		t.Errorf("diagnostics:\n%s\nwant the warning import.attribute_disputed about %s", listed(res.Diagnostics), db)
	}
	if region, has := node(t, loadModel(t), db).Attributes["region"]; has {
		t.Errorf("attributes.region is still %v, want it left out while the evidence gives two values", region)
	}
}

func TestDisputedAttributeHintsDescribeTheFigureAndReport(t *testing.T) {
	for key, words := range map[string][]string{
		"role":   {"report", "supplements.yaml", "without valid_time"},
		"region": {"report", "mismatch"},
	} {
		inWorkspace(t)
		write(t, "scan.json", findings("aq-scan", "2026-11-16T09:00:00Z", true,
			resource("db", "cloudflare.d1", "example-db",
				evidence("a", "declared", "existence", true, key, "first"), evidence("b", "stated", key, "second"))))
		found := take("scan.json").Diagnostics
		if !slices.Equal(codes(found), []string{"import.attribute_disputed"}) {
			t.Fatalf("disputed %s: %s", key, listed(found))
		}
		for _, word := range words {
			if !strings.Contains(found[0].Hint, word) {
				t.Errorf("hint for %s omits %q: %s", key, word, found[0].Hint)
			}
		}
	}
}

// worded is a read of a source that gives the worker a role and its binding
// to the database a purpose.
func worded(source, id, checkedAt string, complete bool, role, purpose string) object {
	worker := resource("worker", "cloudflare.workers", "example-worker", evidence("w1", "declared", "existence", true))
	worker["attributes"] = object{"role": role}
	file := from(source, findings(id, checkedAt, complete, worker,
		resource("db", "cloudflare.d1", "example-database-id", evidence("d1", "declared", "existence", true))))
	file["edges"] = []object{{"id": "binding", "from": "worker", "to": "db", "relation": "configuration", "slot": "DB",
		"attributes": object{"purpose": purpose},
		"evidence":   []object{evidence("b1", "declared", "connection.configured", true, "connection.target", "db")}}}
	return file
}

// texts are what the user reads of the worker and its binding: the role and
// the purpose in model.json, and the texts that the scene of the figure
// draws for them.
type texts struct{ role, purpose, drawnRole, drawnPurpose string }

func shown(t *testing.T, worker, binding string) texts {
	t.Helper()
	m := loadModel(t)
	var got texts
	got.role, _ = node(t, m, worker).Attributes["role"].(string)
	for _, e := range m.Edges {
		if e.ID == binding {
			got.purpose, _ = e.Attributes["purpose"].(string)
		}
	}
	s, err := view.Build(m, &view.Config{})
	if err != nil {
		t.Fatalf("the scene cannot be built: %v", err)
	}
	for _, n := range s.Nodes {
		if n.ID == worker {
			got.drawnRole = n.Role
		}
	}
	for _, e := range s.Edges {
		if slices.Contains(e.SourceIDs, binding) {
			got.drawnPurpose = e.Label
		}
	}
	return got
}

// drawn is what the user reads when the model holds role and purpose. No
// read checks the binding against the real environment, so its line says
// that it is unverified.
func drawn(role, purpose string) texts {
	return texts{role, purpose, role, purpose + "（未照合）"}
}

// A source read again with new wording replaces what its older read wrote,
// and the model and the figure show the new wording under the same IDs (A2).
func TestAReadAgainOfASourceReplacesTheRoleAndThePurposeThatItWrote(t *testing.T) {
	inWorkspace(t)
	write(t, "docs/docs.json", worded("docs", "aq-docs-1", "2026-11-16T09:00:00Z", true, "serves the API", "keeps the orders"))
	first := take("docs")
	worker, binding := idOf(t, first, "docs/docs.json", "worker"), idOf(t, first, "docs/docs.json", "binding")
	if got := shown(t, worker, binding); got != drawn("serves the API", "keeps the orders") {
		t.Fatalf("after the first read the user sees %+v", got)
	}

	write(t, "docs/docs.json", worded("docs", "aq-docs-2", "2026-11-16T10:00:00Z", true, "handles the uploads", "stores the uploads"))
	res := take("docs")
	if res.Status != "ok" || len(res.Diagnostics) != 0 || len(res.Added) != 0 || len(res.Removed) != 0 {
		t.Errorf("status %s, added %q, removed %q, diagnostics:\n%s\nwant ok and nothing else", res.Status, res.Added, res.Removed, listed(res.Diagnostics))
	}
	if idOf(t, res, "docs/docs.json", "worker") != worker || idOf(t, res, "docs/docs.json", "binding") != binding {
		t.Errorf("the read again was taken as other elements than %s and %s", worker, binding)
	}
	if got, want := shown(t, worker, binding), drawn("handles the uploads", "stores the uploads"); got != want {
		t.Errorf("after the read again the user sees %+v, want %+v", got, want)
	}
}

// Two reads of one source that come in one run give what they give in two
// runs: the newer wording.
func TestTheNewerOfTwoReadsOfASourceInOneRunGivesTheWording(t *testing.T) {
	inWorkspace(t)
	write(t, "in/old.json", worded("docs", "aq-docs-1", "2026-11-16T09:00:00Z", false, "serves the API", "keeps the orders"))
	write(t, "in/new.json", worded("docs", "aq-docs-2", "2026-11-16T10:00:00Z", false, "handles the uploads", "stores the uploads"))
	res := take("in")
	worker, binding := idOf(t, res, "in/new.json", "worker"), idOf(t, res, "in/new.json", "binding")
	if got, want := shown(t, worker, binding), drawn("handles the uploads", "stores the uploads"); got != want {
		t.Errorf("the user sees %+v, want %+v", got, want)
	}
}

// What one source wrote stays when another source words it otherwise, and
// when that source reads again. The source that wrote it can replace it.
func TestAnotherSourceDoesNotReplaceTheRoleAndThePurposeThatASourceWrote(t *testing.T) {
	inWorkspace(t)
	write(t, "repo/repo.json", worded("repo", "aq-repo-1", "2026-11-16T09:00:00Z", true, "serves the API", "keeps the orders"))
	first := take("repo")
	worker, binding := idOf(t, first, "repo/repo.json", "worker"), idOf(t, first, "repo/repo.json", "binding")
	for i, checkedAt := range []string{"2026-11-16T09:30:00Z", "2026-11-16T10:00:00Z"} {
		write(t, "docs/docs.json", worded("docs", "aq-docs-"+strconv.Itoa(i+1), checkedAt, true, "the public API", "order storage"))
		take("docs")
		if got, want := shown(t, worker, binding), drawn("serves the API", "keeps the orders"); got != want {
			t.Errorf("after read %d of the documents the user sees %+v, want %+v", i+1, got, want)
		}
	}

	write(t, "repo/repo.json", worded("repo", "aq-repo-2", "2026-11-16T10:30:00Z", true, "handles the uploads", "stores the uploads"))
	take("repo")
	if got, want := shown(t, worker, binding), drawn("handles the uploads", "stores the uploads"); got != want {
		t.Errorf("after the repository is read again the user sees %+v, want %+v", got, want)
	}
}

// The role that the user states stays when a source reads again (A3). The
// purpose, which the user did not state, follows the source.
func TestTheRoleThatTheUserStatesStaysWhenASourceReadsAgain(t *testing.T) {
	inWorkspace(t)
	write(t, "docs/docs.json", worded("docs", "aq-docs-1", "2026-11-16T09:00:00Z", true, "serves the API", "keeps the orders"))
	first := take("docs")
	worker, binding := idOf(t, first, "docs/docs.json", "worker"), idOf(t, first, "docs/docs.json", "binding")
	write(t, "supplements.yaml", []byte("schema_version: \"0.1\"\nstatements:\n  - target_id: "+worker+
		"\n    supports:\n      - key: role\n        value: the user's words\n    source: the user\n    checked_at: \"2026-11-16T09:30:00Z\"\n"))
	req := request()
	req.Supplements = "supplements.yaml"
	if res := importer.Run(req); res.Status != "ok" {
		t.Fatalf("the supplements: %s", listed(res.Diagnostics))
	}

	write(t, "docs/docs.json", worded("docs", "aq-docs-2", "2026-11-16T10:00:00Z", true, "handles the uploads", "stores the uploads"))
	take("docs")
	if got, want := shown(t, worker, binding), drawn("the user's words", "stores the uploads"); got != want {
		t.Errorf("the user sees %+v, want %+v", got, want)
	}
}

func TestEvidenceWrittenByHandStaysWhenAnOldReadCarriesItsAcquisitionID(t *testing.T) {
	inWorkspace(t)
	write(t, "new/scan.json", laterScan("aq-scan-2", "2026-11-16T10:00:00Z"))
	worker := idOf(t, take("new"), "new/scan.json", "worker")

	var m object
	if err := json.Unmarshal(saved(t).model, &m); err != nil {
		t.Fatal(err)
	}
	for _, n := range m["nodes"].([]any) {
		if n := n.(object); n["id"] == worker {
			n["evidence"] = append(n["evidence"].([]any), object{"id": "ev-by-hand", "kind": "stated",
				"supports": []object{{"key": "role", "value": "the entry point"}}, "ref": object{"type": "document", "path": "docs/a.md"},
				"checked_at": "2026-11-16T08:00:00Z", "acquisition_id": "aq-scan-0"})
		}
	}
	write(t, "model.json", m)

	// The read of 08:00 is older than the complete read of 10:00, so nothing
	// of it is taken in or recorded, and what carries its ID in the model was
	// not put there by import.
	write(t, "old/scan.json", laterScan("aq-scan-0", "2026-11-16T08:00:00Z"))
	for range 2 {
		res := take("old")
		if res.Status != "ok" || idOf(t, res, "old/scan.json", "queue-aq-scan-0") != "" {
			t.Fatalf("the old read: status %q\n%s", res.Status, listed(res.Diagnostics))
		}
		if ids := evidenceIDs(node(t, loadModel(t), worker).Evidence); !slices.Contains(ids, "ev-by-hand") {
			t.Fatalf("the worker has the evidence %q, want the item written by hand among them", ids)
		}
	}
}

func TestTheOlderOfTwoReadsComesFirstWhateverTheOrderOfTheFiles(t *testing.T) {
	read := func(id, source, checkedAt, zone, team string) object {
		n := resource("api", "cloudflare.workers", "example-worker", evidence("e1", "declared", "existence", true))
		n["identity"].(object)["zone"] = zone
		n["attributes"] = object{"team": team}
		return from(source, findings(id, checkedAt, true, n))
	}
	for _, order := range [][]string{{"in/a.json", "in/b.json"}, {"in/b.json", "in/a.json"}} {
		inWorkspace(t)
		write(t, "in/a.json", read("aq-b", "probe", "2026-11-16T10:00:00Z", "west", "platform"))
		write(t, "in/b.json", read("aq-a", "repository-scan", "2026-11-16T09:00:00Z", "east", "product"))
		res := take(order...)
		if !slices.Equal(codes(res.Diagnostics), []string{"import.identity_conflict"}) ||
			res.Diagnostics[0].Location.Path != "in/a.json" || res.Diagnostics[0].Location.Pointer != "/nodes/0" ||
			res.Diagnostics[0].TargetID != idOf(t, res, "in/a.json", "api") {
			t.Errorf("files given as %q: diagnostics:\n%s\nwant the warning import.identity_conflict at the candidate of the newer read, in/a.json, about the new node",
				order, listed(res.Diagnostics))
		}
		n := node(t, loadModel(t), idOf(t, res, "in/a.json", "api"))
		if n.Identity["zone"] != "east" || n.Attributes["team"] != "product" {
			t.Errorf("files given as %q: identity.zone is %v and attributes.team is %v, want east and product, the values of the older read",
				order, n.Identity["zone"], n.Attributes["team"])
		}
	}
}

func TestAnUnfinishedReadThatStatesPartOfTheKeysLeavesTheOlderEvidence(t *testing.T) {
	inWorkspace(t)
	write(t, "first/probe.json", from("probe", findings("aq-probe-1", "2026-11-16T09:00:00Z", true,
		resource("db", "cloudflare.d1", "example-database-id", evidence("p1", "deployed", "existence", true, "region", "apac")))))
	write(t, "second/probe.json", from("probe", findings("aq-probe-2", "2026-11-16T10:00:00Z", false,
		resource("db", "cloudflare.d1", "example-database-id", evidence("p1", "deployed", "existence", true)))))
	first := take("first")
	second := take("second")
	want := []string{idOf(t, first, "first/probe.json", "p1"), idOf(t, second, "second/probe.json", "p1")}
	if got := evidenceIDs(node(t, loadModel(t), idOf(t, first, "first/probe.json", "db")).Evidence); !sameSet(got, want) || want[0] == want[1] {
		t.Errorf("the database has the evidence %q, want the older item, whose region the newer read does not state, and the newer one: %q", got, want)
	}
}

func TestCoverageLosesTheElementsThatLeftAndGoesWhenNoneIsLeft(t *testing.T) {
	inWorkspace(t)
	fromCase(t, "binding-removed", "1")
	first := take("findings")
	worker, db, binding := idOf(t, first, "findings/scan.json", "worker"), idOf(t, first, "findings/scan.json", "db"), idOf(t, first, "findings/scan.json", "binding")
	var m object
	if err := json.Unmarshal(saved(t).model, &m); err != nil {
		t.Fatal(err)
	}
	byHand := func(id string, targets ...string) object {
		return object{"id": id, "scope": object{"provider": "cloudflare", "target_ids": targets}, "status": "not_attempted", "checked_at": "2026-11-16T08:00:00Z"}
	}
	m["coverage"] = []object{byHand("cv-both", worker, db), byHand("cv-db", db), byHand("cv-binding", binding), byHand("cv-worker", worker)}
	write(t, "model.json", m)

	fromCase(t, "binding-removed", "2")
	if res := take("findings"); !sameSet(res.Removed, []string{db, binding}) {
		t.Fatalf("removed %q, want the database and the binding\n%s", res.Removed, listed(res.Diagnostics))
	}
	got := map[string][]string{}
	for _, c := range loadModel(t).Coverage {
		got[c.ID] = c.Scope.TargetIDs()
	}
	if len(got) != 2 || !slices.Equal(got["cv-both"], []string{worker}) || !slices.Equal(got["cv-worker"], []string{worker}) {
		t.Errorf("the coverage records and their target_ids are %q, want cv-both and cv-worker, each with the worker alone", got)
	}
}

func TestCoverageNamesAnElementOnceWhenTwoOfItsCandidatesAreThatElement(t *testing.T) {
	inWorkspace(t)
	file := from("probe", findings("aq-probe-1", "2026-11-16T09:00:00Z", true,
		database("unowned", object{"native_id": "database-a"}),
		database("other", object{"native_id": "database-b", "account_id": "example-account-01"}),
		database("owned", object{"native_id": "database-a", "account_id": "example-account-01"})))
	file["coverage"] = []object{{"id": "c1", "status": "not_attempted", "checked_at": "2026-11-16T09:00:00Z",
		"scope": object{"provider": "cloudflare", "target_ids": []string{"unowned", "other", "owned", "other"}}}}
	write(t, "in/probe.json", file)
	res := take("in")
	one, other := idOf(t, res, "in/probe.json", "unowned"), idOf(t, res, "in/probe.json", "other")
	if owned := idOf(t, res, "in/probe.json", "owned"); owned != one || one == other {
		t.Fatalf("the two descriptions of one database became %s and %s, and the other database %s; want one node for the first two\n%s",
			one, owned, other, listed(res.Diagnostics))
	}
	// N2 makes the first and the third candidate one node, which the list
	// names once, where the first of them stood.
	if m := loadModel(t); len(m.Coverage) != 1 || !slices.Equal(m.Coverage[0].Scope.TargetIDs(), []string{one, other}) {
		t.Errorf("the coverage records are %+v, want one record whose target_ids are %q", m.Coverage, []string{one, other})
	}
}

func TestCoverageThatDidNotFinishMakesTheStatusPartial(t *testing.T) {
	for status, want := range map[string]importer.Status{"denied": "partial", "partial": "partial", "unsupported": "partial", "failed": "partial",
		"not_attempted": "ok", "complete": "ok"} {
		inWorkspace(t)
		file := findings("aq-scan-1", "2026-11-16T09:00:00Z", true,
			resource("db", "cloudflare.d1", "example-database-id", evidence("d1", "declared", "existence", true)))
		file["coverage"] = []object{{"id": "c1", "scope": object{"provider": "cloudflare"}, "status": status, "checked_at": "2026-11-16T09:00:00Z"}}
		write(t, "findings/scan.json", file)
		if res := take("findings"); res.Status != want || len(res.Diagnostics) != 0 {
			t.Errorf("a complete read with coverage of the status %s gives the status %q, want %q\n%s", status, res.Status, want, listed(res.Diagnostics))
		}
	}
}

func database(id string, identity object) object {
	return object{"id": id, "kind": "resource", "provider": "cloudflare", "service": "cloudflare.d1", "identity": identity,
		"evidence": []object{evidence("e-"+id, "declared", "existence", true)}}
}

// recordedNodes is what acquisitions.json says of the nodes, by acquisition.
func recordedNodes(t *testing.T) map[string][]object {
	t.Helper()
	var file struct {
		Acquisitions []struct {
			ID    string   `json:"id"`
			Nodes []object `json:"nodes"`
		} `json:"acquisitions"`
	}
	raw, err := os.ReadFile("acquisitions.json")
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(raw, &file); err != nil {
		t.Fatal(err)
	}
	out := map[string][]object{}
	for _, a := range file.Acquisitions {
		out[a.ID] = a.Nodes
	}
	return out
}

// said is a database of the environment dev as the records write it.
func said(node string, identity object) object {
	out := object{"node": node, "kind": "resource", "provider": "cloudflare", "service": "cloudflare.d1", "environment": "dev"}
	maps.Copy(out, identity)
	return out
}

// The model keeps one identity of a node. What each read said of the node,
// and which node it was taken as, stands in the records for as long as the
// read stays in the model.
func TestTheRecordsKeepWhatEachReadSaysOfItsNodes(t *testing.T) {
	inWorkspace(t)
	const account, native = "example-account-01", "database-a"
	same := func(t *testing.T, want map[string][]object) {
		t.Helper()
		a, _ := json.Marshal(recordedNodes(t))
		b, _ := json.Marshal(want)
		if !bytes.Equal(a, b) {
			t.Errorf("the records say of the nodes\n  %s\nwant\n  %s", a, b)
		}
	}

	write(t, "env/env.json", from("env-scan", findings("aq-env-1", "2026-11-16T09:00:00Z", true,
		database("db", object{"logical_name": "orders"}))))
	orders := idOf(t, take("env"), "env/env.json", "db")
	write(t, "probe/probe.json", from("probe", findings("aq-probe-1", "2026-11-16T09:10:00Z", true,
		database("db", object{"native_id": native, "account_id": account, "logical_name": "orders"}))))
	if id := idOf(t, take("probe"), "probe/probe.json", "db"); id != orders {
		t.Fatalf("the probed database became %s, want the node of its name, %s", id, orders)
	}
	same(t, map[string][]object{
		"aq-env-1":   {said(orders, object{"logical_name": "orders"})},
		"aq-probe-1": {said(orders, object{"native_id": native, "account_id": account, "logical_name": "orders"})},
	})

	// A newer read of the first source states another database. The record of
	// the read that it replaces goes, and with it what that read said.
	write(t, "env/env.json", from("env-scan", findings("aq-env-2", "2026-11-16T15:00:00Z", true,
		database("db", object{"logical_name": "invoices"}))))
	invoices := idOf(t, take("env"), "env/env.json", "db")
	same(t, map[string][]object{
		"aq-env-2":   {said(invoices, object{"logical_name": "invoices"})},
		"aq-probe-1": {said(orders, object{"native_id": native, "account_id": account, "logical_name": "orders"})},
	})

	// The probe finds nothing any more, so its node leaves the model, and the
	// records say nothing of a node that left.
	write(t, "probe/probe.json", from("probe", findings("aq-probe-2", "2026-11-16T16:00:00Z", true)))
	if res := take("probe"); !slices.Equal(res.Removed, []string{orders}) {
		t.Fatalf("removed %q, want the probed database alone\n%s", res.Removed, listed(res.Diagnostics))
	}
	same(t, map[string][]object{
		"aq-env-2":   {said(invoices, object{"logical_name": "invoices"})},
		"aq-probe-2": nil,
	})
}

func TestTheRecordsSayNothingOfANodeThatLeft(t *testing.T) {
	inWorkspace(t)
	write(t, "scan/scan.json", findings("aq-scan-1", "2026-11-16T09:00:00Z", true, database("db", object{"native_id": "database-a"})))
	db := idOf(t, take("scan"), "scan/scan.json", "db")
	// Notes name the database and bring no evidence of it.
	named := database("db", object{"native_id": "database-a"})
	named["evidence"] = []object{}
	write(t, "notes/notes.json", from("notes", findings("aq-notes-1", "2026-11-16T09:10:00Z", true, named)))
	if id := idOf(t, take("notes"), "notes/notes.json", "db"); id != db {
		t.Fatalf("the notes name %s, want the scanned database %s", id, db)
	}
	if got := recordedNodes(t)["aq-notes-1"]; len(got) != 1 || got[0]["node"] != db {
		t.Fatalf("the record of the notes says %v, want the database %s", got, db)
	}

	write(t, "scan/scan.json", findings("aq-scan-2", "2026-11-16T10:00:00Z", true))
	if res := take("scan"); !slices.Equal(res.Removed, []string{db}) {
		t.Fatalf("removed %q, want the database alone\n%s", res.Removed, listed(res.Diagnostics))
	}
	if got := recordedNodes(t); len(got["aq-notes-1"]) != 0 {
		t.Errorf("the record of the notes still says %v of a node that left", got["aq-notes-1"])
	}
}

func TestTheRecordOfAReadThatWasReplacedSaysNothingOfItsNodes(t *testing.T) {
	inWorkspace(t)
	// The newer scan replaces the first one. The record of the first one
	// stays, because a node keeps evidence of it (R3), and says nothing of the
	// nodes any more: the read is not what states them now.
	fromCase(t, "named-node-keeps-its-evidence", "1")
	take("findings")
	if got := recordedNodes(t)["aq-scan-1"]; len(got) != 2 {
		t.Fatalf("the record of the first scan says %v, want its two nodes", got)
	}
	fromCase(t, "named-node-keeps-its-evidence", "2")
	take("findings")
	got := recordedNodes(t)
	if nodes, kept := got["aq-scan-1"]; !kept || len(nodes) != 0 {
		t.Errorf("the record of the first scan is kept: %t, and says %v of the nodes; want it kept and silent", kept, nodes)
	}
	if len(got["aq-scan-2"]) != 1 || len(got["aq-code-1"]) == 0 {
		t.Errorf("the records of the reads that stay say %v and %v, want what each read states", got["aq-scan-2"], got["aq-code-1"])
	}
}

// Two nodes of the model can come to be the same by the rules, and import
// does not merge them (N5). A name that is read again then stays with the one
// that it was taken as.
func TestANameReadAgainStaysWithItsNodeAmongNodesThatAreTheSame(t *testing.T) {
	inWorkspace(t)
	const account = "example-account-01"
	// The scan knows the native ID and no name, so the two names below fit
	// the node of the account alone.
	write(t, "scan/scan.json", findings("aq-scan-1", "2026-11-16T09:00:00Z", true, database("db", object{"native_id": "database-a"})))
	scanned := idOf(t, take("scan"), "scan/scan.json", "db")
	write(t, "tf/tf.json", from("tf-scan", findings("aq-tf-1", "2026-11-16T09:10:00Z", true,
		database("db", object{"account_id": account, "logical_name": "orders"}))))
	owned := idOf(t, take("tf"), "tf/tf.json", "db")
	env := func(id, at string) object {
		return from("env-scan", findings(id, at, true, database("db", object{"logical_name": "orders"})))
	}
	write(t, "env/env.json", env("aq-env-1", "2026-11-16T09:20:00Z"))
	if id := idOf(t, take("env"), "env/env.json", "db"); id != owned || owned == scanned {
		t.Fatalf("the name became %s; the node of the account is %s and the scanned one %s; want the node of the account, apart from the scanned one", id, owned, scanned)
	}
	// The probe states the native ID with the account and the name: by the
	// rules the scanned node and the node of the account are the same now.
	write(t, "probe/probe.json", from("probe", findings("aq-probe-1", "2026-11-16T09:30:00Z", true,
		database("db", object{"native_id": "database-a", "account_id": account, "logical_name": "orders"}))))
	if res := take("probe"); idOf(t, res, "probe/probe.json", "db") != scanned || !slices.Equal(codes(res.Diagnostics), []string{"import.duplicate_identity"}) {
		t.Fatalf("the probed database became %s, want the scanned node %s and the warning import.duplicate_identity alone\n%s",
			idOf(t, res, "probe/probe.json", "db"), scanned, listed(res.Diagnostics))
	}

	write(t, "env/env.json", env("aq-env-2", "2026-11-16T15:00:00Z"))
	res := take("env")
	if id := idOf(t, res, "env/env.json", "db"); id != owned || len(res.Added)+len(res.Removed) != 0 {
		t.Errorf("read again, the name became %s, added %q, removed %q; want the node %s as before, and nothing added or removed\n%s",
			id, res.Added, res.Removed, owned, listed(res.Diagnostics))
	}
}

// Evidence written by hand states a node as much as an acquisition does: the
// identity of such a node is all that says where it is known.
func TestANodeWrittenByHandIsStatedByItsIdentity(t *testing.T) {
	inWorkspace(t)
	byHand := object{"id": "ev-by-hand", "kind": "declared", "supports": []object{{"key": "existence", "value": true}},
		"ref": object{"type": "file", "path": "wrangler.toml"}, "checked_at": "2026-11-16T08:00:00Z"}
	write(t, "model.json", object{"schema_version": "0.1", "target": object{"repository": ".", "environment": "dev"},
		"nodes": []object{{"id": "n-by-hand", "kind": "resource", "provider": "cloudflare", "service": "cloudflare.d1",
			"identity": object{"native_id": "database-a", "logical_name": "orders"}, "evidence": []object{byHand}}},
		"edges": []object{}, "coverage": []object{}})
	write(t, "in/probe.json", from("probe", findings("aq-probe-1", "2026-11-16T09:00:00Z", true,
		database("probed", object{"native_id": "database-b", "account_id": "example-account-01", "logical_name": "orders"}))))
	write(t, "in/doc.json", from("docs-architecture", findings("aq-doc-1", "2026-11-16T09:05:00Z", true,
		database("named", object{"logical_name": "orders"}))))
	res := take("in")
	named, probed := idOf(t, res, "in/doc.json", "named"), idOf(t, res, "in/probe.json", "probed")
	if named == "n-by-hand" || named == probed || !slices.Equal(codes(res.Diagnostics), []string{"import.ambiguous_identity"}) {
		t.Errorf("the name became %s (the probed database is %s), want a node of its own and the warning import.ambiguous_identity alone\n%s",
			named, probed, listed(res.Diagnostics))
	}
	takeAgain(t, "in", "import.ambiguous_identity")
}

// R5 is judged once it is known which nodes leave. Here no acquisition states
// either database any more, so both are known by their identity alone and
// the name fits two. One of them leaves in this run, and then the name fits
// the other.
func TestACandidateWithoutEvidenceIsJudgedOnceTheNodesThatLeaveAreKnown(t *testing.T) {
	inWorkspace(t)
	const account = "example-account-01"
	write(t, "probe/probe.json", from("probe", findings("aq-probe-1", "2026-11-16T09:00:00Z", true,
		database("db", object{"native_id": "database-a", "account_id": account, "logical_name": "orders"}))))
	write(t, "probe/tf.json", from("tf-scan", findings("aq-tf-1", "2026-11-16T09:01:00Z", true,
		database("db", object{"account_id": account, "logical_name": "orders"}))))
	first := take("probe")
	stays := idOf(t, first, "probe/probe.json", "db")
	if id := idOf(t, first, "probe/tf.json", "db"); id != stays {
		t.Fatalf("the name with the account became %s, want the probed database %s", id, stays)
	}
	// The probe finds nothing any more: the first database stays on the
	// evidence of the name that joined it.
	os.Remove("probe/tf.json")
	write(t, "probe/probe.json", from("probe", findings("aq-probe-2", "2026-11-16T09:30:00Z", true)))
	if res := take("probe"); len(res.Removed) != 0 {
		t.Fatalf("removed %q, want nothing\n%s", res.Removed, listed(res.Diagnostics))
	}
	write(t, "scan/scan.json", findings("aq-scan-1", "2026-11-16T10:00:00Z", true,
		database("db", object{"native_id": "database-b", "logical_name": "orders"})))
	leaves := idOf(t, take("scan"), "scan/scan.json", "db")
	if leaves == stays {
		t.Fatalf("the scanned database became %s, want a node of its own", leaves)
	}

	write(t, "scan/scan.json", findings("aq-scan-2", "2026-11-16T11:00:00Z", true))
	named := database("named", object{"logical_name": "orders"})
	named["evidence"] = []object{}
	write(t, "scan/notes.json", from("notes", findings("aq-notes-1", "2026-11-16T11:05:00Z", true, named)))
	res := take("scan")
	if id := idOf(t, res, "scan/notes.json", "named"); res.Status != "ok" || id != stays || !slices.Equal(res.Removed, []string{leaves}) ||
		!slices.Equal(codes(res.Diagnostics), []string{"import.removed"}) {
		t.Errorf("status %q, the name became %q, removed %q, want ok, %s, and %s removed with that warning alone\n%s",
			res.Status, id, res.Removed, stays, leaves, listed(res.Diagnostics))
	}
	takeAgain(t, "scan")
}

func TestADescriptionWithoutAnOwnerJoinsOnlyANodeOfItsEnvironment(t *testing.T) {
	inWorkspace(t)
	write(t, "probe/probe.json", from("probe", findings("aq-probe-1", "2026-11-16T09:00:00Z", true,
		database("db", object{"native_id": "database-a", "account_id": "example-account-01"}))))
	known := idOf(t, take("probe"), "probe/probe.json", "db")

	// N2 joins within one environment: the same native ID without an owner
	// in another environment may be another account's resource.
	elsewhere := findings("aq-scan-1", "2026-11-16T10:00:00Z", true, database("db", object{"native_id": "database-a"}))
	elsewhere["acquisition"].(object)["target"].(object)["environment"] = "prod"
	write(t, "scan/scan.json", elsewhere)
	res := take("scan")
	added := idOf(t, res, "scan/scan.json", "db")
	if added == known || !slices.Equal(res.Added, []string{added}) || len(res.Diagnostics) != 0 {
		t.Fatalf("the description of prod became %s, added %q, want a new node and no diagnostic\n%s", added, res.Added, listed(res.Diagnostics))
	}
	if n := node(t, loadModel(t), added); n.Environment != "prod" || n.Identity["account_id"] != nil {
		t.Errorf("the new node has the environment %q and the account %v, want prod and no account", n.Environment, n.Identity["account_id"])
	}
}

func TestANameJoinsTheNodeWhoseOwnerItDoesNotContradict(t *testing.T) {
	inWorkspace(t)
	write(t, "probe/probe.json", from("probe", findings("aq-probe-1", "2026-11-16T09:00:00Z", true,
		database("db", object{"native_id": "database-a", "account_id": "example-account-01", "logical_name": "orders"}))))
	known := idOf(t, take("probe"), "probe/probe.json", "db")

	write(t, "doc/doc.json", from("docs-architecture", findings("aq-doc-1", "2026-11-16T10:00:00Z", true,
		database("ours", object{"logical_name": "orders", "account_id": "example-account-01"}),
		database("theirs", object{"logical_name": "orders", "account_id": "example-account-02"}))))
	res := take("doc")
	ours, theirs := idOf(t, res, "doc/doc.json", "ours"), idOf(t, res, "doc/doc.json", "theirs")
	if ours != known || theirs == known || !slices.Equal(res.Added, []string{theirs}) || len(res.Diagnostics) != 0 {
		t.Errorf("the name in the same account became %s and the name in another account %s, added %q;\nwant the first to be %s, the second a new node, and no diagnostic\n%s",
			ours, theirs, res.Added, known, listed(res.Diagnostics))
	}
}

func TestANameFindsTheNodeThatADescriptionWithoutAnOwnerJoined(t *testing.T) {
	inWorkspace(t)
	// N2 makes the scan's description and the probe's one node, so the name
	// that both carry names one node, not two.
	write(t, "in/probe.json", from("probe", findings("aq-probe-1", "2026-11-16T09:00:00Z", true,
		database("p", object{"native_id": "database-a", "account_id": "example-account-01", "logical_name": "orders"}))))
	write(t, "in/scan.json", findings("aq-scan-1", "2026-11-16T09:10:00Z", true,
		database("s", object{"native_id": "database-a", "logical_name": "orders"})))
	write(t, "in/doc.json", from("docs-architecture", findings("aq-doc-1", "2026-11-16T09:20:00Z", true,
		database("d", object{"logical_name": "orders"}))))
	res := take("in")
	p, s, d := idOf(t, res, "in/probe.json", "p"), idOf(t, res, "in/scan.json", "s"), idOf(t, res, "in/doc.json", "d")
	if p != s || p != d || !slices.Equal(res.Added, []string{p}) || len(res.Diagnostics) != 0 {
		t.Errorf("the three descriptions became %s, %s, and %s, added %q, want one new node and no diagnostic\n%s", p, s, d, res.Added, listed(res.Diagnostics))
	}
}

func takeAgain(t *testing.T, findings string, warnings ...string) {
	t.Helper()
	before := saved(t)
	res := take(findings)
	if !saved(t).equal(before) || !slices.Equal(codes(res.Diagnostics), warnings) {
		t.Errorf("the same input again changed the model or the records, or gave other diagnostics than %q:\n%s\n%s", warnings,
			strings.Join(differences(sorted(t, saved(t).model), sorted(t, before.model)), "\n"), listed(res.Diagnostics))
	}
}

func TestANameJoinsTheNodeThatStaysWhenTheOtherNodeOfTheNameLeaves(t *testing.T) {
	inWorkspace(t)
	orders := func(id, nativeID, account string) object {
		return database(id, object{"native_id": nativeID, "account_id": account, "logical_name": "orders"})
	}
	write(t, "one/probe.json", from("probe", findings("aq-probe-1", "2026-11-16T09:00:00Z", true,
		orders("gone", "database-a", "example-account-01"), orders("stays", "database-b", "example-account-02"))))
	first := take("one")
	gone, stays := idOf(t, first, "one/probe.json", "gone"), idOf(t, first, "one/probe.json", "stays")

	// The newer probe no longer finds the first database, so it leaves in the
	// run that brings the name, and N3 does not count it: the name fits one
	// node, not two.
	write(t, "two/probe.json", from("probe", findings("aq-probe-2", "2026-11-16T10:00:00Z", true,
		orders("stays", "database-b", "example-account-02"))))
	write(t, "two/doc.json", from("docs-architecture", findings("aq-doc-1", "2026-11-16T10:05:00Z", true,
		database("named", object{"logical_name": "orders"}))))
	res := take("two")
	if named := idOf(t, res, "two/doc.json", "named"); named != stays || !slices.Equal(res.Removed, []string{gone}) || len(res.Added) != 0 ||
		!slices.Equal(codes(res.Diagnostics), []string{"import.removed"}) {
		t.Errorf("the name became %s, added %q, removed %q, want it to be %s, the node that stays, with %s removed and that warning alone\n%s",
			named, res.Added, res.Removed, stays, gone, listed(res.Diagnostics))
	}
	takeAgain(t, "two")
}

func besideAProbedNode(t *testing.T, first object) {
	t.Helper()
	byHand := object{"id": "ev-by-hand", "kind": "declared", "supports": []object{{"key": "existence", "value": true}},
		"ref": object{"type": "file", "path": "wrangler.toml"}, "checked_at": "2026-11-16T08:00:00Z"}
	probed := object{"id": "ev-probed", "kind": "deployed", "supports": []object{{"key": "existence", "value": true}},
		"ref": object{"type": "query", "path": "findings/probe.json"}, "checked_at": "2026-11-16T09:00:00Z", "acquisition_id": "aq-probe-1"}
	d1 := func(id string, identity object, item object) object {
		return object{"id": id, "kind": "resource", "provider": "cloudflare", "service": "cloudflare.d1", "identity": identity, "evidence": []object{item}}
	}
	write(t, "model.json", object{"schema_version": "0.1", "target": object{"repository": ".", "environment": "dev"},
		"nodes": []object{
			d1("n-first", first, byHand),
			d1("n-probed", object{"native_id": "database-a", "account_id": "example-account-01", "logical_name": "orders"}, probed),
		},
		"edges": []object{}, "coverage": []object{}})
	write(t, "acquisitions.json", object{"schema_version": "0.1", "acquisitions": []object{{
		"id": "aq-probe-1", "source": "probe", "route": "cli", "operation": "probe",
		"checked_at": "2026-11-16T09:00:00Z", "complete": true, "input": "findings/probe.json", "digest": "sha256:00", "reported": 0}}})
	write(t, "in/probe.json", from("probe", findings("aq-probe-2", "2026-11-16T10:00:00Z", true)))
	write(t, "in/doc.json", from("docs-architecture", findings("aq-doc-1", "2026-11-16T10:05:00Z", true,
		database("named", object{"logical_name": "orders"}))))
}

func TestANodeThatLeavesDoesNotTieTheOtherNodesTogether(t *testing.T) {
	inWorkspace(t)
	// Only the probed node ties the name to the first node: it has the key of
	// the first node (N1) and the name of the candidate (N3). While it takes
	// part, the name goes to the first of the two (N5), so the probed node
	// gets nothing, and the newer probe finds nothing: it leaves. A node that
	// leaves takes no part, so the name makes a node of its own, and the first
	// node gets nothing of it.
	besideAProbedNode(t, object{"native_id": "database-a", "account_id": "example-account-01"})
	res := take("in")
	named := idOf(t, res, "in/doc.json", "named")
	if named == "n-first" || !slices.Equal(res.Added, []string{named}) || !slices.Equal(res.Removed, []string{"n-probed"}) ||
		!slices.Equal(codes(res.Diagnostics), []string{"import.removed"}) {
		t.Fatalf("the name became %s, added %q, removed %q, want a new node, n-probed removed, and that warning alone\n%s",
			named, res.Added, res.Removed, listed(res.Diagnostics))
	}
	if n := node(t, loadModel(t), "n-first"); len(n.Identity) != 2 || len(n.Evidence) != 1 {
		t.Errorf("the first node has the identity %v and %d items of evidence, want what it had: its native ID, its owner, and one item", n.Identity, len(n.Evidence))
	}
	takeAgain(t, "in")
}

func TestANameGoesToTheNodeThatItFitsAmongNodesThatAreTheSame(t *testing.T) {
	inWorkspace(t)
	// The first node has no owner, so N2 makes it the same as the probed node,
	// and N3 makes the name the same as the probed node, which has it. No node
	// has the key of the name, so the name goes to the node that N3 joined it
	// to (N5), not to the first of the two. Its evidence keeps the probed node
	// in the model when the newer probe finds nothing.
	besideAProbedNode(t, object{"native_id": "database-a"})
	duplicate := []string{"import.duplicate_identity"}
	res := take("in")
	if named := idOf(t, res, "in/doc.json", "named"); named != "n-probed" || len(res.Added) != 0 || len(res.Removed) != 0 ||
		!slices.Equal(codes(res.Diagnostics), duplicate) || res.Diagnostics[0].TargetID != "n-probed" {
		t.Fatalf("the name became %s, added %q, removed %q, want n-probed, nothing added or removed, and the warning %s about n-probed alone\n%s",
			named, res.Added, res.Removed, duplicate[0], listed(res.Diagnostics))
	}
	m := loadModel(t)
	if n := node(t, m, "n-first"); len(n.Identity) != 1 || len(n.Evidence) != 1 {
		t.Errorf("the first node has the identity %v and %d items of evidence, want what it had: its native ID and one item", n.Identity, len(n.Evidence))
	}
	if items := node(t, m, "n-probed").Evidence; len(items) != 1 || items[0].AcquisitionID != "aq-doc-1" {
		t.Errorf("the probed node has the evidence %v, want the one item of the document", items)
	}
	takeAgain(t, "in", duplicate...)
}

func TestACandidateThatTwoNodesOfTheModelAreJoinedToGoesToTheFirstOfThem(t *testing.T) {
	inWorkspace(t)
	byHand := func(id string, identity object) object {
		return object{"id": id, "kind": "resource", "provider": "cloudflare", "service": "cloudflare.d1", "identity": identity,
			"evidence": []object{{"id": "ev-" + id, "kind": "declared", "supports": []object{{"key": "existence", "value": true}},
				"ref": object{"type": "file", "path": "wrangler.toml"}, "checked_at": "2026-11-16T08:00:00Z"}}}
	}
	write(t, "model.json", object{"schema_version": "0.1", "target": object{"repository": ".", "environment": "dev"},
		"nodes": []object{byHand("n-unowned", object{"native_id": "database-a"}), byHand("n-named", object{"logical_name": "orders"})},
		"edges": []object{}, "coverage": []object{}})

	// The probe gives the native ID an owner and the name, so N2 makes the
	// first node the same as its candidate and N3 the second. No node has the
	// key of the candidate, and the candidate was joined to none: the two
	// nodes were joined to it. It goes to the first of them in the model.
	write(t, "in/probe.json", from("probe", findings("aq-probe-1", "2026-11-16T09:00:00Z", true,
		database("db", object{"native_id": "database-a", "account_id": "example-account-01", "logical_name": "orders"}))))
	duplicate := []string{"import.duplicate_identity"}
	res := take("in")
	if db := idOf(t, res, "in/probe.json", "db"); db != "n-unowned" || len(res.Added) != 0 || len(res.Removed) != 0 ||
		!slices.Equal(codes(res.Diagnostics), duplicate) || res.Diagnostics[0].TargetID != "n-named" {
		t.Fatalf("the candidate became %s, added %q, removed %q, want n-unowned, nothing added or removed, and the warning %s about n-named alone\n%s",
			db, res.Added, res.Removed, duplicate[0], listed(res.Diagnostics))
	}
	m := loadModel(t)
	if n := node(t, m, "n-unowned"); len(n.Identity) != 3 || len(n.Evidence) != 2 {
		t.Errorf("the first node has the identity %v and %d items of evidence, want the three keys of the candidate and two items", n.Identity, len(n.Evidence))
	}
	if n := node(t, m, "n-named"); len(n.Identity) != 1 || len(n.Evidence) != 1 {
		t.Errorf("the second node has the identity %v and %d items of evidence, want what it had: its name and one item", n.Identity, len(n.Evidence))
	}
	takeAgain(t, "in", duplicate...)
}

func twoOwners(t *testing.T) (gone, stays string) {
	t.Helper()
	owned := func(id, account string) object {
		return database(id, object{"native_id": "database-a", "account_id": account})
	}
	write(t, "one/probe.json", from("probe", findings("aq-probe-1", "2026-11-16T09:00:00Z", true,
		owned("gone", "example-account-01"), owned("stays", "example-account-02"))))
	first := take("one")
	write(t, "two/probe.json", from("probe", findings("aq-probe-2", "2026-11-16T10:00:00Z", true, owned("stays", "example-account-02"))))
	return idOf(t, first, "one/probe.json", "gone"), idOf(t, first, "one/probe.json", "stays")
}

func TestACandidateWithoutEvidenceNamesTheNodeThatStays(t *testing.T) {
	inWorkspace(t)
	gone, stays := twoOwners(t)
	// While both nodes are counted, the description without an owner fits
	// two, joins neither, and would be a new node without evidence (R5). The
	// first node leaves in this run, so the description names the second.
	unowned := database("named", object{"native_id": "database-a"})
	unowned["evidence"] = []object{}
	write(t, "two/scan.json", findings("aq-scan-1", "2026-11-16T10:05:00Z", true, unowned))
	res := take("two")
	if named := idOf(t, res, "two/scan.json", "named"); res.Status != "ok" || named != stays || !slices.Equal(res.Removed, []string{gone}) ||
		!slices.Equal(codes(res.Diagnostics), []string{"import.removed"}) {
		t.Errorf("status %q, the description became %q, removed %q, want ok, %s, and %s removed with that warning alone\n%s",
			res.Status, named, res.Removed, stays, gone, listed(res.Diagnostics))
	}
	takeAgain(t, "two")
}

func TestARunThatEndsForACandidateWithoutEvidenceReportsNothingElse(t *testing.T) {
	inWorkspace(t)
	twoOwners(t)
	before := saved(t)
	// The newer probe would take a node out of the model, and the name fits
	// no node. The run saves nothing, so no node left the model.
	unknown := database("named", object{"logical_name": "orders"})
	unknown["evidence"] = []object{}
	write(t, "two/doc.json", from("docs-architecture", findings("aq-doc-1", "2026-11-16T10:05:00Z", true, unknown)))
	res := take("two")
	if res.Status != "error" || !slices.Equal(codes(res.Diagnostics), []string{"import.no_evidence"}) || !saved(t).equal(before) {
		t.Errorf("status %q, diagnostics:\n%s\nwant an error import.no_evidence alone, and the files as they were", res.Status, listed(res.Diagnostics))
	}
}

func TestWhatAKeptElementNamesIsKeptTooWithTheCoverageOfItsRead(t *testing.T) {
	inWorkspace(t)
	service := func(id, provider string, items ...object) object {
		return object{"id": id, "kind": "service", "provider": provider, "service": provider, "evidence": append([]object{}, items...)}
	}
	scan := findings("aq-scan-1", "2026-11-16T09:00:00Z", true,
		resource("worker", "cloudflare.workers", "example-worker", evidence("w1", "declared", "existence", true)),
		service("mail", "resend", evidence("h1", "hint", "existence", true, "connection.target", "relay")),
		service("relay", "relay", evidence("h2", "hint", "existence", true)))
	scan["coverage"] = []object{
		{"id": "c-done", "scope": object{"provider": "resend"}, "status": "complete", "checked_at": "2026-11-16T09:00:00Z"},
		{"id": "c-open", "scope": object{"provider": "relay"}, "status": "not_attempted", "checked_at": "2026-11-16T09:00:00Z"}}
	write(t, "in/scan.json", scan)
	code := from("code-reading", findings("aq-code-1", "2026-11-16T09:20:00Z", true,
		resource("api", "cloudflare.workers", "example-worker"), service("resend", "resend")))
	code["edges"] = []object{{"id": "send", "from": "api", "to": "resend", "relation": "implementation",
		"evidence": []object{evidence("c1", "code", "call.implemented", true)}}}
	write(t, "in/code.json", code)
	first := take("in")
	mail, relay := idOf(t, first, "in/scan.json", "mail"), idOf(t, first, "in/scan.json", "relay")
	done := idOf(t, first, "in/scan.json", "c-done")

	// The newer scan states neither service. The edge of the code reading
	// names the mail service (R3), and the evidence that the mail service
	// keeps names the relay. The older read keeps its complete coverage
	// while evidence of it stays (M3).
	write(t, "next/scan.json", findings("aq-scan-2", "2026-11-16T11:00:00Z", true,
		resource("worker", "cloudflare.workers", "example-worker", evidence("w1", "declared", "existence", true))))
	res := take("next")
	var about []string
	for _, d := range res.Diagnostics {
		about = append(about, d.Code+" "+d.TargetID)
	}
	if !sameSet(about, []string{"import.kept_superseded " + mail, "import.kept_superseded " + relay}) || len(res.Removed) != 0 {
		t.Errorf("diagnostics %q, removed %q, want a warning import.kept_superseded about each of the two services and nothing removed", about, res.Removed)
	}
	m := loadModel(t)
	if got := evidenceIDs(node(t, m, relay).Evidence); !slices.Equal(got, []string{idOf(t, first, "in/scan.json", "h2")}) {
		t.Errorf("the relay has the evidence %q, want the item it had", got)
	}
	var coverage []string
	for _, c := range m.Coverage {
		coverage = append(coverage, c.ID)
	}
	if !slices.Equal(coverage, []string{done}) {
		t.Errorf("the coverage records are %q, want the complete record of the older read alone, %s", coverage, done)
	}
}

func TestAnAttributeStaysWhenTheReadThatStatesItsValueComesAgain(t *testing.T) {
	inWorkspace(t)
	dated := evidence("d1", "declared", "region", "weur")
	dated["valid_time"] = "2026-01-01T00:00:00Z"
	write(t, "in/scan.json", findings("aq-scan-1", "2026-11-16T09:00:00Z", true,
		resource("db", "cloudflare.d1", "example-database-id", evidence("d0", "declared", "existence", true), dated)))
	db := idOf(t, take("in"), "in/scan.json", "db")
	if region, has := node(t, loadModel(t), db).Attributes["region"]; has {
		t.Fatalf("attributes.region is %v, want none: only evidence about another time states a region", region)
	}

	var m object
	if err := json.Unmarshal(saved(t).model, &m); err != nil {
		t.Fatal(err)
	}
	m["nodes"].([]any)[0].(object)["attributes"] = object{"region": "weur"}
	write(t, "model.json", m)

	// The same read again takes no evidence away, so the value that a person
	// wrote does not go with any (A1).
	if res := take("in"); res.Status != "ok" {
		t.Fatalf("the same read again: %s", listed(res.Diagnostics))
	}
	if region := node(t, loadModel(t), db).Attributes["region"]; region != "weur" {
		t.Errorf("attributes.region is %v after the same read again, want the value that was written by hand", region)
	}
}

func TestTwoItemsThatSayTheSameKeepTheirIDsWhenTheReadComesAgain(t *testing.T) {
	inWorkspace(t)
	// The worker has two items that say the same at other lines, and the
	// file two coverage records of one scope, so only a number added to the
	// ID tells each pair apart. Under idKey the second ID of each pair sorts
	// before the first.
	at := func(id string, line int) object {
		item := evidence(id, "declared", "existence", true)
		item["ref"].(object)["start_line"], item["ref"].(object)["end_line"] = line, line+1
		return item
	}
	file := findings("aq-scan-1", "2026-11-16T09:00:00Z", true,
		resource("worker", "cloudflare.workers", "example-worker", at("a", 3), at("b", 9)))
	file["coverage"] = []object{
		{"id": "c-a", "scope": object{"provider": "cloudflare"}, "status": "complete", "checked_at": "2026-11-16T09:00:00Z"},
		{"id": "c-b", "scope": object{"provider": "cloudflare"}, "status": "not_attempted", "checked_at": "2026-11-16T09:00:00Z"}}
	write(t, "in/scan.json", file)
	first := take("in")
	before := saved(t)
	second := take("in")
	for _, candidate := range []string{"a", "b", "c-a", "c-b"} {
		if was, is := idOf(t, first, "in/scan.json", candidate), idOf(t, second, "in/scan.json", candidate); was != is {
			t.Errorf("the candidate %s had the ID %s and has %s after the same read again", candidate, was, is)
		}
	}
	if !saved(t).equal(before) {
		t.Error("the same read again changed the model or the records")
	}
}

func TestEvidenceThatIsReadAgainKeepsItsPlace(t *testing.T) {
	inWorkspace(t)
	scan := func(id, checkedAt string, line int) object {
		item := evidence("d1", "declared", "existence", true)
		item["ref"].(object)["start_line"], item["ref"].(object)["end_line"] = line, line+1
		return findings(id, checkedAt, true, resource("db", "cloudflare.d1", "example-database-id", item))
	}
	write(t, "first/scan.json", scan("aq-scan-1", "2026-11-16T09:00:00Z", 1))
	write(t, "probe/probe.json", from("probe", findings("aq-probe-1", "2026-11-16T09:30:00Z", true,
		resource("db", "cloudflare.d1", "example-database-id", evidence("p1", "deployed", "existence", true)))))
	write(t, "second/scan.json", scan("aq-scan-2", "2026-11-16T10:00:00Z", 7))
	declared := idOf(t, take("first"), "first/scan.json", "d1")
	deployed := idOf(t, take("probe"), "probe/probe.json", "p1")
	res := take("second")

	// The declaration read again at other lines is the item that the node
	// had, under its ID, so it keeps its place before the item that came
	// after it (IDs).
	db := node(t, loadModel(t), idOf(t, res, "second/scan.json", "db"))
	if got := evidenceIDs(db.Evidence); !slices.Equal(got, []string{declared, deployed}) || db.Evidence[0].Ref.StartLine != 7 {
		t.Errorf("the evidence stands as %q with the first item at line %d, want %q with the declaration of line 7 first",
			got, db.Evidence[0].Ref.StartLine, []string{declared, deployed})
	}
}
