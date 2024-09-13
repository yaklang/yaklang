package go2ssa

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/antlr/antlr4/runtime/Go/antlr/v4"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	fi "github.com/yaklang/yaklang/common/utils/filesys/filesys_interface"
	"github.com/yaklang/yaklang/common/yak/antlr4util"
	"github.com/yaklang/yaklang/common/yak/ssa"

	gol "github.com/yaklang/yaklang/common/yak/antlr4go/parser"
)

type SSABuilder struct {
	ssa.DummyPreHandler
}

type ExData struct {
	exPath       string
	exGlobals    map[string]ssa.Value
	exFuncs      map[string]*ssa.Function
	exTypes      map[string]ssa.Type
	exAliasTypes map[string]*ssa.AliasType
}

var Builder = &SSABuilder{}

func (s *SSABuilder) Create() ssa.Builder {
	return &SSABuilder{}
}

func (s *SSABuilder) InitHandler(fb *ssa.FunctionBuilder) {
	s.InitHandlerOnce.Do(func() {
		container := fb.EmitEmptyContainer()
		fb.GetProgram().GlobalScope = container
	})
}
func (*SSABuilder) FilterPreHandlerFile(path string) bool {
	extension := filepath.Ext(path)
	return extension == ".go" || extension == ".mod"
}

func (s *SSABuilder) PreHandlerProject(fileSystem fi.FileSystem, functionBuilder *ssa.FunctionBuilder, path string) error {
	prog := functionBuilder.GetProgram()
	if prog == nil {
		log.Errorf("program is nil")
		return nil
	}
	if prog.ExtraFile == nil {
		prog.ExtraFile = make(map[string]string)
	}

	dirname, filename := fileSystem.PathSplit(path)
	_ = dirname
	_ = filename

	// go.mod
	if strings.TrimLeft(filename, string(fileSystem.GetSeparators())) == "go.mod" {
		raw, err := fileSystem.ReadFile(path)
		if err != nil {
			log.Warnf("read go.mod error: %v", err)
			return nil
		}
		text := string(raw)
		pattern := `module(.*?)\n`
		re, err := regexp.Compile(pattern)
		if err != nil {
			log.Warnf("compile regexp error: %v", err)
			return nil
		}
		matches := re.FindAllString(text, -1)
		matche := strings.Split(matches[0], " ")
		if len(matches) > 0 {
			path := matche[1]
			prog.ExtraFile["go.mod"] = path[:len(path)-1]
		}
	}

	return nil
}

func (s *SSABuilder) Build(src string, force bool, builder *ssa.FunctionBuilder) error {
	ast, err := Frontend(src, force)
	if err != nil {
		return err
	}

	SpecialTypes := map[string]ssa.Type{
		"comparable": ssa.CreateAnyType(),
		"error":      ssa.CreateErrorType(),
	}
	SpecialValue := map[string]ssa.Value{
		"nil":   builder.EmitConstInstNil(),
		"iota":  builder.EmitConstInst("iota"),
		"true":  builder.EmitConstInst(true),
		"false": builder.EmitConstInst(false),
	}

	builder.SupportClosure = false
	astBuilder := &astbuilder{
		FunctionBuilder: builder,
		cmap:            []map[string]struct{}{},
		structTypes:     map[string]*ssa.ObjectType{},
		aliasTypes:      map[string]*ssa.AliasType{},
		result:          map[string][]string{},
		tpHander:        map[string]func(){},
		labels:          map[string]*ssa.LabelBuilder{},
		extendKey:       map[string]*ExData{},
		specialValues:   SpecialValue,
		specialTypes:    SpecialTypes,
		pkgNameCurrent:  "",
	}
	log.Infof("ast: %s", ast.ToStringTree(ast.GetParser().GetRuleNames(), ast.GetParser()))
	astBuilder.build(ast)
	fmt.Printf("Program: %v done\n", astBuilder.pkgNameCurrent)
	return nil
}

func (*SSABuilder) FilterFile(path string) bool {
	return filepath.Ext(path) == ".go"
}

