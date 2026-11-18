// Package cli is the ponchimap command. Run takes one command line and
// answers with an exit code, text for a person on the standard error, and,
// under --json, one JSON value for a program on the standard output.
package cli

import (
	"context"
	"fmt"
	"io"
	"runtime"
	"runtime/debug"
	"strconv"
	"strings"

	"github.com/ochamu/ponchimap/internal/diag"
)

const tool = "ponchimap"

type command struct {
	name    string
	summary string
	// run answers with an outcome, which Run writes. It writes two things
	// itself: to stdout the text the user asked to see there (the flags of
	// the command, the versions), and to stderr a note that has to come
	// before the diagnostics.
	run func(ctx context.Context, args []string, stdout, stderr io.Writer) outcome
}

// commands is the one list of the commands, in the order help shows them.
// Adding a command adds an entry here and a file with its run function.
var commands = []command{
	{"init", "set up the workspace, a view config to start from, the .gitignore entry, and the Skill for coding agents", runInit},
	{"import", "take findings and supplements into the model", runImport},
	{"lint", "check the model and the view config", runLint},
	{"render", "check, then save the figure and the report", runRender},
	{"version", "print the versions of the tool, D2, and Go", runVersion},
}

// Run runs one command line and returns its exit code.
func Run(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	asJSON := wantsJSON(args)
	// Under --json the standard output holds the JSON value and nothing
	// else, so the text a command writes there itself goes to the standard
	// error with the rest of the text for a person.
	asked := stdout
	if asJSON {
		asked = stderr
	}
	return dispatch(ctx, args, asked, stderr).write(asJSON, stdout, stderr)
}

// wantsJSON reads --json from the raw arguments, with the values the flag
// package takes for a boolean flag. Run has to know before a command parses
// its flags, so that a command line the flags refuse is answered in JSON too.
func wantsJSON(args []string) bool {
	asJSON := false
	for _, arg := range args {
		name, value, hasValue := strings.Cut(arg, "=")
		if name != "-json" && name != "--json" {
			continue
		}
		if !hasValue {
			asJSON = true
		} else if b, err := strconv.ParseBool(value); err == nil {
			asJSON = b
		}
	}
	return asJSON
}

func dispatch(ctx context.Context, args []string, stdout, stderr io.Writer) (o outcome) {
	// Go ends a panicking program with exit code 2, which here means an
	// input the user can correct. A defect of the tool ends as a run error.
	defer func() {
		if v := recover(); v != nil {
			fmt.Fprintf(stderr, "panic: %s\n\n%s", panicText(v), debug.Stack())
			o = outcome{result: runError, diagnostics: diag.List{defect("the tool stopped at a point it must never reach; this is a defect of the tool, please report it")}}
		}
	}()
	if len(args) == 0 {
		return refused("no command is given")
	}
	name, rest := args[0], args[1:]
	switch name {
	case "help", "-h", "-help", "--help":
		io.WriteString(stdout, usage())
		return outcome{}
	}
	for _, c := range commands {
		if c.name == name {
			return c.run(ctx, rest, stdout, stderr)
		}
	}
	return refused("%q is not a command", name)
}

// panicText is what a panic says, when that is safe to show. An error of the
// Go runtime is made of fixed words, numbers, and type names. Any other value
// may quote a label or another text of the input, so only its type is shown.
func panicText(v any) string {
	if err, ok := v.(runtime.Error); ok {
		return err.Error()
	}
	return fmt.Sprintf("a value of type %T", v)
}

func usage() string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s leaves a diagram of the infrastructure an app uses, and a report, in the repository.\n\n", tool)
	fmt.Fprintf(&b, "Usage:\n  %s <command> [flags]\n\nCommands:\n", tool)
	for _, c := range commands {
		fmt.Fprintf(&b, "  %-8s %s\n", c.name, c.summary)
	}
	fmt.Fprintf(&b, "  %-8s %s\n", "help", "print this text")
	fmt.Fprintf(&b, "\nRun \"%s <command> -h\" for the flags of a command.\n", tool)
	return b.String()
}
