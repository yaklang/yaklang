package ssa

import (
	"sync"

	"github.com/yaklang/yaklang/common/consts"
	fi "github.com/yaklang/yaklang/common/utils/filesys/filesys_interface"
	"github.com/yaklang/yaklang/common/utils/memedit"
)

type PreHandlerAnalyzer interface {
	InitHandler(builder *FunctionBuilder)
	FilterPreHandlerFile(string) bool

	ParseAST(string) (FrontAST, error)
	PreHandlerProject(fi.FileSystem, FrontAST, *FunctionBuilder, *memedit.MemEditor) error
	PreHandlerFile(FrontAST, *memedit.MemEditor, *FunctionBuilder)

	AfterPreHandlerProject(builder *FunctionBuilder)
}

type FrontAST interface {
}

type CreateBuilder func() Builder

type Builder interface {
	// create a new builder
	BuildFromAST(FrontAST, *FunctionBuilder) error
	FilterFile(string) bool
	GetLanguage() consts.Language
	PreHandlerAnalyzer
}

type initHanlderFunc func(*FunctionBuilder)

type PreHandlerBase struct {
	InitHandlerOnce       sync.Once
	initHandlerFunc       []initHanlderFunc
	beforeInitHandlerFunc []initHanlderFunc
	languageConfigOpts    []LanguageConfigOpt
}

func (d *PreHandlerBase) AfterPreHandlerProject(builder *FunctionBuilder) {
	d.InitHandler(builder)
}

func NewPreHandlerBase(fs ...initHanlderFunc) *PreHandlerBase {
	return &PreHandlerBase{
		InitHandlerOnce: sync.Once{},
		initHandlerFunc: fs,
	}
}

func (d *PreHandlerBase) WithLanguageConfigOpts(opts ...LanguageConfigOpt) *PreHandlerBase {
	d.languageConfigOpts = opts
	return d
}
func (d *PreHandlerBase) WithPreInitHandler(fs ...initHanlderFunc) *PreHandlerBase {
	d.beforeInitHandlerFunc = fs
	return d
}

var ProjectConfigVariable = "__projectConfig__"

func (d *PreHandlerBase) InitHandler(b *FunctionBuilder) {
	d.InitHandlerOnce.Do(func() {
		// build the global dependency scope
		b.SetEmptyRange()
		b.SetLanguageConfig(d.languageConfigOpts...)
		for _, handlerFunc := range d.beforeInitHandlerFunc {
			handlerFunc(b)
		}
		variable := b.CreateVariable("__dependency__")
		container := b.EmitEmptyContainer()
		b.AssignVariable(variable, container)

		configVariable := b.CreateVariable(ProjectConfigVariable)
		configContainer := b.EmitEmptyContainer()
		b.AssignVariable(configVariable, configContainer)
		// run the init handler functions
		for _, f := range d.initHandlerFunc {
			f(b)
		}
	})
}

func (d *PreHandlerBase) PreHandlerFile(ast FrontAST, editor *memedit.MemEditor, builder *FunctionBuilder) {
}

func (d *PreHandlerBase) FilterPreHandlerFile(string) bool {
	return false
}

func (d *PreHandlerBase) PreHandlerProject(fi.FileSystem, FrontAST, *FunctionBuilder, *memedit.MemEditor) error {
	return nil
}
