package workspace

// Where a command reads and saves when the user names no path, relative to
// the directory the command runs in.
const (
	// Folder is the workspace: what the tool keeps for itself, out of Git.
	Folder = ".ponchimap"
	// Import reads every *.json file of DefaultFindings, and
	// DefaultSupplements when it exists.
	DefaultFindings    = ".ponchimap/findings"
	DefaultSupplements = ".ponchimap/supplements.yaml"

	DefaultModel  = ".ponchimap/model.json"
	DefaultView   = "architecture.view.yaml"
	DefaultReport = ".ponchimap/report.md"
	// The figure is saved in DefaultFigureDir as FigureBaseName with .svg and
	// .png after it.
	DefaultFigureDir = "docs/architecture"
	FigureBaseName   = "overview"
)
