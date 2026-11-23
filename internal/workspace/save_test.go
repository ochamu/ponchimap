package workspace

import (
	"errors"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// seed writes the files that stand under root before a Save: the path of
// each from root, and its content.
func seed(t *testing.T, root string, files map[string]string) {
	t.Helper()
	for path, content := range files {
		path = filepath.Join(root, path)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
}

// tree reads every file under root: the path of each from root, and its
// content. A temporary file that a Save left would be one of them.
func tree(t *testing.T, root string) map[string]string {
	t.Helper()
	files := map[string]string{}
	under := os.DirFS(root)
	err := fs.WalkDir(under, ".", func(path string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return err
		}
		data, err := fs.ReadFile(under, path)
		files[path] = string(data)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return files
}

func holds(t *testing.T, root string, want map[string]string) {
	t.Helper()
	if got := tree(t, root); !maps.Equal(got, want) {
		t.Errorf("the files are %q, want %q", got, want)
	}
}

// failedAt checks that err is the *Error of path, and that its cause names
// no path of the test: a caller shows the path itself.
func failedAt(t *testing.T, err error, path, root string) {
	t.Helper()
	var failed *Error
	if !errors.As(err, &failed) {
		t.Fatalf("Save returned %v, want an *Error", err)
	}
	if failed.Path != path {
		t.Errorf("the error names %q, want %q", failed.Path, path)
	}
	if strings.Contains(failed.Err.Error(), root) {
		t.Errorf("the cause %q names a path", failed.Err)
	}
	if !errors.Is(err, failed.Err) {
		t.Errorf("the error does not unwrap to its cause %q", failed.Err)
	}
}

func TestSaveWritesTheFilesAndMakesTheirDirectories(t *testing.T) {
	root := t.TempDir()
	seed(t, root, map[string]string{"out/overview.svg": "old"})
	files := []File{
		{Path: filepath.Join(root, "out/overview.svg"), Data: []byte("<svg/>")},
		{Path: filepath.Join(root, "docs/infra/report.md"), Data: []byte("# Report\n")},
		{Path: filepath.Join(root, "empty.json"), Data: []byte{}},
	}
	if err := Save(files); err != nil {
		t.Fatal(err)
	}
	holds(t, root, map[string]string{
		"out/overview.svg":     "<svg/>",
		"docs/infra/report.md": "# Report\n",
		"empty.json":           "",
	})
}

func TestSavePreservesExistingModesAndCreatesFilesWith0644(t *testing.T) {
	t.Chdir(t.TempDir())
	seed(t, ".", map[string]string{"private": "old", "shared": "old"})
	if err := os.Chmod("shared", 0o660); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile("with-umask", nil, 0o644); err != nil {
		t.Fatal(err)
	}
	for path, reference := range map[string]string{"private": "private", "shared": "shared", "new": "with-umask"} {
		want, err := os.Stat(reference)
		if err != nil {
			t.Fatal(err)
		}
		if err := Save([]File{{Path: path, Data: []byte("new")}}); err != nil {
			t.Fatal(err)
		}
		got, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if got.Mode().Perm() != want.Mode().Perm() {
			t.Errorf("%s has mode %v, want %v", path, got.Mode().Perm(), want.Mode().Perm())
		}
	}
}

func TestSaveTakesPathsFromTheWorkingDirectory(t *testing.T) {
	t.Chdir(t.TempDir())
	err := Save([]File{
		{Path: "overview.svg", Data: []byte("<svg/>")},
		{Path: filepath.Join("out", "report.md"), Data: []byte("# Report\n")},
	})
	if err != nil {
		t.Fatal(err)
	}
	holds(t, ".", map[string]string{"overview.svg": "<svg/>", "out/report.md": "# Report\n"})
}

func TestSaveRemovesAFileWhoseDataIsNil(t *testing.T) {
	root := t.TempDir()
	seed(t, root, map[string]string{"overview.svg": "old", "overview.png": "old"})
	err := Save([]File{
		{Path: filepath.Join(root, "overview.svg"), Data: []byte("new")},
		{Path: filepath.Join(root, "overview.png")},
		{Path: filepath.Join(root, "never/saved.png")},
	})
	if err != nil {
		t.Fatal(err)
	}
	holds(t, root, map[string]string{"overview.svg": "new"})
}

func TestSavingTheSameFilesAgainLeavesTheSameFiles(t *testing.T) {
	root := t.TempDir()
	seed(t, root, map[string]string{"overview.png": "old"})
	files := []File{
		{Path: filepath.Join(root, "overview.svg"), Data: []byte("<svg/>")},
		{Path: filepath.Join(root, "overview.png")},
		{Path: filepath.Join(root, "out/report.md"), Data: []byte("{}\n")},
	}
	for range 2 {
		if err := Save(files); err != nil {
			t.Fatal(err)
		}
		holds(t, root, map[string]string{"overview.svg": "<svg/>", "out/report.md": "{}\n"})
	}
}

func TestSaveReplacesTheTemporaryFileOfAKilledRun(t *testing.T) {
	root := t.TempDir()
	seed(t, root, map[string]string{"overview.svg": "old", ".overview.svg.tmp": "longer than the new content"})
	err := Save([]File{{Path: filepath.Join(root, "overview.svg"), Data: []byte("new")}})
	if err != nil {
		t.Fatal(err)
	}
	holds(t, root, map[string]string{"overview.svg": "new"})
}

func TestSaveDoesNotWriteThroughALinkAtTheTemporaryName(t *testing.T) {
	root := t.TempDir()
	seed(t, root, map[string]string{"elsewhere": "kept"})
	if err := os.Symlink("elsewhere", filepath.Join(root, ".overview.svg.tmp")); err != nil {
		t.Skip("the test needs a symbolic link:", err)
	}
	err := Save([]File{{Path: filepath.Join(root, "overview.svg"), Data: []byte("new")}})
	if err != nil {
		t.Fatal(err)
	}
	holds(t, root, map[string]string{"overview.svg": "new", "elsewhere": "kept"})
}

func TestSaveRefusesAnEmptyPathBeforeItChangesAnything(t *testing.T) {
	for name, unnamed := range map[string]File{
		"a file to write":  {Data: []byte("new")},
		"a file to remove": {},
	} {
		for at := range 4 {
			root := t.TempDir()
			// A temporary file for an empty path would lie in the working
			// directory.
			t.Chdir(root)
			seed(t, root, map[string]string{"overview.svg": "old", "overview.png": "old"})
			before := tree(t, root)
			files := slices.Insert([]File{
				{Path: "overview.svg", Data: []byte("new")},
				{Path: "overview.png"},
				{Path: filepath.Join("new", "report.md"), Data: []byte("new")},
			}, at, unnamed)
			err := Save(files)
			if err == nil {
				t.Errorf("%s at %d: Save returned no error", name, at)
			} else {
				failedAt(t, err, "", root)
			}
			holds(t, root, before)
			if _, err := os.Stat("new"); !errors.Is(err, fs.ErrNotExist) {
				t.Errorf("%s at %d: Save made a directory", name, at)
			}
		}
	}
}

func TestSaveChangesNoFileWhenOneCannotBeSaved(t *testing.T) {
	content := []byte("new")
	for name, unsaved := range map[string]File{
		"its directory is a file":                       {Path: "kept.txt/report.md", Data: content},
		"it is removed from a directory that is a file": {Path: "kept.txt/report.md"},
		"its name is too long for a file":               {Path: "out/" + strings.Repeat("n", 300), Data: content},
		"a directory stands at its temporary name":      {Path: "report.md", Data: content},
		"it is a directory":                             {Path: "held", Data: content},
		"it is a directory to remove":                   {Path: "held"},
		"it is a symbolic link":                         {Path: "link.svg", Data: content},
	} {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			seed(t, root, map[string]string{
				"overview.svg":        "old",
				"overview.png":        "old",
				"kept.txt":            "old",
				".report.md.tmp/held": "old",
				"held/file":           "old",
			})
			if err := os.Symlink("kept.txt", filepath.Join(root, "link.svg")); err != nil && unsaved.Path == "link.svg" {
				t.Skip("the case needs a symbolic link:", err)
			}
			before := tree(t, root)
			unsaved.Path = filepath.Join(root, unsaved.Path)
			err := Save([]File{
				{Path: filepath.Join(root, "overview.svg"), Data: content},
				{Path: filepath.Join(root, "overview.png")},
				unsaved,
				{Path: filepath.Join(root, "new/report.md"), Data: content},
			})
			failedAt(t, err, unsaved.Path, root)
			holds(t, root, before)
		})
	}
}
