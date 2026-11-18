# Cases for the unverified and mismatch states

Each `NAME.json` is a model, and `NAME.want` lists what `view.Assess` must say about it. The tests in `internal/view` compare the two. Every model loads without a diagnostic, and each one is small enough to try a single rule of the doc comment of `view.Assess` in `internal/view/assess.go`.

A line of a `.want` file is one finding or one defect, in six fields separated by tabs:

    CODE  ABOUT  KEY  SLOT  EVIDENCE  COVERAGE

- CODE is the code of the finding (`unverified.not_checked`, `mismatch.values`, and so on) or of the defect (`model.no_evidence`, and so on).
- ABOUT lists the IDs of the elements that carry the state, separated by commas, in the order of the finding. For a defect it is the one element.
- KEY is the support key of the unit, and SLOT the slot of a connection.
- EVIDENCE lists the evidence IDs the state rests on, and COVERAGE the IDs of the coverage records that apply to the unit. Both are in the order of the model: nodes before edges, and the items of one element in the order they are written. For a defect, EVIDENCE is the evidence at fault.
- `-` stands for an empty field.

The lines of a file are sorted, and the test compares them with the sorted lines of the result. A model without a finding or a defect has an empty `.want` file.

The expected lines were derived by hand from those rules and the models, not from the output of `view.Assess`. To add a case, derive its lines by hand before running the test, and edit a model by hand when a case changes.
