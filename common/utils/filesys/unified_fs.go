package filesys

import (
	"fmt"
	"io/fs"
	"os"
	"strings"

	"github.com/yaklang/yaklang/common/utils"
	fi "github.com/yaklang/yaklang/common/utils/filesys/filesys_interface"
)

type UnifiedFSConfig struct {
	Separator rune
	// 从外部文件系统读取或写入文件名称的时候，需要将文件名后缀转换为真实后缀
	inputExtMap map[string]string // VirtualExt -> RealExt
	// 文件系统输出文件名称的时候，需要将文件名后缀转换为虚拟后缀
	outputExtMap map[string]string // RealExt -> VirtualExt
}

type UnifiedFS struct {
	fs     fi.FileSystem
	config *UnifiedFSConfig
}

type UnifiedFsOption func(config *UnifiedFSConfig)

func NewUnifiedFS(fs fi.FileSystem, opts ...UnifiedFsOption) *UnifiedFS {
	u := &UnifiedFS{
		fs: fs,
		config: &UnifiedFSConfig{
			Separator:    fs.GetSeparators(),
			inputExtMap:  make(map[string]string),
			outputExtMap: make(map[string]string),
		},
	}
	for _, opt := range opts {
		opt(u.config)
	}
	return u
}

func WithUnifiedFsSeparator(sep rune) func(config *UnifiedFSConfig) {
	return func(config *UnifiedFSConfig) {
		config.Separator = sep
	}
}

func WithUnifiedFsExtMap(extReal, extVirtual string) func(config *UnifiedFSConfig) {
	return func(config *UnifiedFSConfig) {
		if config.inputExtMap == nil {
			config.inputExtMap = make(map[string]string)
		}
		if config.outputExtMap == nil {
			config.outputExtMap = make(map[string]string)
		}
		config.inputExtMap[extVirtual] = extReal
		config.outputExtMap[extReal] = extVirtual
	}
}

func (u *UnifiedFS) GetSeparators() rune {
	return u.config.Separator
}

func (u *UnifiedFS) Join(elem ...string) string {
	return joinWithSeparators(u.GetSeparators(), elem...)
}

func (u *UnifiedFS) PathSplit(name string) (string, string) {
	return SplitWithSeparator(name, u.GetSeparators())
}

func (u *UnifiedFS) Base(name string) string {
	return baseWithSeparators(name, u.GetSeparators())
}

func (u *UnifiedFS) Rel(s string, s2 string) (string, error) {
	return u.fs.Rel(s, s2)
}

func (u *UnifiedFS) Ext(name string) string {
	ext := getExtension(name)
	return ext
}

func (u *UnifiedFS) Exists(name string) (bool, error) {
	realPath, _ := u.convertToRealPath(name)
	return u.fs.Exists(realPath)
}

func (u *UnifiedFS) IsAbs(name string) bool {
	return len(name) > 0 && name[0] == byte(u.GetSeparators())
}

func (u *UnifiedFS) Getwd() (string, error) { return ".", nil }

func (u *UnifiedFS) Stat(name string) (fs.FileInfo, error) {
	realPath, _ := u.convertToRealPath(name)
	info, err := u.fs.Stat(realPath)
	if err != nil {
		return nil, err
	}
	_, virtualName := u.PathSplit(name)
	return &UnifiedFileInfo{
		FileInfo: info,
		name:     virtualName,
	}, nil
}
func (u *UnifiedFS) OpenFile(name string, flag int, perm os.FileMode) (fs.File, error) {
	realPath, _ := u.convertToRealPath(name)
	file, err := u.fs.OpenFile(realPath, flag, perm)
	if err != nil {
		return nil, err
	}
	_, virtualName := u.PathSplit(name)
	return &UnifiedFile{
		File: file,
		name: virtualName,
	}, nil
}

func (u *UnifiedFS) Open(name string) (fs.File, error) {
	realPath, _ := u.convertToRealPath(name)
	file, err := u.fs.Open(realPath)
	if err != nil {
		return nil, err
	}
	_, virtualName := u.PathSplit(name)
	return &UnifiedFile{
		File: file,
		name: virtualName,
	}, nil
}

