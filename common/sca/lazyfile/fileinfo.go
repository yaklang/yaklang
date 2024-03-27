package lazyfile

import (
	"io/fs"
	"time"
)

type FileInfo struct {
	name     string
	size     int64
	fileMode fs.FileMode
}

func NewFileInfo(name string, size int64, fileMode fs.FileMode) *FileInfo {
	return &FileInfo{
		name:     name,
		size:     size,
		fileMode: fileMode,
	}
}

func (f *FileInfo) Name() string {
	return f.name
}

func (f *FileInfo) Size() int64 {
	return f.size
}

func (f *FileInfo) Mode() fs.FileMode {
	return f.fileMode
}

func (f *FileInfo) ModTime() time.Time {
	return time.Now()
}

func (f *FileInfo) IsDir() bool {
	return f.fileMode.IsDir()
}

func (f *FileInfo) Sys() any {
	panic("not implemented") // TODO: Implement
}