type astbuilder struct {
	*ssa.FunctionBuilder
	cmap           []map[string]struct{}
	structTypes    map[string]*ssa.ObjectType
	aliasTypes     map[string]*ssa.AliasType
	result         map[string][]string
	tpHander       map[string]func()
	labels         map[string]*ssa.LabelBuilder
	extendKey      map[string]*ExData
	specialValues  map[string]ssa.Value
	specialTypes   map[string]ssa.Type
	pkgNameCurrent string
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
	tcmap := make(map[string]struct{})
	tcmap[key] = struct{}{}
	b.cmap = append(b.cmap, tcmap)
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

func (b *astbuilder) AddGlobalVariable(name string, v ssa.Value) {
	variable := b.CreateMemberCallVariable(b.GetProgram().GlobalScope, b.EmitConstInst(name))
	b.AssignVariable(variable, v)
}

func (b *astbuilder) CheckGlobalVariablePhi(l *ssa.Variable, r ssa.Value) bool {
	name := l.GetName()
	for i, _ := range b.GetProgram().GlobalScope.GetAllMember() {
		if i.String() == name {
			b.GetProgram().GlobalScope.GetAllMember()[i] = r
			return true
		}
	}
	return false
}

func (b *astbuilder) GetGlobalVariableL(name string) (*ssa.Variable, bool) {
	var variable *ssa.Variable
	/*for i, m := range b.GetProgram().GlobalScope.GetAllMember() {
		if i.String() == name {
			variable = m.GetLastVariable()
			return variable, true
		}
	}*/
	return variable, false
}

func (b *astbuilder) GetGlobalVariableR(name string) ssa.Value {
	member, _ := b.GetProgram().GlobalScope.GetStringMember(name)
	return member
}

func (b *astbuilder) GetGlobalVariables() map[string]ssa.Value {
	var variables = make(map[string]ssa.Value)
	for i, m := range b.GetProgram().GlobalScope.GetAllMember() {
		variables[i.String()] = m
	}
	return variables
}

func (b *astbuilder) AddResultDefault(name string) {
	result := b.result[b.Function.GetName()]
	if result == nil {
		result = []string{name}
	} else {
		result = append(result, name)
	}
	b.result[b.Function.GetName()] = result
}

func (b *astbuilder) GetResultDefault() []string {
	return b.result[b.Function.GetName()]
}

func (b *astbuilder) AddExData(exPath string) *ExData {
	exData := &ExData{
		exPath:       exPath,
		exGlobals:    map[string]ssa.Value{},
		exFuncs:      map[string]*ssa.Function{},
		exTypes:      map[string]ssa.Type{},
		exAliasTypes: map[string]*ssa.AliasType{},
	}
	b.extendKey[exPath] = exData
	return exData
}

func (b *astbuilder) GetExData(exPath string) *ExData {
	return b.extendKey[exPath]
}

func (b *ExData) AddExtendFunc(fun *ssa.Function) {
	b.exFuncs[fun.GetName()] = fun
}

func (b *ExData) AddExtendFuncs(funcs map[string]*ssa.Function) {
	for _, f := range funcs {
		b.AddExtendFunc(f)
	}
}

func (b *ExData) GetExtendFuncs() map[string]*ssa.Function {
	return b.exFuncs
}

func (b *ExData) AddExtendType(name string, t ssa.Type) {
	b.exTypes[name] = t
}

func (b *ExData) GetExtendType(name string) ssa.Type {
	if b.exTypes[name] == nil {
		return nil
	}
	return b.exTypes[name]
}

func (b *ExData) AddExtendGlobal(name string, v ssa.Value) {
	b.exGlobals[name] = v
}

func (b *ExData) GetExtendGlobal(name string) ssa.Value {
	if b.exGlobals[name] == nil {
		return nil
	}
	return b.exGlobals[name]
}

func (b *astbuilder) GetLabelByName(name string) *ssa.LabelBuilder {
	if b.labels[name] == nil {
		return nil
	}
	return b.labels[name]
}

// ====================== Object type
func (b *astbuilder) AddStruct(name string, t *ssa.ObjectType) {
	b.structTypes[name] = t
}

func (b *astbuilder) GetStructByStr(name string) ssa.Type {
	if b.structTypes[name] == nil {
		return nil
	}
	return b.structTypes[name]
}

func (b *astbuilder) GetStructAll() map[string]*ssa.ObjectType {
	return b.structTypes
}

// ====================== Alias type
func (b *astbuilder) AddAlias(name string, t *ssa.AliasType) {
	b.aliasTypes[name] = t
}

func (b *astbuilder) DelAliasByStr(name string) {
	delete(b.aliasTypes, name)
}

func (b *astbuilder) GetAliasByStr(name string) ssa.Type {
	if b.aliasTypes[name] == nil {
		return nil
	}
	return b.aliasTypes[name].GetType()
}

func (b *astbuilder) GetAliasAll() map[string]*ssa.AliasType {
	return b.aliasTypes
}

// ====================== Special
func (b *astbuilder) GetSpecialTypeByStr(name string) ssa.Type {
	if b.specialTypes[name] == nil {
		return nil
	}
	return b.specialTypes[name]
}

func (b *astbuilder) GetSpecialValueByStr(name string) ssa.Value {
	if b.specialValues[name] == nil {
		return nil
	}
	return b.specialValues[name]
}
