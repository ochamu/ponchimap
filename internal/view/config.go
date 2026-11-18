package view

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"slices"
	"strconv"
	"strings"
	"sync"

	"github.com/ochamu/ponchimap/internal/diag"
	"github.com/ochamu/ponchimap/internal/input"
	"github.com/ochamu/ponchimap/internal/model"
	"github.com/ochamu/ponchimap/schema"
)

// Config is the view config: aliases, grouping, aggregation, and layout hints.
// It never adds facts; a label here is not evidence.
type Config struct {
	Schema        string            `json:"$schema,omitempty"`
	SchemaVersion string            `json:"schema_version"`
	View          string            `json:"view,omitempty"`
	Title         string            `json:"title,omitempty"`
	Labels        map[string]string `json:"labels,omitempty"`
	Boundaries    []BoundaryAlias   `json:"boundaries,omitempty"`
	Groups        []GroupConfig     `json:"groups,omitempty"`
	Aggregates    []AggregateConfig `json:"aggregates,omitempty"`
	Hidden        []string          `json:"hidden,omitempty"`
	Layout        LayoutConfig      `json:"layout"`
}

// ConfigFile is a view config as it was written: read, and checked against
// the schema, and not yet fitted to a model. Build takes a Config, and the
// way from a file to a Config is Resolve, so no figure is drawn from entries
// that name elements the model lacks.
type ConfigFile struct {
	// name is how diagnostics name the text, and doc holds the place of
	// every entry in it.
	name string
	doc  *input.Document
	// written holds the entries in the order of the text, with the defaults
	// filled in.
	written Config
}

// BoundaryAlias names the real boundary that Member belongs to. The config
// points at a model node so that it never has to repeat a raw account ID.
type BoundaryAlias struct {
	Member string `json:"member"`
	// Level picks the boundary around Member: "owner" (default), "region", or
	// "network".
	Level string `json:"level,omitempty"`
	Label string `json:"label"`
	// at is the JSON Pointer of the entry in the file it was read from.
	// Resolve leaves entries out, so the place of an entry in a fitted config
	// is not its place in the file, which is where a warning has to point. An
	// entry written in code has none.
	at string
}

type GroupConfig struct {
	ID      string   `json:"id"`
	Kind    string   `json:"kind,omitempty"`
	Label   string   `json:"label"`
	Members []string `json:"members"`
	at      string
}

type AggregateConfig struct {
	ID      string   `json:"id"`
	Label   string   `json:"label"`
	Role    string   `json:"role,omitempty"`
	Members []string `json:"members"`
	at      string
}

func entryPointer(at, key string, i int) string {
	if at != "" {
		return at
	}
	return "/" + key + "/" + strconv.Itoa(i)
}

type Engine string

const (
	EngineTALA Engine = "tala"
	EngineELK  Engine = "elk"
)

// Seed picks one of the layouts TALA can give for the same input. It is
// written in the config so that the same input and config give the same
// figure. ELK has no seed and ignores it.
type Seed int64

// UnmarshalJSON reads any JSON number whose value is a whole number, such as
// 2.0 or 1e3, which the schema takes for an integer and encoding/json refuses
// for an int.
func (s *Seed) UnmarshalJSON(raw []byte) error {
	r, ok := new(big.Rat).SetString(string(raw))
	if !ok || !r.IsInt() || !r.Num().IsInt64() {
		return errors.New("a seed is a whole number")
	}
	*s = Seed(r.Num().Int64())
	return nil
}

// LayoutConfig changes where things are drawn, never what is drawn.
type LayoutConfig struct {
	Direction Direction `json:"direction,omitempty"`
	Engine    Engine    `json:"engine,omitempty"`
	Seed      Seed      `json:"seed,omitempty"`
}

// The values a config has where it writes none. The schema states the same
// values as its defaults, for editors to show.
const (
	defaultView          = "overview"
	defaultBoundaryLevel = "owner"
)

// Normalize fills in the defaults (to the right, TALA, seed 1) and rejects
// values that name no layout.
func (l *LayoutConfig) Normalize() error {
	switch l.Direction {
	case "":
		l.Direction = DirRight
	case DirRight, DirDown:
	default:
		return fmt.Errorf("layout.direction %q is not right or down", l.Direction)
	}
	switch l.Engine {
	case "":
		l.Engine = EngineTALA
	case EngineTALA, EngineELK:
	default:
		return fmt.Errorf("layout.engine %q is not tala or elk", l.Engine)
	}
	switch {
	case l.Seed == 0:
		l.Seed = 1
	case l.Seed < 0:
		return fmt.Errorf("layout.seed %d is not 1 or more", l.Seed)
	}
	return nil
}

