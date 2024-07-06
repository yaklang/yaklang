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
)

type VirtualFS struct {
	files    map[string]*VirtualFile
	dirEntry []fs.DirEntry
}

func (vs *VirtualFS) PathSplit(s string) (string, string) {
	return splitWithSeparator(s, vs.GetSeparators())
}

func (vs *VirtualFS) Ext(s string) string {
	return getExtension(s)
}

var _ FileSystem = (*VirtualFS)(nil)

func NewVirtualFs() *VirtualFS {
	vs := &VirtualFS{
		files: make(map[string]*VirtualFile),
	}
	dir := NewVirtualFileDirectory(".", vs)
	vs.files["."] = dir
	return vs
}

func (vs *VirtualFS) ReadFile(name string) ([]byte, error) {
	raw, err := vs.Open(name)
	if err != nil {
		return nil, err
	}
	defer raw.Close()
	return io.ReadAll(raw)
}

func (vs *VirtualFS) GetLocalFSPath() string {
	return ""
}

func (vs *VirtualFS) Open(name string) (fs.File, error) {
	vf, fileName, err := vs.get(false, vs.splite(name)...)
	if err != nil {
		return nil, err
	}
	file, exist := vf.files[fileName]
	if !exist {
		return nil, fmt.Errorf("file [%v] not exist", name)
	}
	if file.fs != nil {
		return nil, fmt.Errorf("file [%v] is a dir", name)
	}
	return NewVirtualFile(file.name, file.content), nil
}

func (vs *VirtualFS) Stat(name string) (fs.FileInfo, error) {
	vf, fileName, err := vs.get(false, vs.splite(name)...)
	if err != nil {
		return nil, err
	}
	file, exist := vf.files[fileName]
	if !exist {
		return nil, fmt.Errorf("file [%v] not exist", name)
	}
	return file.Stat()
}
func (vs *VirtualFS) splite(name string) []string {
	return strings.Split(name, string(vs.GetSeparators()))
}

func (vs *VirtualFS) ReadDir(name string) ([]fs.DirEntry, error) {
	fs, err := vs.getDir(false, strings.Split(name, "/")...)
	if err != nil {
		return nil, err
	}
	return fs.dirEntry, nil
}

func (vs *VirtualFS) Join(name ...string) string { return path.Join(name...) }
func (vs *VirtualFS) GetSeparators() rune        { return '/' }

func (vs *VirtualFS) AddFile(name, content string) {
	v, filename, _ := vs.get(true, strings.Split(name, "/")...)
	vf := NewVirtualFile(filename, content)
	v.addFileByVirtualFile(vf)
}

func (vs *VirtualFS) addFileByVirtualFile(vf *VirtualFile) {
	vs.files[vf.name] = vf
	vs.dirEntry = append(vs.dirEntry, vf.info)
}

func (vs *VirtualFS) RemoveFileOrDir(name string) error {
	vf, filename, err := vs.get(false, strings.Split(name, "/")...)
	if err != nil {
		return err
	}
	if _, ok := vf.files[filename]; ok {
		delete(vf.files, filename)
		return nil
	}
	return fmt.Errorf("file [%v] not exist", name)
}

func (vf *VirtualFS) AddDir(dirName string) *VirtualFile {
	dir := NewVirtualFileDirectory(dirName, NewVirtualFs())
	vf.files[dirName] = dir
	vf.dirEntry = append(vf.dirEntry, dir.info)
	return dir
}

func (vs *VirtualFS) get(create bool, names ...string) (*VirtualFS, string, error) {
	path := names[:len(names)-1]
	filePath := names[len(names)-1]
	vf, err := vs.getDir(create, path...)
	if err != nil {
		return nil, "", err
	}
	return vf, filePath, nil
}

func (v *VirtualFS) getDir(create bool, dirs ...string) (*VirtualFS, error) {
	get := func(v *VirtualFS, dir string) (*VirtualFS, error) {
		vf, ok := v.files[dir]
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
	fs := v
	var err error
	for _, name := range dirs {
		if fs, err = get(fs, name); err != nil {
			return nil, err
		}
	}
	return fs, nil
}

type VirtualFile struct {
	name    string
	content string
	fs      *VirtualFS
	info    *VirtualFileInfo
	index   int
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

var _ fs.FileInfo = (*VirtualFileInfo)(nil)
var _ fs.DirEntry = (*VirtualFileInfo)(nil)

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
