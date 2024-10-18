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

// ========================================== For SSAAPI ==========================================

type SSABuilder struct {
	ssa.DummyPreHandler
}

var Builder = &SSABuilder{}

func (s *SSABuilder) Create() ssa.Builder {
	return &SSABuilder{}
}

func (*SSABuilder) Build(src string, force bool, b *ssa.FunctionBuilder) error {
	b.SupportClass = true
	ast, err := Frontend(src, force)
	if err != nil {
		return err
	}
	build := &builder{
		FunctionBuilder:   b,
		ast:               ast,
		constMap:          make(map[string]ssa.Value),
		fullTypeNameMap:   make(map[string][]string),
		allImportPkgSlice: make([][]string, 0),
		selfPkgPath:       make([]string, 0),
	}
	build.SupportClassStaticModifier = true
	build.VisitCompilationUnit(ast)
	return nil
}

func (*SSABuilder) FilterFile(path string) bool {
	return filepath.Ext(path) == ".java"
}

func (*SSABuilder) GetLanguage() consts.Language {
	return consts.JAVA
}

// ========================================== Build Front End ==========================================

type builder struct {
	*ssa.FunctionBuilder
	ast            javaparser.ICompilationUnitContext
	constMap       map[string]ssa.Value
	bluePrintStack *utils.Stack[*ssa.ClassBluePrint]

	// for full type name
	fullTypeNameMap   map[string][]string
	allImportPkgSlice [][]string
	selfPkgPath       []string
}

func (b *builder) PushBluePrint(bp *ssa.ClassBluePrint) {
	if b.bluePrintStack == nil {
		b.bluePrintStack = utils.NewStack[*ssa.ClassBluePrint]()
	}
	b.bluePrintStack.Push(bp)
}

func (b *builder) PeekCurrentBluePrint() *ssa.ClassBluePrint {
	if b.bluePrintStack == nil {
		return nil
	}
	return b.bluePrintStack.Peek()
}

func (b *builder) PopBluePrint() *ssa.ClassBluePrint {
	if b.bluePrintStack == nil {
		return nil
	}
	return b.bluePrintStack.Pop()
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
	return nil, utils.Errorf("parse AST FrontEnd error: %v", errListener.GetErrorString())
}

func (b *builder) AssignConst(name string, value ssa.Value) bool {
	if ConstValue, ok := b.constMap[name]; ok {
		log.Warnf("const %v has been defined value is %v", name, ConstValue.String())
		return false
	}

	b.constMap[name] = value
	return true
}

func (b *builder) ReadConst(name string) (ssa.Value, bool) {
	v, ok := b.constMap[name]
	return v, ok
}

func (b *builder) AssignClassConst(className, key string, value ssa.Value) {
	name := fmt.Sprintf("%s_%s", className, key)
	b.AssignConst(name, value)
}
func (b *builder) ReadClassConst(className, key string) (ssa.Value, bool) {
	name := fmt.Sprintf("%s_%s", className, key)
	return b.ReadConst(name)
}
