package ssautil

import (
	"embed"
	"errors"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/filesys"
	"os"
	"path"
	"path/filepath"
	"strings"
)

type PackageLoader struct {
	// PackageName is the name of the package to load
	basePath    string
	embedFS     *embed.FS
	ruleHandler func(operator PackageLoaderOperator, packageName string) error
	includePath []string
}

func (p *PackageLoader) GetCurrentBasePathDirectory() string {
	if p.basePath == "" {
		return ""
	}
	if p.embedFS != nil {
		return path.Dir(p.basePath)
	} else {
		return filepath.Dir(p.basePath)
	}
}

func (p *PackageLoader) Join(s ...string) string {
	if p.embedFS != nil {
		return path.Join(s...)
	} else {
		return filepath.Join(s...)
	}
}

func (p *PackageLoader) IncludePathJoin(s ...string) []string {
	var dirs []string
	if name := p.GetCurrentBasePathDirectory(); name != "" {
		dirs = append([]string{name}, p.includePath...)
	} else {
		dirs = p.includePath
	}

	var paths = make([]string, 0, len(dirs))
	for _, dir := range dirs {
		paths = append(paths, p.Join(append([]string{dir}, s...)...))
	}
	return paths
}

func (p *PackageLoader) GetFirstExistedPath(paths ...string) string {
	return utils.GetFirstExistedFile(paths...)
}

func (p *PackageLoader) LoadFilePackage(filepathName string) ([]byte, error) {
	if p.embedFS == nil && !filepath.IsAbs(filepathName) {
		return nil, utils.Error("file path must be absolute")
	}
	if p.embedFS != nil {
		return p.embedFS.ReadFile(filepathName)
	}
	return os.ReadFile(filepathName)
}

type FileDescriptor struct {
	PathName string
	Info     os.FileInfo
	Data     []byte
}

func (p *PackageLoader) LoadDirectoryPackage(directory string) (chan FileDescriptor, error) {
	ch := make(chan FileDescriptor)

	if p.embedFS == nil && !filepath.IsAbs(directory) {
		return nil, utils.Error("directory path must be absolute")
	}

	go func() {
		defer close(ch)
		if p.embedFS != nil {
			err := filesys.Recursive(directory, filesys.WithEmbedFS(*p.embedFS), filesys.WithFileStat(func(pathname string, info os.FileInfo) error {
				if ret := path.Dir(pathname); ret == directory || strings.TrimRight(ret, "/") == strings.TrimRight(directory, "/") {
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
				log.Errorf("load directory package %s failed: %v", directory, err)
			}
		} else {
			absDir, _ := filepath.Abs(directory)
			if absDir == "" {
				absDir = directory
			}
			err := filesys.Recursive(directory, filesys.WithFileStat(func(pathname string, info os.FileInfo) error {
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
				log.Errorf("load directory package %s failed: %v", directory, err)
			}
		}
	}()
	return ch, nil
}

type PackageLoaderOption func(*PackageLoader)

type PackageLoaderOperator interface {
	GetCurrentBasePathDirectory() string
	IncludePathJoin(s ...string) []string
	GetFirstExistedPath(paths ...string) string
	LoadFilePackage(filepath string) ([]byte, error)
	LoadDirectoryPackage(directory string) (chan FileDescriptor, error)
}

func WithPackageLoaderHandler(handler func(operator PackageLoaderOperator, packageName string) error) PackageLoaderOption {
	return func(loader *PackageLoader) {
		loader.ruleHandler = handler
	}
}

func WithEmbedFS(fs embed.FS) PackageLoaderOption {
	return func(loader *PackageLoader) {
		loader.embedFS = &fs
	}
}

func NewPackageLoader(currentBasePath string, opts ...PackageLoaderOption) (*PackageLoader, error) {
	if currentBasePath == "" {
		return nil, utils.Error("base path is required")
	}

	loader := &PackageLoader{
		basePath:    currentBasePath,
		ruleHandler: nil,
		includePath: nil,
	}
	for _, i := range opts {
		i(loader)
	}

	if loader.ruleHandler == nil {
		return nil, errors.New("rule handler is required")
	}

	if len(loader.includePath) <= 0 && loader.basePath == "" {
		return nil, errors.New("include path or base path is required")
	}

	return loader, nil
}

func (p *PackageLoader) LoadPackageByName(f string) error {
	if p.ruleHandler == nil {
		return errors.New("rule handler is required")
	}

	err := p.ruleHandler(p, f)
	if err != nil {
		return utils.Errorf("load package %s failed: %v", f, err)
	}
	return nil
}
