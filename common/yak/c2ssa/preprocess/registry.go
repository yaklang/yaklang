package preprocess

import (
	"io/fs"
	"path"
	"strings"

	"github.com/yaklang/yaklang/common/utils/filesys"
	fi "github.com/yaklang/yaklang/common/utils/filesys/filesys_interface"
)

// HeaderEntry is a registered header file.
type HeaderEntry struct {
	Path    string
	Content []byte
}

// HeaderRegistry indexes project .h headers for include resolution.
type HeaderRegistry struct {
	byPath map[string]*HeaderEntry
}

// BuildHeaderRegistry walks fs and indexes .h files.
// .h.in templates are build inputs only and are intentionally excluded.
func BuildHeaderRegistry(filesystem fi.FileSystem) *HeaderRegistry {
	reg := &HeaderRegistry{
		byPath: make(map[string]*HeaderEntry),
	}
	_ = filesys.Recursive(".", filesys.WithFileSystem(filesystem), filesys.WithFileStat(func(filePath string, info fs.FileInfo) error {
		if info.IsDir() {
			return nil
		}
		reg.indexPath(filesystem, filePath)
		return nil
	}))
	return reg
}

func (r *HeaderRegistry) indexPath(filesystem fi.FileSystem, filePath string) {
	name := path.Base(filePath)
	if strings.HasSuffix(name, ".h.in") {
		return
	}
	if filesystem.Ext(name) != ".h" {
		return
	}
	rel := normalizeSlash(trimDot(filePath))
	data, err := filesystem.ReadFile(filePath)
	if err != nil {
		return
	}
	r.registerEntry(rel, data)
	r.registerAliasKeys(rel, rel)
}

func (r *HeaderRegistry) registerEntry(storedPath string, content []byte) {
	storedPath = normalizeSlash(storedPath)
	r.byPath[storedPath] = &HeaderEntry{
		Path:    storedPath,
		Content: content,
	}
}

func (r *HeaderRegistry) registerAliasKeys(canonical, stored string) {
	canonical = normalizeSlash(canonical)
	stored = normalizeSlash(stored)
	entry := r.byPath[stored]
	if entry == nil {
		return
	}
	r.byPath[canonical] = entry
	r.byPath[path.Base(canonical)] = entry
	if strings.HasPrefix(canonical, "include/") {
		r.byPath[strings.TrimPrefix(canonical, "include/")] = entry
	}
}

// Lookup returns a header by normalized path.
func (r *HeaderRegistry) Lookup(normalizedPath string) (*HeaderEntry, bool) {
	normalizedPath = normalizeSlash(normalizedPath)
	if e, ok := r.byPath[normalizedPath]; ok && e != nil {
		return e, true
	}
	return nil, false
}

// ResolveStoredPath maps a requested include path to the registry key.
func (r *HeaderRegistry) ResolveStoredPath(requested string) (string, bool) {
	requested = normalizeSlash(requested)
	if e, ok := r.byPath[requested]; ok && e != nil {
		return e.Path, true
	}
	base := path.Base(requested)
	if e, ok := r.byPath[base]; ok && e != nil {
		return e.Path, true
	}
	return "", false
}

// UniqueEntries returns each indexed header once.
func (r *HeaderRegistry) UniqueEntries() []*HeaderEntry {
	if r == nil {
		return nil
	}
	seen := make(map[string]bool)
	var out []*HeaderEntry
	for _, e := range r.byPath {
		if e == nil || seen[e.Path] {
			continue
		}
		seen[e.Path] = true
		out = append(out, e)
	}
	return out
}

func trimDot(p string) string {
	p = strings.TrimPrefix(p, "./")
	p = strings.TrimPrefix(p, ".\\")
	p = strings.TrimPrefix(p, "/")
	p = strings.TrimPrefix(p, "\\")
	return p
}
