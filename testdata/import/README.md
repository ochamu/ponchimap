# Import cases

Each folder is one case of `importer.Run`. The test in `internal/importer` runs the steps of a case in order
in one workspace and compares what comes back, and what is saved, with `want.json`.

    NAME/
      note.txt            what the case shows
      model.json          the model before step 1, when the case starts from one
      acquisitions.json   the records before step 1, when the case starts with some
      same-as             the name of a case whose final model this one must equal, array order aside
      1/findings/*.json   the findings given to step 1
      1/supplements.yaml  the supplements given to step 1, when there are any
      1/view.yaml         a view config to resolve against the model after the step, when there is one
      1/want.json         what step 1 must give
      2/...

The workspace is the working directory of the run: the model is `model.json`, the findings are the folder
`findings`, and the supplements are `supplements.yaml`. The target
of a new model is the repository `.` and the environment `dev`. A step replaces the findings folder with its
own and passes the folder; a step without a findings folder passes no findings, and a step without
`supplements.yaml` passes no supplements.

An ID that import issues cannot be written by hand, so the files name it by the candidate it came from:

    @STEP/INPUT#CANDIDATE    the model ID that step STEP gave the candidate of the input, such as
                             @1/findings/scan.json#worker; for a statement of the supplements the candidate
                             is its index, such as @2/supplements.yaml#0
    @STEP/supplements.yaml   the acquisition ID that step STEP gave the supplements

The test replaces these in the inputs, in `view.yaml`, and in `want.json` before it uses them. Using a name of
an earlier step in a later `want.json` is how a case says that an ID stays.

`want.json` holds:

    status        ok, partial, or error
    model         the model that must be saved, or "unchanged" (the file has the bytes it had before the
                  step, or is still absent), or "any" (not compared here). Nodes, edges, coverage, and the
                  evidence of an element are compared without regard to order.
    diagnostics   every diagnostic of the result, in any order: severity, code, path, and, when given,
                  pointer and target_id
    added         the nodes and edges new in the model, in any order
    removed       the nodes and edges that left the model, in any order; added and removed are compared
                  only when given
    mapping       for some candidates, written INPUT#CANDIDATE, the model ID of the result; "" says that
                  nothing of the candidate was kept
    acquisitions  when given, the records that acquisitions.json must hold: id, source, route, complete
    view          when given, the diagnostics of resolving view.yaml against the saved model: code, target_id
    marks         when given, the marks that view.Assess gives the saved model: for every node and edge of
                  the model, by its ID, the kinds of its marks (unverified, mismatch) in any order, or an
                  empty list. The model must have no defect. These values were derived by hand from the
                  rules in the doc comment of view.Assess.

The expected values were derived by hand from the rules in the package comment of `internal/importer`
(`doc.go`), not from the output of the importer. `note.txt` names the rules a case shows by their labels,
such as N1 or M2. To add a case, derive its values by hand first.

For every step the test also checks what holds for all cases: running the step again leaves the model and the
records byte for byte as they are; giving the files in the reverse order gives the same bytes and the same
diagnostics; and after a step whose status is error, the model and the records are byte for byte what they
were before it.
