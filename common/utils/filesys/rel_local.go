package filesys

import (
	"bytes"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	"github.com/yaklang/yaklang/common/utils"
	fi "github.com/yaklang/yaklang/common/utils/filesys/filesys_interface"
)

type RelLocalFs struct {
	rel   string
	cache *utils.CacheWithKey[string, *bytes.Buffer]
}

var _ fi.FileSystem = (*RelLocalFs)(nil)

func (f *RelLocalFs) PathSplit(s string) (string, string) {
	return SplitWithSeparator(s, f.GetSeparators())
}

func (f *RelLocalFs) Ext(s string) string {
	return getExtension(s)
}

func NewRelLocalFs(rel string) *RelLocalFs {
	return &RelLocalFs{
		rel:   rel,
		cache: utils.NewTTLCacheWithKey[string, *bytes.Buffer](15 * time.Second),
	}
}

func (f *RelLocalFs) ReadFile(name string) ([]byte, error) {
	name = f.Join(f.rel, name)
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
func (f *RelLocalFs) Open(name string) (fs.File, error) {
	name = f.Join(f.rel, name)
	return os.Open(name)
}
func (f *RelLocalFs) OpenFile(name string, flag int, perm os.FileMode) (fs.File, error) {
	name = f.Join(f.rel, name)
	return os.OpenFile(name, flag, perm)
}
func (f *RelLocalFs) Stat(name string) (fs.FileInfo, error) {
	name = f.Join(f.rel, name)
	return os.Stat(name)
}
func (f *RelLocalFs) ReadDir(dirname string) ([]fs.DirEntry, error) {
	dirname = f.Join(f.rel, dirname)
	return os.ReadDir(dirname)
}
func (f *RelLocalFs) GetSeparators() rune         { return filepath.Separator }
func (f *RelLocalFs) Join(paths ...string) string { return filepath.Join(paths...) }
func (f *RelLocalFs) IsAbs(name string) bool      { return filepath.IsAbs(name) }
func (f *RelLocalFs) Getwd() (string, error)      { return f.rel, nil }
func (f *RelLocalFs) Exists(path string) (bool, error) {
	path = f.Join(f.rel, path)
	return utils.PathExists(path)
}
func (f *RelLocalFs) Rename(old string, new string) error {
	old = f.Join(f.rel, old)
	new = f.Join(f.rel, new)
	return os.Rename(old, new)
}
func (f *RelLocalFs) Rel(base string, target string) (string, error) {
	base = f.Join(f.rel, base)
	target = f.Join(f.rel, target)
	return filepath.Rel(base, target)
}
func (f *RelLocalFs) WriteFile(name string, data []byte, perm os.FileMode) error {
	name = f.Join(f.rel, name)
	return os.WriteFile(name, data, perm)
}
func (f *RelLocalFs) Delete(name string) error {
	name = f.Join(f.rel, name)
	return os.RemoveAll(name)
}
func (f *RelLocalFs) MkdirAll(name string, perm os.FileMode) error {
	name = f.Join(f.rel, name)
	return os.MkdirAll(name, perm)
}

func (f *RelLocalFs) String() string {
	return fmt.Sprintf("RelLocalFs{rel: %s}", f.rel)
}

func (f *RelLocalFs) Root() string {
	return f.rel
}

func (f *RelLocalFs) ExtraInfo(string) map[string]any { return nil }
func (f *RelLocalFs) Base(p string) string            { return filepath.Base(p) }
