package cli

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"strings"

	"github.com/ochamu/ponchimap/internal/lint"
	"github.com/ochamu/ponchimap/internal/workspace"
)

// newFlagSet gives the flags of a command. The flag package writes nothing
// itself: what it refuses comes back as an error and becomes a diagnostic.
func newFlagSet(command string) *flag.FlagSet {
	flags := flag.NewFlagSet(command, flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	return flags
}

// parseFlags reads the command line of a command. With stop the command ends
// there with the outcome: the command line asks for the flags, which are
// written to stdout, or it cannot be run as written.
func parseFlags(flags *flag.FlagSet, args []string, stdout io.Writer) (o outcome, stop bool) {
	switch err := flags.Parse(args); {
	case errors.Is(err, flag.ErrHelp):
		io.WriteString(stdout, flagsText(flags))
		return outcome{}, true
	case err != nil:
		return refused("%s", err), true
	case flags.NArg() > 0:
		return refused("%q is not a flag, and %s takes flags only", flags.Arg(0), flags.Name()), true
	}
	return outcome{}, false
}

func flagsText(flags *flag.FlagSet) string {
	var list strings.Builder
	flags.VisitAll(func(f *flag.Flag) {
		value, text := flag.UnquoteUsage(f)
		if value != "" {
			value = " " + value
		}
		fmt.Fprintf(&list, "  --%s%s\n      %s\n", f.Name, value, text)
	})
	if list.Len() == 0 {
		return fmt.Sprintf("Usage: %s %s\n\nThe command takes no flags.\n", tool, flags.Name())
	}
	return fmt.Sprintf("Usage: %s %s [flags]\n\nFlags:\n%s", tool, flags.Name(), list.String())
}

// inputFlags are the flags of a command that checks the model and the view
// config.
type inputFlags struct {
	flags       *flag.FlagSet
	model, view string
}

func addInputFlags(flags *flag.FlagSet) *inputFlags {
	in := &inputFlags{flags: flags}
	flags.StringVar(&in.model, "model", workspace.DefaultModel, "the model `file` (default "+workspace.DefaultModel+")")
	flags.StringVar(&in.view, "view", workspace.DefaultView, "the view config `file`; an empty name is no view config (default "+workspace.DefaultView+", which may be missing)")
	addJSONFlag(flags)
	return in
}

// addJSONFlag makes --json a flag of the command, so that it is accepted and
// listed. Run reads it from the raw arguments.
func addJSONFlag(flags *flag.FlagSet) {
	flags.Bool("json", false, "write the result as JSON to the standard output")
}

// viewNamed says that the command line has --view. It is known after the
// flags are parsed.
func (in *inputFlags) viewNamed() bool {
	named := false
	in.flags.Visit(func(f *flag.Flag) { named = named || f.Name == "view" })
	return named
}

// modelPath is --model as unnamed takes it.
func (in *inputFlags) modelPath() flagPath { return flagPath{"--model", "file", in.model} }

// read gives the model and the view config that lint.Check reads. No view
// config is the zero Source. A missing default view config is no view config,
// since a model alone can be checked and drawn.
func (in *inputFlags) read() (model, config lint.Source) {
	model = lint.Source{Path: in.model}
	if in.view != "" && (in.viewNamed() || !missing(in.view)) {
		config = lint.Source{Path: in.view}
	}
	return model, config
}

func missing(path string) bool {
	_, err := os.Stat(path)
	return errors.Is(err, fs.ErrNotExist)
}

// flagPath is a flag that has to name a file or a directory, with what it
// has to name and the path it was given.
type flagPath struct {
	flag, what, path string
}

// unnamed is the outcome that refuses the first flag whose path is empty.
// Without such a flag, stop is false. A shell variable that is empty gives an
// empty path, as in --report "$REPORT", and no later check sees it: joined
// with a file name, an empty --output is the current directory.
func unnamed(paths ...flagPath) (o outcome, stop bool) {
	for _, p := range paths {
		if p.path == "" {
			return refused("%s names no %s", p.flag, p.what), true
		}
	}
	return outcome{}, false
}
