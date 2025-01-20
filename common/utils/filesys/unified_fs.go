package filesys

import (
	fi "github.com/yaklang/yaklang/common/utils/filesys/filesys_interface"
	"io/fs"
	"os"
	"strings"
)

// UnifiedFS is a unified file system that can handle both windows and unix paths
type UnifiedFS struct {
	fi.FileSystem
	separators rune
}

func ConvertToUnifiedFs(fs fi.FileSystem, separators rune) *UnifiedFS {
	return &UnifiedFS{fs, separators}
}

func (u *UnifiedFS) GetSeparators() rune {
	return u.separators
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

func (u *UnifiedFS) Stat(name string) (fs.FileInfo, error) {
	realPath := u.convertToRealPath(name)
	return u.FileSystem.Stat(realPath)
}

func (u *UnifiedFS) Ext(name string) string {
	return getExtension(name)
}

func (u *UnifiedFS) Exists(name string) (bool, error) {
	realPath := u.convertToRealPath(name)
	_, err := u.Open(realPath)
	return err == nil, err
	return false, nil
}

func (u *UnifiedFS) IsAbs(name string) bool {
	return len(name) > 0 && name[0] == byte(u.GetSeparators())
}

func (u *UnifiedFS) Getwd() (string, error) { return ".", nil }

func (u *UnifiedFS) OpenFile(name string, flag int, perm os.FileMode) (fs.File, error) {
	realPath := u.convertToRealPath(name)
	return u.FileSystem.OpenFile(realPath, flag, perm)
}

func (u *UnifiedFS) Rename(old string, new string) error {
	oldPath := u.convertToRealPath(old)
	newPath := u.convertToRealPath(new)
	return u.FileSystem.Rename(oldPath, newPath)
}

func (u *UnifiedFS) WriteFile(name string, data []byte, perm os.FileMode) error {
	realPath := u.convertToRealPath(name)
	return u.FileSystem.WriteFile(realPath, data, perm)
}

func (u *UnifiedFS) Delete(name string) error {
	realPath := u.convertToRealPath(name)
	return u.FileSystem.Delete(realPath)
}

func (u *UnifiedFS) MkdirAll(name string, perm os.FileMode) error {
	realPath := u.convertToRealPath(name)
	return u.FileSystem.MkdirAll(realPath, perm)
}

func (u *UnifiedFS) convertToRealPath(name string) string {
	path := strings.Split(name, string(u.GetSeparators()))
	return u.FileSystem.Join(path...)
}
