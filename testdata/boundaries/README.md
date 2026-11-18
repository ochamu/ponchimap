# Cases for the real boundaries

Each `NAME.json` is a model that loads without a diagnostic. The tests in `internal/view/boundary_test.go` build scenes from them and state what `view.Build` must give: the boundary each node is drawn in, the ID and the label of that boundary, and the warnings.

- `two-accounts.json` has two accounts of one provider with a region and a network each, a node outside the network, an account of another provider, and a node without an owner.
- `two-networks.json` has a node whose evidence names two networks, beside a node in one of them.
- `network-in-two-regions.json` has a network named by nodes of two regions.

The tests that add and hide nodes also run over these models, the samples under `testdata/samples`, and the small models under `testdata/small-models`.

Edit a model by hand when a case changes.
