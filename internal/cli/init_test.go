package cli

import (
	"context"
	"io/fs"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/ochamu/ponchimap/skills"
)

const ignoreEntry = "/.ponchimap/*"

// skillFolders are where Claude Code and Codex read a skill of a project.
var skillFolders = []struct{ kind, folder string }{
	{"claude_code_skill", ".claude/skills/ponchimap"},
	{"codex_skill", ".agents/skills/ponchimap"},
}

func skillFiles(t *testing.T) map[string]string {
	t.Helper()
	files := map[string]string{}
	err := fs.WalkDir(skills.Files, "ponchimap", func(name string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		data, err := fs.ReadFile(skills.Files, name)
		files[strings.TrimPrefix(name, "ponchimap/")] = string(data)
		return err
	})
	if err != nil || len(files) == 0 {
		t.Fatalf("the embedded Skill has the files %v: %v", slices.Collect(maps.Keys(files)), err)
	}
	return files
}

func outsideTheSkill(files map[string]string) map[string]string {
	rest := maps.Clone(files)
	for name := range rest {
		for _, s := range skillFolders {
			if strings.HasPrefix(name, s.folder+"/") {
				delete(rest, name)
			}
		}
	}
	return rest
}

func kinds(out machineOutput) []string {
	var made []string
	for _, artifact := range out.Artifacts {
		made = append(made, artifact.Kind)
	}
	return slices.Compact(made)
}

func initIn(root string, more ...string) answer {
	return run(append([]string{"init", "--root", root}, more...)...)
}

func patterns(text string) []string {
	var list []string
	for line := range strings.SplitSeq(text, "\n") {
		line = strings.TrimSuffix(line, "\r")
		if line != "" && !strings.HasPrefix(line, "#") {
			list = append(list, line)
		}
	}
	return list
}

func isDir(path string) bool {
	info, err := os.Lstat(path)
	return err == nil && info.IsDir()
}

func TestInitMakesTheWorkspaceTheViewConfigAndTheIgnoreEntry(t *testing.T) {
	root := t.TempDir()
	a := initIn(root)
	if a.code != 0 || a.stdout != "" {
		t.Fatalf("exit code %d and standard output %q, want 0 and nothing\n%s", a.code, a.stdout, a.stderr)
	}
	for _, dir := range []string{".ponchimap", ".ponchimap/findings"} {
		if !isDir(filepath.Join(root, dir)) {
			t.Errorf("%s is not a directory after init", dir)
		}
	}
	files := outsideTheSkill(snapshot(t, root))
	if names := slices.Sorted(maps.Keys(files)); !slices.Equal(names, []string{".gitignore", "architecture.view.yaml"}) {
		t.Fatalf("init left the files %v outside the Skill, want .gitignore and architecture.view.yaml alone", names)
	}
	if got := patterns(files[".gitignore"]); !slices.Equal(got, []string{ignoreEntry}) {
		t.Errorf(".gitignore has the patterns %q, want %q alone", got, ignoreEntry)
	}
	if !strings.HasPrefix(files[".gitignore"], "# ") || !strings.HasSuffix(files[".gitignore"], "\n") {
		t.Errorf(".gitignore does not start with a comment on the entry and end with a newline: %q", files[".gitignore"])
	}
	for _, name := range []string{".ponchimap", "architecture.view.yaml", ".gitignore"} {
		if !strings.Contains(a.stderr, name) {
			t.Errorf("the standard error does not name %s:\n%s", name, a.stderr)
		}
	}
	for _, next := range []string{"ponchimap import", "ponchimap render"} {
		if !strings.Contains(a.lastLine(), next) {
			t.Errorf("the last line does not say to run %q next: %s", next, a.lastLine())
		}
	}
}

