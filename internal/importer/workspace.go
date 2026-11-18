package importer

import (
	"bytes"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/ochamu/ponchimap/internal/diag"
	saving "github.com/ochamu/ponchimap/internal/workspace"
)

const (
	recordsName = "acquisitions.json"
	pendingName = recordsName + ".next"
)

type workspace struct {
	folder               string
	records              []record
	current              []byte
	pending, readPending bool
}

func (w *workspace) path(name string) string { return filepath.Join(w.folder, name) }

// A run that stopped after it wrote the model left the records of that model
// in pendingName, and they are read from there. A run that stopped before it
// wrote the model left a pendingName that belongs to a model that never came
// to be, and it is ignored.
func (w *workspace) read(model []byte) diag.List {
	name := recordsName
	pending, err := os.ReadFile(w.path(pendingName))
	w.pending = err == nil
	var header struct {
		ModelDigest string `json:"model"`
	}
	if w.pending && model != nil && json.Unmarshal(pending, &header) == nil && header.ModelDigest == digestOf(model) {
		name, w.readPending = pendingName, true
	}
	text, err := os.ReadFile(w.path(name))
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	file, parseErr := parseRecords(text)
	if err != nil || parseErr != nil {
		return diag.List{recordsUnreadable.at(diag.Location{Path: w.path(name)})}
	}
	w.records, w.current = file.Acquisitions, text
	return nil
}

// save writes the records to pendingName, then the model, then renames
// pendingName, so that a run that stops between two of the steps leaves a
// state that read tells apart.
func (w *workspace) save(modelPath string, oldModel, newModel []byte, records []record) diag.List {
	failed := func(path string, err error) diag.List {
		d := writeFailed.at(diag.Location{Path: path})
		d.Message += ": " + reason(err)
		return diag.List{d}
	}
	text := recordsFile{SchemaVersion: recordsVersion, ModelDigest: digestOf(newModel), Acquisitions: records}.text()
	sameModel := bytes.Equal(newModel, oldModel)
	switch {
	case sameModel && bytes.Equal(text, w.current) && !w.pending:
		return nil
	case sameModel && bytes.Equal(text, w.current) && !w.readPending:
		if err := os.Remove(w.path(pendingName)); err != nil {
			return failed(w.path(pendingName), err)
		}
		return nil
	case sameModel && bytes.Equal(text, w.current):
	default:
		if err := replaceFile(w.path(pendingName), text); err != nil {
			return failed(w.path(pendingName), err)
		}
		if !sameModel {
			// The model is saved as every command saves a file for the user: a
			// directory or a symbolic link at its path is refused.
			if err := saving.Save([]saving.File{{Path: modelPath, Data: newModel}}); err != nil {
				return failed(modelPath, err)
			}
		}
	}
	if err := os.Rename(w.path(pendingName), w.path(recordsName)); err != nil {
		return failed(w.path(recordsName), err)
	}
	return nil
}

func replaceFile(path string, text []byte) error {
	partial := path + ".tmp"
	err := saving.Create(partial, text)
	if err == nil {
		err = os.Rename(partial, path)
	}
	if err != nil {
		os.Remove(partial)
	}
	return err
}
