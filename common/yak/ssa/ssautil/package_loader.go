package ssautil

import (
	"embed"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/filesys"
)

type PackageLoaderOption func(*PackageLoader)

func WithEmbedFS(fs embed.FS) PackageLoaderOption {
	return func(loader *PackageLoader) {
		loader.embedFS = &fs
	}
}

func WithIncludePath(paths ...string) PackageLoaderOption {
	return func(loader *PackageLoader) {
		loader.includePath = paths
	}
}
func WithCurrentPath(path string) PackageLoaderOption {
	return func(loader *PackageLoader) {
		loader.currentPath = path //当前路径
	}
}

type PackageLoader struct {
	embedFS      *embed.FS
	currentPath  string
	includePath  []string
	includedPath map[string]struct{} // for include once
}

func NewPackageLoader(opts ...PackageLoaderOption) *PackageLoader {
	loader := &PackageLoader{
		currentPath:  "",
		includePath:  make([]string, 0),
		includedPath: make(map[string]struct{}),
	}
	for _, i := range opts {
		i(loader)
	}
	return loader
}

func (p *PackageLoader) SetCurrentPath(currentPath string) {
	p.currentPath = currentPath
}

func (p *PackageLoader) join(s ...string) string {
	if p.embedFS != nil {
		return path.Join(s...)
	} else {
		return filepath.Join(s...)
	}
}

func (p *PackageLoader) AddIncludePath(s ...string) {
	p.includePath = append(p.includePath, s...)
}

func (p *PackageLoader) FilePath(wantPath string, once bool) (string, error) {
	return p.getPath(wantPath, once, utils.IsFile)
}

func (p *PackageLoader) DirPath(wantPath string, once bool) (string, error) {
	return p.getPath(wantPath, once, utils.IsDir)
}

func (p *PackageLoader) getPath(want string, once bool, f func(string) bool) (string, error) {
	// found path in current path
	tmpPath := append([]string{p.currentPath}, p.includePath...)
	for _, path := range tmpPath {
		filePath := p.join(path, want)
		if f(filePath) {
			if once {
				if _, ok := p.includedPath[filePath]; ok {
					// only check included, in once = true
					return "", utils.Errorf("file or directory %s already included", want)
				}
				p.includedPath[filePath] = struct{}{}
			}
			return filePath, nil
		}
	}
	return "", utils.Errorf("file or directory %s not found in include path", want)
}

func (p *PackageLoader) LoadFilePackage(packageName string, once bool) (string, []byte, error) {
	path, err := p.FilePath(packageName, once)
	if err != nil {
		return "", nil, err
	}
	if p.embedFS != nil {
		data, err := p.embedFS.ReadFile(path)
		return path, data, err
	}
	data, err := os.ReadFile(path)
	return path, data, err
}

type FileDescriptor struct {
	PathName string
	Info     os.FileInfo
	Data     []byte
}

func (p *PackageLoader) LoadDirectoryPackage(packageName string, once bool) (chan FileDescriptor, error) {
	ch := make(chan FileDescriptor)

	go func() {
		defer close(ch)
		if p.embedFS != nil {
			err := filesys.Recursive(packageName, filesys.WithEmbedFS(*p.embedFS), filesys.WithFileStat(func(pathname string, info os.FileInfo) error {
				if ret := path.Dir(pathname); ret == packageName || strings.TrimRight(ret, "/") == strings.TrimRight(packageName, "/") {
					data, err := p.embedFS.ReadFile(pathname)
					if err != nil {
						return err
					}
					ch <- FileDescriptor{
						PathName: pathname,
						Info:     info,
						Data:     data,
					}
				}
				return nil
			}))
			if err != nil {
				log.Errorf("load directory package %s failed: %v", packageName, err)
			}
		} else {
			absDir, err := p.DirPath(packageName, once)
			if err != nil {
				log.Errorf("load directory package %s failed: %v", packageName, err)
				return
			}
			err = filesys.Recursive(absDir, filesys.WithFileStat(func(pathname string, info os.FileInfo) error {
				if ret := path.Dir(pathname); filepath.Clean(ret) == filepath.Clean(absDir) {
					data, err := os.ReadFile(pathname)
					if err != nil {
						return err
					}
					ch <- FileDescriptor{
						PathName: pathname,
						Info:     info,
						Data:     data,
					}
				}
				return nil
			}))
			if err != nil {
				log.Errorf("load directory package %s failed: %v", packageName, err)
			}
		}
	}()
	return ch, nil
}
