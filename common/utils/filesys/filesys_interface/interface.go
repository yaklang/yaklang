package filesys_interface

import (
	"io/fs"
	"os"
)

type ReadOnlyFileSystem interface {
	fs.ReadDirFS
	fs.ReadFileFS
	// OpenFile like Open but opens the named file with specified flag and perm.
	OpenFile(name string, flag int, perm os.FileMode) (fs.File, error)
	// Stat returns a FileInfo describing the file.
	// If there is an error, it should be of type *PathError.
	Stat(name string) (fs.FileInfo, error)

	// ExtraInfo returns extra information about the fs.
	ExtraInfo(string) map[string]any
}
type PathFileSystem interface {
	GetSeparators() rune
	Join(elem ...string) string
	Base(string) string
	PathSplit(string) (string, string)
	Ext(string) string
	IsAbs(string) bool
	Getwd() (string, error)
	Exists(string) (bool, error)
	Rel(string, string) (string, error)
}

type WriteFileSystem interface {
	Rename(string, string) error
	WriteFile(string, []byte, os.FileMode) error
	Delete(string) error
	MkdirAll(string, os.FileMode) error
}

// TrashFileSystem defines the methods of an abstract filesystem that can throw files away.
// It's optional to implement this interface.
type TrashFileSystem interface {
	Throw(filenames ...string) error
}

// SyncFileSystem defines the methods of an abstract filesystem that can sync.
// It's optional to implement this interface.
type SyncFileSystem interface {
	Sync() error
}

// FileSystem defines the methods of an abstract filesystem.
type FileSystem interface {
	ReadOnlyFileSystem
	PathFileSystem
	WriteFileSystem
}
