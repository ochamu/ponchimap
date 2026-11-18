# Lint cases

Each directory `NAME` is one input of `lint.Check`: a `model.json`, a `view.yaml` when the case has a view config, and `want`, which lists the diagnostics the input must give, in order. The tests in `internal/lint` call `lint.Check` with the paths of the two files and compare the whole list with `want`. `Checked` must be returned exactly when `want` has no error line.

A line of a `want` file has seven fields separated by tabs:

    SEVERITY  FILE  LINE:COLUMN  CODE  POINTER  TARGET_ID  CONTAINS

FILE is `model` or `view`, the file the diagnostic names. `-` stands for the empty pointer, for no target ID, for no required text, and, in LINE:COLUMN, for no position. CONTAINS is a text the message must contain. Every diagnostic must have a hint.

The lines were derived by hand, not from the output of the tool: the lines of the loaders from `schema/model.schema.json`, `schema/view.schema.json`, and what `model.Parse`, `view.ParseConfig` and `view.Resolve` check beyond them, and the lines of `lint` from what `lint.Check` adds. To add a case, derive its lines by hand before running the test, and keep one member on a line where a position matters.

Outside the cases about evidence below, the nodes are of kind `service` with `code` evidence of `call.implemented: true`, and the edges are of relation `implementation`, so that every element is backed and a case shows only what it is about.

## Cases about evidence

Eleven cases show what `lint` reports from `view.Assess`. The model of each is a copy of a case of `testdata/marks/`. The `.want` file of that case says what `view.Assess` gives, so it decides the code of each line and the element or the evidence the line is about. The place comes from the text of the model.

| Case | Model in `testdata/marks/` | Code |
|---|---|---|
| `unverified-relation` | `only-a-relation-is-unverified` | `unverified.not_checked` |
| `superseded-by-a-denied-read` | `confirmed-then-denied-later` | `unverified.superseded` |
| `absence-unbacked` | `listing-stopped-half-way` | `unverified.absence_unbacked` |
| `stated-only` | `relations-known-from-weak-evidence` | `unverified.stated_only` |
| `mismatch-on-a-node` | `document-and-setting-differ` | `mismatch.values` |
| `mismatch-across-a-slot` | `slot-declared-a-deployed-b-no-coverage` | `mismatch.values` |
| `absent-in-a-complete-read` | `absent-named-target-complete` | `mismatch.absent` |
| `defect-evidence-only-denies` | `defect-no-evidence-speaks-for-the-node` | `model.no_evidence` |
| `defect-contradictory-evidence` | `defect-evidence-states-and-denies` | `model.contradictory_evidence` |
| `defect-relation-evidence` | `defect-evidence-of-another-relation` | `model.relation_evidence` |
| `hidden-defect` | `defect-evidence-of-another-relation` | `model.relation_evidence` |

A finding is a warning at the first element that carries the state. Its message ends with the key of the unit in brackets when the finding has a key. A defect is an error. `model.no_evidence` stands at the `evidence` of the element and is about the element. The other two stand at each piece of evidence at fault and are about that evidence.

`superseded-by-a-denied-read` and `absence-unbacked` hold a coverage record that is not complete, so each has the warning of that record as its second line.

`hidden-defect` hides the edge with the defect in its view config, and the error stays. `no-evidence` shows the same for a node.

## Cases about boundaries

Four cases pass every check of the two files, and the figure is drawn. Their warnings come from `view.Build`, which derives the boundaries, and `lint` adds the place.

`boundary-unbacked` and `group-in-parts` share one model: two nodes whose evidence states a `scope.owner` beside `call.implemented`. The owners differ, and neither node has a region.

In `boundary-unbacked` the second label asks for the region boundary of a node without a region. The warning stands at that entry of the view config and is about the node. In `group-in-parts` a group lists both nodes, so it is drawn once in each owner, and the warning stands at the group. The two owners are of one provider and have no name in the view config, so both are drawn under one label, and the case has one `view.boundary_label_shared` for each. That warning points at `/boundaries`, which this view config lacks, so its line has no position.

`network-without-region` and `boundary-contested` have no view config, and their warnings stand at the node in the model. In the first, two nodes name one network and the second node states no region. In the second, the second node names two networks.
