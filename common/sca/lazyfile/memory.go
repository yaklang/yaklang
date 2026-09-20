package lazyfile

import (
	"bytes"
	"io/fs"
	"path"
	"sync"
)

type memoryFile struct {
	*bytes.Reader
	name string
	size int64
}

func (f *memoryFile) Stat() (fs.FileInfo, error) {
	return NewFileInfo(path.Base(f.name), f.size, 0444), nil
}
func (*memoryFile) Close() error { return nil }

// NewMemory gives a parser independent seek state without creating a host file.
// The caller must budget data before constructing the file and not mutate it.
func NewMemory(name string, data []byte) *LazyFile {
	f := &memoryFile{bytes.NewReader(data), name, int64(len(data))}
	lf := &LazyFile{fileName: name, openOnce: new(sync.Once)}
	lf.finalOpen = func() (fs.File, error) { lf.File = f; return f, nil }
	return lf
}
