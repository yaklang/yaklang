package ssaapi

import (
	"runtime/debug"

	"os"
	"path/filepath"

	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/memedit"
	js2ssa "github.com/yaklang/yaklang/common/yak/JS2ssa"
	"github.com/yaklang/yaklang/common/yak/java/java2ssa"
	"github.com/yaklang/yaklang/common/yak/php/php2ssa"
	"github.com/yaklang/yaklang/common/yak/ssa"
	"github.com/yaklang/yaklang/common/yak/ssa4analyze"
	"github.com/yaklang/yaklang/common/yak/yak2ssa"
)

type Language string

const (
	Yak  Language = "yak"
	JS   Language = "js"
	PHP  Language = "php"
	JAVA Language = "java"
)

type Build func(string, bool, *ssa.FunctionBuilder) error

var (
	LanguageBuilders = map[Language]Build{
		Yak:  yak2ssa.Build,
		JS:   js2ssa.Build,
		PHP:  php2ssa.Build,
		JAVA: java2ssa.Build,
	}
)

func parse(c *config, prog *ssa.Program) (ret *ssa.Program, err error) {
	defer func() {
		if r := recover(); r != nil {
			ret = nil
			err = utils.Errorf("parse error with panic : %v", r)
			debug.PrintStack()
		}
	}()

	if prog == nil {
		prog = ssa.NewProgram(c.DatabaseProgramName)
	}
	editor := memedit.NewMemEditor(c.code)
	prog.PushEditor(editor)
	prog.WithProgramBuilderCacheHitter(c.DatabaseProgramCacheHitter)
	if c.isFilePath {
		dir := c.code
		path, err := findJavaFiles(dir)
		c.filePath = path
		if err != nil {
			return nil, err
		}
	}

	prog.Build = func(s string, fb *ssa.FunctionBuilder) error {
		return c.Build(s, c.ignoreSyntaxErr, fb)
	}

	builder := prog.GetAndCreateFunctionBuilder("main", "main")
	if builder.GetEditor() == nil {
		builder.SetEditor(editor)
	}
	builder.WithExternLib(c.externLib)
	builder.WithExternValue(c.externValue)
	builder.WithExternMethod(c.externMethod)
	builder.WithDefineFunction(c.defineFunc)
	if c.isFilePath {
		dir := c.code
		paths, err := findJavaFiles(dir)
		c.filePath = paths
		if err != nil {
			return nil, err
		}

		for _, path := range paths {
			c.code, err = readJavaContent(path)
			if err != nil {
				return nil, err
			}
			if err := prog.Build(c.code, builder); err != nil {
				return nil, err
			}
			builder.Finish()
			ssa4analyze.RunAnalyzer(prog)
			prog.Finish()
		}
		return prog, nil
	}

	if err := prog.Build(c.code, builder); err != nil {
		return nil, err
	}

	builder.Finish()
	ssa4analyze.RunAnalyzer(prog)
	prog.Finish()
	return prog, nil
}

func feed(c *config, prog *ssa.Program, code string) {
	builder := prog.GetAndCreateFunctionBuilder("main", "main")
	if err := c.Build(code, c.ignoreSyntaxErr, builder); err != nil {
		return
	}
	builder.Finish()
	ssa4analyze.RunAnalyzer(prog)
}

func findJavaFiles(dir string) ([]string, error) {
	var javaFiles []string
	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if !info.IsDir() && filepath.Ext(path) == ".java" {
			javaFiles = append(javaFiles, path)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return javaFiles, nil
}

func readJavaContent(path string) (string, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}

	return string(content), err
}
