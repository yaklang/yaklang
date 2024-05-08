package filesys

import "io/fs"

// FileSystem defines the methods of an abstract filesystem.
type FileSystem interface {
	Open(name string) (fs.File, error)

	// Stat returns a FileInfo describing the file.
	// If there is an error, it should be of type *PathError.
	Stat(name string) (fs.FileInfo, error)
	// ReadDir reads the named directory
	// and returns a list of directory entries sorted by filename.
	ReadDir(name string) ([]fs.DirEntry, error)

	Join(elem ...string) string

	GetSeparators() rune

	GetLocalFSPath() string
}
