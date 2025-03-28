package ssa

import (
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
	includePaths := p.GetIncludeFiles()
	fs := p.Loader.GetFilesysFileSystem()
	dir, _ := fs.PathSplit(b.GetEditor().GetFilename())
	p.Loader.AddIncludePath(dir)
	currentMode := b.Included
	defer func() {
		b.Included = currentMode
		p.Loader.SetIncludePaths(includePaths)
	}()
	_, data, err := p.Loader.LoadFilePackage(filename, once)
	if err != nil {
		return err
	}
	mainProgram := b.GetProgram().GetApplication()
	subProg, exist := mainProgram.UpStream.Get(data.GetPureSourceHash())
	if exist {
		subProg.LazyBuild()
		b.includeStack.Push(subProg)
		return nil
	}
	//include file not .php need to build
	//program := mainProgram.createSubProgram(data.GetPureSourceHash(), Library)
	//builder := program.GetAndCreateFunctionBuilder(string(MainFunctionName), string(MainFunctionName))
	//fullFilename := fs.Join(dir, filename)
	//err = mainProgram.Build(fullFilename, data, builder)
	//mainProgram.LazyBuild()
	//b.includeStack.Push(program)
	return nil
	//return err
}

func (b *FunctionBuilder) BuildDirectoryPackage(name []string, once bool) (*Program, error) {
	p := b.GetProgram()

	path := p.Loader.GetFilesysFileSystem().Join(name...)
	ch, err := p.Loader.LoadDirectoryPackage(path, once)
	if err != nil {
		return nil, err
	}
	app := p.GetApplication()
	app.ProcessInfof("Build package %v", name)
	for v := range ch {
		_path := p.Loader.GetCurrentPath()
		p.Loader.SetCurrentPath(path)

		raw, err := p.Loader.GetFilesysFileSystem().ReadFile(v.FileName)
		if err != nil {
			log.Errorf("Build with file loader failed: %s", err)
			continue
		}
		// var build
		build := app.Build
		if build != nil {
			err = build(v.FileName, memedit.NewMemEditor(string(raw)), b)
		} else {
			log.Errorf("BUG: Build function is nil in package %s", p.Name)
		}
		p.Loader.SetCurrentPath(_path)

		if err != nil {
			// return err
			continue
		}
	}
	// TODO: get program from name, but in some case, package name not same with path
	if lib, _ := p.GetLibrary(strings.Join(name, ".")); lib != nil {
		// lib.Finish()
		return lib, nil
	}
	return nil, utils.Errorf("Build package %v failed", name)
}
