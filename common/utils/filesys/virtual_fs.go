package filesys

import (
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"strings"
	"time"

	"github.com/yaklang/yaklang/common/utils"
	fi "github.com/yaklang/yaklang/common/utils/filesys/filesys_interface"
	"github.com/yaklang/yaklang/common/utils/omap"
)

type VirtualFS struct {
	files    *omap.OrderedMap[string, *VirtualFile]
	dirEntry []fs.DirEntry
}

func (f *VirtualFS) PathSplit(s string) (string, string) {
	return SplitWithSeparator(s, f.GetSeparators())
}
func (f *VirtualFS) Ext(s string) string { return getExtension(s) }
func (f *VirtualFS) IsAbs(name string) bool {
	return len(name) > 0 && name[0] == byte(f.GetSeparators())
}
func (f *VirtualFS) Getwd() (string, error)             { return ".", nil }
func (f *VirtualFS) Exists(path string) (bool, error)   { _, err := f.Open(path); return err == nil, err }
func (f *VirtualFS) Rename(string, string) error        { return utils.Error("implement me") }
func (f *VirtualFS) Rel(string, string) (string, error) { return "", utils.Error("implement me") }
func (f *VirtualFS) WriteFile(name string, data []byte, mode os.FileMode) error {
	f.AddFile(name, string(data))
	return nil
}

func (f *VirtualFS) Delete(path string) error {
	return f.RemoveFileOrDir(path)
}
func (f *VirtualFS) ExtraInfo(string) map[string]any { return nil }

func (f *VirtualFS) MkdirAll(path string, mode os.FileMode) error {
	f.AddDir(path)
	return nil
}

func (f *VirtualFS) Base(s string) string {
	return path.Base(s)
}

var _ fi.FileSystem = (*VirtualFS)(nil)

func NewVirtualFs() *VirtualFS {
	vs := &VirtualFS{
		files: omap.NewEmptyOrderedMap[string, *VirtualFile](),
	}
	dir := NewVirtualFileDirectory(".", vs)
	vs.files.Set(".", dir)
	return vs
}

func (f *VirtualFS) ReadFile(name string) ([]byte, error) {
	raw, err := f.Open(name)
	if err != nil {
		return nil, err
	}
	defer raw.Close()
	return io.ReadAll(raw)
}

func (f *VirtualFS) GetLocalFSPath() string {
	return ""
}

func (f *VirtualFS) Open(name string) (fs.File, error) {
	vf, fileName, err := f.get(false, f.splite(name)...)
	if err != nil {
		return nil, err
	}
	file, exist := vf.files.Get(fileName)
	if !exist {
		return nil, fmt.Errorf("file [%v] not exist", name)
	}
	if file.fs != nil {
		return nil, fmt.Errorf("file [%v] is a dir", name)
	}
	return NewVirtualFile(file.name, file.content), nil
}

func (f *VirtualFS) OpenFile(name string, flag int, perm os.FileMode) (fs.File, error) {
	isCreate := flag&os.O_CREATE == os.O_CREATE
	vf, fileName, err := f.get(false, f.splite(name)...)
	if err != nil {
		return nil, err
	}
	file, exist := vf.files.Get(fileName)
	if !exist {
		if isCreate {
			vf.AddFile(fileName, "")
			file, exist = vf.files.Get(fileName)
		}
	}
	if !exist {
		return nil, fmt.Errorf("file [%v] not exist", name)
	}
	if file.fs != nil {
		return nil, fmt.Errorf("file [%v] is a dir", name)
	}
	return file, nil
}

func (f *VirtualFS) Stat(name string) (fs.FileInfo, error) {
	vf, fileName, err := f.get(false, f.splite(name)...)
	if err != nil {
		return nil, err
	}
	file, exist := vf.files.Get(fileName)
	if !exist {
		return nil, fmt.Errorf("file [%v] not exist", name)
	}
	return file.Stat()
}

func (f *VirtualFS) splite(name string) []string {
	return strings.Split(name, string(f.GetSeparators()))
}

func (f *VirtualFS) ReadDir(name string) ([]fs.DirEntry, error) {
	fs, err := f.getDir(false, strings.Split(name, "/")...)
	if err != nil {
		return nil, err
	}
	return fs.dirEntry, nil
}

func (f *VirtualFS) Join(name ...string) string { return path.Join(name...) }
func (f *VirtualFS) GetSeparators() rune        { return '/' }

func (f *VirtualFS) AddFile(name, content string) {
	v, filename, _ := f.get(true, f.splite(name)...)
	vf := NewVirtualFile(filename, content)
	v.addFileByVirtualFile(vf)
}

func (f *VirtualFS) addFileByVirtualFile(vf *VirtualFile) {
	if _, ok := f.files.Get(vf.name); ok {
		return
	}
	f.files.Set(vf.name, vf)
	f.dirEntry = append(f.dirEntry, vf.info)
}

