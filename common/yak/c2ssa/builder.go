package c2ssa

import (
	"fmt"
	"path/filepath"

	"github.com/antlr/antlr4/runtime/Go/antlr/v4"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	fi "github.com/yaklang/yaklang/common/utils/filesys/filesys_interface"
	"github.com/yaklang/yaklang/common/utils/memedit"
	"github.com/yaklang/yaklang/common/yak/antlr4util"
	"github.com/yaklang/yaklang/common/yak/ssa"

	cparser "github.com/yaklang/yaklang/common/yak/antlr4c/parser"
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
	return extension == ".c" || extension == ".h"
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
		"void":    ssa.CreateAnyType(),
		"bool":    ssa.CreateBooleanType(),
		"complex": ssa.CreateAnyType(),
	}
	SpecialValue := map[string]ssa.Value{}

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
	log.Infof("ast: %s", ast.ToStringTree(ast.GetParser().GetRuleNames(), ast.GetParser()))
	astBuilder.build(ast)
	fmt.Printf("Program: %v done\n", astBuilder.pkgNameCurrent)
	return nil
}

func (*SSABuilder) FilterFile(path string) bool {
	return filepath.Ext(path) == ".c"
}

func (*SSABuilder) GetLanguage() consts.Language {
	return consts.C
}

type astbuilder struct {
	*ssa.FunctionBuilder
	cmap           []map[string]struct{}
	importMap      map[string]*PackageInfo
	result         map[string][]string
	tpHandler      map[string]func()
	labels         map[string]*ssa.LabelBuilder
	specialValues  map[string]ssa.Value
	specialTypes   map[string]ssa.Type
	pkgNameCurrent string
	SetGlobal      bool
}

func Frontend(src string, must bool) (*cparser.CompilationUnitContext, error) {
	errListener := antlr4util.NewErrorListener()
	lexer := cparser.NewCLexer(antlr.NewInputStream(src))
	lexer.RemoveErrorListeners()
	lexer.AddErrorListener(errListener)
	tokenStream := antlr.NewCommonTokenStream(lexer, antlr.TokenDefaultChannel)
	parser := cparser.NewCParser(tokenStream)
	parser.RemoveErrorListeners()
	parser.AddErrorListener(errListener)
	parser.SetErrorHandler(antlr.NewDefaultErrorStrategy())
	ast := parser.CompilationUnit().(*cparser.CompilationUnitContext)
	if must || len(errListener.GetErrors()) == 0 {
		return ast, nil
	}
	return nil, utils.Errorf("parse AST FrontEnd error : %v", errListener.GetErrorString())
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

func (b *astbuilder) GetStructAll() map[string]ssa.Type {
	objs := make(map[string]ssa.Type)
	for s, o := range b.GetProgram().ExportType {
		objs[s] = o
	}
	return objs
}

func (b *astbuilder) GetAliasAll() map[string]*ssa.AliasType {
	objs := make(map[string]*ssa.AliasType)
	for s, o := range b.GetProgram().ExportType {
		if o, ok := o.(*ssa.AliasType); ok {
			objs[s] = o
		}
	}
	return objs
}

func (b *astbuilder) GetGlobalVariables() map[string]ssa.Value {
	variables := make(map[string]ssa.Value)
	for i, m := range b.GetProgram().GlobalScope.GetAllMember() {
		variables[i.String()] = m
	}
	return variables
}

func (b *astbuilder) GetDefaultValue(ityp ssa.Type) ssa.Value {
	switch ityp.GetTypeKind() {
	case ssa.NumberTypeKind:
		return b.EmitConstInst(0)
	case ssa.StringTypeKind:
		return b.EmitConstInst("")
	case ssa.BooleanTypeKind:
		return b.EmitConstInst(false)
	case ssa.FunctionTypeKind:
		return b.EmitUndefined("func")
	case ssa.AliasTypeKind:
		alias, _ := ssa.ToAliasType(ityp)
		return b.GetDefaultValue(alias.GetType())
	case ssa.StructTypeKind, ssa.ObjectTypeKind, ssa.InterfaceTypeKind, ssa.SliceTypeKind, ssa.MapTypeKind:
		return b.EmitMakeBuildWithType(ityp, nil, nil)
	default:
		return b.EmitConstInst(0)
	}
}

func (b *astbuilder) addSpecialValue(n string, v ssa.Value) {
	if _, ok := b.specialValues[n]; !ok {
		b.specialValues[n] = v
	}
}

func (b *astbuilder) getSpecialValue(n string) (ssa.Value, bool) {
	if v, ok := b.specialValues[n]; ok {
		return v, true
	}
	return nil, false
}

func (b *astbuilder) GetLabelByName(name string) *ssa.LabelBuilder {
	if b.labels[name] == nil {
		b.labels[name] = b.BuildLabel(name)
	}

	return b.labels[name]
}
