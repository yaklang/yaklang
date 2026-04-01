package obfuscation

import (
	_ "github.com/yaklang/yaklang/common/yak/ssa2llvm/obfuscation/builtin"
	"github.com/yaklang/yaklang/common/yak/ssa2llvm/obfuscation/core"
)

type Stage = core.Stage

const (
	StageSSAPre  = core.StageSSAPre
	StageSSAPost = core.StageSSAPost
	StageLLVM    = core.StageLLVM
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

func Apply(ctx *Context, names []string) error {
	return core.Apply(ctx, names)
}

func List() []Info {
	return core.List()
}

func ListByKind(kind Kind) []string {
	return core.ListByKind(kind)
}

func NormalizeNames(names []string) []string { return core.NormalizeNames(names) }
