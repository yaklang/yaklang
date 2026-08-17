package aotlib

import (
	"os"
	"path/filepath"
)

func FileIsExisted(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func FileIsFile(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

func FileIsDir(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

func FileJoin(parts ...string) string   { return filepath.Join(parts...) }
func FileGetBase(path string) string    { return filepath.Base(path) }
func FileGetExt(path string) string     { return filepath.Ext(path) }
func FileGetDirPath(path string) string { return filepath.Dir(path) }
func FileClean(path string) string      { return filepath.Clean(path) }
func FileIsAbs(path string) bool        { return filepath.IsAbs(path) }
func FileAbs(path string) string {
	p, err := filepath.Abs(path)
	if err != nil {
		return path
	}
	return p
}

// FileExports mirrors the file module's export table (the AOT-supported
// subset). Entries match common/yak/yaklib.FileExport signatures.
var FileExports = map[string]any{
	"IsExisted":  FileIsExisted,
	"IsFile":     FileIsFile,
	"IsDir":      FileIsDir,
	"Join":       FileJoin,
	"GetBase":    FileGetBase,
	"GetExt":     FileGetExt,
	"GetDirPath": FileGetDirPath,
	"Clean":      FileClean,
	"IsAbs":      FileIsAbs,
	"Abs":        FileAbs,
}
