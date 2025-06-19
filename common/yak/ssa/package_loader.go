package ssa

import (
	"strings"

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
	//针对yak这种，不经过lazyBuild，就需要进行特殊处理，如果我们没有拿到，但是符合后缀，我们就
	languageConfig := mainProgram.config
	if !languageConfig.ShouldBuild(filename) {
		return nil
	}
	//todo: modify config parse init build，fix main/@main
	//目前代码仅仅为yak实现，因为yak不经过lazy build，并且yak的include过于特殊
	program := mainProgram.createSubProgram(data.GetPureSourceHash(), Library)
	builder := program.GetAndCreateFunctionBuilder("", string(MainFunctionName))
	//模拟编译，编译两次
	program.SetPreHandler(true)
	languageBuilder := languageConfig.LanguageBuilder
	if languageBuilder == nil {
		log.Errorf("language builder is nil")
		return nil
	}
	languageBuilder.PreHandlerFile(data, builder)
	program.SetPreHandler(false)
	err = mainProgram.Build(filename, data, builder)
	if err != nil {
		return err
	}
	mainProgram.LazyBuild()
	builder.Finish()
	b.includeStack.Push(program)
	return nil
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
