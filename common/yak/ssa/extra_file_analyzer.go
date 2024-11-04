package ssa

import (
	"sync"

	"github.com/yaklang/yaklang/common/consts"
	fi "github.com/yaklang/yaklang/common/utils/filesys/filesys_interface"
	"github.com/yaklang/yaklang/common/utils/memedit"
)

type PreHandlerAnalyzer interface {
	InitHandler(builder *FunctionBuilder)
	DoInitHandlerFunc(builder *FunctionBuilder)
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
	DoHandlerOnce   sync.Once
	InitHandlerFunc []func()
}

func (d *DummyPreHandler) PreHandlerFile(editor *memedit.MemEditor, builder *FunctionBuilder) {
}

func (d *DummyPreHandler) FilterPreHandlerFile(string) bool {
	return false
}

func (d *DummyPreHandler) PreHandlerProject(fi.FileSystem, *FunctionBuilder, string) error {
	return nil
}
func (d *DummyPreHandler) InitHandler(b *FunctionBuilder) {
}
func (d *DummyPreHandler) DoInitHandlerFunc(b *FunctionBuilder) {
	d.DoHandlerOnce.Do(func() {
		d.InitHandlerFunc = append(d.InitHandlerFunc, func() {
			b.SetEmptyRange()
			variable := b.CreateVariable("__dependency__")
			container := b.EmitEmptyContainer()
			b.AssignVariable(variable, container)
		})
		for _, f := range d.InitHandlerFunc {
			f()
		}
	})
}
