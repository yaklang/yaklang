package filesys

import (
	fi "github.com/yaklang/yaklang/common/utils/filesys/filesys_interface"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
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
	return splitWithSeparator(name, u.GetSeparators())
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
	realPath := u.convertToRealPath(name)
	_, err := u.Open(realPath)
	return err == nil, err
}

func (u *UnifiedFS) IsAbs(name string) bool {
	return len(name) > 0 && name[0] == byte(u.GetSeparators())
}

func (u *UnifiedFS) Getwd() (string, error) { return ".", nil }

func (u *UnifiedFS) Stat(name string) (fs.FileInfo, error) {
	realPath := u.convertToRealPath(name)
	info, err := u.fs.Stat(realPath)
	if err != nil {
		return nil, err
	}
	_, realName := u.PathSplit(name)
	return &UnifiedFileInfo{
		FileInfo: info,
		name:     realName,
	}, nil
}
func (u *UnifiedFS) OpenFile(name string, flag int, perm os.FileMode) (fs.File, error) {
	realPath := u.convertToRealPath(name)
	file, err := u.fs.OpenFile(realPath, flag, perm)
	if err != nil {
		return nil, err
	}
	return &UnifiedFile{
		File:     file,
		u:        u,
		realPath: realPath,
	}, nil
}

func (u *UnifiedFS) Open(name string) (fs.File, error) {
	realPath := u.convertToRealPath(name)
	file, err := u.fs.Open(realPath)
	if err != nil {
		return nil, err
	}
	return &UnifiedFile{
		File:     file,
		u:        u,
		realPath: realPath,
	}, nil
}

func (u *UnifiedFS) ReadFile(name string) ([]byte, error) {
	realPath := u.convertToRealPath(name)
	return u.fs.ReadFile(realPath)
}

func (u *UnifiedFS) ExtraInfo(s string) map[string]any {
	return u.fs.ExtraInfo(s)
}

func (u *UnifiedFS) Rename(old string, new string) error {
	oldPath := u.convertToRealPath(old)
	newPath := u.convertToRealPath(new)
	return u.fs.Rename(oldPath, newPath)
}

func (u *UnifiedFS) WriteFile(name string, data []byte, perm os.FileMode) error {
	realPath := u.convertToRealPath(name)
	return u.fs.WriteFile(realPath, data, perm)
}

func (u *UnifiedFS) Delete(name string) error {
	realPath := u.convertToRealPath(name)
	return u.fs.Delete(realPath)
}

func (u *UnifiedFS) MkdirAll(name string, perm os.FileMode) error {
	realPath := u.convertToRealPath(name)
	return u.fs.MkdirAll(realPath, perm)
}

func (u *UnifiedFS) ReadDir(name string) ([]fs.DirEntry, error) {
	realPath := u.convertToRealPath(name)

	// 获取原始目录条目
	entries, err := u.fs.ReadDir(realPath)
	if err != nil {
		return nil, err
	}

	// 转换每个条目
	unifiedEntries := make([]fs.DirEntry, 0, len(entries))
	for _, entry := range entries {
		// 构造完整真实路径
		fullRealPath := u.fs.Join(realPath, entry.Name())

		unifiedEntries = append(unifiedEntries, &UnifiedDirEntry{
			DirEntry: entry,
			u:        u,
			realPath: fullRealPath,
		})
	}

	return unifiedEntries, nil
}

func (u *UnifiedFS) convertToRealPath(name string) string {
	pathComponents := strings.Split(name, string(u.config.Separator))
	realPath := u.fs.Join(pathComponents...)

	ext := filepath.Ext(realPath)
	if ext == "" {
		return realPath
	}
	if virtualExt, ok := u.config.inputExtMap[ext]; ok {
		realPath = strings.TrimSuffix(realPath, ext) + virtualExt
	}
	return realPath
}

type UnifiedDirEntry struct {
	fs.DirEntry
	u        *UnifiedFS
	realPath string
}

func (e *UnifiedDirEntry) Name() string {
	// 应用反向转换规则（存储名 → 虚拟名）
	base := e.DirEntry.Name()
	ext := getExtension(base)
	// 如果是目录则直接返回
	if e.IsDir() {
		return base
	}
	// 查找反向映射
	if originalExt, ok := e.u.config.outputExtMap[ext]; ok {
		return strings.TrimSuffix(base, ext) + originalExt
	}
	return base
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

// UnifiedFile 是一个包装器，用于在 fs.File 上应用虚拟路径映射
type UnifiedFile struct {
	fs.File
	u        *UnifiedFS
	realPath string
}

func (f *UnifiedFile) Name() string {
	// 获取原始文件名
	base := filepath.Base(f.realPath)
	ext := getExtension(base)
	if ext == "" {
		return base
	}

	// 查找反向映射规则
	if virtualExt, ok := f.u.config.outputExtMap[ext]; ok {
		return strings.TrimSuffix(base, ext) + virtualExt
	}
	return base
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
