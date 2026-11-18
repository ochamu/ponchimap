package view

import (
	"github.com/ochamu/ponchimap/internal/assets"
	"github.com/ochamu/ponchimap/internal/model"
)

// FrameOf and LineOf say how the evidence of an element draws it, whether
// or not a figure shows the element.
func FrameOf(n *model.Node) Frame { return frameOf(n) }

func LineOf(e *model.Edge) Line {
	line, _ := lineOf(e)
	return line
}

// TitleOf is the name of a node on its card and in the report: the name its
// findings gave the service, else the service key.
func TitleOf(n *model.Node) string {
	if name, _ := n.Attributes["service_name"].(string); name != "" {
		return name
	}
	return n.Service
}

// iconOf is the icon of a node: the category its findings gave the service,
// which the schema holds to an icon name, else the generic icon.
func iconOf(n *model.Node) string {
	if category, _ := n.Attributes["category"].(string); category != "" {
		return category
	}
	return assets.GenericIcon
}
