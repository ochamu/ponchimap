// Package schema embeds the public JSON Schemas of the tool. The schema files
// beside this one are the contract: editors and other tools read them
// directly, and the tool checks its inputs against the same bytes.
package schema

import _ "embed"

// Model is model.schema.json, the schema of the configuration model.
//
//go:embed model.schema.json
var Model []byte

// View is view.schema.json, the schema of the view config.
//
//go:embed view.schema.json
var View []byte

// Findings is findings.schema.json, the schema of what one acquisition found.
// It refers to model.schema.json for the shapes it shares with the model.
//
//go:embed findings.schema.json
var Findings []byte

// Supplements is supplements.schema.json, the schema of the facts that the
// user adds. It refers to model.schema.json for the shapes it shares with the
// model.
//
//go:embed supplements.schema.json
var Supplements []byte
