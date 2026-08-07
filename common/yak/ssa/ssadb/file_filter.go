package ssadb

import (
	"context"
	"strings"

	"github.com/yaklang/yaklang/common/utils/chanx"
)

// FileFilterMode selects how path sets are applied.
type FileFilterMode int

const (
	FileFilterNone FileFilterMode = iota
	FileFilterExclude
	FileFilterInclude
)

// BuildFilePathSet builds a lookup set for include/exclude matching.
// programName (optional) also indexes program-prefix-stripped forms.
func BuildFilePathSet(files []string, programName string) map[string]struct{} {
	if len(files) == 0 {
		return nil
	}
	set := make(map[string]struct{}, len(files)*4)
	add := func(path string) {
		if path == "" {
			return
		}
		n := NormalizeFilePath(path)
		set[n] = struct{}{}
		set[strings.TrimPrefix(n, "/")] = struct{}{}
	}
	for _, filePath := range files {
		add(filePath)
		if programName != "" {
			add(StripProgramNamePrefix(filePath, programName))
		}
	}
	return set
}

// PathInFileSet reports whether filePath matches any key in set.
func PathInFileSet(filePath string, set map[string]struct{}, programName string) bool {
	if len(set) == 0 || filePath == "" {
		return false
	}
	normalized := NormalizeFilePath(filePath)
	if _, ok := set[normalized]; ok {
		return true
	}
	if _, ok := set[strings.TrimPrefix(normalized, "/")]; ok {
		return true
	}
	if programName != "" {
		stripped := NormalizeFilePath(StripProgramNamePrefix(filePath, programName))
		if _, ok := set[stripped]; ok {
			return true
		}
		if _, ok := set[strings.TrimPrefix(stripped, "/")]; ok {
			return true
		}
	}
	return false
}

// PathPassesFileFilter keeps empty paths (extern/lib) under both include and exclude.
// Empty include set → reject; empty exclude set → keep.
func PathPassesFileFilter(filePath string, pathSet map[string]struct{}, mode FileFilterMode, programName string) bool {
	if mode == FileFilterNone || len(pathSet) == 0 {
		return mode != FileFilterInclude
	}
	if filePath == "" {
		return true
	}
	inSet := PathInFileSet(filePath, pathSet, programName)
	switch mode {
	case FileFilterInclude:
		return inSet
	case FileFilterExclude:
		return !inSet
	default:
		return true
	}
}

// NormalizeFilePath ensures a leading "/".
func NormalizeFilePath(filePath string) string {
	if filePath == "" {
		return ""
	}
	if !strings.HasPrefix(filePath, "/") {
		return "/" + filePath
	}
	return filePath
}

// StripProgramNamePrefix removes "/prog/" or "/prog(timestamp)/" prefix when present.
func StripProgramNamePrefix(filePath, programName string) string {
	if filePath == "" || programName == "" {
		return filePath
	}
	path := strings.TrimPrefix(filePath, "/")
	parts := strings.Split(path, "/")
	if len(parts) == 0 {
		return filePath
	}
	first := parts[0]
	if first == programName || strings.HasPrefix(first, programName+"(") {
		if len(parts) > 1 {
			return "/" + strings.Join(parts[1:], "/")
		}
		return "/"
	}
	return filePath
}

// ResolveIrCodeFilePath returns the normalized source path for ir.
// pathCache maps SourceCodeHash → path to avoid repeated editor loads.
func ResolveIrCodeFilePath(ir *IrCode, pathCache map[string]string) string {
	if ir == nil || ir.IsEmptySourceCodeHash() {
		return ""
	}
	hash := ir.SourceCodeHash
	if pathCache != nil {
		if path, ok := pathCache[hash]; ok {
			return path
		}
	}
	editor, err := GetEditorByHash(hash)
	if err != nil || editor == nil {
		if pathCache != nil {
			pathCache[hash] = ""
		}
		return ""
	}
	path := editor.GetFilePath()
	if path == "" {
		path = editor.GetUrl()
	}
	path = NormalizeFilePath(path)
	if pathCache != nil {
		pathCache[hash] = path
	}
	return path
}

// IrCodePassesFileFilter is PathPassesFileFilter for IrCode with hash→path cache.
func IrCodePassesFileFilter(ir *IrCode, pathSet map[string]struct{}, mode FileFilterMode, programName string, pathCache map[string]string) bool {
	if mode == FileFilterNone || len(pathSet) == 0 {
		return mode != FileFilterInclude
	}
	return PathPassesFileFilter(ResolveIrCodeFilePath(ir, pathCache), pathSet, mode, programName)
}

// FilterIrCodeChan applies include/exclude file filtering to an IrCode stream.
func FilterIrCodeChan(ctx context.Context, in <-chan *IrCode, files []string, mode FileFilterMode, programName string) <-chan *IrCode {
	pathSet := BuildFilePathSet(files, programName)
	if mode == FileFilterInclude && len(pathSet) == 0 {
		return emptyIrCodeChan()
	}
	if mode == FileFilterNone || (mode == FileFilterExclude && len(pathSet) == 0) {
		return in
	}
	outC := chanx.NewUnlimitedChan[*IrCode](ctx, 100)
	go func() {
		defer outC.Close()
		pathCache := make(map[string]string)
		for ir := range in {
			if !IrCodePassesFileFilter(ir, pathSet, mode, programName, pathCache) {
				continue
			}
			outC.SafeFeed(ir)
		}
	}()
	return outC.OutputChannel()
}
