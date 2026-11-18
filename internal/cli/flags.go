package cli

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
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

// location is where a path leads on the file system: the deepest part of the
// path that exists, and the rest of the path below it, which is empty unless
// the file is yet to be made.
type location struct {
	existing fs.FileInfo
	rest     string
}

func locate(path string) location {
	// The path is asked for first as it is written. The kernel reads
	// "link/.." through the link, and filepath.Abs removes the two elements,
	// which gives the path of another file.
	if info, err := os.Stat(path); err == nil {
		return location{existing: info}
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		abs = filepath.Clean(path)
	}
	rest := ""
	for {
		if info, err := os.Stat(abs); err == nil {
			return location{info, rest}
		}
		parent := filepath.Dir(abs)
		if parent == abs {
			// Not even the root of the path exists, so the rest is all of it.
			return location{rest: filepath.Join(abs, rest)}
		}
		abs, rest = parent, filepath.Join(filepath.Base(abs), rest)
	}
}

// is says that two locations are one file. The rests are compared without
// their case: a file system that ignores case makes one file of Report.md
// and report.md.
func (l location) is(other location) bool {
	samePart := l.existing == nil && other.existing == nil || os.SameFile(l.existing, other.existing)
	return samePart && strings.EqualFold(l.rest, other.rest)
}
