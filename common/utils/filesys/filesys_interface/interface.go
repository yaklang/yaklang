package filesys_interface

import (
	"io/fs"
	"os"
)

// FileSystem defines the methods of an abstract filesystem.
type FileSystem interface {
	ReadFile(name string) ([]byte, error)

	Open(name string) (fs.File, error)
	OpenFile(name string, flag int, perm os.FileMode) (fs.File, error)
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
	ExtraInfo(string) map[string]any
}

type TrashFS interface {
	Throw(filenames ...string) error
}

type SyncFile interface {
	Sync() error
}