func (f *VirtualFS) RemoveFileOrDir(name string) error {
	vf, filename, err := f.get(true, f.splite(name)...)
	if err != nil {
		return err
	}
	if f, ok := vf.files.Get(filename); ok {
		vf.files.Delete(filename)
		vf.dirEntry = utils.RemoveSliceItem(vf.dirEntry, fs.DirEntry(f.info))
		return nil
	}
	return fmt.Errorf("file [%v] not exist", name)
}

func (f *VirtualFS) AddDir(dirName string) *VirtualFile {
	v, filename, _ := f.get(true, f.splite(dirName)...)
	var dir *VirtualFile
	if filename == "" {
		dir = NewVirtualFileDirectory("", f)
		v.files.Set("", dir)
	} else {
		dir = NewVirtualFileDirectory(filename, NewVirtualFs())
		v.addFileByVirtualFile(dir)
	}
	return dir
}

func (f *VirtualFS) get(create bool, names ...string) (*VirtualFS, string, error) {
	path := names[:len(names)-1]
	filePath := names[len(names)-1]
	vf, err := f.getDir(create, path...)
	if err != nil {
		return nil, "", err
	}
	return vf, filePath, nil
}

func (f *VirtualFS) getDir(create bool, dirs ...string) (*VirtualFS, error) {
	get := func(v *VirtualFS, dir string) (*VirtualFS, error) {
		vf, ok := v.files.Get(dir)
		if !ok {
			if !create {
				return nil, utils.Errorf("directory [%s] not exists", dir)
			}
			vf = v.AddDir(dir)
		}
		if vf.fs == nil {
			return nil, utils.Errorf("this directory [%s] is not directory, just a file", dir)
		}
		return vf.fs, nil
	}
	fs := f
	var err error
	for _, name := range dirs {
		if fs, err = get(fs, name); err != nil {
			return nil, err
		}
	}
	return fs, nil
}

func (f *VirtualFS) String() string {
	if f == nil {
		return "<nil>"
	}

	var builder strings.Builder
	builder.WriteString("VirtualFS{")

	var handFunc func(string, *VirtualFS)
	handFunc = func(n string, fs *VirtualFS) {
		if n == "." {
			return
		}
		fs.files.ForEach(func(name string, file *VirtualFile) bool {
			if name == "." || name == "" {
				return true
			}
			if file.fs != nil {
				builder.WriteString(fmt.Sprintf("%s/", name))
				handFunc(name, file.fs)
			} else {
				builder.WriteString(name)
			}
			return true
		})
	}
	handFunc("", f)

	builder.WriteString("}")
	return builder.String()
}

type VirtualFile struct {
	name    string
	content string
	fs      *VirtualFS
	info    *VirtualFileInfo
	index   int
}

func (f *VirtualFile) FS() fi.FileSystem {
	return f.fs
}

var _ fs.File = (*VirtualFile)(nil)

func NewVirtualFile(name string, content string) *VirtualFile {
	return &VirtualFile{
		name:    name,
		content: content,
		info:    NewVirtualFileInfo(name, int64(len(content)), false),
	}
}

func NewVirtualFileDirectory(dirName string, dir *VirtualFS) *VirtualFile {
	return &VirtualFile{
		name: dirName,
		fs:   dir,
		info: NewVirtualFileInfo(dirName, int64(0), true),
	}
}

func (vf *VirtualFile) Stat() (fs.FileInfo, error) {
	return vf.info, nil
}

func (vf *VirtualFile) Read(p []byte) (int, error) {
	if vf.fs != nil {
		return 0, fs.ErrInvalid
	}

	var err error
	n := copy(p, vf.content[vf.index:])
	vf.index += n
	// if n <= len(vf.content) {
	if vf.index == len(vf.content) {
		err = io.EOF
	}

	return n, err
}

func (vf *VirtualFile) Close() error {
	return nil
}

type VirtualFileInfo struct {
	name string
	size int64
	mod  fs.FileMode
}

var (
	_ fs.FileInfo = (*VirtualFileInfo)(nil)
	_ fs.DirEntry = (*VirtualFileInfo)(nil)
)

func NewVirtualFileInfo(name string, size int64, isDir bool) *VirtualFileInfo {
	if isDir {
		return &VirtualFileInfo{
			name: name,
			size: size,
			mod:  fs.ModeDir,
		}
	}
	return &VirtualFileInfo{
		name: name,
		size: size,
		mod:  fs.ModeType,
	}
}

func (vi *VirtualFileInfo) Name() string {
	return vi.name
}

func (vi *VirtualFileInfo) Size() int64 {
	return vi.size
}

func (vi *VirtualFileInfo) Mode() os.FileMode {
	return vi.mod
}

func (vi *VirtualFileInfo) ModTime() time.Time {
	return time.Time{}
}

func (vi *VirtualFileInfo) IsDir() bool {
	return vi.mod == fs.ModeDir
}

func (vi *VirtualFileInfo) Sys() any {
	return nil
}

func (vi *VirtualFileInfo) Info() (fs.FileInfo, error) {
	return vi, nil
}

func (vi *VirtualFileInfo) Type() fs.FileMode {
	return vi.mod
}
