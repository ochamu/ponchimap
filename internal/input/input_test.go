package input_test

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/ochamu/ponchimap/internal/input"
)

// The location names the file, so the message leaves the path out.
func TestAFileThatCannotBeReadIsOneDiagnostic(t *testing.T) {
	path := filepath.Join(t.TempDir(), "missing.json")
	raw, diags := input.ReadFile(path)
	if raw != nil || len(diags) != 1 || diags[0].Code != "input.unreadable" {
		t.Fatalf("got %d bytes and %v, want no text and one input.unreadable", len(raw), diags)
	}
	if d := diags[0]; d.Location.Path != path || d.Hint == "" || strings.Contains(d.Message, path) {
		t.Errorf("diagnostic %v with the hint %q, want the path in the location only, and a hint", d, d.Hint)
	}
}
