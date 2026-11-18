package workspace

import (
	"context"
	"os/exec"
	"strings"
)

// Tracked asks Git for the files of the workspace under root that it tracks
// although the ignore rules keep them out. An ignore rule does not end the
// tracking of such a file, so the entry of Init alone leaves it in Git. The
// files are named from root. What Git writes to its standard error is not
// shown. found is false when there is no git to ask. Where Git ends with an
// error, most often because root lies in no repository, no file is tracked.
func Tracked(ctx context.Context, root string) (files []string, found bool) {
	git, err := exec.LookPath("git")
	if err != nil {
		return nil, false
	}
	// Without core.fsmonitor=false, ls-files runs the program that the
	// configuration of the repository names there. The tool reads other
	// people's repositories and must not run a program of one.
	out, err := exec.CommandContext(ctx, git, "-c", "core.fsmonitor=false", "-C", root,
		"ls-files", "-z", "--cached", "--ignored", "--exclude-standard", "--", Folder).Output()
	if err != nil {
		return nil, true
	}
	for name := range strings.SplitSeq(string(out), "\x00") {
		if name != "" {
			files = append(files, name)
		}
	}
	return files, true
}
