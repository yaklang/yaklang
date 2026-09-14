package preprocess

import (
	"io"
	"path"
	"strings"

	fi "github.com/yaklang/yaklang/common/utils/filesys/filesys_interface"
)

// ResolvedHeader is a project or external header ready for macro collection.
type ResolvedHeader struct {
	Path    string
	Content []byte
}

// IncludeResolver resolves #include paths against the header registry.
type IncludeResolver struct {
	registry   *HeaderRegistry
	config     PreprocessConfig
	externalFS []fi.FileSystem
}

func NewIncludeResolver(registry *HeaderRegistry, config PreprocessConfig) *IncludeResolver {
	var roots []fi.FileSystem
	roots = append(roots, config.ExternalIncludeFS...)
	roots = append(roots, openExternalIncludeFS(config.ExternalIncludeDirs)...)
	return &IncludeResolver{
		registry:   registry,
		config:     config,
		externalFS: roots,
	}
}

// Resolve finds a project header for an #include directive.
func (r *IncludeResolver) Resolve(includePath string, system bool, includingFile string) (storedPath string, ok bool) {
	h, ok := r.ResolveHeader(includePath, system, includingFile)
	if !ok {
		return "", false
	}
	return h.Path, true
}

// ResolveHeader finds a project or external header and returns its contents.
func (r *IncludeResolver) ResolveHeader(includePath string, system bool, includingFile string) (ResolvedHeader, bool) {
	includePath = normalizeSlash(includePath)
	if stored, ok := r.resolveProject(includePath, system, includingFile); ok {
		if e, found := r.registry.Lookup(stored); found && e != nil {
			return ResolvedHeader{Path: e.Path, Content: e.Content}, true
		}
	}
	if stored, content, ok := readExternalHeader(r.externalFS, includePath); ok {
		return ResolvedHeader{Path: stored, Content: content}, true
	}
	return ResolvedHeader{}, false
}

func (r *IncludeResolver) resolveProject(includePath string, system bool, includingFile string) (string, bool) {
	if system && r.config.SkipSystemIncludes && len(r.externalFS) == 0 {
		if r.looksLikeSystemHeader(includePath) {
			return "", false
		}
	}

	candidates := r.candidatePaths(includePath, system, includingFile)
	for _, c := range candidates {
		if stored, found := r.registry.ResolveStoredPath(c); found {
			return stored, true
		}
	}
	return "", false
}

func (r *IncludeResolver) looksLikeSystemHeader(p string) bool {
	if !strings.Contains(p, "/") && !strings.Contains(p, "\\") {
		return true
	}
	return false
}

func (r *IncludeResolver) candidatePaths(includePath string, system bool, includingFile string) []string {
	var out []string
	seen := make(map[string]bool)
	add := func(p string) {
		p = normalizeSlash(p)
		if p == "" || seen[p] {
			return
		}
		seen[p] = true
		out = append(out, p)
	}

	add(includePath)

	if !system {
		dir := path.Dir(normalizeSlash(includingFile))
		if dir != "." && dir != "" {
			add(path.Join(dir, includePath))
		}
	}

	for _, incDir := range r.config.IncludeDirs {
		add(path.Join(incDir, includePath))
	}

	if !strings.HasPrefix(includePath, "include/") {
		add(path.Join("include", includePath))
	}

	return out
}

// Close releases zip archives opened as external include roots.
func (r *IncludeResolver) Close() error {
	if r == nil {
		return nil
	}
	var first error
	for _, root := range r.externalFS {
		c, ok := root.(io.Closer)
		if !ok {
			continue
		}
		if err := c.Close(); err != nil && first == nil {
			first = err
		}
	}
	return first
}
