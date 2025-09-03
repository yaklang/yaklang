package java2ssa

import (
	"fmt"
	"path/filepath"

	"github.com/antlr/antlr4/runtime/Go/antlr/v4"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/antlr4util"
	javaparser "github.com/yaklang/yaklang/common/yak/java/parser"
	"github.com/yaklang/yaklang/common/yak/ssa"
)

var INNER_CLASS_SPLIT = "$"

// ========================================== For SSAAPI ==========================================

type SSABuilder struct {
	*ssa.PreHandlerInit
}

var Builder ssa.Builder = &SSABuilder{}

func (*SSABuilder) ParseAST(src string) (ssa.FrontAST, error) {
	return Frontend(src, false)
}
func (*SSABuilder) BuildFromAST(raw ssa.FrontAST, b *ssa.FunctionBuilder) error {
	ast, ok := raw.(javaparser.ICompilationUnitContext)
	if !ok {
		return utils.Errorf("invalid AST type: %T, expected javaparser.ICompilationUnitContext", raw)
	}
	build := &singleFileBuilder{
		FunctionBuilder:   b,
		constMap:          make(map[string]ssa.Value),
		fullTypeNameMap:   make(map[string][]string),
		allImportPkgSlice: make([][]string, 0),
		selfPkgPath:       make([]string, 0),
	}
	build.initImport()
	build.VisitCompilationUnit(ast)
	return nil
}

func (*SSABuilder) FilterFile(path string) bool {
	ext := filepath.Ext(path)
	return ext == ".java"
}

func (*SSABuilder) GetLanguage() consts.Language {
	return consts.JAVA
}

// ========================================== Build Front End ==========================================

type singleFileBuilder struct {
	*ssa.FunctionBuilder
	constMap map[string]ssa.Value

	// for full type name
	fullTypeNameMap   map[string][]string
	allImportPkgSlice [][]string
	selfPkgPath       []string

	// framework support for spring boot
	currentUIModel ssa.Value
	isInController bool
}

func Frontend(src string, force bool) (javaparser.ICompilationUnitContext, error) {
	errListener := antlr4util.NewErrorListener()
	lexer := javaparser.NewJavaLexer(antlr.NewInputStream(src))
	lexer.RemoveErrorListeners()
	lexer.AddErrorListener(errListener)
	tokenStream := antlr.NewCommonTokenStream(lexer, antlr.TokenDefaultChannel)
	parser := javaparser.NewJavaParser(tokenStream)
	parser.RemoveErrorListeners()
	parser.AddErrorListener(errListener)
	parser.SetErrorHandler(antlr.NewDefaultErrorStrategy())
	ast := parser.CompilationUnit()
	if force || len(errListener.GetErrors()) == 0 {
		return ast, nil
	}
	return ast, utils.Errorf("parse AST FrontEnd error: %v", errListener.GetErrorString())
}

func (b *singleFileBuilder) AssignConst(name string, value ssa.Value) bool {
	if ConstValue, ok := b.constMap[name]; ok {
		log.Warnf("const %v has been defined value is %v", name, ConstValue.String())
		return false
	}

	b.constMap[name] = value
	return true
}

func (b *singleFileBuilder) ReadConst(name string) (ssa.Value, bool) {
	v, ok := b.constMap[name]
	return v, ok
}

func (b *singleFileBuilder) AssignClassConst(className, key string, value ssa.Value) {
	name := fmt.Sprintf("%s_%s", className, key)
	b.AssignConst(name, value)
}
func (b *singleFileBuilder) ReadClassConst(className, key string) (ssa.Value, bool) {
	name := fmt.Sprintf("%s_%s", className, key)
	return b.ReadConst(name)
}

func (b *singleFileBuilder) SwitchFunctionBuilder(s *ssa.StoredFunctionBuilder) func() {
	t := b.StoreFunctionBuilder()
	b.LoadBuilder(s)
	return func() {
		b.LoadBuilder(t)
	}
}

func (b *singleFileBuilder) LoadBuilder(s *ssa.StoredFunctionBuilder) {
	b.FunctionBuilder = s.Current
	b.LoadFunctionBuilder(s.Store)
}

func (b *singleFileBuilder) initImport() {
	b.allImportPkgSlice = append(b.allImportPkgSlice, []string{"java", "lang", "*"})
}
