package lint

import (
	"encoding/json"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"sync"

	"github.com/ochamu/ponchimap/internal/diag"
	"github.com/ochamu/ponchimap/internal/model"
	"github.com/ochamu/ponchimap/schema"
)

func modelProblems(m *model.Model, at func(pointer string) diag.Location) diag.List {
	list := assessed(m, at)
	list = append(list, unknownSupportKeys(m, at)...)
	list = append(list, servicesDescribedTwice(m, at)...)
	return append(list, incompleteCoverage(m, at)...)
}

// servicesDescribedTwice warns about a node whose name or kind of service
// differs from the first node with its service key, so that one product is
// not drawn under two names or with two icons. A missing value is one value.
func servicesDescribedTwice(m *model.Model, at func(pointer string) diag.Location) diag.List {
	first := map[string]map[string]any{}
	var list diag.List
	for i, n := range m.Nodes {
		earlier, seen := first[n.Service]
		if !seen {
			first[n.Service] = n.Attributes
			continue
		}
		for _, key := range []string{"category", "service_name"} {
			if earlier[key] != n.Attributes[key] {
				list = append(list, diag.Diagnostic{Severity: diag.Warning, Code: "model.service_differs", TargetID: n.ID,
					Location: at("/nodes/" + strconv.Itoa(i)),
					Message:  "the attributes." + key + " of the node differs from that of the first node with the same service key",
					Hint:     "Write the same service_name and category on every node with the same service key, in every findings file, and import again."})
			}
		}
	}
	return list
}

type element struct {
	pointer  string
	id       string
	evidence []model.Evidence
}

func elements(m *model.Model) []element {
	all := make([]element, 0, len(m.Nodes)+len(m.Edges))
	for i, n := range m.Nodes {
		all = append(all, element{"/nodes/" + strconv.Itoa(i), n.ID, n.Evidence})
	}
	for i, e := range m.Edges {
		all = append(all, element{"/edges/" + strconv.Itoa(i), e.ID, e.Evidence})
	}
	return all
}

var knownSupportKeys = sync.OnceValue(func() []string {
	var s struct {
		Defs struct {
			Support struct {
				AllOf []struct {
					If struct {
						Properties struct {
							Key struct {
								Const string `json:"const"`
							} `json:"key"`
						} `json:"properties"`
					} `json:"if"`
				} `json:"allOf"`
			} `json:"support"`
		} `json:"$defs"`
	}
	var keys []string
	if err := json.Unmarshal(schema.Model, &s); err == nil {
		for _, rule := range s.Defs.Support.AllOf {
			if key := rule.If.Properties.Key.Const; key != "" {
				keys = append(keys, key)
			}
		}
	}
	if len(keys) == 0 {
		panic("lint: the schema gives no key under $defs.support.allOf")
	}
	return keys
})

func unknownSupportKeys(m *model.Model, at func(pointer string) diag.Location) diag.List {
	type use struct {
		first      diag.Location
		evidenceID string
		items      int
	}
	uses := map[string]*use{}
	known := knownSupportKeys()
	for _, e := range elements(m) {
		for j, ev := range e.evidence {
			for k, s := range ev.Supports {
				if slices.Contains(known, s.Key) {
					continue
				}
				here := at(e.pointer + "/evidence/" + strconv.Itoa(j) + "/supports/" + strconv.Itoa(k) + "/key")
				u := uses[s.Key]
				if u == nil {
					u = &use{first: here, evidenceID: ev.ID}
					uses[s.Key] = u
				} else if here.Line < u.first.Line || (here.Line == u.first.Line && here.Column < u.first.Column) {
					u.first, u.evidenceID = here, ev.ID
				}
				u.items++
			}
		}
	}
	hint := "The keys that the schema defines are: " + strings.Join(known, ", ") + ". An item with another key is kept, and it backs no frame, line, or mark. Correct the key if it was meant to be one of these."
	var list diag.List
	for _, u := range uses {
		list = append(list, diag.Diagnostic{Severity: diag.Warning, Code: "model.unknown_support_key", TargetID: u.evidenceID, Location: u.first, Hint: hint,
			Message: fmt.Sprintf("the key of the item is not one that the schema defines; the model uses it in %s", Count(u.items, "item"))})
	}
	return list
}

var coverageGaps = map[model.CoverageStatus]struct{ code, message, hint string }{
	model.CovPartial: {
		code:    "coverage.partial",
		message: "the status of the record is partial: a part of its scope was not read",
		hint:    "Fetch the rest of the scope, and record it. Until then the figure may lack a part of what the scope holds.",
	},
	model.CovDenied: {
		code:    "coverage.denied",
		message: "the status of the record is denied: its scope was not read for lack of permission",
		hint:    "Grant read access to the scope, and query it again.",
	},
	model.CovUnsupported: {
		code:    "coverage.unsupported",
		message: "the status of the record is unsupported: this version cannot read its scope",
		hint:    "Nothing in the scope can be queried by this version. Add evidence by hand if the figure needs it.",
	},
	model.CovNotAttempted: {
		code:    "coverage.not_attempted",
		message: "the status of the record is not_attempted: its scope was not queried",
		hint:    "Query the real environment for the scope.",
	},
	model.CovFailed: {
		code:    "coverage.failed",
		message: "the status of the record is failed: the read of its scope did not finish",
		hint:    "Read the reason of the record, and query the scope again.",
	},
}

func incompleteCoverage(m *model.Model, at func(pointer string) diag.Location) diag.List {
	var list diag.List
	for i, c := range m.Coverage {
		if gap, open := coverageGaps[c.Status]; open {
			list = append(list, diag.Diagnostic{Severity: diag.Warning, Code: gap.code, TargetID: c.ID,
				Location: at("/coverage/" + strconv.Itoa(i) + "/status"), Message: gap.message, Hint: gap.hint})
		}
	}
	return list
}

// Count is n and the noun, in the plural unless n is 1.
func Count(n int, noun string) string {
	if n == 1 {
		return "1 " + noun
	}
	return strconv.Itoa(n) + " " + noun + "s"
}
