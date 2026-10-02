// Package promptloader owns the old PLAN engine's private prompt resources.
// Shared ReAct templates and Yakit approval schemas remain in aicommon.
package promptloader

import (
	"embed"
	"fmt"
	"io/fs"
)

//go:embed data
var resources embed.FS

func MustLoad(name string) string {
	if !fs.ValidPath(name) {
		panic(fmt.Sprintf("invalid legacy prompt path %q", name))
	}
	data, err := resources.ReadFile("data/" + name)
	if err != nil {
		panic(fmt.Sprintf("load legacy prompt %q: %v", name, err))
	}
	return string(data)
}
