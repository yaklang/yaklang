package ssaapi

import (
	"fmt"
	"path"
	"strings"

	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
)

func (c *Config) hasYakEntryFiles() bool {
	language := c.GetLanguage()
	if language == "" && c.LanguageBuilder != nil {
		language = c.LanguageBuilder.GetLanguage()
	}
	return language == ssaconfig.Yak && len(c.GetCompileEntryFiles()) != 0
}

// Entry files choose Yak's executable roots. Retain the complete filesystem
// and prehandler/source registry so include targets remain available, without
// also executing those targets as independent roots in an arbitrary AST order.
func (c *Config) yakRootFiles(files []string) ([]string, error) {
	if !c.hasYakEntryFiles() {
		return files, nil
	}
	normalize := func(file string) string {
		return strings.TrimPrefix(path.Clean(strings.ReplaceAll(file, "\\", "/")), "/")
	}
	available := make(map[string]string, len(files))
	for _, file := range files {
		available[normalize(file)] = file
	}
	var roots []string
	seen := make(map[string]bool)
	for _, group := range c.GetCompileEntryFiles() {
		for _, entry := range strings.Split(group, ",") {
			entry = strings.TrimSpace(entry)
			file, found := available[normalize(entry)]
			if !found && c.programPath != "" {
				file, found = available[normalize(path.Join(strings.ReplaceAll(c.programPath, "\\", "/"), strings.ReplaceAll(entry, "\\", "/")))]
			}
			if entry == "" || !found {
				return nil, fmt.Errorf("%w: Yak entry %q is not a compilable source", ErrNoFoundCompiledFile, entry)
			}
			if !seen[file] {
				seen[file] = true
				roots = append(roots, file)
			}
		}
	}
	return roots, nil
}