func TestTheViewConfigOfInitDrawsTheFigureOfNoViewConfig(t *testing.T) {
	root := t.TempDir()
	if a := initIn(root); a.code != 0 {
		t.Fatalf("init: exit code %d\n%s", a.code, a.stderr)
	}
	model, _ := sample("long-labels-and-mismatches")
	config := filepath.Join(root, "architecture.view.yaml")
	if a := run("lint", "--model", model, "--view", config); a.code != 0 {
		t.Fatalf("lint with the view config of init: exit code %d\n%s", a.code, a.stderr)
	}
	with, without := newTarget(t), newTarget(t)
	if a := with.render(model, config); a.code != 0 {
		t.Fatalf("render with the view config of init: exit code %d\n%s", a.code, a.stderr)
	}
	if a := without.render(model, ""); a.code != 0 {
		t.Fatalf("render without a view config: exit code %d\n%s", a.code, a.stderr)
	}
	if readFile(t, with.svg()) != readFile(t, without.svg()) {
		t.Error("the figure with the view config of init is not the figure without a view config")
	}
}

func TestInitAgainKeepsWhatTheUserChanged(t *testing.T) {
	root := t.TempDir()
	if a := initIn(root); a.code != 0 {
		t.Fatalf("init: exit code %d\n%s", a.code, a.stderr)
	}
	writeFile(t, filepath.Join(root, "architecture.view.yaml"), "schema_version: \"0.1\"\ntitle: \"Mine\"\nlayout:\n  engine: elk\n")
	writeFile(t, filepath.Join(root, ".gitignore"), "node_modules/\n.ponchimap/*\n!.ponchimap/model.json\n")
	writeFile(t, filepath.Join(root, ".ponchimap", "model.json"), "{}\n")
	writeFile(t, filepath.Join(root, ".ponchimap", "findings", "hand.json"), "{}\n")
	before := snapshot(t, root)

	a := initIn(root, "--json")
	if a.code != 0 {
		t.Fatalf("init again: exit code %d\n%s", a.code, a.stderr)
	}
	if after := snapshot(t, root); !maps.Equal(before, after) {
		t.Errorf("init again changed the files:\nbefore %q\nafter  %q", before, after)
	}
	if out := machine(t, a.stdout); out.Status != "ok" || len(out.Artifacts) != 0 || len(out.Diagnostics) != 0 {
		t.Errorf("init again answers %+v, want ok with no artifact and no diagnostic", out)
	}
	if !strings.Contains(a.stderr, "line 2 ") {
		t.Errorf("the standard error does not point at line 2 of .gitignore, which names the workspace:\n%s", a.stderr)
	}
	for _, s := range skillFolders {
		if line := "kept " + filepath.Join(root, s.folder) + " "; !strings.Contains(a.stderr, line) {
			t.Errorf("the standard error has no line %q:\n%s", line, a.stderr)
		}
	}
}

func TestInitMakesOnlyWhatIsMissing(t *testing.T) {
	root := t.TempDir()
	custom := writeFile(t, filepath.Join(root, "architecture.view.yaml"), "schema_version: \"0.1\"\n")
	if err := os.Mkdir(filepath.Join(root, ".ponchimap"), 0o755); err != nil {
		t.Fatal(err)
	}
	a := initIn(root, "--json")
	if a.code != 0 {
		t.Fatalf("exit code %d\n%s", a.code, a.stderr)
	}
	if !isDir(filepath.Join(root, ".ponchimap", "findings")) {
		t.Error("the folder of the findings is not made")
	}
	if got := readFile(t, custom); got != "schema_version: \"0.1\"\n" {
		t.Errorf("the view config of the user became %q", got)
	}
	if got := patterns(readFile(t, filepath.Join(root, ".gitignore"))); !slices.Equal(got, []string{ignoreEntry}) {
		t.Errorf(".gitignore has the patterns %q, want %q", got, ignoreEntry)
	}
	if made := kinds(machine(t, a.stdout)); !slices.Equal(made, []string{"gitignore", "findings", "claude_code_skill", "codex_skill"}) {
		t.Errorf("the artifacts are %v, want the things this run made: gitignore, findings, and the Skill", made)
	}
}

