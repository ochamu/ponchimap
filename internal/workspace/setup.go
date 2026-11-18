package workspace

import (
	"bytes"
	_ "embed"
	"errors"
	"io/fs"
	"os"
	"path"
	"path/filepath"

	"github.com/ochamu/ponchimap/skills"
)

// Kind names what a Step is about, in the words of the machine output.
type Kind string

const (
	KindGitignore       Kind = "gitignore"
	KindWorkspace       Kind = "workspace"
	KindFindings        Kind = "findings"
	KindViewConfig      Kind = "view_config"
	KindClaudeCodeSkill Kind = "claude_code_skill"
	KindCodexSkill      Kind = "codex_skill"
)

// Agent is a coding agent that Init places the Skill for.
type Agent struct {
	Kind Kind
	Name string
	// Folder is where the agent reads the Skill, relative to the root.
	Folder string
}

// Agents are the places measured with Claude Code 2.1.311 and Codex CLI
// 0.169.0: neither reads a skill of a project from the folder of the other.
var Agents = []Agent{
	{KindClaudeCodeSkill, "Claude Code", ".claude/skills/ponchimap"},
	{KindCodexSkill, "Codex", ".agents/skills/ponchimap"},
}

// Action is what Init did with one thing.
type Action int

const (
	// Made: nothing stood at the path, or the .gitignore had no line for the
	// workspace, and Init made the thing or added the line.
	Made Action = iota
	// Kept: the thing was there, and Init left it as it was.
	Kept
	// Unusable: the .gitignore cannot take the entry, and Init left it as it
	// was.
	Unusable
	// Differs: a file of the Skill was there with other bytes than this
	// version's, and Init left it as it was.
	Differs
)

// Step is one thing Init sees to, and what it did with it.
type Step struct {
	Kind Kind
	// Path is the root joined with the place of the thing.
	Path   string
	Action Action
	// Line is set for a kept .gitignore: the number, from 1, of its first
	// line that names the workspace.
	Line int
	// Why is set for an unusable .gitignore: ErrNotRegular, ErrReadOnly, or
	// the cause of a failed read, without the path.
	Why error
}

var (
	ErrNotFolder = errors.New("something other than a folder stands at the path")
	ErrReadOnly  = errors.New("is read-only")
)

//go:embed templates/architecture.view.yaml
var viewTemplate []byte

type row struct {
	kind      Kind
	place     string
	isFolder  bool
	ifMissing func(path string) error
	ifThere   func(step Step, info fs.FileInfo) (Step, error)
}

func folder(kind Kind, place string) row {
	return row{kind: kind, place: place, isFolder: true, ifMissing: makeFolder, ifThere: keep}
}

func file(kind Kind, place string, content []byte, ifThere func(Step, fs.FileInfo) (Step, error)) row {
	save := func(path string) error { return Save([]File{{Path: path, Data: content}}) }
	return row{kind: kind, place: place, ifMissing: save, ifThere: ifThere}
}

// The .gitignore comes before the workspace, so that a run that stops halfway
// leaves no workspace for Git to see.
var rows = append([]row{
	file(KindGitignore, ".gitignore", withEntry(nil), addEntry),
	folder(KindWorkspace, Folder),
	folder(KindFindings, DefaultFindings),
	file(KindViewConfig, DefaultView, viewTemplate, keep),
}, skillRows()...)

func skillRows() []row {
	skill, err := fs.Sub(skills.Files, "ponchimap")
	if err != nil {
		panic(err)
	}
	var placed []row
	for _, a := range Agents {
		err := fs.WalkDir(skill, ".", func(name string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return err
			}
			content, err := fs.ReadFile(skill, name)
			if err != nil {
				return err
			}
			placed = append(placed, file(a.Kind, path.Join(a.Folder, name), content, sameAs(content)))
			return nil
		})
		if err != nil {
			panic(err)
		}
	}
	return placed
}

// The root comes first, so that a file in the way is found at its own path:
// below it, Lstat fails with another error.
func (r row) folders() []string {
	var places []string
	for dir := path.Dir(r.place); dir != "."; dir = path.Dir(dir) {
		places = append([]string{dir}, places...)
	}
	if r.isFolder {
		places = append(places, r.place)
	}
	return places
}

// Init sets up root, an existing directory, for the other commands and the
// agents: the entry in .gitignore, the workspace with its folder for
// findings, a view config to start from, and the Skill in the folder of each
// of the Agents. It makes what is missing and keeps what is there, so a run
// after one that stopped halfway does the rest, and a run after a complete
// one changes nothing. What a failed run made stays.
//
// Where something other than a folder stands in the place of a folder, Init
// changes nothing. The steps are the ones Init did, in order. The error is an
// *Error, and the steps are then the ones before it.
func Init(root string) ([]Step, error) {
	for _, r := range rows {
		for _, place := range r.folders() {
			path := filepath.Join(root, place)
			if err := inTheWay(path); err != nil {
				return nil, failed(File{Path: path}, err)
			}
		}
	}
	var steps []Step
	for _, r := range rows {
		step, err := r.seeTo(filepath.Join(root, r.place))
		if err != nil {
			return steps, err
		}
		steps = append(steps, step)
	}
	return steps, nil
}

func inTheWay(path string) error {
	if _, err := os.Lstat(path); errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	info, err := os.Stat(path)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		// A symbolic link that leads nowhere stands at the path.
		return ErrNotFolder
	case err != nil:
		return err
	case !info.IsDir():
		return ErrNotFolder
	}
	return nil
}

func (r row) seeTo(path string) (Step, error) {
	step := Step{Kind: r.kind, Path: path}
	info, err := os.Lstat(path)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		step.Action = Made
		return step, r.ifMissing(path)
	case err != nil:
		return step, failed(File{Path: path}, err)
	}
	return r.ifThere(step, info)
}

func makeFolder(path string) error {
	if err := os.Mkdir(path, 0o755); err != nil {
		return failed(File{Path: path}, err)
	}
	return nil
}

func keep(step Step, _ fs.FileInfo) (Step, error) {
	step.Action = Kept
	return step, nil
}

// What is no regular file is kept unread, like the view config.
func sameAs(content []byte) func(Step, fs.FileInfo) (Step, error) {
	return func(step Step, info fs.FileInfo) (Step, error) {
		step.Action = Kept
		if !info.Mode().IsRegular() {
			return step, nil
		}
		if text, err := os.ReadFile(step.Path); err != nil || !bytes.Equal(text, content) {
			step.Action = Differs
		}
		return step, nil
	}
}

// addEntry sees to the .gitignore that stands at the path of the step. A
// file with a line for the workspace is kept. One that cannot take the entry
// is left alone: Git does not read a .gitignore that is a symbolic link, and
// Save, which renames, would replace a file that the user made read-only.
func addEntry(step Step, info fs.FileInfo) (Step, error) {
	if !info.Mode().IsRegular() {
		step.Action, step.Why = Unusable, ErrNotRegular
		return step, nil
	}
	text, err := os.ReadFile(step.Path)
	if err != nil {
		step.Action, step.Why = Unusable, failed(File{}, err).Err
		return step, nil
	}
	if line := namingLine(text); line > 0 {
		step.Action, step.Line = Kept, line
		return step, nil
	}
	if info.Mode()&0o200 == 0 {
		step.Action, step.Why = Unusable, ErrReadOnly
		return step, nil
	}
	step.Action = Made
	return step, Save([]File{{Path: step.Path, Data: withEntry(text)}})
}
