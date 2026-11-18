# Small models

Two models of a Worker and a D1 database in `dev`.

- `worker-and-d1.json` has the Worker, the D1, and the binding between them. Only the configuration declares them, and nothing is checked against the real environment. `worker-and-d1.view.yaml` is a view config for it, with labels and a logical group.
- `worker-d1-and-mail.json` has another Worker and D1, and the code of the Worker also calls a mail API. The real environment confirms the Worker and its account, the read of the D1 was denied, and the binding was not read.

The tests in `internal/view` assert the display these models must produce. The tests in `internal/report` build the report from the same models. The tests in `internal/lint` and `internal/model` check that they load. The view configs under `testdata/view` are fitted to `worker-d1-and-mail.json`.
