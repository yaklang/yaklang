package ssa

import (
	"path"
	"strings"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils/memedit"
)

func (b *FunctionBuilder) AddIncludePath(path string) {
	p := b.GetProgram()
	p.Loader.AddIncludePath(path)
}

func (b *FunctionBuilder) BuildFilePackage(filename string, once bool) error {
	p := b.GetProgram()
	file, data, err := p.Loader.LoadFilePackage(filename, once)
	if err != nil {
		return err
	}

	_path := p.Loader.GetCurrentPath()
	p.Loader.SetCurrentPath(path.Dir(file))

	err = p.Build(file, data, b)

	p.Loader.SetCurrentPath(_path)
	return err
}

func (b *FunctionBuilder) BuildDirectoryPackage(name string, once bool) error {
	p := b.GetProgram()
	ch, err := p.Loader.LoadDirectoryPackage(name, once)
	if err != nil {
		return err
	}
	for v := range ch {

		_path := p.Loader.GetCurrentPath()
		p.Loader.SetCurrentPath(path.Dir(v.FileName))

		raw, err := p.Loader.GetFilesysFileSystem().ReadFile(v.FileName)
		if err != nil {
			log.Errorf("Build with file loader failed: %s", err)
			continue
		}
		err = p.Build(v.FileName, memedit.NewMemEditor(string(raw)), b)
		p.Loader.SetCurrentPath(_path)

		if err != nil {
			return err
		}
	}
	return nil
}

func (b *FunctionBuilder) AddCurrentPackagePath(path []string) *FunctionBuilder {
	prog := b.GetProgram()
	pkgName := strings.Join(path, ".")
	if pkgName != "" {
		lib := prog.NewLibrary(pkgName, path)
		return lib.GetAndCreateFunctionBuilder(pkgName, "init")
	}
	return nil

}

func (b *FunctionBuilder) GetCurrentPackagePath() []string {
	p := b.GetProgram()
	pkgPath := p.Loader.GetPackagePath()
	return pkgPath
}
