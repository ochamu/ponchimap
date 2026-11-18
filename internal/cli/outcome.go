package cli

import (
	"encoding/json"
	"fmt"
	"io"

	"github.com/ochamu/ponchimap/internal/diag"
)

// result is how a command ended.
type result int

const (
	ok result = iota
	// partial is a result the user can use although a part of it is missing.
	partial
	// inputError is a failure the user ends by correcting the command line or
	// an input file.
	inputError
	// runError is a failure of the run itself.
	runError
)

// endings is the one place where a result becomes the status a program reads
// and the exit code. A command names a result and never a number.
var endings = [...]struct {
	status   string
	exitCode int
}{
	ok:         {"ok", 0},
	partial:    {"partial", 0},
	inputError: {"error", 2},
	runError:   {"error", 4},
}

// outcome is what a command has to say when it ends.
type outcome struct {
	result      result
	diagnostics diag.List
	// artifacts are the files this run saved and the folders it made, in the
	// order it did.
	artifacts []artifact
	// lines are for a person, after the diagnostics.
	lines []string
}

// artifact is one saved file or made folder in the machine output. Layout is
// there for the SVG, Scale for the PNG, and Added, Removed, and IDs for the
// model that import saved.
type artifact struct {
	Kind string `json:"kind"`
	// Path is made from the path as the command line names it.
	Path   string       `json:"path"`
	Layout *drawnLayout `json:"layout,omitzero"`
	Scale  int          `json:"scale,omitzero"`
	// Added and Removed are the IDs of the nodes and edges that the import put
	// into the model and took out of it. IDs says which element of the model
	// each candidate of the findings became.
	Added   []string   `json:"added,omitzero"`
	Removed []string   `json:"removed,omitzero"`
	IDs     []imported `json:"ids,omitzero"`
}

// drawnLayout is the layout the SVG was drawn with.
type drawnLayout struct {
	Engine    string `json:"engine"`
	Direction string `json:"direction"`
	// Seed is there for TALA only. ELK has none.
	Seed int64 `json:"seed,omitempty"`
}

// write puts the outcome out and returns the exit code. Every text for a
// person goes to stderr, with or without --json, so stdout holds nothing but
// what the user asked for.
func (o outcome) write(asJSON bool, stdout, stderr io.Writer) int {
	for _, d := range o.diagnostics {
		fmt.Fprintln(stderr, d)
		if d.Hint != "" {
			fmt.Fprintln(stderr, "  hint: "+d.Hint)
		}
	}
	for _, line := range o.lines {
		fmt.Fprintln(stderr, line)
	}
	end := endings[o.result]
	if asJSON {
		artifacts := o.artifacts
		if artifacts == nil {
			artifacts = []artifact{}
		}
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		enc.Encode(struct {
			Status      string     `json:"status"`
			Diagnostics diag.List  `json:"diagnostics"`
			Artifacts   []artifact `json:"artifacts"`
		}{end.status, o.diagnostics, artifacts})
	}
	return end.exitCode
}

// refused is the outcome of a command line that cannot be run as written.
func refused(format string, args ...any) outcome {
	return outcome{result: inputError, diagnostics: diag.List{usageProblem(fmt.Sprintf(format, args...))}}
}

// usageProblem says what of the command line is wrong. The message may quote
// the command line, which the user wrote on this terminal.
func usageProblem(message string) diag.Diagnostic {
	return diag.Diagnostic{Severity: diag.Error, Code: "usage.invalid", Message: message,
		Hint: fmt.Sprintf("Run \"%s help\" to see the commands, or a command with -h to see its flags.", tool)}
}

func defect(message string) diag.Diagnostic {
	return diag.Diagnostic{Severity: diag.Error, Code: "tool.defect", Message: message,
		Hint: "Report the defect with inputs that cause it."}
}
