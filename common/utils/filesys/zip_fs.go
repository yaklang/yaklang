package filesys

import (
	"archive/zip"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	fi "github.com/yaklang/yaklang/common/utils/filesys/filesys_interface"
	"github.com/yaklang/yaklang/common/utils/memfile"
)

type ZipFS struct {
	r      *zip.Reader
	forest *utils.PathForest
}

func (z *ZipFS) IsAbs(s string) bool {
	return false
}

func (z *ZipFS) Getwd() (string, error) {
	return ".", nil
}

func (z *ZipFS) Exists(s string) (bool, error) {
	info, err := z.Stat(s)
	if err != nil {
		return false, nil
	}
	return info != nil, nil
}

func (z *ZipFS) Rename(s string, s2 string) error {
	return utils.Error("unsupported on readonly zipfs")
}

// Rel is calc relative path
func (z *ZipFS) Rel(s string, s2 string) (string, error) {
	return "", utils.Error("unsupported on readonly zipfs")
}

func (z *ZipFS) WriteFile(s string, bytes []byte, mode os.FileMode) error {
	return utils.Error("unsupported on readonly zipfs")
}

func (z *ZipFS) Delete(s string) error {
	return utils.Error("unsupported on readonly zipfs")
}

func (z *ZipFS) MkdirAll(s string, mode os.FileMode) error {
	return utils.Error("unsupported on readonly zipfs")
}

type zipDir struct {
	zipfile *zip.File
}

func (z zipDir) Stat() (fs.FileInfo, error) {
	return z.zipfile.FileInfo(), nil
}

func (z zipDir) Read(bytes []byte) (int, error) {
	return 0, utils.Error("zipDir cannot be read")
}

func (z zipDir) Close() error {
	return nil
}

var _ fs.File = (*zipDir)(nil)

func zipPathClean(p string) string {
	p = path.Clean(p)
	if p == "." {
		return "./"
	}
	if p == "/" {
		return "./"
	}
	if !strings.HasPrefix(p, "./") {
		p = "./" + p
	}
	return p
}

func (z *ZipFS) Open(name string) (fs.File, error) {
	raw, err := z.ReadFile(name)
	if err != nil {
		p, err := z.forest.Get(name)
		if err != nil {
			return nil, err
		}
		if p == nil {
			return nil, os.ErrNotExist
		}
		if p.Value == nil {
			return nil, os.ErrNotExist
		}
		f, ok := p.Value.(*zip.File)
		if !ok {
			return nil, os.ErrNotExist
		}
		return zipDir{zipfile: f}, nil
	}
	return memfile.New(raw), nil
}

func (z *ZipFS) OpenFile(name string, flag int, perm os.FileMode) (fs.File, error) {
	return z.Open(name)
}

func (z *ZipFS) Stat(name string) (fs.FileInfo, error) {
	if name == "." {
		name = ""
	}
	name = z.Clean(name)
	f, err := z.forest.Get(name)
	if err != nil {
		return nil, err
	}
	if f == nil {
		return nil, os.ErrNotExist
	}

	if f.Value == nil {
		if len(f.Children) > 0 {
			return &VirtualFileInfo{
				name: name,
				mod:  fs.ModeDir,
			}, nil
		}
		return nil, os.ErrNotExist
	}
	v, ok := f.Value.(*zip.File)
	if !ok {
		return nil, os.ErrNotExist
	}
	return v.FileInfo(), nil
}

var _ fi.FileSystem = (*ZipFS)(nil)

func (z *ZipFS) Clean(name string) string {
	name = filepath.ToSlash(name)
	return zipPathClean(name)
}

func (z *ZipFS) Join(name ...string) string {
	return path.Join(name...)
}

func (z *ZipFS) GetSeparators() rune {
	return '/'
}

func (z *ZipFS) PathSplit(s string) (string, string) {
	return SplitWithSeparator(s, z.GetSeparators())
}

func (z *ZipFS) Ext(i string) string {
	return getExtension(i)
}

func (z *ZipFS) ReadFile(name string) ([]byte, error) {
	name = z.Clean(name)
	node, err := z.forest.Get(name)
	if err != nil {
		return nil, utils.Wrapf(err, "get %v failed", name)
	}
	if node == nil || utils.IsNil(node.Value) {
		return nil, os.ErrNotExist
	}
	f, ok := node.Value.(*zip.File)
	if !ok {
		return nil, os.ErrNotExist
	}
	if f.FileInfo().IsDir() {
		return nil, utils.Wrapf(os.ErrNotExist, "%v is dir", name)
	}
	rc, err := f.Open()
	if err != nil {
		return nil, err
	}
	defer rc.Close()
	return io.ReadAll(rc)
}

func (f *ZipFS) String() string {
	// TODO
	return ""
}

type ZipDirEntry struct {
	name string
	info fs.FileInfo
}

func (z *ZipDirEntry) Name() string {
	return z.name
}

func (z *ZipDirEntry) IsDir() bool {
	return z.info.IsDir()
}

func (z *ZipDirEntry) Type() fs.FileMode {
	if z.info.IsDir() {
		return fs.ModeDir
	}
	return 0o666
}

func (z *ZipDirEntry) Info() (fs.FileInfo, error) {
	return z.info, nil
}

var _ fs.DirEntry = (*ZipDirEntry)(nil)

func (z *ZipFS) ReadDir(name string) ([]fs.DirEntry, error) {
	name = z.Clean(name)
	node, err := z.forest.Get(name)
	if err != nil {
		return nil, utils.Wrapf(err, "get %#v failed", name)
	}
	if node == nil {
		return nil, os.ErrNotExist
	}

	var entries []fs.DirEntry
	for _, c := range node.Children {
		f, ok := c.Value.(*zip.File)
		if ok {
			entries = append(entries, &ZipDirEntry{
				name: c.Name,
				info: f.FileInfo(),
			})
		} else {
			entries = append(entries, &ZipDirEntry{
				name: c.Name,
				info: NewVirtualFileInfo(c.Name, 0, true),
			})
		}
	}
	return entries, nil
}
func (f *ZipFS) ExtraInfo(string) map[string]any { return nil }
func (f *ZipFS) Base(p string) string            { return path.Base(p) }

func NewZipFSRaw(i io.ReaderAt, size int64) (*ZipFS, error) {
	reader, err := zip.NewReader(i, size)
	if err != nil {
		return nil, err
	}

	forest, err := utils.GeneratePathTrees()
	if err != nil {
		return nil, err
	}

	for _, f := range reader.File {
		name := f.Name
		err := forest.AddPath(zipPathClean(name), f)
		if err != nil {
			log.Warnf("BUG: cache zip tree failed: %v", err)
			continue
		}
	}
	forest.ReadOnly()
	return &ZipFS{r: reader, forest: forest}, nil
}

func NewZipFSFromString(i string) (*ZipFS, error) {
	mf := memfile.New([]byte(i))
	return NewZipFSRaw(mf, int64(len([]byte(i))))
}

func NewZipFSFromLocal(i string) (*ZipFS, error) {
	local := NewLocalFs()
	f, err := local.Open(i)
	if err != nil {
		return nil, err
	}
	ra, ok := f.(io.ReaderAt)
	if !ok {
		return nil, err
	}
	info, err := f.Stat()
	if err != nil {
		return nil, err
	}
	return NewZipFSRaw(ra, info.Size())
}
