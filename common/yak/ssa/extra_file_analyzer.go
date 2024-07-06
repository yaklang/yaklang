package ssa

import "github.com/yaklang/yaklang/common/utils/filesys"

type ExtraFileAnalyzer interface {
	EnableExtraFileAnalyzer() bool
	ExtraFileAnalyze(filesys.FileSystem, string) error
}

type Builder interface {
	Build(string, bool, *FunctionBuilder) error
	FilterFile(string) bool

	ExtraFileAnalyzer
}

var _ ExtraFileAnalyzer = &DummyExtraFileAnalyzer{}

type DummyExtraFileAnalyzer struct {
}

func (d *DummyExtraFileAnalyzer) EnableExtraFileAnalyzer() bool {
	return false
}

func (d *DummyExtraFileAnalyzer) ExtraFileAnalyze(filesys.FileSystem, string) error {
	return nil
}
