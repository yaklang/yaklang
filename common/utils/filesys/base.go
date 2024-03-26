package filesys

import (
	"embed"
	"github.com/gobwas/glob"
	"github.com/kr/fs"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
)

type dirChain struct {
	dirGlob string
	globIns glob.Glob
	opts    []Option
}

type exactChain struct {
	dirpath string
	opts    []Option
}

type embedFs struct {
	f embed.FS
}

func (e embedFs) ReadDir(dirname string) ([]os.FileInfo, error) {
	ns, err := e.f.ReadDir(dirname)
	if err != nil {
		return nil, err
	}
	var infos = make([]os.FileInfo, 0, len(ns))
	for _, n := range ns {
		info, err := n.Info()
		if err != nil {
			return nil, err
		}
		infos = append(infos, info)
	}
	return infos, nil
}

func (e embedFs) Lstat(name string) (os.FileInfo, error) {
	f, err := e.f.Open(name)
	if err != nil {
		//_, err := e.f.ReadDir(name)
		//if err != nil {
		//	return nil, err
		//}
		//var i os.FileInfo = embedDirInfo(name)
		//return i, nil
		return nil, err
	}
	return f.Stat()
}

func (e embedFs) Join(elem ...string) string {
	return path.Join(elem...)
}

func fromEmbedFS(fs2 embed.FS) fs.FileSystem {
	return &embedFs{fs2}
}

// local filesystem
type localFs struct{}

func (f *localFs) ReadDir(dirname string) ([]os.FileInfo, error) { return ioutil.ReadDir(dirname) }

func (f *localFs) Lstat(name string) (os.FileInfo, error) { return os.Lstat(name) }

func (f *localFs) Join(elem ...string) string { return filepath.Join(elem...) }
