// Package workspace saves the files a command leaves in the user's repository.
package workspace

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
)

// File is one file as it must be after Save: Data is its whole content, and a
// nil Data means the file must not exist.
type File struct {
	Path string
	Data []byte
}

// Save puts the files in place, in the order given. A target where something
// other than a regular file stands, such as a directory or a symbolic link,
// is refused. The content of every file is first written to a temporary file
// next to its target. When a target is refused or a content cannot be
// written, no target has changed: the files that an earlier run saved stay as
// they were. Then the temporary files are renamed over their targets one by
// one, which is not atomic across files: when a rename or a removal fails,
// the files before it in the order are replaced and the rest are not, so a
// caller puts the file that records the others last.
//
// Saving the same files again gives the same state. A failed Save removes its
// temporary files and leaves the directories it made. A killed one leaves
// temporary files, which the next Save of the same paths replaces. A path
// stands once in the files. An empty path is refused like a target that is
// no regular file. The error is an *Error.
func Save(files []File) error {
	for _, f := range files {
		if err := blocked(f.Path); err != nil {
			return failed(f, err)
		}
	}
	for _, f := range files {
		if f.Data == nil {
			continue
		}
		if err := os.MkdirAll(filepath.Dir(f.Path), 0o755); err != nil {
			return failed(f, err)
		}
	}
	for i, f := range files {
		if f.Data == nil {
			continue
		}
		if err := Create(temporary(f.Path), f.Data); err != nil {
			discard(files[:i+1])
			return failed(f, err)
		}
	}
	for i, f := range files {
		var err error
		if f.Data != nil {
			err = os.Rename(temporary(f.Path), f.Path)
		} else if err = os.Remove(f.Path); errors.Is(err, fs.ErrNotExist) {
			err = nil
		}
		if err != nil {
			discard(files[i:])
			return failed(f, err)
		}
	}
	return nil
}

var (
	// ErrNotRegular is why Save refuses a target, and why Init leaves a
	// .gitignore alone: a directory, a symbolic link, or another thing that
	// is no regular file stands at the path.
	ErrNotRegular = errors.New("is not a regular file")
	errEmptyPath  = errors.New("is an empty path")
)

// blocked says why no file can be put at path or removed from it, or nothing.
// A directory at a target fails the rename only after the files before it
// were replaced, so every target is looked at before anything is written.
func blocked(path string) error {
	if path == "" {
		// Lstat calls an empty path a file that does not exist.
		return errEmptyPath
	}
	info, err := os.Lstat(path)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return nil
	case err != nil:
		return err
	case !info.Mode().IsRegular():
		return ErrNotRegular
	}
	return nil
}

// temporary is where the content for path waits until it is renamed over
// path. It is in the directory of path, so that the rename stays on one file
// system, and it has one name per path, so that the file a killed run left is
// replaced by the next run and does not stay for good.
func temporary(path string) string {
	return filepath.Join(filepath.Dir(path), "."+filepath.Base(path)+".tmp")
}

// Create makes the file at path anew with data, for a caller that then
// renames it over its target. A repository can hold anything under a name
// that a command writes to. Whatever is there is removed and the file is made
// anew, never opened: content written to a symbolic link would land in the
// file that it points to.
func Create(path string, data []byte) error {
	os.Remove(path)
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return err
	}
	_, err = f.Write(data)
	if err == nil {
		err = f.Sync()
	}
	if closeErr := f.Close(); err == nil {
		err = closeErr
	}
	return err
}

// discard removes the temporary files of the files.
func discard(files []File) {
	for _, f := range files {
		if f.Data != nil {
			os.Remove(temporary(f.Path))
		}
	}
}

// Error says which file could not be saved. Err is the cause without the
// path (the Err of an *fs.PathError or *os.LinkError), so that a caller can
// show the path once.
type Error struct {
	Path string
	Err  error
}

func (e *Error) Error() string { return e.Path + ": " + e.Err.Error() }

func (e *Error) Unwrap() error { return e.Err }

func failed(f File, err error) *Error {
	var pathErr *fs.PathError
	var linkErr *os.LinkError
	switch {
	case errors.As(err, &pathErr):
		err = pathErr.Err
	case errors.As(err, &linkErr):
		err = linkErr.Err
	}
	return &Error{Path: f.Path, Err: err}
}