func TestInitNamesWhatItMadeAsArtifacts(t *testing.T) {
	root := t.TempDir()
	a := initIn(root, "--json")
	if a.code != 0 {
		t.Fatalf("exit code %d\n%s", a.code, a.stderr)
	}
	out := machine(t, a.stdout)
	want := []struct{ kind, path string }{
		{"gitignore", filepath.Join(root, ".gitignore")},
		{"workspace", filepath.Join(root, ".ponchimap")},
		{"findings", filepath.Join(root, ".ponchimap", "findings")},
		{"view_config", filepath.Join(root, "architecture.view.yaml")},
	}
	names := slices.Sorted(maps.Keys(skillFiles(t)))
	for _, s := range skillFolders {
		for _, name := range names {
			want = append(want, struct{ kind, path string }{s.kind, filepath.Join(root, s.folder, name)})
		}
	}
	if out.Status != "ok" || len(out.Artifacts) != len(want) {
		t.Fatalf("init answers %+v, want ok with %d artifacts", out, len(want))
	}
	for i, w := range want {
		got := out.Artifacts[i]
		if got.Kind != w.kind || got.Path != w.path || !slices.Equal(got.keys, []string{"kind", "path"}) {
			t.Errorf("artifact %d is %s at %s with the members %v, want %s at %s with kind and path alone", i, got.Kind, got.Path, got.keys, w.kind, w.path)
		}
	}
}

func TestInitAddsTheIgnoreEntryOnlyWhenNoLineNamesTheWholeWorkspace(t *testing.T) {
	cases := []struct {
		name, before string
		added        bool
	}{
		{"other patterns", "node_modules/\ndist/\n", true},
		{"no newline at the end", "node_modules", true},
		{"the name in the words of a comment", "# .ponchimap/ is the workspace\n", true},
		{"another name that starts the same", ".ponchimap-notes/\n", true},
		{"the name under another folder", "docs/.ponchimap/\n", true},
		{"one file of it", ".ponchimap/report.md\n", true},
		{"one file of it let in", "!/.ponchimap/model.json\n", true},
		{"spaces before the pattern, which Git reads as a part of it", " /.ponchimap/\n", true},
		{"a tab after the pattern, which Git reads as a part of it", ".ponchimap/\t\n", true},
		{"the folder", ".ponchimap/\n", false},
		{"the folder in a file with CRLF", "node_modules/\r\n.ponchimap/\r\n", false},
		{"the name", "dist/\n.ponchimap\n", false},
		{"the folder from the root", "/.ponchimap/\n", false},
		{"the name from the root", "/.ponchimap\n", false},
		{"its files", "/.ponchimap/*\n", false},
		{"its files at any depth", "/.ponchimap/**\n", false},
		{"the folder at any depth", "**/.ponchimap/\n", false},
		{"its files, with one let in", ".ponchimap/*\n!.ponchimap/model.json\n", false},
		{"a pattern that lets all of it in", "!/.ponchimap/\n", false},
		{"the pattern commented out", "# /.ponchimap/*\n", false},
		{"the folder, with spaces after it", ".ponchimap/  \n", false},
		{"the folder after a byte order mark", "\ufeff.ponchimap/\n", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			root := t.TempDir()
			path := writeFile(t, filepath.Join(root, ".gitignore"), c.before)
			a := initIn(root, "--json")
			if a.code != 0 {
				t.Fatalf("exit code %d\n%s", a.code, a.stderr)
			}
			after := readFile(t, path)
			listed := slices.ContainsFunc(machine(t, a.stdout).Artifacts, func(m machineArtifact) bool { return m.Kind == "gitignore" })
			if listed != c.added {
				t.Errorf("the artifacts hold .gitignore: %v, want %v", listed, c.added)
			}
			if !c.added {
				if after != c.before {
					t.Errorf(".gitignore became %q, want it as it was: %q", after, c.before)
				}
				return
			}
			kept := c.before
			if !strings.HasSuffix(kept, "\n") {
				kept += "\n"
			}
			if !strings.HasPrefix(after, kept) {
				t.Fatalf(".gitignore became %q, want what was there first: %q", after, kept)
			}
			added := patterns(strings.TrimPrefix(after, kept))
			if !slices.Equal(added, []string{ignoreEntry}) {
				t.Errorf("init added the patterns %q, want %q alone", added, ignoreEntry)
			}
		})
	}
}

func TestInitWritesTheIgnoreEntryWithTheLineEndingsOfTheFile(t *testing.T) {
	root := t.TempDir()
	path := writeFile(t, filepath.Join(root, ".gitignore"), "node_modules/\r\ndist/\r\n")
	if a := initIn(root); a.code != 0 {
		t.Fatalf("exit code %d\n%s", a.code, a.stderr)
	}
	after := readFile(t, path)
	if strings.Contains(strings.ReplaceAll(after, "\r\n", ""), "\n") || !strings.HasSuffix(after, ignoreEntry+"\r\n") {
		t.Errorf(".gitignore became %q, want every line to end with CRLF and the entry last", after)
	}
}

