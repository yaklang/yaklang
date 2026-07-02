package preprocess

import (
	"strings"

	fi "github.com/yaklang/yaklang/common/utils/filesys/filesys_interface"
)

// PreprocessConfig controls include search paths and predefined macros.
type PreprocessConfig struct {
	IncludeDirs        []string
	Defines            map[string]string
	SkipSystemIncludes bool
	MaxIncludeDepth    int
}

// DefaultConfig returns sensible defaults for project preprocessing.
func DefaultConfig() PreprocessConfig {
	return PreprocessConfig{
		SkipSystemIncludes: true,
		MaxIncludeDepth:    64,
		Defines:            make(map[string]string),
	}
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
