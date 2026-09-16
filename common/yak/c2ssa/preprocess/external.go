package preprocess

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils/filesys"
	fi "github.com/yaklang/yaklang/common/utils/filesys/filesys_interface"
)

// openExternalIncludeFS opens host directories and zip archives as include roots.
func openExternalIncludeFS(dirs []string) []fi.FileSystem {
	var out []fi.FileSystem
	seen := make(map[string]bool)
	for _, dir := range dirs {
		dir = strings.TrimSpace(dir)
		if dir == "" {
			continue
		}
		abs, err := filepath.Abs(dir)
		if err != nil {
			abs = dir
		}
		if seen[abs] {
			continue
		}
		seen[abs] = true
		fs, err := openOneExternalRoot(abs)
		if err != nil {
			log.Warnf("c preprocess: skip external include root %q: %v", abs, err)
			continue
		}
		if fs != nil {
			out = append(out, fs)
		}
	}
	return out
}

// OpenExternalRoot opens a host directory or zip/jar as an include filesystem.
func OpenExternalRoot(path string) (fi.FileSystem, error) {
	return openOneExternalRoot(path)
}

func openOneExternalRoot(path string) (fi.FileSystem, error) {
	ext := strings.ToLower(filepath.Ext(path))
	if ext == ".zip" || ext == ".jar" {
		return filesys.NewZipFSFromLocal(path)
	}
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return nil, os.ErrNotExist
	}
	return filesys.NewRelLocalFs(path), nil
}

func readExternalHeader(roots []fi.FileSystem, includePath string) (stored string, content []byte, ok bool) {
	if len(roots) == 0 || includePath == "" {
		return "", nil, false
	}
	includePath = normalizeSlash(includePath)
	candidates := []string{
		includePath,
		"include/" + includePath,
		"usr/include/" + includePath,
	}
	seen := make(map[string]bool)
	for _, root := range roots {
		if root == nil {
			continue
		}
		for _, c := range candidates {
			if seen[c] {
				continue
			}
			seen[c] = true
			data, err := readFSFile(root, c)
			if err != nil || data == nil {
				continue
			}
			return "ext:" + c, data, true
		}
		seen = make(map[string]bool)
	}
	return "", nil, false
}

func readFSFile(fs fi.FileSystem, name string) ([]byte, error) {
	data, err := fs.ReadFile(name)
	if err == nil {
		return data, nil
	}
	if sep := fs.GetSeparators(); sep != '/' {
		alt := strings.ReplaceAll(name, "/", string(sep))
		if alt != name {
			return fs.ReadFile(alt)
		}
	}
	return nil, err
}
