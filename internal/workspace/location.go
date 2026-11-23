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
		writes[i] = locate(path)
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
		read := locate(path)
		for i, write := range writes {
			if read.is(write) {
				return fmt.Errorf("input %q and output %q refer to the same file", path, outputs[i])
			}
		}
	}
	return nil
}

type location struct {
	existing fs.FileInfo
	rest     string
}

func locate(path string) location {
	var rest string
	for {
		if info, err := os.Stat(path); err == nil {
			return location{info, rest}
		}
		if _, err := os.Lstat(path); err == nil {
			// A dangling or cyclic link is not a place to create a file.
			return location{}
		}
		// Dir and Join clean away "link/.." before Stat can see which
		// directory actually contains the missing file.
		parent, name := filepath.Split(path)
		if parent == "" {
			parent = "."
		} else if len(parent) > len(filepath.VolumeName(parent))+1 {
			parent = parent[:len(parent)-1]
		}
		if parent == path {
			return location{}
		}
		rest = filepath.Join(name, rest)
		path = parent
	}
}

func (l location) is(other location) bool {
	// A missing name may be created on a case-insensitive file system.
	return os.SameFile(l.existing, other.existing) && strings.EqualFold(l.rest, other.rest)
}
