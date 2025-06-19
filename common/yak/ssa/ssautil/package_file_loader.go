package ssautil

import (
	"io/fs"

	"github.com/yaklang/yaklang/common/utils/memedit"

	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/filesys"
	fi "github.com/yaklang/yaklang/common/utils/filesys/filesys_interface"
)

type PackageLoaderOption func(*PackageFileLoader)

func WithFileSystem(fs fi.FileSystem) PackageLoaderOption {
	return func(loader *PackageFileLoader) {
		loader.fs = fs
	}
}

func WithBasePath(path string) PackageLoaderOption {
	return func(loader *PackageFileLoader) {
		loader.basePath = path
	}
}
func WithIncludePath(paths ...string) PackageLoaderOption {
	return func(loader *PackageFileLoader) {
		loader.includePath = paths
	}
}

func WithCurrentPath(path string) PackageLoaderOption {
	return func(loader *PackageFileLoader) {
		loader.currentPath = path // 当前路径
	}
}

type PackageFileLoader struct {
	fs           fi.FileSystem
	currentPath  string
	includePath  []string
	includedPath map[string]struct{} // for include once
	packagePath  []string
	basePath     string
}

func (p *PackageFileLoader) GetFilesysFileSystem() fi.FileSystem {
	return p.fs
}

func NewPackageLoader(opts ...PackageLoaderOption) *PackageFileLoader {
	loader := &PackageFileLoader{
		currentPath:  "",
		includePath:  make([]string, 0),
		includedPath: make(map[string]struct{}),
	}
	for _, f := range opts {
		f(loader)
	}
	if loader.fs == nil {
		loader.fs = filesys.NewLocalFs()
	}
	return loader
}

func (p *PackageFileLoader) SetCurrentPath(currentPath string) {
	p.currentPath = currentPath
}

func (p *PackageFileLoader) SetIncludePaths(paths []string) {
	p.includePath = paths
}
func (p *PackageFileLoader) GetCurrentPath() string {
	return p.currentPath
}

func (p *PackageFileLoader) AddPackagePath(path []string) {
	p.packagePath = path
}

func (p *PackageFileLoader) GetPackagePath() []string {
	return p.packagePath
}
func (p *PackageFileLoader) GetBasePath() string {
	return p.basePath
}

func (p *PackageFileLoader) AddIncludePath(s ...string) {
	p.includePath = append(p.includePath, s...)
}

// GetIncludeFiles get include path
func (p *PackageFileLoader) GetIncludeFiles() []string {
	return p.includePath
}

func (p *PackageFileLoader) FilePath(wantPath string, once bool) (string, error) {
	return p.getPath(wantPath, once,
		func(fi fs.FileInfo) bool { return !fi.IsDir() },
	)
}

func (p *PackageFileLoader) DirPath(wantPath string, once bool) (string, error) {
	return p.getPath(wantPath, once,
		func(fi fs.FileInfo) bool { return fi.IsDir() },
	)
}

func (p *PackageFileLoader) getPath(want string, once bool, f func(fs.FileInfo) bool) (string, error) {
	fs := p.fs
	if fs == nil {
		return "", utils.Errorf("file system is nil")
	}
	// found path in current path
	tmpPath := append([]string{p.currentPath}, p.includePath...)
	for _, path := range tmpPath {
		filePath := fs.Join(path, want)
		info, err := fs.Stat(filePath)
		if err != nil {
			continue
		}
		if f(info) {
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

func (p *PackageFileLoader) LoadFilePackage(packageName string, once bool) (string, *memedit.MemEditor, error) {
	if p.fs == nil {
		return "", nil, utils.Errorf("file system is nil")
	}
	path, err := p.FilePath(packageName, once)
	if err != nil {
		return "", nil, err
	}
	data, err := p.fs.ReadFile(path)
	if len(data) == 0 {
		return "", nil, utils.Errorf("file %s is empty", path)
	}
	return path, memedit.NewMemEditor(string(data)), err
}

type FileDescriptor struct {
	FileName string
	Info     fs.FileInfo
}

func (p *PackageFileLoader) LoadDirectoryPackage(packageName string, once bool) (chan FileDescriptor, error) {
	ch := make(chan FileDescriptor)

	absDir, err := p.DirPath(packageName, once)
	if err != nil {
		return nil, err
	}
	go func() {
		defer close(ch)
		err = filesys.Recursive(
			absDir,
			filesys.WithFileSystem(p.fs),
			filesys.WithRecursiveDirectory(false),
			filesys.WithFileStat(func(s string, info fs.FileInfo) error {
				ch <- FileDescriptor{
					FileName: s,
					Info:     info,
				}
				return fs.SkipDir
			}),
		)
		if err != nil {
			log.Errorf("load directory package %s failed: %v", packageName, err)
		}
	}()
	return ch, nil
}
