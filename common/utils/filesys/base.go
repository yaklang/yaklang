package filesys

import (
	"bytes"
	"embed"
	"github.com/yaklang/yaklang/common/utils"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"time"
)

type embedFs struct {
	f embed.FS
}

func (e *embedFs) PathSplit(s string) (string, string) {
	return splitWithSeparator(s, e.GetSeparators())
}

func (e *embedFs) Ext(s string) string {
	return getExtension(s)
}

var _ FileSystem = (*embedFs)(nil)

func (e *embedFs) ReadFile(name string) ([]byte, error) {
	fn, err := e.f.Open(name)
	if err != nil {
		return nil, err
	}
	defer fn.Close()
	return io.ReadAll(fn)
}
func (e *embedFs) ReadDir(dirname string) ([]fs.DirEntry, error) { return e.f.ReadDir(dirname) }
func (e *embedFs) Open(name string) (fs.File, error)             { return e.f.Open(name) }
func (e *embedFs) Stat(name string) (fs.FileInfo, error) {
	fn, err := e.f.Open(name)
	if err != nil {
		return nil, err
	}
	return fn.Stat()
}

func (e *embedFs) GetSeparators() rune { return '/' }

func (f *embedFs) Join(name ...string) string {
	return path.Join(name...)
}

func NewEmbedFS(fs embed.FS) FileSystem {
	return &embedFs{fs}
}

// local filesystem
type LocalFs struct {
	cache *utils.CacheWithKey[string, *bytes.Buffer]
}

func (f *LocalFs) PathSplit(s string) (string, string) {
	return splitWithSeparator(s, f.GetSeparators())
}

func (f *LocalFs) Ext(s string) string {
	return getExtension(s)
}

func NewLocalFs() *LocalFs {
	return &LocalFs{
		cache: utils.NewTTLCacheWithKey[string, *bytes.Buffer](15 * time.Second),
	}
}

var _ FileSystem = (*LocalFs)(nil)

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
func (f *LocalFs) Open(name string) (fs.File, error)             { return os.Open(name) }
func (f *LocalFs) Stat(name string) (fs.FileInfo, error)         { return os.Stat(name) }
func (f *LocalFs) ReadDir(dirname string) ([]fs.DirEntry, error) { return os.ReadDir(dirname) }
func (f *LocalFs) GetSeparators() rune                           { return filepath.Separator }
func (f *LocalFs) Join(name ...string) string                    { return filepath.Join(name...) }