// The configuration of the user is kept out, so that an ignore file or a
// signing key of this machine does not decide the test.
func git(t *testing.T, dir string, args ...string) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not installed")
	}
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	cmd := exec.Command("git", append([]string{"-C", dir, "-c", "user.name=test", "-c", "user.email=test@example.com"}, args...)...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return string(out)
}

func TestAfterInitGitIgnoresTheWorkspaceAndSeesTheViewConfig(t *testing.T) {
	for _, sub := range []string{".", "apps/web"} {
		t.Run(sub, func(t *testing.T) {
			repo := t.TempDir()
			git(t, repo, "init", "-q")
			root := filepath.Join(repo, sub)
			if err := os.MkdirAll(root, 0o755); err != nil {
				t.Fatal(err)
			}
			a := initIn(root)
			if a.code != 0 || strings.Contains(a.stderr, "warning") {
				t.Fatalf("exit code %d, want 0 and no warning\n%s", a.code, a.stderr)
			}
			for _, name := range []string{"model.json", "acquisitions.json", ".model.json.tmp", "findings/hand.json"} {
				writeFile(t, filepath.Join(root, ".ponchimap", name), "{}\n")
			}
			status := git(t, repo, "status", "--porcelain", "--untracked-files=all")
			if strings.Contains(status, ".ponchimap/") {
				t.Errorf("Git sees a file of the workspace:\n%s", status)
			}
			for _, file := range []string{"architecture.view.yaml", ".claude/skills/ponchimap/SKILL.md", ".agents/skills/ponchimap/SKILL.md"} {
				if !strings.Contains(status, file) {
					t.Errorf("Git does not see %s, which the user may commit:\n%s", file, status)
				}
			}
			if sub != "." && exists(filepath.Join(repo, ".gitignore")) {
				t.Error("init wrote a .gitignore outside the root it was given")
			}
		})
	}
}

func TestAfterInitALineOfTheUserLetsOneFileOfTheWorkspaceIn(t *testing.T) {
	repo := t.TempDir()
	git(t, repo, "init", "-q")
	if a := initIn(repo); a.code != 0 {
		t.Fatalf("exit code %d\n%s", a.code, a.stderr)
	}
	ignore := filepath.Join(repo, ".gitignore")
	writeFile(t, ignore, readFile(t, ignore)+"!/.ponchimap/model.json\n")
	writeFile(t, filepath.Join(repo, ".ponchimap", "model.json"), "{}\n")
	writeFile(t, filepath.Join(repo, ".ponchimap", "acquisitions.json"), "{}\n")
	status := git(t, repo, "status", "--porcelain", "--untracked-files=all")
	if !strings.Contains(status, ".ponchimap/model.json") || strings.Contains(status, "acquisitions.json") {
		t.Errorf("Git sees:\n%swant the model and not the records", status)
	}
}

func TestInitReportsTheFilesOfTheWorkspaceThatGitTracksAndLeavesThemTracked(t *testing.T) {
	repo := t.TempDir()
	git(t, repo, "init", "-q")
	writeFile(t, filepath.Join(repo, ".ponchimap", "model.json"), "{}\n")
	writeFile(t, filepath.Join(repo, ".ponchimap", "report.md"), "# report\n")
	writeFile(t, filepath.Join(repo, "README.md"), "# app\n")
	git(t, repo, "add", "-A")
	git(t, repo, "commit", "-q", "-m", "first")

	a := initIn(repo, "--json")
	if a.code != 0 {
		t.Fatalf("exit code %d, want 0: init did its part\n%s", a.code, a.stderr)
	}
	out := machine(t, a.stdout)
	d := only(t, out.Diagnostics, "init.tracked")
	if out.Status != "ok" || d.Severity != "warning" || d.Hint == "" || d.Location.Path != filepath.Join(repo, ".ponchimap") {
		t.Errorf("init answers %s with %+v, want ok with a warning at the workspace that has a hint", out.Status, d)
	}
	if !strings.Contains(d.Message, "2 files") {
		t.Errorf("the warning does not say that Git tracks 2 files: %s", d.Message)
	}
	if !strings.Contains(a.stderr, "warning init.tracked: ") || !strings.Contains(a.stderr, "\n  hint: ") {
		t.Errorf("the standard error has no init.tracked warning with its hint:\n%s", a.stderr)
	}
	tracked := git(t, repo, "ls-files", "--", ".ponchimap")
	if tracked != ".ponchimap/model.json\n.ponchimap/report.md\n" {
		t.Errorf("Git tracks %q after init, want the two files as before", tracked)
	}
	if changed := git(t, repo, "status", "--porcelain", "--", ".ponchimap"); changed != "" {
		t.Errorf("init changed what Git has of the workspace:\n%s", changed)
	}
	if !exists(filepath.Join(repo, ".ponchimap", "model.json")) {
		t.Error("the model is gone")
	}
}

