package preprocess

import (
	"path"
	"strings"
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
	if config.ExternalIncludeDirs == nil {
		config.ExternalIncludeDirs = EnsureExternalIncludeDirs()
	}
	reg := BuildHeaderRegistryWithExtensions(fs, config.HeaderExtensions)
	return &CPreprocessProject{
		fs:       fs,
		registry: reg,
		resolver: NewIncludeResolver(reg, config),
		config:   config,
	}
}

// isPlatformVariantHeader reports whether a header belongs to another target
// platform, so its macros must not enter the include-agnostic table.
//
// C projects keep per-platform variants next to the portable headers
// (Zend/zend_config.w32.h, main/config.w32.h, ext/mysqlnd/config-win.h,
// win32/*). Those files exist unconditionally in the tree but are only ever
// included by a build for that platform. Collecting them into the shared macro
// table makes every translation unit of the host build see the other
// platform's macros: a Windows config header defines _WIN32 and includes
// <windows.h>, which defines `interface` as `struct` and `not` as `!`, so
// identifiers named `interface` or `not` stop parsing and Windows code paths
// become active on Linux. They still contribute normally when a translation
// unit includes them explicitly.
func isPlatformVariantHeader(p string) bool {
	// External roots store headers as "ext:<include path>".
	p = strings.TrimPrefix(normalizeSlash(p), "ext:")
	p = strings.ToLower(p)
	for _, seg := range strings.Split(p, "/") {
		switch seg {
		case "win32", "win64", "windows", "mswin", "msvc":
			return true
		}
	}
	base := path.Base(p)
	// External runtime headers for another platform reach the project through
	// the c-std-headers pack (php-src's com_dotnet headers include <ocidl.h>).
	if otherPlatformHeaderNames[base] {
		return true
	}
	for _, suffix := range []string{
		".w32.h", ".w64.h", ".win.h", ".win32.h", ".win64.h",
		"-win.h", "-win32.h", "-win64.h",
		"_win.h", "_win32.h", "_win64.h",
	} {
		if strings.HasSuffix(base, suffix) {
			return true
		}
	}
	return false
}

// otherPlatformHeaderNames lists external headers that belong to another
// target platform or must not be expanded across the whole project.
//
// windows.h and the COM headers it pulls in (basetyps.h is where `interface`
// becomes `struct`); iso646.h, which turns `and`, `or` and `not` into operators
// even though C only applies it on an explicit include.
var otherPlatformHeaderNames = map[string]bool{
	"windows.h":     true,
	"winsock.h":     true,
	"winsock2.h":    true,
	"ws2tcpip.h":    true,
	"winnt.h":       true,
	"winbase.h":     true,
	"windef.h":      true,
	"minwindef.h":   true,
	"basetsd.h":     true,
	"basetyps.h":    true,
	"guiddef.h":     true,
	"objbase.h":     true,
	"objidl.h":      true,
	"ocidl.h":       true,
	"oleauto.h":     true,
	"ole2.h":        true,
	"combaseapi.h":  true,
	"unknwn.h":      true,
	"rpc.h":         true,
	"iso646.h":      true,
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
			// Platform variants are collected only when a translation unit
			// actually includes them.
			if isPlatformVariantHeader(entry.Path) {
				continue
			}
			seen[entry.Path] = true
			tables := p.collectProjectMacrosFromSource(entry.Path, string(entry.Content))
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

// Close releases external include roots (zip archives).
func (p *CPreprocessProject) Close() error {
	if p == nil || p.resolver == nil {
		return nil
	}
	return p.resolver.Close()
}
