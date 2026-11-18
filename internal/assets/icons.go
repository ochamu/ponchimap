package assets

import (
	"embed"
	"strings"
)

// GenericIcon is the icon of a service that names no category.
const GenericIcon = "generic"

//go:embed icons/*.svg
var iconFS embed.FS

func bundledIcons() map[string][]byte {
	entries, err := iconFS.ReadDir("icons")
	if err != nil {
		panic("assets: the embedded icons cannot be listed: " + err.Error())
	}
	icons := make(map[string][]byte, len(entries))
	for _, e := range entries {
		svg, err := iconFS.ReadFile("icons/" + e.Name())
		if err != nil {
			panic("assets: the embedded icon cannot be read: " + err.Error())
		}
		icons[strings.TrimSuffix(e.Name(), ".svg")] = svg
	}
	if icons[GenericIcon] == nil {
		panic("assets: the generic icon is missing from the embedded set")
	}
	return icons
}
