package filesys

import (
	"bytes"
	"embed"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/yaklang/yaklang/common/yak/yaklib/codec"

	"github.com/yaklang/yaklang/common/utils"
	fi "github.com/yaklang/yaklang/common/utils/filesys/filesys_interface"
)

type embedFs struct {
	f embed.FS
}

func (f *embedFs) PathSplit(s string) (string, string) {
	return SplitWithSeparator(s, f.GetSeparators())
}

func (f *embedFs) Ext(s string) string {
	return getExtension(s)
}

var _ fi.FileSystem = (*embedFs)(nil)

func (f *embedFs) ReadFile(name string) ([]byte, error) {
	fn, err := f.f.Open(name)
	if err != nil {
		return nil, err
	}
	defer fn.Close()
	return io.ReadAll(fn)
}
func (f *embedFs) ReadDir(dirname string) ([]fs.DirEntry, error) { return f.f.ReadDir(dirname) }
func (f *embedFs) Open(name string) (fs.File, error)             { return f.f.Open(name) }
func (f *embedFs) OpenFile(name string, flag int, perm os.FileMode) (fs.File, error) {
	return f.f.Open(name)
}

func (f *embedFs) Stat(name string) (fs.FileInfo, error) {
	fn, err := f.f.Open(name)
	if err != nil {
		return nil, err
	}
	return fn.Stat()
}

func (f *embedFs) GetSeparators() rune                         { return '/' }
func (f *embedFs) Join(paths ...string) string                 { return path.Join(paths...) }
func (f *embedFs) IsAbs(name string) bool                      { return len(name) > 0 && name[0] == byte(f.GetSeparators()) }
func (f *embedFs) Getwd() (string, error)                      { return "", nil }
func (f *embedFs) Exists(path string) (bool, error)            { _, err := f.f.Open(path); return err == nil, err }
func (f *embedFs) Rename(string, string) error                 { return utils.Error("implement me") }
func (f *embedFs) Rel(string, string) (string, error)          { return "", utils.Error("implement me") }
func (f *embedFs) WriteFile(string, []byte, os.FileMode) error { return utils.Error("implement me") }
func (f *embedFs) Delete(string) error                         { return utils.Error("implement me") }
func (f *embedFs) MkdirAll(string, os.FileMode) error          { return utils.Error("implement me") }
func (f *embedFs) ExtraInfo(string) map[string]any             { return nil }
func (f *embedFs) Base(p string) string                        { return path.Base(p) }

func (f *embedFs) String() string {
	// TODO
	return ""
}

func NewEmbedFS(fs embed.FS) fi.FileSystem {
	return &embedFs{fs}
}

func CreateEmbedFSHash(f embed.FS) (string, error) {
	var hashes []string
	err := Recursive(".", WithFileSystem(NewEmbedFS(f)), WithFileStat(func(s string, info fs.FileInfo) error {
		result, err := f.ReadFile(s)
		if err != nil {
			return err
		}
		hash := codec.Sha256(result)
		hashes = append(hashes, hash)
		return nil
	}))
	if err != nil {
		return "", err
	}
	if len(hashes) <= 0 {
		return "", utils.Error("no file found")
	}
	sort.Strings(hashes)
	return codec.Sha256([]byte(strings.Join(hashes, "|"))), nil
}

// local FileSystem
type LocalFs struct {
	cache *utils.CacheWithKey[string, *bytes.Buffer]
}

func (f *LocalFs) PathSplit(s string) (string, string) {
	return SplitWithSeparator(s, f.GetSeparators())
}

func (f *LocalFs) Ext(s string) string {
	return getExtension(s)
}

func NewLocalFs() *LocalFs {
	return &LocalFs{
		cache: utils.NewTTLCacheWithKey[string, *bytes.Buffer](15 * time.Second),
	}
}

var _ fi.FileSystem = (*LocalFs)(nil)

func (f *LocalFs) ReadFile(name string) ([]byte, error) {
	if f.cache == nil {
		return os.ReadFile(name)
	}
	if v, ok := f.cache.Get(name); ok {
		return v.Bytes(), nil
	}
	data, err := os.ReadFile(name)
	if err == nil {
		f.cache.Set(name, bytes.NewBuffer(data))
	}
	return data, err
}
func (f *LocalFs) Open(name string) (fs.File, error) { return os.Open(name) }
func (f *LocalFs) OpenFile(name string, flag int, perm os.FileMode) (fs.File, error) {
	return os.OpenFile(name, flag, perm)
}
func (f *LocalFs) Stat(name string) (fs.FileInfo, error)          { return os.Stat(name) }
func (f *LocalFs) ReadDir(dirname string) ([]fs.DirEntry, error)  { return os.ReadDir(dirname) }
func (f *LocalFs) GetSeparators() rune                            { return filepath.Separator }
func (f *LocalFs) Join(paths ...string) string                    { return filepath.Join(paths...) }
func (f *LocalFs) IsAbs(name string) bool                         { return filepath.IsAbs(name) }
func (f *LocalFs) Getwd() (string, error)                         { return os.Getwd() }
func (f *LocalFs) Exists(path string) (bool, error)               { return utils.PathExists(path) }
func (f *LocalFs) Rename(old string, new string) error            { return os.Rename(old, new) }
func (f *LocalFs) Rel(base string, target string) (string, error) { return filepath.Rel(base, target) }
func (f *LocalFs) WriteFile(name string, data []byte, perm os.FileMode) error {
	return os.WriteFile(name, data, perm)
}
func (f *LocalFs) Delete(name string) error                     { return os.RemoveAll(name) }
func (f *LocalFs) MkdirAll(name string, perm os.FileMode) error { return os.MkdirAll(name, perm) }
func (f *LocalFs) ExtraInfo(string) map[string]any              { return nil }
func (f *LocalFs) Base(p string) string                         { return filepath.Base(p) }

func (f *LocalFs) String() string { return "" }
