# Model loading cases

Each `NAME.json` is an input of `model.Parse`, and `NAME.want` lists the diagnostics it must give, in order. The tests in `internal/model` compare the two. A `valid-*` case must load.

A line of a `.want` file has five fields separated by tabs:

    LINE:COLUMN  CODE  POINTER  TARGET_ID  CONTAINS

`-` stands for the empty pointer, for no target ID, and for no required text. CONTAINS is a text the message must contain, such as the name of a missing property.

The codes and pointers were derived by hand from `schema/model.schema.json` and what `model.Parse` checks beyond it, not from the output of the loader. To add a case, derive its lines by hand before running the test.

`secret-values.json` holds the text `secret-token` at every place where a value is refused. No diagnostic may show it.
