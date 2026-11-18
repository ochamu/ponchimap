package cli

import (
	"context"
	"io"

	"github.com/ochamu/ponchimap/internal/diag"
	"github.com/ochamu/ponchimap/internal/lint"
)

func runLint(_ context.Context, args []string, stdout, _ io.Writer) outcome {
	flags := newFlagSet("lint")
	in := addInputFlags(flags)
	if o, stop := parseFlags(flags, args, stdout); stop {
		return o
	}
	if o, stop := unnamed(in.modelPath()); stop {
		return o
	}
	model, config := in.read()
	_, list := lint.Check(lint.Input{Model: model, View: config})

	failures := 0
	for _, d := range list {
		if d.Severity == diag.Error {
			failures++
		}
	}
	o := outcome{diagnostics: list, lines: []string{lint.Count(failures, "error") + ", " + lint.Count(len(list)-failures, "warning")}}
	if failures > 0 {
		o.result = inputError
	}
	return o
}