func TestInitSaysNothingAboutTrackedFilesThatTheUserKeepsInGit(t *testing.T) {
	for name, ignore := range map[string]string{
		"one file let in":           "/.ponchimap/*\n!/.ponchimap/model.json\n",
		"the pattern commented out": "# /.ponchimap/*\n",
	} {
		t.Run(name, func(t *testing.T) {
			repo := t.TempDir()
			git(t, repo, "init", "-q")
			writeFile(t, filepath.Join(repo, ".gitignore"), ignore)
			writeFile(t, filepath.Join(repo, ".ponchimap", "model.json"), "{}\n")
			git(t, repo, "add", "-A")
			git(t, repo, "commit", "-q", "-m", "first")

			a := initIn(repo, "--json")
			if out := machine(t, a.stdout); a.code != 0 || out.Status != "ok" || len(out.Diagnostics) != 0 {
				t.Errorf("exit code %d and %+v, want 0 and ok without a diagnostic\n%s", a.code, out, a.stderr)
			}
			if got := readFile(t, filepath.Join(repo, ".gitignore")); got != ignore {
				t.Errorf(".gitignore became %q, want it as it was", got)
			}
		})
	}
}

func TestInitAsksGitAboutTheWorkspaceAlone(t *testing.T) {
	repo := t.TempDir()
	git(t, repo, "init", "-q")
	writeFile(t, filepath.Join(repo, "dist", "app.js"), "\n")
	git(t, repo, "add", "-A")
	git(t, repo, "commit", "-q", "-m", "first")
	writeFile(t, filepath.Join(repo, ".gitignore"), "dist/\n")

	a := initIn(repo, "--json")
	if out := machine(t, a.stdout); a.code != 0 || len(out.Diagnostics) != 0 {
		t.Errorf("exit code %d and %+v, want 0 and no diagnostic: the tracked file that is ignored is not in the workspace\n%s", a.code, out.Diagnostics, a.stderr)
	}
}

// A repository can name a program that Git runs when it lists files. init
// reads repositories of other people and must not run it.
func TestInitDoesNotRunAProgramThatTheRepositoryNames(t *testing.T) {
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip("there is no sh to write the program for")
	}
	repo := t.TempDir()
	marker := filepath.Join(t.TempDir(), "ran")
	program := writeFile(t, filepath.Join(t.TempDir(), "fsmonitor.sh"), "#!/bin/sh\n: > '"+marker+"'\n")
	if err := os.Chmod(program, 0o755); err != nil {
		t.Fatal(err)
	}
	git(t, repo, "init", "-q")
	writeFile(t, filepath.Join(repo, ".ponchimap", "model.json"), "{}\n")
	git(t, repo, "add", "-A")
	git(t, repo, "commit", "-q", "-m", "first")
	writeFile(t, filepath.Join(repo, ".gitignore"), ignoreEntry+"\n")
	git(t, repo, "config", "core.fsmonitor", program)

	git(t, repo, "ls-files", "--cached", "--ignored", "--exclude-standard")
	if !exists(marker) {
		t.Skip("this Git does not run the program when it lists files, so the test cannot tell")
	}
	if err := os.Remove(marker); err != nil {
		t.Fatal(err)
	}
	a := initIn(repo, "--json")
	only(t, machine(t, a.stdout).Diagnostics, "init.tracked")
	if exists(marker) {
		t.Error("init ran the program that core.fsmonitor of the repository names")
	}
}

