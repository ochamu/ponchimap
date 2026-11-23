package workspace

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// SamePath also compares files that do not yet exist, by the directories
// that contain them.
func SamePath(first, second string) bool { return locate(first).is(locate(second)) }

// CheckPaths rejects outputs that alias another path or pass through a
// broken or cyclic link. Callers include temporary files in outputs and
// omit unused inputs or supply an empty path for them.
func CheckPaths(inputs, outputs []string) error {
	writes := make([]location, len(outputs))
	for i, path := range outputs {
		writes[i] = entry(path)
		if writes[i].existing == nil {
			return fmt.Errorf("cannot resolve output path %q", path)
		}
		for j, other := range writes[:i] {
			if writes[i].is(other) {
				return fmt.Errorf("output paths %q and %q refer to the same file", outputs[j], path)
			}
		}
	}
	for _, path := range inputs {
		if path == "" {
			continue
		}
		// A write harms an input that is a link in two places: the file the
		// link leads to, and the link itself.
		for _, read := range []location{locate(path), entry(path)} {
			for i, write := range writes {
				if read.is(write) {
					return fmt.Errorf("input %q and output %q refer to the same file", path, outputs[i])
				}
			}
		}
	}
	return nil
}

type location struct {
	existing fs.FileInfo
	rest     string
}

// entry is where a write to path lands. Save never follows a link at the
// last element of a path: it refuses one at a target and removes one at a
// temporary name. So a link there is a name in its directory, whether or not
// it leads anywhere.
func entry(path string) location {
	if info, err := os.Lstat(path); err != nil || info.Mode()&fs.ModeSymlink == 0 {
		return locate(path)
	}
	parent, name := split(path)
	at := locate(parent)
	at.rest = filepath.Join(at.rest, name)
	return at
}

func locate(path string) location {
	var rest string
	for {
		if info, err := os.Stat(path); err == nil {
			return location{info, rest}
		}
		if _, err := os.Lstat(path); err == nil {
			// Nothing can be made below a dangling or cyclic link.
			return location{}
		}
		parent, name := split(path)
		if parent == path {
			return location{}
		}
		rest = filepath.Join(name, rest)
		path = parent
	}
}

// split keeps the parent as it is written. Dir and Join clean away "link/.."
// before Stat can see which directory actually contains the file.
func split(path string) (parent, name string) {
	parent, name = filepath.Split(path)
	if parent == "" {
		parent = "."
	} else if len(parent) > len(filepath.VolumeName(parent))+1 {
		parent = parent[:len(parent)-1]
	}
	return parent, name
}

func (l location) is(other location) bool {
	// A missing name may be created on a case-insensitive file system.
	return os.SameFile(l.existing, other.existing) && strings.EqualFold(l.rest, other.rest)
}
