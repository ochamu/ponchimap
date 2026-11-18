package lint

import (
	"testing"

	"github.com/ochamu/ponchimap/internal/diag"
)

// Every way a user can make view.Build fail has a diagnostic of its own, so
// no input of Check gives tool.defect. The error is made here instead.
func TestAnErrorOfBuildIsADefectOfTheTool(t *testing.T) {
	b := built{in: Input{Model: Source{Path: "model.json"}, View: Source{Path: "view.yaml"}}}
	d := b.failure()
	if d.Severity != diag.Error || d.Code != "tool.defect" || d.Location != (diag.Location{Path: "model.json"}) || d.TargetID != "" || d.Hint == "" {
		t.Errorf("got %+v, want an error tool.defect with a hint that names only the model file", d)
	}
}
