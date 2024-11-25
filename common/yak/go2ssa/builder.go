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
		PreHandlerInit: ssa.NewPreHandlerInit(initHandler),
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

func (s *SSABuilder) PreHandlerFile(editor *memedit.MemEditor, builder *ssa.FunctionBuilder) {
	builder.GetProgram().GetApplication().Build("", editor, builder)
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
	file, err := fileSystem.ReadFile(path)
	if err != nil {
		log.Errorf("read file %s error: %v", path, err)
		return nil
	}

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
	prog.Build(path, memedit.NewMemEditor(string(file)), functionBuilder)
	prog.GetIncludeFiles()
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

func (b *astbuilder) SetImportPackage(useName, trueName string, path string, pos ssa.CanStartStopToken) {
	p := &PackageInfo{
		Name: trueName,
		Path: path,
		Pos:  pos,
	}
	b.importMap[useName] = p
}

func (b *astbuilder) GetImportPackage(name string) (*ssa.Program, string) {
	prog := b.GetProgram()
	if b.importMap[name] == nil {
		return nil, ""
	}
	lib, _ := prog.GetLibrary(b.importMap[name].Name)
	return lib, b.importMap[name].Path
}

func (b *astbuilder) GetLabelByName(name string) *ssa.LabelBuilder {
	if b.labels[name] == nil {
		return nil
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

func (b *astbuilder) GetStructAll() map[string]*ssa.ObjectType {
	var objs map[string]*ssa.ObjectType = make(map[string]*ssa.ObjectType)
	for s, o := range b.GetProgram().ExportType {
		if o, ok := o.(*ssa.ObjectType); ok {
			objs[s] = o
		}
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
