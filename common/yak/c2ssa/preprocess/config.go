package preprocess

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/yaklang/yaklang/common/consts"
	fi "github.com/yaklang/yaklang/common/utils/filesys/filesys_interface"
)

// PreprocessConfig controls include search paths and predefined macros.
type PreprocessConfig struct {
	IncludeDirs []string
	// ExternalIncludeDirs are host filesystem roots searched for headers that
	// are not in the project. Default is $YAKIT_HOME/c-headers (directory and
	// any .zip files in it). Put downloaded C header packs there.
	ExternalIncludeDirs []string
	// ExternalIncludeFS is additional include roots (tests, in-memory packs).
	ExternalIncludeFS  []fi.FileSystem
	Defines            map[string]string
	SkipSystemIncludes bool
	MaxIncludeDepth    int
}

// DefaultConfig returns sensible defaults for project preprocessing.
// External include roots come from $YAKIT_HOME/c-headers.
func DefaultConfig() PreprocessConfig {
	return PreprocessConfig{
		SkipSystemIncludes:  true,
		MaxIncludeDepth:     64,
		Defines:             make(map[string]string),
		ExternalIncludeDirs: DetectExternalIncludeDirs(),
	}
}

// DetectExternalIncludeDirs finds downloaded C header packs under YAKIT_HOME.
func DetectExternalIncludeDirs() []string {
	dir := consts.GetDefaultCHeadersDir()
	return collectCHeaderRoots(dir)
}

func collectCHeaderRoots(dir string) []string {
	info, err := os.Stat(dir)
	if err != nil || !info.IsDir() {
		return nil
	}
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) == 0 {
		return nil
	}
	var out []string
	seen := map[string]bool{}
	add := func(p string) {
		if p == "" || seen[p] {
			return
		}
		seen[p] = true
		out = append(out, p)
	}
	add(dir)
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() {
			continue
		}
		if strings.EqualFold(filepath.Ext(name), ".zip") {
			add(filepath.Join(dir, name))
		}
	}
	return out
}

// DetectIncludeDirs heuristically discovers common C project include directories.
func DetectIncludeDirs(fs fi.FileSystem) []string {
	candidates := []string{
		"include",
		"include/openssl",
		"include/internal",
		"include/crypto",
		"apps/include",
		"apps",
		"crypto",
		"ssl",
	}
	var out []string
	seen := make(map[string]bool)
	for _, dir := range candidates {
		if dirExists(fs, dir) && !seen[dir] {
			seen[dir] = true
			out = append(out, normalizeSlash(dir))
		}
	}
	return out
}

func dirExists(fs fi.FileSystem, dir string) bool {
	entries, err := fs.ReadDir(dir)
	return err == nil && len(entries) > 0
}

func normalizeSlash(path string) string {
	return strings.ReplaceAll(path, "\\", "/")
}