func (u *UnifiedFS) ReadFile(name string) ([]byte, error) {
	realPath, _ := u.convertToRealPath(name)
	return u.fs.ReadFile(realPath)
}

func (u *UnifiedFS) ExtraInfo(s string) map[string]any {
	return u.fs.ExtraInfo(s)
}

func (u *UnifiedFS) Rename(old string, new string) error {
	oldPath, err := u.convertToRealPath(old)
	if err != nil {
		return err
	}
	newPath, err := u.convertToRealPath(new)
	if err != nil {
		return err
	}
	return u.fs.Rename(oldPath, newPath)
}

func (u *UnifiedFS) WriteFile(name string, data []byte, perm os.FileMode) error {
	realPath, _ := u.convertToRealPath(name)
	return u.fs.WriteFile(realPath, data, perm)
}

func (u *UnifiedFS) Delete(name string) error {
	realPath, _ := u.convertToRealPath(name)
	return u.fs.Delete(realPath)
}

func (u *UnifiedFS) MkdirAll(name string, perm os.FileMode) error {
	realPath, _ := u.convertToRealPath(name)
	return u.fs.MkdirAll(realPath, perm)
}

func (u *UnifiedFS) ReadDir(name string) ([]fs.DirEntry, error) {
	realPath, _ := u.convertToRealPath(name)
	entries, err := u.fs.ReadDir(realPath)
	if err != nil {
		return nil, err
	}
	unifiedEntries := make([]fs.DirEntry, 0, len(entries))
	for _, entry := range entries {
		entry.Name()
		unifiedEntries = append(unifiedEntries, &UnifiedDirEntry{
			DirEntry: entry,
			name:     u.convertToVirtualPath(entry.Name()),
		})
	}
	return unifiedEntries, nil
}

func (u *UnifiedFS) convertToRealPath(name string) (string, error) {
	allPath := strings.Split(name, string(u.GetSeparators()))
	realPath := u.fs.Join(allPath...)
	// 真实的文件系统存在 realFileName.VirtualExt
	exist, err := u.fs.Exists(realPath)
	if err == nil && exist {
		return realPath, utils.Error("convert virtual ext to real ext failed, file already exists")
	}

	ext := u.fs.Ext(realPath)
	if ext == "" {
		return realPath, nil
	}
	if realExt, ok := u.config.inputExtMap[ext]; ok {
		realPath = strings.TrimSuffix(realPath, ext) + realExt
	}
	return realPath, nil
}

func (u *UnifiedFS) convertToVirtualPath(name string) string {
	allPath := strings.Split(name, string(u.fs.GetSeparators()))
	virtualPath := u.Join(allPath...)
	ext := u.Ext(virtualPath)
	if ext == "" {
		return virtualPath
	}
	if virtualExt, ok := u.config.outputExtMap[ext]; ok {
		virtualPath = strings.TrimSuffix(virtualPath, ext) + virtualExt
	}
	return virtualPath
}

func (f *UnifiedFS) String() string {
	return fmt.Sprintf("%s",f.fs)
}

type UnifiedDirEntry struct {
	fs.DirEntry
	name string
}

func (e *UnifiedDirEntry) Name() string {
	return e.name
}

func (e *UnifiedDirEntry) Info() (fs.FileInfo, error) {
	info, err := e.DirEntry.Info()
	if err != nil {
		return nil, err
	}
	return &UnifiedFileInfo{
		FileInfo: info,
		name:     e.Name(),
	}, nil
}

type UnifiedFileInfo struct {
	fs.FileInfo
	name string
}

func (i *UnifiedFileInfo) Name() string {
	return i.name
}

type UnifiedFile struct {
	fs.File
	name string
}

func (f *UnifiedFile) Name() string {
	return f.name
}

func (f *UnifiedFile) Stat() (fs.FileInfo, error) {
	info, err := f.File.Stat()
	if err != nil {
		return nil, err
	}
	return &UnifiedFileInfo{
		FileInfo: info,
		name:     f.Name(),
	}, nil
}
