package filesys

import (
	"bytes"
	"embed"
	"errors"
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

// ErrNoFileFound is returned when CreateEmbedFSHash finds no files to process
var ErrNoFileFound = errors.New("no file found")

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

// NewEmbedSubFS creates a FileSystem from an embed.FS, rooted at the given subdirectory.
// This is useful when //go:embed mydir embeds files under "mydir/", but the caller
// wants to read files relative to "mydir/" (e.g. ReadFile("file.txt") instead of
// ReadFile("mydir/file.txt")). In CI, gzip-embed transform replaces this with
// PreprocessingEmbed which naturally strips the directory prefix.
func NewEmbedSubFS(emb embed.FS, subDir string) fi.FileSystem {
	sub, err := fs.Sub(emb, subDir)
	if err != nil {
		return &embedFs{emb} // fallback: use full path
	}
	return &subFS{sub}
}

type subFS struct {
	fs fs.FS
}

func (f *subFS) ReadFile(name string) ([]byte, error) {
	return fs.ReadFile(f.fs, name)
}

func (f *subFS) Open(name string) (fs.File, error) {
	return f.fs.Open(name)
}

func (f *subFS) OpenFile(name string, flag int, perm os.FileMode) (fs.File, error) {
	return f.fs.Open(name)
}

func (f *subFS) Stat(name string) (fs.FileInfo, error) {
	return fs.Stat(f.fs, name)
}

func (f *subFS) ReadDir(name string) ([]fs.DirEntry, error) {
	return fs.ReadDir(f.fs, name)
}

func (f *subFS) PathSplit(s string) (string, string) {
	return SplitWithSeparator(s, f.GetSeparators())
}

func (f *subFS) Ext(s string) string {
	return getExtension(s)
}

func (f *subFS) GetSeparators() rune { return '/' }
func (f *subFS) Join(elem ...string) string { return path.Join(elem...) }
func (f *subFS) Base(name string) string { return path.Base(name) }
func (f *subFS) IsAbs(name string) bool { return len(name) > 0 && name[0] == '/' }
func (f *subFS) Getwd() (string, error) { return ".", nil }
func (f *subFS) Exists(name string) (bool, error) {
	_, err := fs.Stat(f.fs, name)
	return err == nil, err
}
func (f *subFS) Rel(basepath, targpath string) (string, error) {
	if strings.HasPrefix(targpath, basepath) {
		return strings.TrimPrefix(targpath, basepath), nil
	}
	return "", errors.New("cannot make relative path")
}
func (f *subFS) Rename(oldname, newname string) error {
	return errors.New("rename not supported in read-only embed sub filesystem")
}
func (f *subFS) WriteFile(name string, data []byte, perm os.FileMode) error {
	return errors.New("write not supported in read-only embed sub filesystem")
}
func (f *subFS) Delete(name string) error {
	return errors.New("delete not supported in read-only embed sub filesystem")
}
func (f *subFS) MkdirAll(path string, perm os.FileMode) error {
	return errors.New("mkdir not supported in read-only embed sub filesystem")
}
func (f *subFS) ExtraInfo(name string) map[string]any {
	return map[string]any{"type": "embed_sub_fs"}
}
func (f *subFS) GetHash() (string, error) {
	return "", nil
}

func CreateEmbedFSHash(f embed.FS, opts ...Option) (string, error) {
	var hashes []string

	// Build options: start with embed FS, then add our file stat handler
	// Extension filtering (WithIncludeExts/WithExcludeExts) is handled by wrapping onFileStat
	// in the option functions themselves, so it will be executed in Recursive
	allOpts := make([]Option, 0, len(opts)+2)
	allOpts = append(allOpts, WithFileSystem(NewEmbedFS(f)))
	allOpts = append(allOpts, WithFileStat(func(s string, info fs.FileInfo) error {
		result, err := f.ReadFile(s)
		if err != nil {
			return err
		}
		hash := codec.Sha256(result)
		hashes = append(hashes, hash)
		return nil
	}))
	allOpts = append(allOpts, opts...)

	err := Recursive(".", allOpts...)
	if err != nil {
		return "", err
	}
	if len(hashes) <= 0 {
		return "", ErrNoFileFound
	}

	// Sort by path first to ensure consistent ordering
	sort.Strings(hashes)
	return codec.Sha256([]byte(strings.Join(hashes, "|"))), nil
}

// CreateLocalFSHash calculates the same hash as CreateEmbedFSHash but from a
// local directory instead of an embed.FS. It is used by embed-fs-hash to
// generate hash files from a repository checkout without compiling the binary.
func CreateLocalFSHash(dir string, opts ...Option) (string, error) {
	var hashes []string

	allOpts := make([]Option, 0, len(opts)+3)
	allOpts = append(allOpts, WithFileSystem(NewLocalFs()))
	// embed.FS excludes files and directories whose names start with "." or "_",
	// so replicate that behavior to keep local hashes identical to embed hashes.
	allOpts = append(allOpts, WithDirStat(func(s string, info fs.FileInfo) error {
		if strings.HasPrefix(info.Name(), ".") || strings.HasPrefix(info.Name(), "_") {
			return SkipDir
		}
		return nil
	}))
	allOpts = append(allOpts, WithFileStat(func(s string, info fs.FileInfo) error {
		if strings.HasPrefix(info.Name(), ".") || strings.HasPrefix(info.Name(), "_") {
			return nil
		}
		result, err := os.ReadFile(s)
		if err != nil {
			return err
		}
		hashes = append(hashes, codec.Sha256(result))
		return nil
	}))
	allOpts = append(allOpts, opts...)

	err := Recursive(dir, allOpts...)
	if err != nil {
		return "", err
	}
	if len(hashes) <= 0 {
		return "", ErrNoFileFound
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
