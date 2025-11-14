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
	dir := b.GetEditor().GetFolderPath()
	p.Loader.AddIncludePath(dir)
	currentMode := b.Included
	defer func() {
		b.Included = currentMode
		p.Loader.SetIncludePaths(includePaths)
	}()

	log.Debugf("[INCLUDE] 开始处理 include: %s (once=%v)", filename, once)

	_, editor, err := p.Loader.LoadFilePackage(filename, once)
	if err != nil {
		log.Debugf("[INCLUDE] 加载文件失败: %s, error: %v", filename, err)
		return err
	}
	mainProgram := b.GetProgram().GetApplication()
	editor = mainProgram.CreateEditor([]byte(editor.GetSourceCode()), filename)
	fileHash := editor.GetPureSourceHash()

	log.Debugf("[INCLUDE] 文件: %s, hash: %s", filename, fileHash)

	subProg, exist := mainProgram.UpStream.Get(fileHash)
	if exist {
		log.Debugf("[INCLUDE] ✓ 缓存命中! 文件: %s (hash: %s) 从缓存读取，跳过编译", filename, fileHash)
		subProg.LazyBuild()
		b.includeStack.Push(subProg)
		return nil
	}

	log.Debugf("[INCLUDE] ✗ 缓存未命中! 文件: %s (hash: %s) 需要重新编译", filename, fileHash)
	//针对yak这种，不经过lazyBuild，就需要进行特殊处理，如果我们没有拿到，但是符合后缀，我们就
	languageConfig := mainProgram.config
	if !languageConfig.ShouldBuild(filename) {
		log.Debugf("[INCLUDE] 跳过构建: %s (不符合构建条件)", filename)
		return nil
	}

	log.Debugf("[INCLUDE] >>> 开始编译: %s <<<", filename)

	// 创建子程序用于存储include文件的编译结果
	program := mainProgram.GetSubProgram(fileHash)
	builder := program.GetAndCreateFunctionBuilder("", string(MainFunctionName))

	languageBuilder := languageConfig.LanguageBuilder
	if languageBuilder == nil {
		log.Errorf("language builder is nil")
		return nil
	}

	// 解析AST
	ast, err := languageBuilder.ParseAST(editor.GetSourceCode(), nil)
	if err != nil {
		log.Debugf("[INCLUDE] 解析AST失败: %s, error: %v", filename, err)
		return utils.Errorf("parse file %s error: %v", filename, err)
	}

	// 直接使用mainProgram.Build编译，它会自动处理PreHandler和正式Build
	err = mainProgram.Build(ast, editor, builder)
	if err != nil {
		log.Debugf("[INCLUDE] 构建失败: %s, error: %v", filename, err)
		return err
	}

	program.LazyBuild()
	builder.Finish()
	b.includeStack.Push(program)

	log.Debugf("[INCLUDE] <<< 编译完成: %s >>>", filename)

	// 修复：将子程序的编译结果存入缓存
	log.Debugf("[INCLUDE] 将文件 %s 的编译结果存入缓存 (hash: %s)", filename, fileHash)
	mainProgram.UpStream.Set(fileHash, program) // program是子程序，不是mainProgram
	log.Debugf("[INCLUDE] ✓ 缓存已更新! 下次include %s 将直接使用缓存", filename)

	return nil
}

func (b *FunctionBuilder) BuildDirectoryPackage(name []string, once bool) (*Program, error) {
	p := b.GetProgram()

	path := p.Loader.GetFilesysFileSystem().Join(name...)
	ch, err := p.Loader.LoadDirectoryPackage(path, once)
	if err != nil {
		return nil, err
	}

	languageConfig := p.GetApplication().config
	languageBuilder := languageConfig.LanguageBuilder
	if languageBuilder == nil {
		return nil, utils.Errorf("language builder is nil")
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
		ast, err := languageBuilder.ParseAST(string(raw), nil)
		if err != nil {
			log.Errorf("Parse file %s error: %v", v.FileName, err)
			continue
		}
		// var build
		build := app.Build
		if build != nil {
			err = build(ast, memedit.NewMemEditor(string(raw)), b)
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
