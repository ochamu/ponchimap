package lint

import (
	"github.com/ochamu/ponchimap/internal/diag"
	"github.com/ochamu/ponchimap/internal/input"
	"github.com/ochamu/ponchimap/internal/model"
	"github.com/ochamu/ponchimap/internal/view"
)

// built is what view.Build is given, and the documents its findings are
// placed in.
type built struct {
	in        Input
	m         *model.Model
	modelDoc  *input.Document
	configDoc *input.Document
}

func (b built) inModel(pointer string) diag.Location {
	at := b.modelDoc.Locate(pointer, false)
	at.Path = b.in.Model.Path
	return at
}

func (b built) inConfig(pointer string) diag.Location {
	at := b.configDoc.Locate(pointer, false)
	at.Path = b.in.View.Path
	return at
}

// failure is what an input gives when view.Build refuses it: the input got
// past the checks that were to catch its cause, so it is a defect of the tool.
// The text of the error of Build may name IDs of the input and is not shown.
func (b built) failure() diag.Diagnostic {
	d := toolDefect
	d.Location.Path = b.in.Model.Path
	return d
}

// warnings places what view.Build drew otherwise than asked. A warning about
// an entry of the view config has the pointer of the entry. A warning about
// the model alone has none and names a node, so it is shown at the node in
// the model. So is any warning when no view config was given: the place it
// points at is in no file.
func (b built) warnings(s *view.Scene) diag.List {
	nodes := pointersOf(b.m)
	var list diag.List
	for _, d := range s.Warnings {
		if d.Location.Pointer != "" && b.in.View.given() {
			d.Location = b.inConfig(d.Location.Pointer)
		} else {
			d.Location = b.inModel(nodes[d.TargetID])
		}
		list = append(list, d)
	}
	return list
}
