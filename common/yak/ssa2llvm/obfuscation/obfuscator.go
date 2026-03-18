package obfuscation

import (
	"github.com/yaklang/go-llvm"
	"github.com/yaklang/yaklang/common/yak/ssa"
	_ "github.com/yaklang/yaklang/common/yak/ssa2llvm/obfuscation/builtin"
	"github.com/yaklang/yaklang/common/yak/ssa2llvm/obfuscation/core"
)

type Stage = core.Stage

const (
	StageSSA  = core.StageSSA
	StageLLVM = core.StageLLVM
)

type Kind = core.Kind

const (
	KindSSA    = core.KindSSA
	KindHybrid = core.KindHybrid
	KindLLVM   = core.KindLLVM
)

type Context = core.Context
type Obfuscator = core.Obfuscator
type Info = core.Info

var Default = core.Default

func Register(obfuscator Obfuscator) {
	core.Register(obfuscator)
}

func ApplySSA(program *ssa.Program, entryFunction string, names []string) error {
	return core.ApplySSA(&core.Context{
		SSA:           program,
		EntryFunction: entryFunction,
	}, names)
}

func ApplyLLVM(module llvm.Module, entryFunction string, names []string) error {
	return core.ApplyLLVM(&core.Context{
		LLVM:          module,
		EntryFunction: entryFunction,
	}, names)
}

func List() []Info {
	return core.List()
}

func ListByKind(kind Kind) []string {
	return core.ListByKind(kind)
}

func NormalizeNames(names []string) []string { return core.NormalizeNames(names) }
