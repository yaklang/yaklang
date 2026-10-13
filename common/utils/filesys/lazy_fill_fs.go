package filesys

import (
	"io/fs"
	"os"
	"sync"

	"github.com/yaklang/yaklang/common/utils"
	fi "github.com/yaklang/yaklang/common/utils/filesys/filesys_interface"
)

// ContentSource loads one file's body by its path in the wrapped
// filesystem.
type ContentSource func(path string) ([]byte, error)

// LazyFillFS wraps a structure-only filesystem (directories + empty file
// stubs) and fills file bodies on demand through the content source: the
// first ReadFile of a stub loads the body and writes it back into the
// wrapped tree, so repeated reads are plain memory hits and the filled
// body shows up in the wrapped tree's listings and Stat sizes. The wrapped
// filesystem keeps everything else (listings, Stat, path rules).
type LazyFillFS struct {
	fi.FileSystem // the structure-only tree; stubs are filled in place

	readContent ContentSource

	mu sync.Mutex // guards the stub write-backs
}

var _ fi.FileSystem = (*LazyFillFS)(nil)

// NewLazyFillFS wraps a structure-only filesystem whose file bodies load
// lazily through src.
func NewLazyFillFS(underlying fi.FileSystem, src ContentSource) *LazyFillFS {
	return &LazyFillFS{FileSystem: underlying, readContent: src}
}

// ReadFile serves the filled memory hit, else loads the body through the
// content source and writes it back into the wrapped tree (replacing the
// empty stub, which also makes Stat report the real size). The content
// read runs outside the lock — a slow fetch never blocks listings; only
// the cheap stub replacement takes it.
func (l *LazyFillFS) ReadFile(name string) ([]byte, error) {
	if data, err := l.FileSystem.ReadFile(name); err == nil && len(data) > 0 {
		return data, nil
	}
	if l.readContent == nil {
		return nil, utils.Errorf("file [%v] not exist", name)
	}
	data, err := l.readContent(name)
	if err != nil {
		return nil, err
	}
	l.mu.Lock()
	_ = l.FileSystem.Delete(name)
	_ = l.FileSystem.WriteFile(name, data, 0o644)
	l.mu.Unlock()
	return data, nil
}

func (l *LazyFillFS) Open(name string) (fs.File, error) {
	data, err := l.ReadFile(name)
	if err != nil {
		return nil, err
	}
	return NewVirtualFile(name, string(data)), nil
}

func (l *LazyFillFS) OpenFile(name string, flag int, perm os.FileMode) (fs.File, error) {
	return l.Open(name)
}