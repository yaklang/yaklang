package pass

import (
	"github.com/yaklang/yaklang/common/yak/ssa"
)

// program pass
type Pass interface {
	Run(*ssa.Program)
}

var (
	pass = make([]Pass, 0)
)

func RegisterPass(p Pass) {
	pass = append(pass, p)
}

func GetPass() []Pass {
	return pass
}

// function pass
type FunctionPass interface {
	RunOnFunction(*ssa.Function)
}

var (
	funcPass = make([]FunctionPass, 0)
)

func RegisterFunctionPass(f FunctionPass) {
	funcPass = append(funcPass, f)
}

// all function pass merge to one program pass
type functionPass struct {
}

func init() {
	RegisterPass(&functionPass{})
}

func (fun *functionPass) Run(prog *ssa.Program) {
	for _, pkg := range prog.Packages {
		for _, f := range pkg.Funcs {
			for _, fp := range funcPass {
				fp.RunOnFunction(f)
			}
		}
	}
}
