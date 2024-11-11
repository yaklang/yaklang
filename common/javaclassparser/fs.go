package javaclassparser

import (
	"io/fs"
	"os"

	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/utils/memfile"
)

type FS struct {
	*filesys.ZipFS
}

var _ fs.FS = (*FS)(nil)
var _ fs.ReadFileFS = (*FS)(nil)
var _ fs.ReadDirFS = (*FS)(nil)

func NewJarFSFromLocal(path string) (*FS, error) {
	zipFS, err := filesys.NewZipFSFromLocal(path)
	if err != nil {
		return nil, err
	}
	return NewJarFS(zipFS), nil
}
func NewJarFS(zipFs *filesys.ZipFS) *FS {
	return &FS{
		ZipFS: zipFs,
	}
}
func (z *FS) ReadFile(name string) ([]byte, error) {
	// fmt.Printf("start parse file: %s\n", name)
	data, err := z.ZipFS.ReadFile(name)
	if err != nil {
		return nil, err
	}
	cf, err := Parse(data)
	if err != nil {
		return nil, err
	}
	source, err := cf.Dump()
	if err != nil {
		return nil, err
	}
	return []byte(source), nil
}
func (f *FS) OpenFile(name string, flag int, perm os.FileMode) (fs.File, error) {
	return f.Open(name)
}
func (z *FS) Open(name string) (fs.File, error) {
	raw, err := z.ZipFS.ReadFile(name)
	if err != nil {
		return nil, err
	}
	cf, err := Parse(raw)
	if err != nil {
		return nil, err
	}
	source, err := cf.Dump()
	if err != nil {
		return nil, err
	}
	return memfile.New([]byte(source)), nil
}
