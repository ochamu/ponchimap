# Samples

Each folder is an app drawn as a figure. `model.json` is the model and `view.yaml` is the view config. `expected.json` is the display derived by hand from the two files, without running the code.

The tests in `internal/view` compare the display with `expected.json`, and the tests in `internal/report` read it too. The tests in `internal/cli`, `internal/lint`, `internal/model` and `internal/render/d2render` also run over the samples.

CI renders every sample and fails unless the PNGs of `order-service` and `long-labels-and-mismatches` are byte-identical to `docs/images/order-service.png` and `docs/images/long-labels-and-mismatches.png`, the figures in the README.

- `small-order-app` has seven nodes on Cloudflare, Firebase, and two outside services.
- `dev-prod-and-shared` has a development and a production environment beside the resources they share, in nested boundaries.
- `long-labels-and-mismatches` has long Japanese labels, evidence that disagrees, and elements not checked against the real environment.
- `order-service` has 25 nodes and 34 edges of one order service. Its view config hides one node and folds three into one.
- `order-service-with-supabase` is `order-service` with one more node, Supabase Postgres, and an edge to it.
