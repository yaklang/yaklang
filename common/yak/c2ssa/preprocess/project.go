package preprocess

import (
	"sync"

	fi "github.com/yaklang/yaklang/common/utils/filesys/filesys_interface"
)

// CPreprocessProject holds header registry and include resolution for a filesystem.
type CPreprocessProject struct {
	fs       fi.FileSystem
	registry *HeaderRegistry
	resolver *IncludeResolver
	config   PreprocessConfig

	// headerMacroTables caches macros collected from all project headers (include-agnostic).
	// Built once per project; TUs clone and merge their own #define/#undef on top.
	headerMacroOnce   sync.Once
	headerMacroTables MacroTables
}

// BuildProject constructs a preprocessor project from fs and config.
func BuildProject(fs fi.FileSystem, config PreprocessConfig) *CPreprocessProject {
	if len(config.IncludeDirs) == 0 {
		config.IncludeDirs = DetectIncludeDirs(fs)
	}
	if config.Defines == nil {
		config.Defines = make(map[string]string)
	}
	if config.MaxIncludeDepth == 0 {
		config.MaxIncludeDepth = 64
	}
	reg := BuildHeaderRegistry(fs)
	return &CPreprocessProject{
		fs:       fs,
		registry: reg,
		resolver: NewIncludeResolver(reg, config),
		config:   config,
	}
}

// getHeaderMacroTables returns macros from all registered headers, computed once per project.
func (p *CPreprocessProject) getHeaderMacroTables() MacroTables {
	p.headerMacroOnce.Do(func() {
		out := NewMacroTables()
		seen := make(map[string]bool)
		for _, entry := range p.registry.UniqueEntries() {
			if entry == nil || seen[entry.Path] {
				continue
			}
			seen[entry.Path] = true
			tables := p.collectMacrosFromSource(entry.Path, string(entry.Content))
			out.MergeFrom(tables)
		}
		p.headerMacroTables = out
	})
	return p.headerMacroTables
}

// Registry returns the header registry.
func (p *CPreprocessProject) Registry() *HeaderRegistry {
	return p.registry
}

// Config returns project preprocess configuration.
func (p *CPreprocessProject) Config() PreprocessConfig {
	return p.config
}

// ReadHeader returns header content by stored registry path.
func (p *CPreprocessProject) ReadHeader(storedPath string) ([]byte, bool) {
	e, ok := p.registry.Lookup(storedPath)
	if !ok {
		return nil, false
	}
	return e.Content, true
}