func TestInitKeepsAReadOnlyGitignoreThatNamesTheWorkspace(t *testing.T) {
	root := t.TempDir()
	path := writeFile(t, filepath.Join(root, ".gitignore"), ".ponchimap/\n")
	if err := os.Chmod(path, 0o444); err != nil {
		t.Fatal(err)
	}
	a := initIn(root, "--json")
	if out := machine(t, a.stdout); a.code != 0 || out.Status != "ok" || len(out.Diagnostics) != 0 {
		t.Errorf("exit code %d and %+v, want 0 and ok without a diagnostic\n%s", a.code, out, a.stderr)
	}
}

func TestInitSaysThatItDidNotLookForTrackedFilesOnlyWhereGitIsMissing(t *testing.T) {
	const told = "tracked files were not looked for"
	t.Run("no repository", func(t *testing.T) {
		if _, err := exec.LookPath("git"); err != nil {
			t.Skip("git is not installed")
		}
		root := t.TempDir()
		// The folder for temporary files may lie in a repository.
		t.Setenv("GIT_CEILING_DIRECTORIES", filepath.Dir(root))
		a := initIn(root)
		if a.code != 0 || strings.Contains(a.stderr, told) || strings.Contains(a.stderr, "warning") {
			t.Errorf("exit code %d, want 0 and neither a line that says %q nor a warning\n%s", a.code, told, a.stderr)
		}
	})
	t.Run("no git", func(t *testing.T) {
		root := t.TempDir()
		t.Setenv("PATH", t.TempDir())
		a := initIn(root)
		if a.code != 0 || !strings.Contains(a.stderr, told) || strings.Contains(a.stderr, "warning") {
			t.Errorf("exit code %d, want 0 and a line, not a warning, that says %q\n%s", a.code, told, a.stderr)
		}
		if got := patterns(readFile(t, filepath.Join(root, ".gitignore"))); !slices.Equal(got, []string{ignoreEntry}) {
			t.Errorf(".gitignore has the patterns %q, want %q although there is no git", got, ignoreEntry)
		}
	})
}

func TestInitLeavesAGitignoreItCannotUseAndMakesTheRest(t *testing.T) {
	cases := map[string]func(t *testing.T, path string){
		"a folder": func(t *testing.T, path string) {
			if err := os.Mkdir(path, 0o755); err != nil {
				t.Fatal(err)
			}
		},
		"a symbolic link, which Git does not read": func(t *testing.T, path string) {
			shared := writeFile(t, filepath.Join(filepath.Dir(path), "shared-ignore"), "node_modules/\n")
			if err := os.Symlink(shared, path); err != nil {
				t.Skip("symbolic links cannot be made here")
			}
		},
		"a read-only file": func(t *testing.T, path string) {
			writeFile(t, path, "node_modules/\n")
			if err := os.Chmod(path, 0o444); err != nil {
				t.Fatal(err)
			}
		},
	}
	for name, prepare := range cases {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			path := filepath.Join(root, ".gitignore")
			prepare(t, path)
			before, _ := os.Lstat(path)
			a := initIn(root, "--json")
			if a.code != 0 {
				t.Fatalf("exit code %d, want 0\n%s", a.code, a.stderr)
			}
			out := machine(t, a.stdout)
			d := only(t, out.Diagnostics, "init.gitignore_unusable")
			if out.Status != "partial" || d.Severity != "warning" || d.Hint == "" || d.Location.Path != path {
				t.Errorf("init answers %s with %+v, want partial with a warning at the .gitignore that has a hint", out.Status, d)
			}
			if made := kinds(out); !slices.Equal(made, []string{"workspace", "findings", "view_config", "claude_code_skill", "codex_skill"}) {
				t.Errorf("the artifacts are %v, want the rest: workspace, findings, view_config, and the Skill", made)
			}
			if after, err := os.Lstat(path); err != nil || after.Mode() != before.Mode() {
				t.Errorf("the .gitignore is no longer what it was")
			}
			for file, content := range outsideTheSkill(snapshot(t, root)) {
				if file != "architecture.view.yaml" && strings.Contains(content, ".ponchimap") {
					t.Errorf("init wrote the entry into %s: %q", file, content)
				}
			}
		})
	}
}

