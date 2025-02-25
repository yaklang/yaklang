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
	ExtMap    map[string]string
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
			ExtMap: make(map[string]string),
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

func WithUnifiedFsExtMap(extBefore, extAfter string) func(config *UnifiedFSConfig) {
	return func(config *UnifiedFSConfig) {
		if config.ExtMap == nil {
			config.ExtMap = make(map[string]string)
		}
		config.ExtMap[extBefore] = extAfter
		if _, exists := config.ExtMap[extAfter]; !exists {
			config.ExtMap[extAfter] = extBefore
		}
	}
}

type OperationType int

const (
	ReadOperation OperationType = iota
	WriteOperation
)

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
	realPath := u.convertToRealPathWithOp(name, ReadOperation)
	_, err := u.Open(realPath)
	return err == nil, err
}

func (u *UnifiedFS) IsAbs(name string) bool {
	return len(name) > 0 && name[0] == byte(u.GetSeparators())
}

func (u *UnifiedFS) Getwd() (string, error) { return ".", nil }

func (u *UnifiedFS) Stat(name string) (fs.FileInfo, error) {
	realPath := u.convertToRealPathWithOp(name, ReadOperation)
	return u.fs.Stat(realPath)
}

func (u *UnifiedFS) OpenFile(name string, flag int, perm os.FileMode) (fs.File, error) {
	realPath := u.convertToRealPathWithOp(name, ReadOperation)
	return u.fs.OpenFile(realPath, flag, perm)
}

func (u *UnifiedFS) Open(name string) (fs.File, error) {
	realPath := u.convertToRealPathWithOp(name, ReadOperation)
	return u.fs.Open(realPath)
}

func (u *UnifiedFS) ReadFile(name string) ([]byte, error) {
	realPath := u.convertToRealPathWithOp(name, ReadOperation)
	return u.fs.ReadFile(realPath)
}

func (u *UnifiedFS) ExtraInfo(s string) map[string]any {
	return u.fs.ExtraInfo(s)
}

func (u *UnifiedFS) Rename(old string, new string) error {
	oldPath := u.convertToRealPathWithOp(old, WriteOperation)
	newPath := u.convertToRealPathWithOp(new, WriteOperation)
	return u.fs.Rename(oldPath, newPath)
}

func (u *UnifiedFS) WriteFile(name string, data []byte, perm os.FileMode) error {
	realPath := u.convertToRealPathWithOp(name, WriteOperation)
	return u.fs.WriteFile(realPath, data, perm)
}

func (u *UnifiedFS) Delete(name string) error {
	realPath := u.convertToRealPathWithOp(name, WriteOperation)
	return u.fs.Delete(realPath)
}

func (u *UnifiedFS) MkdirAll(name string, perm os.FileMode) error {
	realPath := u.convertToRealPathWithOp(name, WriteOperation)
	return u.fs.MkdirAll(realPath, perm)
}

func (u *UnifiedFS) ReadDir(name string) ([]fs.DirEntry, error) {
	realPath := u.convertToRealPathWithOp(name, ReadOperation)

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

func (u *UnifiedFS) convertToRealPathWithOp(name string, op OperationType) string {
	pathComponents := strings.Split(name, string(u.config.Separator))
	realPath := u.fs.Join(pathComponents...)

	ext := filepath.Ext(realPath)
	if ext == "" {
		return realPath
	}

	switch op {
	case ReadOperation:
		if mappedExt, ok := u.config.ExtMap[ext]; ok {
			realPath = strings.TrimSuffix(realPath, ext) + mappedExt
		}
	case WriteOperation:
		if originalExt, ok := u.config.ExtMap[ext]; ok {
			realPath = strings.TrimSuffix(realPath, ext) + originalExt
		}
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
	if originalExt, ok := e.u.config.ExtMap[ext]; ok {
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
