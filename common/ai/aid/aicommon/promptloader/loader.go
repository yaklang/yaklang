// Package promptloader is the single source for embedded AI prompt resources.
// Local builds embed editable source files; release builds embed their gzip archive.
package promptloader

import (
	"fmt"
	"path"
	"strings"
)

// ReadFile returns a copy of a prompt resource using its path below prompts/.
func ReadFile(name string) ([]byte, error) {
	if name == "" || path.IsAbs(name) || strings.Contains(name, "\\") || path.Clean(name) != name || name == ".." || strings.HasPrefix(name, "../") {
		return nil, fmt.Errorf("invalid prompt path %q", name)
	}
	return readPrompt(name)
}

func Load(name string) (string, error) {
	data, err := ReadFile(name)
	return string(data), err
}

func MustLoad(name string) string {
	content, err := Load(name)
	if err != nil {
		panic(fmt.Sprintf("load AI prompt %q: %v", name, err))
	}
	return content
}

func MustReadFile(name string) []byte {
	content, err := ReadFile(name)
	if err != nil {
		panic(fmt.Sprintf("load AI prompt %q: %v", name, err))
	}
	return content
}
