package importer

import (
	"errors"

	"github.com/ochamu/ponchimap/internal/diag"
	"github.com/ochamu/ponchimap/internal/input"
)

// No text of a notice takes a value of an input.
type notice struct {
	severity      diag.Severity
	code          string
	message, hint string
}

func (n notice) at(where diag.Location) diag.Diagnostic {
	return diag.Diagnostic{Severity: n.severity, Code: n.code, Message: n.message, Location: where, Hint: n.hint}
}

func (n notice) about(where diag.Location, id string) diag.Diagnostic {
	d := n.at(where)
	d.TargetID = id
	return d
}

var (
	unreadableInput = notice{diag.Error, "tool.defect",
		"an input passed every check but the tool could not read it; this is a defect of the tool, please report it",
		"Report the defect with an input that causes it."}
	brokenModel = notice{diag.Error, "tool.defect",
		"the model that import built does not pass the checks of a model; this is a defect of the tool, please report it",
		"Report the defect with the inputs that cause it."}

	noTarget = notice{diag.Error, "import.no_target",
		"the model file does not exist, and no target was given for a new model",
		"Give the repository and the environment that the new model is about, or name a model file that exists."}
	noEnvironment = notice{diag.Error, "import.no_target",
		"the model file does not exist, and the findings do not state one environment for a new model to be about",
		"Give the environment that the new model is about, or name a model file that exists. Without one, import takes the environment that the findings state as their target, when all of them state the same."}
	duplicateID = notice{diag.Error, "import.duplicate_id",
		"another candidate of this file already has this ID",
		"Give each candidate node, edge, evidence item, and coverage record of a findings file an ID of its own."}
	unknownNode = notice{diag.Error, "import.unknown_reference",
		"no candidate node of this file has this ID",
		"Name a candidate node of the same file. A findings file cannot name an ID of the model; describe a node that the model has by its identity."}
	unknownElement = notice{diag.Error, "import.unknown_reference",
		"no candidate node or edge of this file has this ID",
		"Name a candidate node or edge of the same file. A findings file cannot name an ID of the model; describe a node that the model has by its identity."}
	duplicateAcquisition = notice{diag.Error, "import.duplicate_acquisition",
		"another input of this run has the same acquisition ID",
		"An acquisition ID names one read. Pass each findings file once, and give each read an ID of its own."}
	acquisitionChanged = notice{diag.Error, "import.acquisition_changed",
		"the acquisition records hold this acquisition ID with another content",
		"An acquisition ID names one read. Give the new read a new ID and the same source."}
	noEvidence = notice{diag.Error, "import.no_evidence",
		"the candidate matches no element of the model and carries no evidence",
		"A candidate without evidence only names an element that the model already has. Add evidence, or give the candidate the identity of that element."}
	recordsUnreadable = notice{diag.Error, "import.records_unreadable",
		"the acquisition records cannot be read",
		"Restore the file, or remove it. Without it, import treats the evidence of earlier acquisitions as written by hand, and a later read does not replace it."}
	writeFailed = notice{diag.Error, "import.write_failed",
		"the file could not be written",
		"Check that the folder exists and can be written, then run the import again."}

	noFindings = notice{diag.Warning, "import.no_findings",
		"the folder holds no *.json file",
		"Put the findings files directly into the folder, or pass their paths."}
	incompleteAcquisition = notice{diag.Warning, "import.incomplete_acquisition",
		"the acquisition did not finish; the earlier evidence of its source that it does not restate stays in the model",
		"Run the investigation again when it can finish."}
	reported = notice{diag.Warning, "import.reported",
		"the producer of the findings reported a problem; its text stands at this place of the input",
		"Read the entry in the findings file."}
	unknownTarget = notice{diag.Warning, "import.unknown_target",
		"the statement names an ID that the model does not have, so the statement was left out",
		"Name a node or an edge of the model, or remove the statement."}
	ambiguousIdentity = notice{diag.Warning, "import.ambiguous_identity",
		"more than one node fits the candidate, so it was joined to none of them",
		"Give the candidate the native_id or the owner, account_id or project_id, that tells the nodes apart."}
	duplicateIdentity = notice{diag.Warning, "import.duplicate_identity",
		"the node has the identity of an earlier node of the model; import does not merge the two, and a candidate goes to the one that has its key, or else to the one whose identity it fits",
		"Remove one of the two nodes, or give them identities that differ."}
	identityConflict = notice{diag.Warning, "import.identity_conflict",
		"the candidate gives a key of the identity another value than the node has; the node keeps the value it had",
		"Check which value is right, and correct the source of the other."}
	removed = notice{diag.Warning, "import.removed",
		"the element lost all its evidence and left the model",
		"An entry of the view config that names the ID gets a warning at the next render. Remove the entry."}
	keptSuperseded = notice{diag.Warning, "import.kept_superseded",
		"the newer read of the source no longer states the element; it stays with the evidence it had, because another element or the target scope names it",
		"Remove what names the element if the element is gone, or check why the newer read does not state it."}
	disputed = map[string]notice{
		"role": {diag.Warning, "import.attribute_disputed",
			"the evidence gives more than one value for role, so attributes.role was left out",
			"Check which value is right. The figure shows the values side by side as a mismatch."},
		"region": {diag.Warning, "import.attribute_disputed",
			"the evidence gives more than one value for region, so attributes.region was left out",
			"Check which value is right. The figure shows the values side by side as a mismatch."},
	}
)

func unreadable(what, path string, err error) diag.Diagnostic {
	return diag.Diagnostic{Severity: diag.Error, Code: "input.unreadable", Location: diag.Location{Path: path},
		Message: "the " + what + " cannot be read: " + reason(err), Hint: "Check that the path names a " + what + " that can be read."}
}

// reason is the text of an error of the file system without the path, which
// the location of the diagnostic holds already.
func reason(err error) string {
	if inner := errors.Unwrap(err); inner != nil {
		err = inner
	}
	return err.Error()
}

func locationIn(path string, doc *input.Document, pointer string) diag.Location {
	at := doc.Locate(pointer, false)
	at.Path = path
	return at
}
