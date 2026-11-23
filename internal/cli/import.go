package cli

import (
	"context"
	"flag"
	"fmt"
	"io"
	"path/filepath"
	"slices"

	"github.com/ochamu/ponchimap/internal/diag"
	"github.com/ochamu/ponchimap/internal/importer"
	"github.com/ochamu/ponchimap/internal/lint"
	"github.com/ochamu/ponchimap/internal/model"
	"github.com/ochamu/ponchimap/internal/workspace"
)

// imported is one candidate of a findings file and what it became in the
// model. ID is empty when nothing of the candidate was kept.
type imported struct {
	Input     string `json:"input"`
	Kind      string `json:"kind"`
	Candidate string `json:"candidate"`
	ID        string `json:"id"`
}

func runImport(_ context.Context, args []string, stdout, stderr io.Writer) outcome {
	flags := newFlagSet("import")
	var findings, supplements, modelPath, environment string
	flags.StringVar(&findings, "input", workspace.DefaultFindings, "a findings `file`, or a directory whose *.json files are findings (default "+workspace.DefaultFindings+")")
	flags.StringVar(&supplements, "supplements", workspace.DefaultSupplements, "the supplements `file`; an empty name is no supplements (default "+workspace.DefaultSupplements+", which may be missing)")
	flags.StringVar(&modelPath, "model", workspace.DefaultModel, "the model `file`, which is updated, or made when it does not exist (default "+workspace.DefaultModel+")")
	flags.StringVar(&environment, "environment", "", "the `environment` that a new model is about; without it, the one that the findings state as their target")
	addJSONFlag(flags)
	if o, stop := parseFlags(flags, args, stdout); stop {
		return o
	}
	if o, stop := unnamed(flagPath{"--model", "file", modelPath}, flagPath{"--input", "file or directory", findings}); stop {
		return o
	}
	named := map[string]bool{}
	flags.Visit(func(f *flag.Flag) { named[f.Name] = true })
	// The supplements are the user's own notes, and most models have none: at
	// the default place they may be missing. A file that the command line
	// names has to be there.
	if supplements != "" && !named["supplements"] && missing(supplements) {
		supplements = ""
	}

	req := importer.Request{Model: modelPath, Findings: []string{findings}, Supplements: supplements,
		Workspace: filepath.Dir(modelPath),
		Check: func(path string, text []byte) diag.List {
			_, list := lint.Check(lint.Input{Model: lint.Source{Path: path, Text: text}})
			return list
		}}
	if missing(modelPath) {
		req.Target = &model.Target{Repository: ".", Environment: environment}
	} else if named["environment"] {
		fmt.Fprintf(stderr, "%s exists; --environment is for a new model and is not used\n", modelPath)
	}
	res := importer.Run(req)

	o := outcome{diagnostics: res.Diagnostics}
	switch res.Status {
	case importer.StatusError:
		o.result = inputError
		// A file that cannot be written and a defect are no fault of the input.
		if slices.ContainsFunc(res.Diagnostics, func(d diag.Diagnostic) bool {
			return d.Severity == diag.Error && (d.Code == "import.write_failed" || d.Code == "tool.defect")
		}) {
			o.result = runError
		}
		return o
	case importer.StatusPartial:
		o.result = partial
	}
	saved := artifact{Kind: "model", Path: modelPath, Added: slices.Clone(res.Added), Removed: slices.Clone(res.Removed), IDs: []imported{}}
	if saved.Added == nil {
		saved.Added = []string{}
	}
	if saved.Removed == nil {
		saved.Removed = []string{}
	}
	for _, m := range res.IDs {
		saved.IDs = append(saved.IDs, imported{m.Input, m.Kind, m.Candidate, m.ID})
	}
	o.artifacts = []artifact{saved, {Kind: "acquisitions", Path: req.RecordsPath()}}
	o.lines = []string{fmt.Sprintf("%s: %s, %s; %d added, %d removed", modelPath,
		lint.Count(len(res.Model.Nodes), "node"), lint.Count(len(res.Model.Edges), "edge"), len(res.Added), len(res.Removed))}
	return o
}
