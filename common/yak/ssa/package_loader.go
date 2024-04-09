package ssa

import (
	"github.com/yaklang/yaklang/common/utils"
	"path"
)

func (b *FunctionBuilder) AddIncludePath(path string) {
	p := b.GetProgram()
	p.loader.AddIncludePath(path)
}

func (b *FunctionBuilder) BuildFilePackage(filename string, once bool) error {
	p := b.GetProgram()
	file, data, err := p.loader.LoadFilePackage(filename, once)
	if err != nil {
		return err
	}
	tmpFile := b.CurrentFile
	b.CurrentFile = file
	p.loader.SetCurrentPath(path.Dir(file))
	err = p.Build(utils.UnsafeBytesToString(data), b)
	b.CurrentFile = tmpFile // recover
	return err
}

func (b *FunctionBuilder) BuildDirectoryPackage(name string, once bool) error {
	p := b.GetProgram()
	ch, err := p.loader.LoadDirectoryPackage(name, once)
	if err != nil {
		return err
	}
	for v := range ch {
		file := b.CurrentFile
		b.CurrentFile = v.PathName
		p.loader.SetCurrentPath(path.Dir(file))
		err := p.Build(utils.UnsafeBytesToString(v.Data), b)
		b.CurrentFile = file // recover

		if err != nil {
			return err
		}
	}
	return nil
}
