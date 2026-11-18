package model_test

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/ochamu/ponchimap/internal/model"
)

func asReadBack(m *model.Model) *model.Model {
	if len(m.Target.Scope) == 0 {
		m.Target.Scope = nil
	}
	for i := range m.Nodes {
		if len(m.Nodes[i].Identity) == 0 {
			m.Nodes[i].Identity = nil
		}
		if len(m.Nodes[i].Attributes) == 0 {
			m.Nodes[i].Attributes = nil
		}
	}
	for i := range m.Edges {
		if len(m.Edges[i].Attributes) == 0 {
			m.Edges[i].Attributes = nil
		}
	}
	return m
}

func TestASampleModelWrittenInTheStableFormReadsBackTheSame(t *testing.T) {
	paths, _ := filepath.Glob("../../testdata/samples/*/model.json")
	if len(paths) == 0 {
		t.Fatal("found no models in testdata/samples")
	}
	paths = append(paths, "../../testdata/small-models/worker-and-d1.json", "../../testdata/small-models/worker-d1-and-mail.json")
	for _, path := range paths {
		first, diags := load(t, path)
		if first == nil || len(diags) > 0 {
			t.Errorf("%s: %v", path, diags)
			continue
		}
		text := model.Marshal(first)
		second, diags := model.Parse(path, text)
		if second == nil || len(diags) > 0 {
			t.Errorf("%s: the written model does not read back: %v", path, diags)
			continue
		}
		if !reflect.DeepEqual(second, asReadBack(first)) {
			t.Errorf("%s: the model read back differs from the one written", path)
		}
		if again := model.Marshal(second); !bytes.Equal(again, text) {
			t.Errorf("%s: writing the model that was read back gives another text", path)
		}
	}
}

func handBuilt() *model.Model {
	return &model.Model{
		SchemaVersion: "0.1",
		Target:        model.Target{Repository: ".", Environment: "dev", Scope: []model.Scope{}},
		Nodes: []model.Node{
			{
				ID: "n1", Kind: model.KindResource, Provider: "cloudflare", Service: "cloudflare.d1",
				Identity: map[string]any{
					"version":    json.Number("1.50"),
					"native_id":  json.Number("12345678901234567890"),
					"account_id": "a<b>&c",
				},
				Attributes: map[string]any{"role": "利用者データの保存"},
				Evidence: []model.Evidence{{
					ID: "ev1", Kind: model.EvDeclared,
					Supports:  []model.Support{{Key: model.KeyExistence, Value: true}},
					Ref:       model.Ref{Type: "file", Path: "wrangler.toml", StartLine: 7, EndLine: 10},
					CheckedAt: "2026-11-16T09:00:00Z",
				}},
			},
			{ID: "n2", Kind: model.KindService, Provider: "resend", Service: "resend", Identity: map[string]any{}},
		},
		Edges:    []model.Edge{{ID: "e1", From: "n1", To: "n2", Relation: model.RelImplementation, Evidence: []model.Evidence{}}},
		Coverage: []model.Coverage{{ID: "cv1", Status: model.CovNotAttempted, CheckedAt: "2026-11-16T09:00:00Z"}},
	}
}

const handBuiltText = `{
  "schema_version": "0.1",
  "target": {
    "repository": ".",
    "environment": "dev"
  },
  "nodes": [
    {
      "id": "n1",
      "kind": "resource",
      "provider": "cloudflare",
      "service": "cloudflare.d1",
      "identity": {
        "account_id": "a<b>&c",
        "native_id": 12345678901234567890,
        "version": 1.50
      },
      "attributes": {
        "role": "利用者データの保存"
      },
      "evidence": [
        {
          "id": "ev1",
          "kind": "declared",
          "supports": [
            {
              "key": "existence",
              "value": true
            }
          ],
          "ref": {
            "type": "file",
            "path": "wrangler.toml",
            "start_line": 7,
            "end_line": 10
          },
          "checked_at": "2026-11-16T09:00:00Z"
        }
      ]
    },
    {
      "id": "n2",
      "kind": "service",
      "provider": "resend",
      "service": "resend",
      "evidence": []
    }
  ],
  "edges": [
    {
      "id": "e1",
      "from": "n1",
      "to": "n2",
      "relation": "implementation",
      "evidence": []
    }
  ],
  "coverage": [
    {
      "id": "cv1",
      "scope": {},
      "status": "not_attempted",
      "checked_at": "2026-11-16T09:00:00Z"
    }
  ]
}
`

const emptyText = `{
  "schema_version": "0.1",
  "target": {
    "repository": ".",
    "environment": "dev"
  },
  "nodes": [],
  "edges": [],
  "coverage": []
}
`

// The hand-built model has no list at all for the evidence of n2 and no scope
// for cv1, and the empty model has no list of nodes, edges, or coverage. Each
// is written as an empty one, which the schema reads; null is refused.
func TestAMissingListIsWrittenAsAnEmptyOne(t *testing.T) {
	empty := func() *model.Model {
		return &model.Model{SchemaVersion: "0.1", Target: model.Target{Repository: ".", Environment: "dev"}}
	}
	for name, c := range map[string]struct {
		build func() *model.Model
		want  string
	}{
		"hand-built": {handBuilt, handBuiltText},
		"empty":      {empty, emptyText},
	} {
		m := c.build()
		got := model.Marshal(m)
		if string(got) != c.want {
			t.Errorf("%s: the model is written as\n%s\nwant\n%s", name, got, c.want)
		}
		if !reflect.DeepEqual(m, c.build()) {
			t.Errorf("%s: Marshal changed the model it was given", name)
		}
		if back, diags := model.Parse(name, got); back == nil || len(diags) > 0 {
			t.Errorf("%s: the written model does not read back: %v", name, diags)
		}
	}
}
