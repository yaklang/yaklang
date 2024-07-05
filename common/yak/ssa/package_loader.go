package ssa

import (
	"path"
	"strings"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
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

func (b *FunctionBuilder) BuildDirectoryPackage(name []string, once bool) (*Program, error) {
	p := b.GetProgram()

	path := p.Loader.GetFilesysFileSystem().Join(name...)
	ch, err := p.Loader.LoadDirectoryPackage(path, once)
	if err != nil {
		return nil, err
	}
	for v := range ch {
		_path := p.Loader.GetCurrentPath()
		p.Loader.SetCurrentPath(path)

		raw, err := p.Loader.GetFilesysFileSystem().ReadFile(v.FileName)
		if err != nil {
			log.Errorf("Build with file loader failed: %s", err)
			continue
		}
		err = p.Build(v.FileName, memedit.NewMemEditor(string(raw)), b)
		p.Loader.SetCurrentPath(_path)

		if err != nil {
			// return err
			continue
		}
	}
	// TODO: get program from name, but in some case, package name not same with path
	if prog, err := GetProgram(strings.Join(name, "."), Library); err == nil {
		// prog.Finish()
		return prog, nil
	}
	return nil, utils.Errorf("Build package %v failed", name)
}
