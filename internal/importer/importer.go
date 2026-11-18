package importer

import (
	"cmp"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"slices"

	"github.com/ochamu/ponchimap/internal/diag"
	"github.com/ochamu/ponchimap/internal/input"
	"github.com/ochamu/ponchimap/internal/model"
)

type Request struct {
	Model       string   // the model file; it need not exist yet
	Findings    []string // findings files, or folders whose *.json files are findings
	Supplements string   // the supplements file; "" when none is given
	Workspace   string   // the folder of acquisitions.json
	// Target is the target of a model that does not exist yet. With an empty
	// Environment the new model is about the one environment that the
	// findings state as their target.
	Target *model.Target
	// Check reports what is wrong with the model that is about to be saved.
	// It may be nil.
	Check func(path string, text []byte) diag.List
}

type Result struct {
	Status      Status
	Model       *model.Model // as saved; nil when Diagnostics has an error
	IDs         []Mapping    // every candidate of every input
	Added       []string     // nodes and edges new in the model
	Removed     []string     // nodes and edges that left the model
	Diagnostics diag.List
}

type Status string

const (
	StatusOK      Status = "ok"
	StatusPartial Status = "partial"
	StatusError   Status = "error"
)

type Mapping struct {
	Input     string // the file, as given
	Kind      string // node, edge, evidence, coverage
	Candidate string // the ID in that file
	ID        string // the ID in the model; "" when nothing of it was kept
}

// Run takes the findings and the supplements of the request into the model
// and saves it. When any input is wrong it saves nothing: which inputs went
// in decides what a newer read replaces, so taking in only the correct ones
// would leave a model that the user cannot account for.
func Run(req Request) *Result {
	found, saved := run(req)
	found.Sort()
	if found.HasErrors() {
		return &Result{Status: StatusError, Diagnostics: found}
	}
	saved.Diagnostics = found
	return saved
}

func run(req Request) (diag.List, *Result) {
	paths, found := findingsFiles(req.Findings)

	in := given{modelPath: req.Model}
	var modelText []byte
	made := false
	switch raw, err := os.ReadFile(req.Model); {
	case errors.Is(err, fs.ErrNotExist) && req.Target == nil:
		return append(found, noTarget.at(diag.Location{Path: req.Model})), nil
	case errors.Is(err, fs.ErrNotExist):
		in.model, made = &model.Model{SchemaVersion: "0.1", Target: *req.Target}, true
	case err != nil:
		return append(found, unreadable("file", req.Model, err)), nil
	default:
		// The text is kept: its digest tells which records belong to the
		// model, and its places go into diagnostics about elements that leave.
		m, list := model.Parse(req.Model, raw)
		if found = append(found, list...); m == nil {
			return found, nil
		}
		in.model, modelText = m, raw
		in.text, _ = input.DecodeJSON(raw)
	}

	for _, path := range paths {
		f, list := readFindings(path)
		found = append(found, list...)
		if f != nil {
			in.findings = append(in.findings, f)
		}
	}
	if req.Supplements != "" {
		s, list := readSupplements(req.Supplements)
		found = append(found, list...)
		in.supplements = s
	}
	if made && in.model.Target.Environment == "" && !found.HasErrors() {
		stated := map[string]bool{}
		for _, f := range in.findings {
			if f.environment != "" {
				stated[f.environment] = true
			}
		}
		if len(stated) != 1 {
			return append(found, noEnvironment.at(diag.Location{Path: req.Model})), nil
		}
		for environment := range stated {
			in.model.Target.Environment = environment
		}
	}

	ws := workspace{folder: req.Workspace}
	list := ws.read(modelText)
	found = append(found, list...)
	in.records = ws.records
	found = append(found, checkAcquisitions(in.findings, in.records)...)
	if found.HasErrors() {
		return found, nil
	}

	out := fold(in)
	if found = append(found, out.found...); found.HasErrors() {
		return found, nil
	}
	text := model.Marshal(out.model)
	saved, list := model.Parse(req.Model, text)
	if len(list) > 0 {
		return append(found, brokenModel.at(diag.Location{Path: req.Model})), nil
	}
	if req.Check != nil {
		for _, d := range req.Check(req.Model, text) {
			if d.Severity == diag.Error {
				found = append(found, d)
			}
		}
		if found.HasErrors() {
			return found, nil
		}
	}
	if written, _ := input.DecodeJSON(text); written != nil {
		for i := range out.inSaved {
			line := written.Locate(out.inSaved[i].Location.Pointer, false)
			out.inSaved[i].Location.Line, out.inSaved[i].Location.Column = line.Line, line.Column
		}
	}
	found = append(found, out.inSaved...)

	if found = append(found, ws.save(req.Model, modelText, text, out.records)...); found.HasErrors() {
		return found, nil
	}
	res := &Result{Status: StatusOK, Model: saved, IDs: out.ids, Added: out.added, Removed: out.removed}
	if out.partial {
		res.Status = StatusPartial
	}
	slices.SortFunc(res.IDs, func(a, b Mapping) int {
		return cmp.Or(cmp.Compare(a.Input, b.Input), cmp.Compare(a.Kind, b.Kind), cmp.Compare(a.Candidate, b.Candidate))
	})
	slices.Sort(res.Added)
	slices.Sort(res.Removed)
	return found, res
}

func findingsFiles(given []string) (paths []string, found diag.List) {
	add := func(path string) {
		if !slices.Contains(paths, path) {
			paths = append(paths, path)
		}
	}
	for _, path := range given {
		info, err := os.Stat(path)
		if err != nil || !info.IsDir() {
			add(path)
			continue
		}
		entries, err := os.ReadDir(path)
		if err != nil {
			found = append(found, unreadable("folder", path, err))
			continue
		}
		holds := false
		for _, entry := range entries {
			if match, _ := filepath.Match("*.json", entry.Name()); match && !entry.IsDir() {
				add(filepath.Join(path, entry.Name()))
				holds = true
			}
		}
		if !holds {
			found = append(found, noFindings.at(diag.Location{Path: path}))
		}
	}
	return paths, found
}

// checkAcquisitions is the last stage of reading: an acquisition ID names one
// read, so two inputs of a run do not share one, and an input does not carry
// the ID of a recorded acquisition with another content.
func checkAcquisitions(inputs []*findings, records []record) diag.List {
	var found diag.List
	digests := map[string]string{}
	for _, r := range records {
		digests[r.ID] = r.Digest
	}
	inOrder := slices.SortedFunc(slices.Values(inputs), func(a, b *findings) int { return cmp.Compare(a.path, b.path) })
	seen := map[string]bool{}
	for _, f := range inOrder {
		at := locationIn(f.path, f.doc, "/acquisition/id")
		if seen[f.acq.ID] {
			found = append(found, duplicateAcquisition.at(at))
		}
		seen[f.acq.ID] = true
		if recorded, known := digests[f.acq.ID]; known && recorded != f.acq.Digest {
			found = append(found, acquisitionChanged.at(at))
		}
	}
	return found
}
