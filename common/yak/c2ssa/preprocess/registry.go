package preprocess

import (
	"io/fs"
	"path"
	"strings"

	"github.com/yaklang/yaklang/common/utils/filesys"
	fi "github.com/yaklang/yaklang/common/utils/filesys/filesys_interface"
)

// HeaderKind classifies indexed header sources.
type HeaderKind int

const (
	HeaderStatic HeaderKind = iota
	HeaderHInTemplate
)

// HeaderEntry is a registered header file.
type HeaderEntry struct {
	Path    string
	Content []byte
	Kind    HeaderKind
}

// HeaderRegistry indexes project headers for include resolution.
type HeaderRegistry struct {
	byPath   map[string]*HeaderEntry
	hInAlias map[string]string // virtual .h path -> stored .h.in path
}

// BuildHeaderRegistry walks fs and indexes .h and .h.in files.
func BuildHeaderRegistry(filesystem fi.FileSystem) *HeaderRegistry {
	reg := &HeaderRegistry{
		byPath:   make(map[string]*HeaderEntry),
		hInAlias: make(map[string]string),
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
	rel := normalizeSlash(trimDot(filePath))
	if strings.HasSuffix(name, ".h.in") {
		data, err := filesystem.ReadFile(filePath)
		if err != nil {
			return
		}
		r.registerEntry(rel, data, HeaderHInTemplate)
		virtual := strings.TrimSuffix(rel, ".in")
		r.hInAlias[virtual] = rel
		r.registerAliasKeys(virtual, rel)
		return
	}
	if filesystem.Ext(name) == ".h" {
		data, err := filesystem.ReadFile(filePath)
		if err != nil {
			return
		}
		r.registerEntry(rel, data, HeaderStatic)
		r.registerAliasKeys(rel, rel)
	}
}

func (r *HeaderRegistry) registerEntry(storedPath string, content []byte, kind HeaderKind) {
	storedPath = normalizeSlash(storedPath)
	r.byPath[storedPath] = &HeaderEntry{
		Path:    storedPath,
		Content: content,
		Kind:    kind,
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
	if alt, ok := r.hInAlias[requested]; ok {
		return alt, true
	}
	base := path.Base(requested)
	if e, ok := r.byPath[base]; ok && e != nil {
		return e.Path, true
	}
	return "", false
}

func trimDot(p string) string {
	p = strings.TrimPrefix(p, "./")
	p = strings.TrimPrefix(p, ".\\")
	p = strings.TrimPrefix(p, "/")
	p = strings.TrimPrefix(p, "\\")
	return p
}
