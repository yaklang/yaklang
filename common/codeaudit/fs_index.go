package codeaudit

import (
	"os"
	"path/filepath"
	"strings"
)

// excludedDirs is the set of directory names to always skip during traversal.
var excludedDirs = map[string]bool{
	".git": true, ".svn": true, ".hg": true,
	"node_modules": true, "__pycache__": true,
	".idea": true, ".vscode": true,
	"target": true, "build": true, ".gradle": true,
	"out": true, "dist": true,
}

// excludedTestPathFragments are path fragments that indicate test directories.
var excludedTestPathFragments = []string{
	"/src/test/", "/src/tests/",
	"/test/java/", "/tests/java/",
	"/test-fixtures/", "/testfixtures/",
}

// FSIndex is a project file system index.
type FSIndex struct {
	Root         string
	AllFiles     []string            // all file absolute paths
	FilesByName  map[string][]string // file basename → path list
	FilesScanned int
}

// BuildFSIndex traverses a directory and builds a file index.
func BuildFSIndex(root string, opts *ProbeOptions) *FSIndex {
	idx := &FSIndex{
		Root:        root,
		AllFiles:     []string{},
		FilesByName:  map[string][]string{},
		FilesScanned: 0,
	}

	_ = filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() {
			base := filepath.Base(path)
			if excludedDirs[base] {
				return filepath.SkipDir
			}
			return nil
		}

		// Skip test paths
		if IsExcludedPath(path, opts) {
			return nil
		}

		abs, _ := filepath.Abs(path)
		idx.AllFiles = append(idx.AllFiles, abs)
		idx.FilesScanned++

		base := filepath.Base(abs)
		idx.FilesByName[base] = append(idx.FilesByName[base], abs)

		return nil
	})

	return idx
}

// FindByName returns files whose basename contains the given substring.
func (idx *FSIndex) FindByName(nameSubstr string) []string {
	var out []string
	for name, paths := range idx.FilesByName {
		if strings.Contains(name, nameSubstr) {
			out = append(out, paths...)
		}
	}
	return out
}

// FindByExactName returns files whose basename exactly matches.
func (idx *FSIndex) FindByExactName(name string) []string {
	return idx.FilesByName[name]
}

// FindByExtension returns files whose name ends with the given extension.
func (idx *FSIndex) FindByExtension(ext string) []string {
	if !strings.HasPrefix(ext, ".") {
		ext = "." + ext
	}
	var out []string
	for _, fp := range idx.AllFiles {
		if strings.HasSuffix(strings.ToLower(fp), strings.ToLower(ext)) {
			out = append(out, fp)
		}
	}
	return out
}

// ReadFileLimited reads file content with a size limit.
func ReadFileLimited(path string, maxSize int64) (string, bool) {
	info, err := os.Stat(path)
	if err != nil {
		return "", false
	}
	if info.Size() > maxSize {
		// Still read, but only up to maxSize
		f, err := os.Open(path)
		if err != nil {
			return "", false
		}
		defer f.Close()
		buf := make([]byte, maxSize)
		n, _ := f.Read(buf)
		return string(buf[:n]), true
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", false
	}
	return string(data), true
}

// IsExcludedPath determines whether a path should be excluded from scanning.
func IsExcludedPath(path string, opts *ProbeOptions) bool {
	normalized := filepath.ToSlash(path)

	// Skip test paths
	for _, frag := range excludedTestPathFragments {
		if strings.Contains(normalized, frag) {
			return true
		}
	}

	// Apply scope-exclude filter
	if opts != nil {
		for _, exc := range opts.ScopeExclude {
			if exc != "" && strings.Contains(normalized, exc) {
				return true
			}
		}

		// Apply scope-modules filter: path must contain at least one module
		if len(opts.ScopeModules) > 0 {
			matched := false
			for _, mod := range opts.ScopeModules {
				if mod != "" && strings.Contains(normalized, "/"+mod+"/") {
					matched = true
					break
				}
			}
			if !matched {
				return true
			}
		}
	}

	return false
}
