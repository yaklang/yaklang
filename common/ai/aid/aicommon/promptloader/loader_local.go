//go:build !gzip_embed

package promptloader

import "embed"

//go:embed prompts
var sourceFS embed.FS

func readPrompt(name string) ([]byte, error) {
	return sourceFS.ReadFile("prompts/" + name)
}