func TestInitRefusesARootThatIsNoDirectory(t *testing.T) {
	dir := t.TempDir()
	file := writeFile(t, filepath.Join(dir, "file"), "")
	for name, root := range map[string]string{"a missing folder": filepath.Join(dir, "missing"), "a file": file} {
		assertRefused(t, name, initIn(root))
	}
	if exists(filepath.Join(dir, "missing")) {
		t.Error("init made the root that the command line named")
	}
	assertRefused(t, "a word after the flags", run("init", "here"))
	if a := initIn(dir); a.code != 0 {
		t.Errorf("a root that is a directory: exit code %d, want 0\n%s", a.code, a.stderr)
	}
}

func TestInitChangesNothingWhereAFileStandsInThePlaceOfAFolder(t *testing.T) {
	for _, place := range []string{".ponchimap", ".ponchimap/findings", ".claude", ".agents/skills", ".claude/skills/ponchimap"} {
		t.Run(place, func(t *testing.T) {
			root := t.TempDir()
			writeFile(t, filepath.Join(root, place), "mine\n")
			before := snapshot(t, root)
			a := initIn(root, "--json")
			if a.code != 4 {
				t.Fatalf("exit code %d, want 4\n%s", a.code, a.stderr)
			}
			out := machine(t, a.stdout)
			if d := only(t, out.Diagnostics, "output.unwritable"); out.Status != "error" || d.Location.Path != filepath.Join(root, place) || !strings.Contains(d.Message, "folder cannot be made") {
				t.Errorf("init answers %s with %+v, want error with a diagnostic about the folder at %s", out.Status, d, place)
			}
			if after := snapshot(t, root); !maps.Equal(before, after) {
				t.Errorf("init changed the files:\nbefore %q\nafter  %q", before, after)
			}
		})
	}
}

func TestInitKeepsWhateverStandsWhereTheViewConfigGoes(t *testing.T) {
	root := t.TempDir()
	elsewhere := writeFile(t, filepath.Join(t.TempDir(), "shared.view.yaml"), "schema_version: \"0.1\"\ntitle: \"Shared\"\n")
	link := filepath.Join(root, "architecture.view.yaml")
	if err := os.Symlink(elsewhere, link); err != nil {
		t.Skip("symbolic links cannot be made here")
	}
	if a := initIn(root); a.code != 0 {
		t.Fatalf("exit code %d\n%s", a.code, a.stderr)
	}
	if info, err := os.Lstat(link); err != nil || info.Mode()&os.ModeSymlink == 0 {
		t.Error("the symbolic link at the place of the view config is gone")
	}
	if got := readFile(t, elsewhere); got != "schema_version: \"0.1\"\ntitle: \"Shared\"\n" {
		t.Errorf("the file the link points to became %q", got)
	}
}

func TestInitPlacesTheSkillWhereClaudeCodeAndCodexReadIt(t *testing.T) {
	root := t.TempDir()
	a := initIn(root)
	if a.code != 0 {
		t.Fatalf("exit code %d\n%s", a.code, a.stderr)
	}
	want := skillFiles(t)
	files := snapshot(t, root)
	for _, s := range skillFolders {
		placed := map[string]string{}
		for name, content := range files {
			if rest, ok := strings.CutPrefix(name, s.folder+"/"); ok {
				placed[rest] = content
			}
		}
		if !maps.Equal(placed, want) {
			t.Errorf("%s holds %v, want the embedded Skill %v", s.folder, slices.Sorted(maps.Keys(placed)), slices.Sorted(maps.Keys(want)))
		}
		if line := "created " + filepath.Join(root, s.folder) + " "; !strings.Contains(a.stderr, line) {
			t.Errorf("the standard error has no line %q:\n%s", line, a.stderr)
		}
	}
	for _, agent := range []string{"Claude Code", "Codex"} {
		if !strings.Contains(a.lastLine(), agent) {
			t.Errorf("the last line does not say to ask %s next: %s", agent, a.lastLine())
		}
	}
}

