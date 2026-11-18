# View config loading cases

Each `NAME.yaml` is a view config. The tests in `internal/view` read it with `view.ParseConfig` and fit it to the model `testdata/small-models/worker-d1-and-mail.json` with `Resolve`, and compare the diagnostics of the two steps with `NAME.want`, in order. A `valid-*` case must be read.

The model has the nodes `n101`, `n102`, `n103` and the edges `e101`, `e102`, `e103`.

A line of a `.want` file has six fields separated by tabs:

    SEVERITY  LINE:COLUMN  CODE  POINTER  TARGET_ID  CONTAINS

`-` stands for the empty pointer, for no target ID, and for no required text. CONTAINS is a text the message must contain, such as the name of a missing property. Every diagnostic must have a hint.

The lines were derived by hand from `schema/view.schema.json` and what `ParseConfig` and `Resolve` check beyond it, not from the output of the loader. To add a case, derive its lines by hand before running the test.

`secret-values.yaml` holds a form of the text `secret-token` at eleven of the thirteen places where a value is refused. The other two hold an empty string and a number. No diagnostic may show the text.

`hidden-members` and `members-listed-twice` hold what `Resolve` refuses in a config that names only elements of the model.
