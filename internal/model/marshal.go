package model

import (
	"bytes"
	"encoding/json"
	"fmt"
)

// Marshal writes m in the stable form. Parse(name, Marshal(m)) gives m back,
// except that an identity, attributes, or target.scope that is empty comes
// back as nil, since the stable form leaves it out.
func Marshal(m *Model) []byte {
	// encoding/json writes a nil list and a nil scope as null, which the
	// schema refuses.
	out := *m
	out.Nodes = make([]Node, len(m.Nodes))
	for i, n := range m.Nodes {
		n.Evidence = append([]Evidence{}, n.Evidence...)
		out.Nodes[i] = n
	}
	out.Edges = make([]Edge, len(m.Edges))
	for i, e := range m.Edges {
		e.Evidence = append([]Evidence{}, e.Evidence...)
		out.Edges[i] = e
	}
	out.Coverage = make([]Coverage, len(m.Coverage))
	for i, c := range m.Coverage {
		if c.Scope == nil {
			c.Scope = Scope{}
		}
		out.Coverage[i] = c
	}

	var text bytes.Buffer
	enc := json.NewEncoder(&text)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(out); err != nil {
		// A model that was read holds JSON values only, so the code that built
		// this one has a defect. The text of the error may quote a value of
		// the model, so only its type is shown.
		panic(fmt.Sprintf("model: the model holds a value that cannot be written as JSON (%T)", err))
	}
	return text.Bytes()
}
