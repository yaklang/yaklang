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
}

type Builder interface {
	// create a new builder
	Create() Builder

	Build(string, bool, *FunctionBuilder) error
	FilterFile(string) bool
	GetLanguage() consts.Language
	PreHandlerAnalyzer
}

var _ PreHandlerAnalyzer = &DummyPreHandler{}

type DummyPreHandler struct {
	InitHandlerOnce sync.Once
}

func (d *DummyPreHandler) PreHandlerFile(editor *memedit.MemEditor, builder *FunctionBuilder) {
}

func (d *DummyPreHandler) FilterPreHandlerFile(string) bool {
	return false
}

func (d *DummyPreHandler) PreHandlerProject(fi.FileSystem, *FunctionBuilder, string) error {
	return nil
}
func (d *DummyPreHandler) InitHandler(builder *FunctionBuilder) {
	container := builder.EmitEmptyContainer()
	variable := builder.CreateMemberCallVariable(container, builder.EmitConstInst("$staticScope$"))
	emptyContainer := builder.EmitEmptyContainer()
	builder.AssignVariable(variable, emptyContainer)
	builder.AssignVariable(builder.CreateVariable("global-container"), container)
	builder.GetProgram().GlobalScope = container
}
