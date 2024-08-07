package go2ssa

import (
	"path/filepath"

	"github.com/antlr/antlr4/runtime/Go/antlr/v4"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/antlr4util"
	"github.com/yaklang/yaklang/common/yak/ssa"

	gol "github.com/yaklang/yaklang/common/yak/antlr4go/parser"
)

type SSABuilder struct {
	ssa.DummyExtraFileAnalyzer
}

var Builder = &SSABuilder{}

func (*SSABuilder) Build(src string, force bool, builder *ssa.FunctionBuilder) error {
	ast, err := Frontend(src, force)
	if err != nil {
		return err
	}
	builder.SupportClosure = true
	astBuilder := &astbuilder{
		FunctionBuilder: builder,
		cmap:            []map[string]struct{}{},
		globalv:         map[string]ssa.Value{},
		structTypes:     map[string]*ssa.ObjectType{},
		aliasTypes:      map[string]*ssa.AliasType{},
		result:          []string{},
		buildInPackage: 	map[string][]string{},
	}
	log.Infof("ast: %s", ast.ToStringTree(ast.GetParser().GetRuleNames(), ast.GetParser()))
	astBuilder.build(ast)
	return nil
}

func (*SSABuilder) FilterFile(path string) bool {
	return filepath.Ext(path) == ".go"
}


type astbuilder struct {
	*ssa.FunctionBuilder
	cmap []map[string]struct{}
	globalv		map[string]ssa.Value
	structTypes map[string]*ssa.ObjectType
	aliasTypes  map[string]*ssa.AliasType
	result      []string
	buildInPackage map[string][]string
}

func Frontend(src string, must bool) (*gol.SourceFileContext, error) {
	errListener := antlr4util.NewErrorListener()
	lexer := gol.NewGoLexer(antlr.NewInputStream(src))
	lexer.RemoveErrorListeners()
	lexer.AddErrorListener(errListener)
	tokenStream := antlr.NewCommonTokenStream(lexer, antlr.TokenDefaultChannel)
	parser := gol.NewGoParser(tokenStream)
	parser.RemoveErrorListeners()
	parser.AddErrorListener(errListener)
	parser.SetErrorHandler(antlr.NewDefaultErrorStrategy())
	ast := parser.SourceFile().(*gol.SourceFileContext)
	if must || len(errListener.GetErrors()) == 0 {
		return ast, nil
	}
	return nil, utils.Errorf("parse AST FrontEnd error : %v", errListener.GetErrorString())
}

func (b *astbuilder) AddToCmap(key string) {
	b.cmap[len(b.cmap)-1][key] = struct{}{}
}

func (b *astbuilder) GetFromCmap(key string) bool {
	for _, m := range b.cmap {
		if _, ok := m[key]; ok {
			return true
		} 
	}
	return false
}

func (b *astbuilder) InCmapLevel() {
	b.cmap = append(b.cmap, make(map[string]struct{}))
}

func (b *astbuilder) OutCmapLevel() {
    b.cmap = b.cmap[:len(b.cmap)-1]
}

func (*SSABuilder) GetLanguage() consts.Language {
	return consts.GO
}

func (b* astbuilder) AddGlobalVariable(name string, v ssa.Value){
	b.globalv[name] = v
}

func (b* astbuilder) GetGlobalVariable(name string) ssa.Value {
	if b.globalv[name] == nil {
		return nil
	}
	return b.globalv[name]
}

func (b *astbuilder) AddResultDefault(name string){
	b.result = append(b.result, name)
}

func (b *astbuilder) GetResultDefault() []string {
	return b.result
}

func (b *astbuilder) CleanResultDefault() {
    b.result = []string{}
}

// TODO: add build in package as a right value
func (b *astbuilder) AddBuildInPackage(name string, p []string) {
    b.buildInPackage[name] = p
}

func (b *astbuilder) GetBuildInPackage(name string) []string {
	if b.buildInPackage[name] == nil {
		return nil
	}
    return b.buildInPackage[name]
}

// ====================== Object type
func (b *astbuilder) AddStruct(name string, t *ssa.ObjectType) {
	b.structTypes[name] = t
}

func (b *astbuilder) GetStructByStr(name string) *ssa.ObjectType {
	if b.structTypes[name] == nil {
		return nil
	}
	return b.structTypes[name]
}

// ====================== Alias type
func (b *astbuilder) AddAlias(name string, t *ssa.AliasType) {
	b.aliasTypes[name] = t 
}

func (b *astbuilder) GetAliasByStr(name string) ssa.Type {
	if b.aliasTypes[name] == nil {
		return nil
	}
	return b.aliasTypes[name]
}