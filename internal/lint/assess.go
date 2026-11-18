package lint

import (
	"strconv"

	"github.com/ochamu/ponchimap/internal/diag"
	"github.com/ochamu/ponchimap/internal/model"
	"github.com/ochamu/ponchimap/internal/view"
)

// wording is fixed per code, so that no value of the input gets into a
// message or a hint.
type wording struct{ message, hint string }

var defectWording = map[view.DefectCode]wording{
	view.DefectNoEvidence: {
		message: "no evidence speaks for the element: it lists none, or only evidence that denies it",
		hint:    "Add evidence that states the element, or remove the element. Hiding it in the view config does not make it drawable.",
	},
	view.DefectContradictoryEvidence: {
		message: "the evidence both states and denies the relation of its edge",
		hint:    "Give the evidence one reading: correct the value it supports, or split it into the sources it stands for.",
	},
	view.DefectRelationEvidence: {
		message: "the direct evidence backs another kind of relation than the edge's",
		hint:    "Correct the relation of the edge, or the key the evidence supports: configuration is backed by connection.configured or connection.target, implementation by call.implemented, observation by traffic.observed, dependency by lifecycle.requires.",
	},
}

var findingWording = map[view.FindingCode]wording{
	view.UnverifiedNotChecked: {
		message: "the declaration is not checked against the real environment",
		hint:    "Query the real environment and add what it shows as deployed evidence. Until then the figure marks the element as unverified.",
	},
	view.UnverifiedSuperseded: {
		message: "a later query of the real environment did not finish, so the earlier confirmation is not counted",
		hint:    "Query the real environment again. The coverage record of the query that did not finish says why it stopped.",
	},
	view.UnverifiedAbsenceUnbacked: {
		message: "evidence says this is absent, and no completed coverage record of the same acquisition backs it",
		hint:    "Give the evidence and the coverage record of that query the same acquisition_id, and record the coverage as complete only when its whole scope was read.",
	},
	view.UnverifiedStatedOnly: {
		message: "the relation rests on a document or a statement only",
		hint:    "Add evidence from a declaration, the code, or the real environment. Until then the figure marks the relation as unverified.",
	},
	view.MismatchValues: {
		message: "comparable sources give different values",
		hint:    "Compare the evidence of the element, and correct the source that is out of date. Both values are kept, and the figure marks the element as a mismatch.",
	},
	view.MismatchAbsent: {
		message: "what is declared was not found by a completed query of the real environment",
		hint:    "Deploy it, or remove the declaration. If the query read another scope, correct its coverage record.",
	},
}

func wordingOfFinding(f view.Finding) wording {
	w := findingWording[f.Code]
	// The key is one that the schema defines, so it is not a value of the
	// input. The slot, the evidence, and the coverage of a finding are.
	if f.Key != "" {
		w.message += " (" + f.Key + ")"
	}
	return w
}

// assessed adds only the places and the words to what view.Assess derives:
// which evidence backs which element is decided there alone.
func assessed(m *model.Model, at func(pointer string) diag.Location) diag.List {
	return located(view.Assess(m), pointersOf(m), at)
}

// One map holds the nodes, the edges, and their evidence: the IDs of a model
// share one namespace, and the loader refuses an ID declared twice.
func pointersOf(m *model.Model) map[string]string {
	pointers := map[string]string{}
	for _, e := range elements(m) {
		pointers[e.id] = e.pointer
		for j, ev := range e.evidence {
			pointers[ev.ID] = e.pointer + "/evidence/" + strconv.Itoa(j)
		}
	}
	return pointers
}

func located(a *view.Assessment, pointers map[string]string, at func(pointer string) diag.Location) diag.List {
	place := func(id, below string) diag.Location { return at(pointers[id] + below) }
	var list diag.List
	for _, d := range a.Defects {
		w := defectWording[d.Code]
		// A defect that names no evidence is about evidence the element
		// lacks, so it is shown where the element lists its evidence.
		if len(d.Evidence) == 0 {
			list = append(list, diag.Diagnostic{Severity: diag.Error, Code: string(d.Code), TargetID: d.ID,
				Location: place(d.ID, "/evidence"), Message: w.message, Hint: w.hint})
		}
		for _, id := range d.Evidence {
			list = append(list, diag.Diagnostic{Severity: diag.Error, Code: string(d.Code), TargetID: id,
				Location: place(id, ""), Message: w.message, Hint: w.hint})
		}
	}
	for _, f := range a.Findings {
		w := wordingOfFinding(f)
		// The first element is the one view says to point at.
		about := ""
		if len(f.About) > 0 {
			about = f.About[0]
		}
		list = append(list, diag.Diagnostic{Severity: diag.Warning, Code: string(f.Code), TargetID: about,
			Location: place(about, ""), Message: w.message, Hint: w.hint})
	}
	return list
}
