package preprocess

import (
	"path"
	"strings"
)

// IncludeResolver resolves #include paths against the header registry.
type IncludeResolver struct {
	registry *HeaderRegistry
	config   PreprocessConfig
}

func NewIncludeResolver(registry *HeaderRegistry, config PreprocessConfig) *IncludeResolver {
	return &IncludeResolver{registry: registry, config: config}
}

// Resolve finds a project header for an #include directive.
func (r *IncludeResolver) Resolve(includePath string, system bool, includingFile string) (storedPath string, ok bool) {
	includePath = normalizeSlash(includePath)
	if system && r.config.SkipSystemIncludes {
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