var configSchema = sync.OnceValue(func() *input.Schema { return input.MustCompile(schema.View) })

// ParseConfig reads a view config from YAML text. name is how diagnostics
// name the text, usually a file path. The file is nil exactly when the list
// has an error. What the config says about the model is not checked here:
// Resolve does that.
func ParseConfig(name string, raw []byte) (*ConfigFile, diag.List) {
	doc, list := input.DecodeYAML(raw)
	if doc != nil {
		checked := configSchema().Check(doc)
		list = append(list, checked...)
		if !slices.ContainsFunc(checked, func(d diag.Diagnostic) bool { return d.Code == input.UnsupportedVersion }) {
			list = append(list, duplicateIDs(doc)...)
		}
	}
	for i := range list {
		list[i].Location.Path = name
		if doc != nil {
			list[i].TargetID = containerIDAt(doc, list[i].Location)
		}
	}
	list.Sort()
	if list.HasErrors() {
		return nil, list
	}
	c, err := configOf(doc.Value)
	if err != nil {
		return nil, diag.List{{Severity: diag.Error, Code: "tool.defect", Location: diag.Location{Path: name},
			Message: "the view config passed every check but the tool could not read it; this is a defect of the tool, please report it",
			Hint:    "Report the defect with a view config that causes it."}}
	}
	return &ConfigFile{name: name, doc: doc, written: *c}, list
}

// configOf turns a value that passed the schema into a Config and fills in
// the defaults. The value goes through JSON so that the config holds exactly
// what was checked.
func configOf(value any) (*Config, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	var c Config
	if err := dec.Decode(&c); err != nil {
		return nil, err
	}
	if c.View == "" {
		c.View = defaultView
	}
	for i := range c.Boundaries {
		if c.Boundaries[i].Level == "" {
			c.Boundaries[i].Level = defaultBoundaryLevel
		}
	}
	for i := range c.Groups {
		if c.Groups[i].Kind == "" {
			c.Groups[i].Kind = string(GroupLogical)
		}
	}
	return &c, c.Layout.Normalize()
}

// containers are where the view config declares an ID of its own.
var containers = []string{"groups", "aggregates"}

// declaredID is the ID that the container at index i of the list named kind
// declares, when it has the ID form.
func declaredID(root any, kind string, i int) string {
	doc, _ := root.(map[string]any)
	list, _ := doc[kind].([]any)
	if i < 0 || i >= len(list) {
		return ""
	}
	container, _ := list[i].(map[string]any)
	id, _ := container["id"].(string)
	if !model.ValidID(id) {
		return ""
	}
	return id
}

// containerIDAt is the ID of the group or the aggregate of the view config
// doc that holds the location. A key written twice gives two values the same
// pointer, and the document keeps the last: a location that stands before the
// container that its pointer names is in the value that was replaced, and has
// no ID.
func containerIDAt(doc *input.Document, at diag.Location) string {
	tokens := strings.Split(at.Pointer, "/")
	if len(tokens) < 3 || !slices.Contains(containers, tokens[1]) {
		return ""
	}
	i, err := strconv.Atoi(tokens[2])
	if err != nil {
		return ""
	}
	container := doc.Locate(strings.Join(tokens[:3], "/"), false)
	if at.Line < container.Line || (at.Line == container.Line && at.Column < container.Column) {
		return ""
	}
	return declaredID(doc.Value, tokens[1], i)
}

// duplicateIDs reports an ID that two groups or aggregates declare. It reads
// the decoded value defensively, so that it runs on a document that failed
// the schema too.
func duplicateIDs(doc *input.Document) diag.List {
	type declaration struct {
		pointer, id string
		at          diag.Location
	}
	var declared []declaration
	root, _ := doc.Value.(map[string]any)
	for _, kind := range containers {
		list, _ := root[kind].([]any)
		for i := range list {
			if id := declaredID(doc.Value, kind, i); id != "" {
				pointer := "/" + kind + "/" + strconv.Itoa(i) + "/id"
				declared = append(declared, declaration{pointer, id, doc.Locate(pointer, false)})
			}
		}
	}
	// The first declaration of an ID is the one first in the text.
	slices.SortStableFunc(declared, func(a, b declaration) int {
		if a.at.Line != b.at.Line {
			return a.at.Line - b.at.Line
		}
		return a.at.Column - b.at.Column
	})
	var list diag.List
	first := map[string]string{}
	for _, d := range declared {
		if at, dup := first[d.id]; dup {
			list = append(list, diag.Diagnostic{Severity: diag.Error, Code: "view.duplicate_id", Location: d.at,
				Message: "the ID is already used at " + at, Hint: "Give each group and aggregate an ID of its own."})
			continue
		}
		first[d.id] = d.pointer
	}
	return list
}
