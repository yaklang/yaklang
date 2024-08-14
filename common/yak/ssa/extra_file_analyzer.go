package ssa

import (
	"github.com/yaklang/yaklang/common/consts"
	fi "github.com/yaklang/yaklang/common/utils/filesys/filesys_interface"
	"github.com/yaklang/yaklang/common/utils/memedit"
)

type PreHandlerAnalyzer interface {
	InitHandler(builder *FunctionBuilder)
	PreHandlerProject(fi.FileSystem, *FunctionBuilder, string) error
	PreHandlerFile(editor *memedit.MemEditor, builder *FunctionBuilder)
}

type Builder interface {
	Build(string, bool, *FunctionBuilder) error
	FilterFile(string) bool
	GetLanguage() consts.Language
	PreHandlerAnalyzer
}

var _ PreHandlerAnalyzer = &DummyPreHandler{}

type DummyPreHandler struct{}

func (d *DummyPreHandler) PreHandlerFile(editor *memedit.MemEditor, builder *FunctionBuilder) {
}

func (d *DummyPreHandler) PreHandlerProject(fi.FileSystem, *FunctionBuilder, string) error {
	return nil
}
func (d *DummyPreHandler) InitHandler(builder *FunctionBuilder) {}
