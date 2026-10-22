package sfvm

import (
	"sort"

	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/memedit"
)

// Source is the one body a pattern root and its hits read. Paths is the stable
// file list. Read returns that path's text for this root.
type Source interface {
	Paths() []string
	Read(path string) (string, error)
}

// SharedBodySource is optional. A source that already owns an immutable body
// lets editors reuse it instead of copying the text into another string.
type SharedBodySource interface {
	SharedBody(path string) (*memedit.SafeString, bool)
}

// mapSource is the in-memory snapshot used by NewPatternRoot.
type mapSource struct {
	files map[string]string
	paths []string
}

func newMapSource(files map[string]string) *mapSource {
	if files == nil {
		files = map[string]string{}
	}
	paths := make([]string, 0, len(files))
	for path := range files {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	return &mapSource{files: files, paths: paths}
}

func (m *mapSource) Paths() []string {
	if m == nil {
		return nil
	}
	return m.paths
}

func (m *mapSource) Read(path string) (string, error) {
	if m == nil {
		return "", utils.Error("nil source")
	}
	content, ok := m.files[path]
	if !ok {
		return "", utils.Errorf("pattern root: file %q is not available", path)
	}
	return content, nil
}

func (m *mapSource) Files() map[string]string {
	if m == nil {
		return nil
	}
	return m.files
}

// callbackSource is an on-demand list. It has one reader, not a map plus a reader.
type callbackSource struct {
	paths  []string
	read   func(string) (string, error)
	shared func(string) (*memedit.SafeString, error)
}

func (c *callbackSource) Paths() []string {
	if c == nil {
		return nil
	}
	return c.paths
}

func (c *callbackSource) Read(path string) (string, error) {
	if c == nil || c.read == nil {
		return "", utils.Errorf("pattern root: file %q is not available", path)
	}
	return c.read(path)
}

func (c *callbackSource) SharedBody(path string) (*memedit.SafeString, bool) {
	if c == nil || c.shared == nil {
		return nil, false
	}
	raw, err := c.shared(path)
	if err != nil || raw == nil {
		return nil, false
	}
	return raw, true
}