func TestInitKeepsAnEditedSkillAndWarnsAboutItsFolderAlone(t *testing.T) {
	root := t.TempDir()
	if a := initIn(root); a.code != 0 {
		t.Fatalf("init: exit code %d\n%s", a.code, a.stderr)
	}
	edited := filepath.Join(root, ".agents", "skills", "ponchimap", "SKILL.md")
	writeFile(t, edited, "my own Skill\n")
	before := snapshot(t, root)

	a := initIn(root, "--json")
	if a.code != 0 {
		t.Fatalf("exit code %d, want 0\n%s", a.code, a.stderr)
	}
	if after := snapshot(t, root); !maps.Equal(before, after) {
		t.Errorf("init changed the files:\nbefore %q\nafter  %q", before, after)
	}
	out := machine(t, a.stdout)
	d := only(t, out.Diagnostics, "init.skill_differs")
	if out.Status != "ok" || d.Severity != "warning" || d.Hint == "" || d.Location.Path != filepath.Join(root, ".agents", "skills", "ponchimap") {
		t.Errorf("init answers %s with %+v, want ok with a warning at the folder of the Skill for Codex that has a hint", out.Status, d)
	}
	if len(out.Artifacts) != 0 {
		t.Errorf("init made %+v, want nothing", out.Artifacts)
	}
}

func TestInitPutsBackADeletedFileOfTheSkill(t *testing.T) {
	root := t.TempDir()
	if a := initIn(root); a.code != 0 {
		t.Fatalf("init: exit code %d\n%s", a.code, a.stderr)
	}
	before := snapshot(t, root)
	deleted := filepath.Join(root, ".claude", "skills", "ponchimap", "SKILL.md")
	if err := os.Remove(deleted); err != nil {
		t.Fatal(err)
	}
	a := initIn(root, "--json")
	if a.code != 0 {
		t.Fatalf("exit code %d\n%s", a.code, a.stderr)
	}
	if after := snapshot(t, root); !maps.Equal(before, after) {
		t.Errorf("init did not put the file back as it was:\nbefore %q\nafter  %q", before, after)
	}
	out := machine(t, a.stdout)
	if len(out.Artifacts) != 1 || out.Artifacts[0].Kind != "claude_code_skill" || out.Artifacts[0].Path != deleted || len(out.Diagnostics) != 0 {
		t.Errorf("init answers %+v, want the one file it put back and no diagnostic", out)
	}
}

func TestInitKeepsWhateverStandsWhereAFileOfTheSkillGoes(t *testing.T) {
	root := t.TempDir()
	elsewhere := writeFile(t, filepath.Join(t.TempDir(), "SKILL.md"), "a Skill of my own\n")
	link := filepath.Join(root, ".claude", "skills", "ponchimap", "SKILL.md")
	if err := os.MkdirAll(filepath.Dir(link), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(elsewhere, link); err != nil {
		t.Skip("symbolic links cannot be made here")
	}
	a := initIn(root, "--json")
	if out := machine(t, a.stdout); a.code != 0 || out.Status != "ok" || len(out.Diagnostics) != 0 {
		t.Errorf("exit code %d and %+v, want 0 and ok without a diagnostic\n%s", a.code, out, a.stderr)
	}
	if info, err := os.Lstat(link); err != nil || info.Mode()&os.ModeSymlink == 0 {
		t.Error("the symbolic link at the place of SKILL.md is gone")
	}
	if got := readFile(t, elsewhere); got != "a Skill of my own\n" {
		t.Errorf("the file the link points to became %q", got)
	}
}

func TestACanceledInitMakesNothing(t *testing.T) {
	root := t.TempDir()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	a := runWith(ctx, "init", "--root", root, "--json")
	if a.code != 4 {
		t.Fatalf("exit code %d, want 4\n%s", a.code, a.stderr)
	}
	only(t, machine(t, a.stdout).Diagnostics, "run.canceled")
	if files := snapshot(t, root); len(files) != 0 || exists(filepath.Join(root, ".ponchimap")) {
		t.Errorf("a canceled init left %v", files)
	}
}

// init is the first command a new user runs, so help lists it first.
func TestHelpListsInitFirst(t *testing.T) {
	help := run("help").stdout
	first, lint := strings.Index(help, "\n  init "), strings.Index(help, "\n  lint ")
	if first < 0 || lint < first {
		t.Errorf("help does not list init before lint:\n%s", help)
	}
}
