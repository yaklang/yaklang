package filesys

import (
	"io/fs"
	"os"
	"strings"
)

// FileSystem defines the methods of an abstract filesystem.
type FileSystem interface {
	ReadFile(name string) ([]byte, error)

	Open(name string) (fs.File, error)
	// RelOpen(name string) (fs.File, error)

	// Stat returns a FileInfo describing the file.
	// If there is an error, it should be of type *PathError.
	Stat(name string) (fs.FileInfo, error)
	// RelStat(name string) (fs.FileInfo, error)
	// ReadDir reads the named directory
	// and returns a list of directory entries sorted by filename.
	ReadDir(name string) ([]fs.DirEntry, error)

	Join(elem ...string) string
	// Rel(targpath string) (string, error)

	GetSeparators() rune

	PathSplit(string) (string, string)
	Ext(string) string
	IsAbs(string) bool
	Getwd() (string, error)
	Exists(string) (bool, error)
	Rename(string, string) error
	Rel(string, string) (string, error)
	WriteFile(string, []byte, os.FileMode) error
	Delete(string) error
	MkdirAll(string, os.FileMode) error
}

type Trash interface {
	Throw(filenames ...string) error
}

func splitWithSeparator(path string, sep rune) (string, string) {
	if len(path) == 0 {
		return "", ""
	}
	idx := strings.LastIndex(path, string(sep))
	if idx == -1 {
		return "", path
	}
	return path[:idx], path[idx+1:]
}

func getExtension(path string) string {
	if len(path) == 0 {
		return ""
	}
	idx := strings.LastIndex(path, ".")
	if idx == -1 {
		return ""
	}
	return path[idx:]
}
