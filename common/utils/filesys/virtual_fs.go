package filesys

import (
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"strings"
	"time"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

type VirtualFS struct {
	files    map[string]*VirtualFile
	dirEntry []fs.DirEntry
}

var _ FileSystem = (*VirtualFS)(nil)

func NewVirtualFs() *VirtualFS {
	vs := &VirtualFS{
		files: make(map[string]*VirtualFile),
	}
	return vs
}

func (vs *VirtualFS) Open(name string) (fs.File, error) {
	vf, fileName, err := vs.get(vs.splite(name)...)
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
	return file, nil
}

func (vs *VirtualFS) Stat(name string) (fs.FileInfo, error) {
	vf, fileName, err := vs.get(vs.splite(name)...)
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

func (vs *VirtualFS) get(names ...string) (*VirtualFS, string, error) {
	path := names[:len(names)-1]
	filePath := names[len(names)-1]
	vf, err := vs.getDir(path...)
	if err != nil {
		return nil, "", err
	}
	return vf, filePath, nil
}

func (vs *VirtualFS) ReadDir(name string) ([]fs.DirEntry, error) {
	fs, err := vs.getDir(strings.Split(name, "/")...)
	if err != nil {
		return nil, err
	}
	return fs.CurrentDir(), nil
}

func (vs *VirtualFS) Join(name ...string) string { return path.Join(name...) }
func (vs *VirtualFS) GetSeparators() rune        { return '/' }

func (vs *VirtualFS) CurrentDir() []fs.DirEntry {
	return vs.dirEntry
}

func (vs *VirtualFS) IsFileOrDirExist(name string) bool {
	_, exist := vs.files[name]
	if exist {
		return true
	}
	return false
}

func (vs *VirtualFS) AddFileByString(name, content string) {
	vf := NewVirtualFile(name, content)
	vs.AddFile(vf)
}

func (vs *VirtualFS) AddFile(vf *VirtualFile) {
	if vs.IsFileOrDirExist(vf.name) {
		log.Errorf("file [%v] already exists,now overwrite.\n", vf.name)
	}
	vs.files[vf.name] = vf
	// info, _ := vf.Stat()
	vs.dirEntry = append(vs.dirEntry, vf.info)
}

func (vs *VirtualFS) RemoveFile(name string) {
	if vs.IsFileOrDirExist(name) {
		delete(vs.files, name)
	}
	log.Errorf("file [%s] does not exist", name)
}

func (v *VirtualFS) AddDirByString(dirNames ...string) error {
	vf, current, err := v.get(dirNames...)
	if err != nil {
		return err
	}
	vf.AddDirByFS(current, NewVirtualFs())
	return nil
}

func (v *VirtualFS) AddFileToDir(dir, file, content string) error {
	vf, err := v.getDir(strings.Split(dir, "/")...)
	if err != nil {
		return err
	}
	vf.AddFileByString(file, content)
	return nil
}

func (vf *VirtualFS) AddDirByFS(dirName string, fs *VirtualFS) error {
	dir := NewVirtualFileDirectory(dirName, fs)
	vf.files[dirName] = dir
	vf.dirEntry = append(vf.dirEntry, dir.info)
	return nil
}

func (vs *VirtualFS) RemoveDir(dirName string) error {
	if vs.IsFileOrDirExist(dirName) {
		delete(vs.files, dirName)
		return nil
	}
	return fmt.Errorf("dir [%s] does not exist", dirName)
}

func (v *VirtualFS) getDir(dirs ...string) (*VirtualFS, error) {
	get := func(v *VirtualFS, dir string) (*VirtualFS, error) {
		vf, ok := v.files[dir]
		if !ok {
			return nil, utils.Errorf("directory [%s] not exists", dir)
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
