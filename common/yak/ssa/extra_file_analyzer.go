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
	PreHandlerProject(fi.FileSystem, *FunctionBuilder, string) error
	PreHandlerFile(editor *memedit.MemEditor, builder *FunctionBuilder)
	AfterPreHandlerProject(builder *FunctionBuilder)
}

type Builder interface {
	// create a new builder
	Create() Builder

	Build(string, bool, *FunctionBuilder) error
	FilterFile(string) bool
	GetLanguage() consts.Language
	PreHandlerAnalyzer
}

type initHanlderFunc func(*FunctionBuilder)

type PreHandlerInit struct {
	InitHandlerOnce sync.Once
	initHandlerFunc []initHanlderFunc
}

func (d *PreHandlerInit) AfterPreHandlerProject(builder *FunctionBuilder) {
	return
}

func NewPreHandlerInit(fs ...initHanlderFunc) *PreHandlerInit {
	return &PreHandlerInit{
		InitHandlerOnce: sync.Once{},
		initHandlerFunc: fs,
	}
}

var ProjectConfigVariable = "__projectConfig__"

func (d *PreHandlerInit) InitHandler(b *FunctionBuilder) {
	d.InitHandlerOnce.Do(func() {
		// build the global dependency scope
		b.SetEmptyRange()
		variable := b.CreateVariable("__dependency__")
		container := b.EmitEmptyContainer()
		b.AssignVariable(variable, container)

		configVariable := b.CreateVariable(ProjectConfigVariable)
		configContainer := b.EmitEmptyContainer()
		b.AssignVariable(configVariable, configContainer)

		// run the init handler functions
		for _, f := range d.initHandlerFunc {
			// xx
			f(b)
		}
	})
}

func (d *PreHandlerInit) PreHandlerFile(editor *memedit.MemEditor, builder *FunctionBuilder) {
}

func (d *PreHandlerInit) FilterPreHandlerFile(string) bool {
	return false
}

func (d *PreHandlerInit) PreHandlerProject(fi.FileSystem, *FunctionBuilder, string) error {
	return nil
}
