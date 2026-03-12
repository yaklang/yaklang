package obfuscation

import (
	"github.com/yaklang/go-llvm"
	"github.com/yaklang/yaklang/common/yak/ssa"
	_ "github.com/yaklang/yaklang/common/yak/ssa2llvm/obfuscation/builtin"
	"github.com/yaklang/yaklang/common/yak/ssa2llvm/obfuscation/core"
)

type SSAObfuscator = core.SSAObfuscator

type LLVMObfuscator = core.LLVMObfuscator

var Default = core.Default

func RegisterSSA(obfuscator SSAObfuscator) {
	core.RegisterSSA(obfuscator)
}

func RegisterLLVM(obfuscator LLVMObfuscator) {
	core.RegisterLLVM(obfuscator)
}

func ApplySSA(program *ssa.Program, names []string) error {
	return core.ApplySSA(program, names)
}

func ApplyLLVM(module llvm.Module, names []string) error {
	return core.ApplyLLVM(module, names)
}

func ListSSA() []string {
	return core.ListSSA()
}

func ListLLVM() []string {
	return core.ListLLVM()
}

func NormalizeNames(names []string) []string {
	return core.NormalizeNames(names)
}
