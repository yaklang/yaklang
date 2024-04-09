package ssa

import "github.com/yaklang/yaklang/common/utils"

func (b *FunctionBuilder) AddIncludePath(path string) {
	p := b.GetProgram()
	p.loader.AddIncludePath(path)
}

func (b *FunctionBuilder) BuildFilePackage(filename string, once bool) error {
	p := b.GetProgram()
	path, data, err := p.loader.LoadFilePackage(filename, once)
	if err != nil {
		return err
	}

	file := b.CurrentFile
	b.CurrentFile = path
	err = p.Build(utils.UnsafeBytesToString(data), b)
	b.CurrentFile = file // recover
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
		err := p.Build(utils.UnsafeBytesToString(v.Data), b)
		b.CurrentFile = file // recover

		if err != nil {
			return err
		}
	}
	return nil
}
