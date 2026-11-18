package cli

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/ochamu/ponchimap/internal/assets"
)

// namedFindings is one read of the repository: a Lambda that the findings
// name and give a kind, and a mail API that they leave as its key.
func namedFindings(acquisition, name string) string {
	return fmt.Sprintf(`{"schema_version": "0.1",
 "acquisition": {"id": %q, "source": "repo", "route": "repository", "operation": "read serverless.yml",
  "target": {"repository": ".", "environment": "production"}, "checked_at": "2026-11-17T09:00:00Z", "complete": true},
 "nodes": [
  {"id": "thumbs", "kind": "resource", "provider": "aws", "service": "aws.lambda", "identity": {"native_id": "make-thumbnails"},
   "attributes": {"role": "サムネイルを作る", "service_name": %q, "category": "compute"},
   "evidence": [{"id": "thumbs-declared", "kind": "declared", "supports": [{"key": "existence", "value": true}],
    "ref": {"type": "file", "path": "serverless.yml", "start_line": 1, "end_line": 9}, "checked_at": "2026-11-17T09:00:00Z"}]},
  {"id": "mail", "kind": "service", "provider": "acme", "service": "acme.mail", "attributes": {"role": "通知を送る"},
   "evidence": [{"id": "mail-code", "kind": "code", "supports": [{"key": "call.implemented", "value": true}],
    "ref": {"type": "file", "path": "src/notify.ts", "start_line": 3, "end_line": 8}, "checked_at": "2026-11-17T09:00:00Z"}]}],
 "edges": [
  {"id": "thumbs-mails", "from": "thumbs", "to": "mail", "relation": "implementation", "attributes": {"purpose": "完了を知らせる"},
   "evidence": [{"id": "thumbs-mails-code", "kind": "code", "supports": [{"key": "call.implemented", "value": true}],
    "ref": {"type": "file", "path": "src/notify.ts", "start_line": 3, "end_line": 8}, "checked_at": "2026-11-17T09:00:00Z"}]}]}`, acquisition, name)
}

func iconCount(svg, icon string) int {
	return strings.Count(svg, base64.StdEncoding.EncodeToString(assets.Bundled().Icon(icon)))
}

// The name and the kind a findings file gives a node reach the model, the
// card, and the report, and a later read of the same source changes them.
// A node without them is drawn with its key and the generic icon.
func TestTheNameAndKindOfTheFindingsReachTheFigureAndTheReport(t *testing.T) {
	t.Chdir(t.TempDir())
	if a := run("init"); a.code != 0 {
		t.Fatalf("init ends with %d:\n%s", a.code, a.stderr)
	}
	for _, read := range []struct{ acquisition, name string }{
		{"repo-20261117-090000", "AWS Lambda"},
		{"repo-20261118-090000", "Amazon Lambda"},
	} {
		writeFile(t, ".ponchimap/findings/repo.json", namedFindings(read.acquisition, read.name))
		if a := run("import", "--environment", "production"); a.code != 0 {
			t.Fatalf("import of %s ends with %d:\n%s", read.acquisition, a.code, a.stderr)
		}
		if a := run("render"); a.code != 0 {
			t.Fatalf("render of %s ends with %d:\n%s", read.acquisition, a.code, a.stderr)
		}

		var m struct {
			Nodes []struct {
				ID, Service string
				Attributes  map[string]any
			}
		}
		if err := json.Unmarshal([]byte(readFile(t, defaultModel)), &m); err != nil {
			t.Fatal(err)
		}
		ids := map[string]string{}
		for _, n := range m.Nodes {
			ids[n.Service] = n.ID
			if n.Service == "aws.lambda" && (n.Attributes["service_name"] != read.name || n.Attributes["category"] != "compute") {
				t.Errorf("%s: the model keeps %v for the Lambda, want service_name %q and category compute", read.acquisition, n.Attributes, read.name)
			}
		}

		svg := readFile(t, "docs/architecture/overview.svg")
		if !strings.Contains(svg, ">"+read.name+"<") || strings.Contains(svg, "aws.lambda") || !strings.Contains(svg, ">acme.mail<") {
			t.Errorf("%s: the cards are not titled %q and acme.mail, or the figure shows the key aws.lambda", read.acquisition, read.name)
		}
		if c, g := iconCount(svg, "compute"), iconCount(svg, assets.GenericIcon); c != 1 || g != 1 {
			t.Errorf("%s: the figure draws %d compute and %d generic icons, want one of each", read.acquisition, c, g)
		}

		report := readFile(t, ".ponchimap/report.md")
		lambda, mail := ids["aws.lambda"], ids["acme.mail"]
		for _, want := range []string{
			"### " + lambda + " " + read.name + "\n",
			"### " + mail + " acme.mail\n",
			fmt.Sprintf("[%s](#%s) %s", lambda, lambda, read.name),
			fmt.Sprintf("- 属性: `category = \"compute\"`、`service_name = %q`\n", read.name),
		} {
			if !strings.Contains(report, want) {
				t.Errorf("%s: the report does not say %q", read.acquisition, want)
			}
		}
	}
}
