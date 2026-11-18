package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"

	"github.com/ochamu/ponchimap/internal/diag"
	"github.com/ochamu/ponchimap/internal/lint"
	"github.com/ochamu/ponchimap/internal/workspace"
)

func runInit(ctx context.Context, args []string, stdout, _ io.Writer) outcome {
	flags := newFlagSet("init")
	root := flags.String("root", ".", "the `directory` to set up (default .)")
	addJSONFlag(flags)
	if o, stop := parseFlags(flags, args, stdout); stop {
		return o
	}
	switch info, err := os.Stat(*root); {
	case errors.Is(err, fs.ErrNotExist):
		return refused("--root names %q, which does not exist", *root)
	case err == nil && !info.IsDir():
		return refused("--root names %q, which is not a directory", *root)
	}
	if ctx.Err() != nil {
		return outcome{result: runError, diagnostics: diag.List{canceled("the run was canceled before anything was made")}}
	}

	steps, err := workspace.Init(*root)
	var o outcome
	for _, s := range steps {
		if s.Action == workspace.Made {
			o.artifacts = append(o.artifacts, artifact{Kind: string(s.Kind), Path: s.Path})
		}
		switch {
		case s.Action == workspace.Unusable:
			o.result = partial
			o.diagnostics = append(o.diagnostics, gitignoreUnusable(s))
		case !slices.ContainsFunc(workspace.Agents, func(a workspace.Agent) bool { return a.Kind == s.Kind }):
			o.lines = append(o.lines, lineOf(s))
		}
	}
	for _, a := range workspace.Agents {
		o = withSkill(o, *root, a, steps)
	}
	if err != nil {
		o.result = runError
		o.diagnostics = append(o.diagnostics, notMade(*root, err))
		return o
	}

	tracked, found := workspace.Tracked(ctx, *root)
	// Canceling the run stops Git, so its answer says nothing then. The
	// workspace is set up all the same.
	if ctx.Err() != nil {
		o.result = runError
		o.diagnostics = append(o.diagnostics, canceled("the run was canceled while Git listed the tracked files; the workspace is set up"))
		return o
	}
	switch {
	case !found:
		o.lines = append(o.lines, "tracked files were not looked for: git is not found")
	case len(tracked) > 0:
		o.diagnostics = append(o.diagnostics, stillTracked(len(tracked), filepath.Join(*root, workspace.Folder)))
	}

	// The agents and the other commands read and save from the directory they
	// run in.
	where := "this folder"
	if filepath.Clean(*root) != "." {
		where = *root
	}
	o.lines = append(o.lines, fmt.Sprintf("next: in %s, ask your coding agent (Claude Code or Codex) to make the architecture diagram, or put findings files (*.json) in %s and run \"%s import\", \"%s lint\", and \"%s render\"",
		where, workspace.DefaultFindings, tool, tool, tool))
	return o
}

func withSkill(o outcome, root string, a workspace.Agent, steps []workspace.Step) outcome {
	var seen, made int
	differs := false
	for _, s := range steps {
		if s.Kind != a.Kind {
			continue
		}
		seen++
		switch s.Action {
		case workspace.Made:
			made++
		case workspace.Differs:
			differs = true
		}
	}
	if seen == 0 {
		return o
	}
	folder := filepath.Join(root, a.Folder)
	whose := " (the Skill for " + a.Name + ")"
	switch made {
	case seen:
		o.lines = append(o.lines, "created "+folder+whose)
	case 0:
		o.lines = append(o.lines, "kept "+folder+whose)
	default:
		o.lines = append(o.lines, fmt.Sprintf("added %s to %s%s", lint.Count(made, "missing file"), folder, whose))
	}
	if differs {
		o.diagnostics = append(o.diagnostics, diag.Diagnostic{Severity: diag.Warning, Code: "init.skill_differs", Location: diag.Location{Path: folder},
			Message: fmt.Sprintf("the Skill for %s is not the one of this version of %s; init kept it as it is", a.Name, tool),
			Hint:    "To take the Skill of this version, remove " + folder + " and run init again, which loses the files that differ. To keep them, do nothing."})
	}
	return o
}

func lineOf(s workspace.Step) string {
	gitignore := s.Kind == workspace.KindGitignore
	switch {
	case s.Action == workspace.Made && gitignore:
		return fmt.Sprintf("added the line %q to %s", workspace.IgnoreEntry, s.Path)
	case s.Action == workspace.Made:
		return "created " + s.Path
	case gitignore:
		return fmt.Sprintf("kept %s: line %d names the workspace already", s.Path, s.Line)
	}
	return "kept " + s.Path
}

func gitignoreUnusable(s workspace.Step) diag.Diagnostic {
	var message string
	switch s.Why {
	case workspace.ErrNotRegular:
		message = "the .gitignore is not a regular file, so the workspace was not added to it"
	case workspace.ErrReadOnly:
		message = "the .gitignore is read-only, so the workspace was not added to it"
	default:
		message = "the .gitignore cannot be read, so the workspace was not added to it: " + s.Why.Error()
	}
	return diag.Diagnostic{Severity: diag.Warning, Code: "init.gitignore_unusable", Location: diag.Location{Path: s.Path},
		Message: message,
		Hint:    "Make .gitignore a regular file that can be read and written and run init again, or keep " + workspace.Folder + " out of Git with a rule of your own. Git does not read a .gitignore that is a symbolic link."}
}

func stillTracked(files int, folder string) diag.Diagnostic {
	return diag.Diagnostic{Severity: diag.Warning, Code: "init.tracked", Location: diag.Location{Path: folder},
		Message: fmt.Sprintf("Git already tracks %s of the workspace, and an ignore rule does not end that; init changed nothing in Git", lint.Count(files, "file")),
		Hint:    fmt.Sprintf("List them with \"git ls-files -ci --exclude-standard -- %s\". To stop tracking one and keep the file, run \"git rm --cached\" with its path and commit; earlier commits still hold it.", folder)}
}

// notMade is the diagnostic for the folder or the file that stopped
// workspace.Init, which gives no other error than a *workspace.Error.
func notMade(root string, err error) diag.Diagnostic {
	failed := err.(*workspace.Error)
	if failed.Path != filepath.Join(root, workspace.Folder) && failed.Path != filepath.Join(root, workspace.DefaultFindings) && !errors.Is(err, workspace.ErrNotFolder) {
		return unwritable(err)
	}
	return diag.Diagnostic{Severity: diag.Error, Code: "output.unwritable", Location: diag.Location{Path: failed.Path},
		Message: "the folder cannot be made: " + failed.Err.Error(),
		Hint:    "Move what stands at the path away, or check that the folder above it can be written to; then run init again. What init made stays, and init keeps it."}
}

func canceled(message string) diag.Diagnostic {
	return diag.Diagnostic{Severity: diag.Error, Code: "run.canceled", Message: message, Hint: "Run the command again."}
}
