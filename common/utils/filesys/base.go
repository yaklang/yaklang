package filesys

import (
	"embed"
	"io/fs"
	"os"
	"path"
	"path/filepath"
)

type embedFs struct {
	f embed.FS
}

var _ FileSystem = (*embedFs)(nil)

func (e *embedFs) ReadDir(dirname string) ([]fs.DirEntry, error) { return e.f.ReadDir(dirname) }
func (e *embedFs) Open(name string) (fs.File, error)             { return e.f.Open(name) }
func (e *embedFs) Stat(name string) (fs.FileInfo, error) {
	fn, err := e.f.Open(name)
	if err != nil {
		return nil, err
	}
	return fn.Stat()
}

func (e *embedFs) GetSeparators() rune { return '/' }
func (f *embedFs) Join(name ...string) string {
	return path.Join(name...)
}

func fromEmbedFS(fs2 embed.FS) FileSystem {
	return &embedFs{fs2}
}

// local filesystem
type LocalFs string

func NewLocalFs(path string) LocalFs {
	return LocalFs(path)
}

var _ FileSystem = (LocalFs)("")

func (f LocalFs) Open(name string) (fs.File, error)             { return os.Open(f.Join(name)) }
func (f LocalFs) Stat(name string) (fs.FileInfo, error)         { return os.Stat(f.Join(name)) }
func (f LocalFs) ReadDir(dirname string) ([]fs.DirEntry, error) { return os.ReadDir(f.Join(dirname)) }
func (f LocalFs) GetSeparators() rune                           { return filepath.Separator }
func (f LocalFs) Join(name ...string) string {
	p := append([]string{string(f)}, name...)
	return filepath.Join(p...)
}
