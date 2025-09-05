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
	"github.com/yaklang/yaklang/common/utils/memedit"
	"github.com/yaklang/yaklang/common/yak/antlr4util"
	"github.com/yaklang/yaklang/common/yak/ssa"

	gol "github.com/yaklang/yaklang/common/yak/antlr4go/parser"
)

type SSABuilder struct {
	*ssa.PreHandlerInit
}

var Builder = &SSABuilder{}

func (s *SSABuilder) Create() ssa.Builder {
	return &SSABuilder{
		PreHandlerInit: ssa.NewPreHandlerInit(initHandler).WithLanguageConfigOpts(
			ssa.WithLanguageConfigBind(true),
			ssa.WithLanguageConfigVirtualImport(true),
			ssa.WithLanguageBuilder(s),
		),
	}
}

func initHandler(fb *ssa.FunctionBuilder) {
	container := fb.EmitEmptyContainer()
	fb.GetProgram().GlobalScope = container
}

func (*SSABuilder) FilterPreHandlerFile(path string) bool {
	extension := filepath.Ext(path)
	return extension == ".go" || extension == ".mod"
}

func (s *SSABuilder) PreHandlerFile(ast ssa.FrontAST, editor *memedit.MemEditor, builder *ssa.FunctionBuilder) {
	builder.GetProgram().GetApplication().Build(ast, editor, builder)
}

