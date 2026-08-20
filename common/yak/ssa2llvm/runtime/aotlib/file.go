package aotlib

import (
	"fmt"
	"io"
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

func FileJoin(parts ...string) string        { return filepath.Join(parts...) }
func FileGetBase(path string) string         { return filepath.Base(path) }
func FileGetExt(path string) string          { return filepath.Ext(path) }
func FileGetDirPath(path string) string      { return filepath.Dir(path) }
func FileClean(path string) string           { return filepath.Clean(path) }
func FileIsAbs(path string) bool             { return filepath.IsAbs(path) }
func FileSplit(path string) (string, string) { return filepath.Split(path) }
func FileAbs(path string) string {
	p, err := filepath.Abs(path)
	if err != nil {
		return path
	}
	return p
}

func FileMkdirAll(path string) error { return os.MkdirAll(path, 0o755) }

func FileRemove(path string) error { return os.Remove(path) }

func FileReadFile(path string) (string, error) {
	b, err := os.ReadFile(path)
	return string(b), err
}

func FileSave(path string, content any) error {
	var data []byte
	switch v := content.(type) {
	case string:
		data = []byte(v)
	case []byte:
		data = v
	default:
		data = []byte(fmt.Sprintf("%v", v))
	}
	if dir := filepath.Dir(path); dir != "" && dir != "." {
		_ = os.MkdirAll(dir, 0o755)
	}
	return os.WriteFile(path, data, 0o644)
}

func FileTempFileName(prefix string) string {
	f, err := os.CreateTemp("", prefix+"*")
	if err != nil {
		return filepath.Join(os.TempDir(), prefix)
	}
	name := f.Name()
	f.Close()
	return name
}

func FileOpen(path string) (any, error) {
	// yak's file.Open creates the file when missing and opens read-write.
	return os.OpenFile(path, os.O_CREATE|os.O_RDWR, os.ModePerm)
}

func FileCat(path string) (string, error) {
	b, err := os.ReadFile(path)
	return string(b), err
}

func FileWalk(root string, fn any) error {
	cb, ok := fn.(func(string, os.FileInfo, error) error)
	if !ok {
		return fmt.Errorf("file.Walk callback has unsupported type %T", fn)
	}
	return filepath.Walk(root, cb)
}

func FileCopy(dst, src string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, in)
	return err
}

// FileExports mirrors the file module's export table (the AOT-supported
// subset). Entries match common/yak/yaklib.FileExport signatures.
var FileExports = map[string]any{
	"IsExisted":    FileIsExisted,
	"IsFile":       FileIsFile,
	"IsDir":        FileIsDir,
	"Join":         FileJoin,
	"GetBase":      FileGetBase,
	"GetExt":       FileGetExt,
	"GetDirPath":   FileGetDirPath,
	"Clean":        FileClean,
	"IsAbs":        FileIsAbs,
	"Abs":          FileAbs,
	"Split":        FileSplit,
	"MkdirAll":     FileMkdirAll,
	"Remove":       FileRemove,
	"ReadFile":     FileReadFile,
	"Save":         FileSave,
	"TempFileName": FileTempFileName,
	"Open":         FileOpen,
	"Cat":          FileCat,
	"Walk":         FileWalk,
	"Copy":         FileCopy,
}
