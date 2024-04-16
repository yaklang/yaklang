package filesys

import (
	"fmt"
	"github.com/yaklang/yaklang/common/log"
	"io/fs"
	"os"
	"path/filepath"
	"time"
)

type VirtualFS struct {
	path  string
	files map[string]*VirtualFile
}

type VirtualFile struct {
	name    string
	content string
	dir     *VirtualFS
}

type VirtualFileInfo struct {
	name  string
	size  int64
	isDir bool
}

func NewVirtualFs(path string) *VirtualFS {
	return &VirtualFS{
		path:  path,
		files: make(map[string]*VirtualFile),
	}
}

func NewVirtualFile(name string, content string) *VirtualFile {
	return &VirtualFile{
		name:    name,
		content: content,
	}
}

func NewVirtualFileInfo(name string, size int64, isDir bool) *VirtualFileInfo {
	return &VirtualFileInfo{name: name, size: size, isDir: isDir}
}

func NewVirtualFileDirectory(dirName string, dir *VirtualFS) *VirtualFile {
	return &VirtualFile{
		name: dirName,
		dir:  dir,
	}
}

func (vs *VirtualFS) ReadDir(dirName string) ([]os.FileInfo, error) {
	var fileInfos []os.FileInfo
	if dirName != "" {
		dirFile, exist := vs.files[dirName]
		if !exist {
			return nil, fmt.Errorf("dir [%v] not exist", dirName)
		}
		if dirFile.dir == nil {
			return nil, fmt.Errorf("file [%v] is not a dir", dirName)
		}
		for _, file := range dirFile.dir.files {
			fileInfo, err := file.Stat()
			if err != nil {
				return nil, err
			}
			fileInfos = append(fileInfos, fileInfo)
		}
		return fileInfos, nil
	}
	return nil, fmt.Errorf("directory name is a null character")
}

func (vs *VirtualFS) Lstat(name string) (os.FileInfo, error) {
	return NewVirtualFileInfo(name, 0, true), nil
}

func (vs *VirtualFS) Join(elem ...string) string {
	return filepath.Join(elem...)
}

func (vs *VirtualFS) Open(name string) (fs.File, error) {
	file, exist := vs.files[name]
	if !exist {
		return nil, fmt.Errorf("file [%v] not exist", name)
	}
	if file.dir != nil {
		return nil, fmt.Errorf("file [%v] is a dir", name)
	}
	return file, nil
}

func (vs *VirtualFS) AddFileForce(vf *VirtualFile) {
	err := vs.addFile(vf, true)
	if err != nil {
		log.Warn(err)
	}
}

func (vs *VirtualFS) AddFile(vf *VirtualFile) {
	err := vs.addFile(vf, false)
	if err != nil {
		log.Error(err)
	}
}

func (vs *VirtualFS) addFile(vf *VirtualFile, forceAdd bool) error {
	if vs.IsFileExist(vf.name) {
		if forceAdd {
			err := fmt.Errorf("file [%v] already exists,now forcefully overwrite.\n", vf.name)
			vs.files[vf.name] = vf
			return err
		}
		return fmt.Errorf("file [%s] already exists", vf.name)
	}
	vs.files[vf.name] = vf
	return nil
}

func (vs *VirtualFS) IsFileExist(name string) bool {
	_, exist := vs.files[name]
	if exist {
		return true
	}
	return false
}

func (vs *VirtualFS) RemoveFile(name string) error {
	if vs.IsFileExist(name) {
		delete(vs.files, name)
		return nil
	}
	return fmt.Errorf("file [%s] does not exist", name)
}

func (vs *VirtualFS) GetContent(name string) (string, error) {
	if vs.IsFileExist(name) {
		if vs.files[name].dir != nil {
			return "", fmt.Errorf("file [%s] is a directory", name)
		}
		return vs.files[name].content, nil
	}
	return "", fmt.Errorf("file [%s] does not exist", name)
}

func (vs *VirtualFS) AddDir(dirName string, dir *VirtualFS) {
	err := vs.addDir(dirName, dir, false)
	if err != nil {
		log.Error(err)
	}
}

func (vs *VirtualFS) AddDirForce(dirName string, dir *VirtualFS) {
	err := vs.addDir(dirName, dir, true)
	if err != nil {
		log.Warn(err)
	}
}

func (vs *VirtualFS) RemoveDir(dirName string) error {
	if vs.IsFileExist(dirName) {
		delete(vs.files, dirName)
		return nil
	}
	return fmt.Errorf("dir [%s] does not exist", dirName)
}

func (vs *VirtualFS) addDir(dirName string, dir *VirtualFS, isForce bool) error {
	if !isForce {
		if vs.IsFileExist(dirName) {
			return fmt.Errorf("directory [%s] already exists,please use AddDirForce.", dirName)
		}
		vs.files[dirName] = NewVirtualFileDirectory(dirName, dir)
		return nil
	}
	err := fmt.Errorf("directory [%v] already exists,now forcefully overwrite.\n", dirName)
	vs.files[dirName] = NewVirtualFileDirectory(dirName, dir)
	return err
}

func (vf *VirtualFile) Stat() (fs.FileInfo, error) {
	return NewVirtualFileInfo(vf.name, int64(len(vf.content)), false), nil
}

func (vf *VirtualFile) Read(p []byte) (int, error) {
	if vf.dir != nil {
		return 0, fs.ErrInvalid
	}
	n := copy(p, vf.content)
	return n, nil
}

func (vf *VirtualFile) Close() error {
	return nil
}

func (vi *VirtualFileInfo) Name() string {
	return vi.name
}
func (vi *VirtualFileInfo) Size() int64 {
	return 0
}
func (vi *VirtualFileInfo) Mode() os.FileMode {
	return os.ModeDevice
}
func (vi *VirtualFileInfo) ModTime() time.Time {
	return time.Time{}
}
func (vi *VirtualFileInfo) IsDir() bool {
	return vi.isDir
}
func (vi *VirtualFileInfo) Sys() any {
	return nil
}