func (s *SSABuilder) PreHandlerProject(fileSystem fi.FileSystem, ast ssa.FrontAST, functionBuilder *ssa.FunctionBuilder, editor *memedit.MemEditor) error {
	prog := functionBuilder.GetProgram()
	if prog == nil {
		log.Errorf("program is nil")
		return nil
	}
	if prog.ExtraFile == nil {
		prog.ExtraFile = make(map[string]string)
	}

	filename := editor.GetFilename()
	// go.mod
	if strings.TrimLeft(filename, string(fileSystem.GetSeparators())) == "go.mod" {
		text := editor.GetSourceCode()
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
	prog.Build(ast, editor, functionBuilder)
	prog.GetIncludeFiles()
	return nil
}

func (s *SSABuilder) ParseAST(src string) (ssa.FrontAST, error) {
	return Frontend(src)
}

func (s *SSABuilder) BuildFromAST(raw ssa.FrontAST, builder *ssa.FunctionBuilder) error {
	ast, ok := raw.(*gol.SourceFileContext)
	if !ok {
		return utils.Errorf("invalid AST type: expected *gol.SourceFileContext, got %T", raw)
	}

	SpecialTypes := map[string]ssa.Type{
		"comparable": ssa.CreateAnyType(),
		"error":      ssa.CreateErrorType(),
	}
	SpecialValue := map[string]interface{}{
		"nil":   nil,
		"iota":  "iota",
		"true":  true,
		"false": false,
	}

	builder.SupportClosure = false
	astBuilder := &astbuilder{
		FunctionBuilder: builder,
		cmap:            []map[string]struct{}{},
		importMap:       map[string]*PackageInfo{},
		result:          map[string][]string{},
		tpHandler:       map[string]func(){},
		labels:          map[string]*ssa.LabelBuilder{},
		specialValues:   SpecialValue,
		specialTypes:    SpecialTypes,
		pkgNameCurrent:  "",
	}
	// log.Infof("ast: %s", ast.ToStringTree(ast.GetParser().GetRuleNames(), ast.GetParser()))
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
	importMap      map[string]*PackageInfo
	result         map[string][]string
	tpHandler      map[string]func()
	labels         map[string]*ssa.LabelBuilder
	specialValues  map[string]interface{}
	specialTypes   map[string]ssa.Type
	pkgNameCurrent string
	SetGlobal      bool
}

func Frontend(src string) (*gol.SourceFileContext, error) {
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
	if len(errListener.GetErrors()) == 0 {
		return ast, nil
	}
	return ast, utils.Errorf("parse AST FrontEnd error : %v", errListener.GetErrorString())
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

func (b *astbuilder) AddGlobalVariable(name string, value ssa.Value) {
	scope := b.CurrentBlock.ScopeTable
	for _, v := range scope.GetAllVariables() {
		if object := v.GetValue().GetObject(); object != nil && object.GetId() == value.GetId() {
			variable := b.CreateMemberCallVariable(b.GetProgram().GlobalScope, b.EmitConstInstPlaceholder(v.GetName()))
			b.AssignVariable(variable, v.GetValue())
		}
	}
	variable := b.CreateMemberCallVariable(b.GetProgram().GlobalScope, b.EmitConstInstPlaceholder(name))
	b.AssignVariable(variable, value)
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
	return variable, false
}

func (b *astbuilder) GetGlobalVariableR(name string) ssa.Value {
	global := b.GetProgram().GlobalScope
	member, _ := global.GetStringMember(name)
	for i, m := range global.GetAllMember() {
		variable := b.CreateVariable(i.String())
		b.AssignVariable(variable, m)
	}
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

func (b *astbuilder) SetImportPackage(useName, trueName string, path string, pos ssa.CanStartStopToken) {
	p := &PackageInfo{
		Name: trueName,
		Path: path,
		Pos:  pos,
	}
	b.importMap[useName] = p
}

func (b *astbuilder) GetImportPackage(n string) (*ssa.Program, string) {
	prog := b.GetProgram()
	path := ""
	name := n

	if m, ok := b.importMap[n]; ok {
		path = m.Path
		name = m.Name
	}

	lib, _ := prog.GetOrCreateLibrary(name)
	return lib, path
}

func (b *astbuilder) GetImportPackageUser(n string) (*ssa.Program, string) {
	prog := b.GetProgram()
	path := ""
	name := n

	if m, ok := b.importMap[n]; ok {
		path = m.Path
		name = m.Name
	}

	lib, _ := prog.GetLibrary(name)
	return lib, path
}

func (b *astbuilder) GetLabelByName(name string) *ssa.LabelBuilder {
	if b.labels[name] == nil {
		b.labels[name] = b.BuildLabel(name)
	}

	return b.labels[name]
}

// ====================== Object type
func (b *astbuilder) AddStruct(name string, t ssa.Type) {
	b.GetProgram().SetExportType(name, t)
}

func (b *astbuilder) GetStructByStr(name string) ssa.Type {
	if t, ok := b.GetProgram().GetExportType(name); ok {
		return t
	}
	return nil
}

func (b *astbuilder) GetStructAll() map[string]ssa.Type {
	objs := make(map[string]ssa.Type)
	for s, o := range b.GetProgram().ExportType {
		objs[s] = o
	}

	return objs
}

// ====================== Alias type
func (b *astbuilder) AddAlias(name string, t *ssa.AliasType) {
	b.GetProgram().SetExportType(name, t)
}

func (b *astbuilder) DelAliasByStr(name string) {
	delete(b.GetProgram().ExportType, name)
}

func (b *astbuilder) GetAliasByStr(name string) ssa.Type {
	if t, ok := b.GetProgram().GetExportType(name); ok {
		if obj, ok := t.(*ssa.AliasType); ok {
			return obj
		}
	}
	return nil
}

func (b *astbuilder) GetAliasAll() map[string]*ssa.AliasType {
	var objs map[string]*ssa.AliasType = make(map[string]*ssa.AliasType)
	for s, o := range b.GetProgram().ExportType {
		if o, ok := o.(*ssa.AliasType); ok {
			objs[s] = o
		}
	}

	return objs
}

// ====================== Special
func (b *astbuilder) GetSpecialTypeByStr(name string) ssa.Type {
	if b.specialTypes[name] == nil {
		return nil
	}
	return b.specialTypes[name]
}

func (b *astbuilder) CheckSpecialValueByStr(name string) (interface{}, bool) {
	key := b.specialValues[name]
	_ = key
	if b.specialValues[name] == nil {
		return key, false
	}
	return key, true
}

type PackageInfo struct {
	Name string
	Path string
	Pos  ssa.CanStartStopToken
}

func (b *astbuilder) SwitchFunctionBuilder(s *ssa.StoredFunctionBuilder) func() {
	t := b.StoreFunctionBuilder()
	b.LoadBuilder(s)
	return func() {
		b.LoadBuilder(t)
	}
}

func (b *astbuilder) LoadBuilder(s *ssa.StoredFunctionBuilder) {
	b.FunctionBuilder = s.Current
	b.LoadFunctionBuilder(s.Store)
}
