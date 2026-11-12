package ssa2source

import (
	"github.com/yaklang/yaklang/common/yak/reify/core/ssa2source/pass"
	"github.com/yaklang/yaklang/common/yak/ssa"
)

// ApplyPasses runs selected SSA preparation passes before source rendering.
func ApplyPasses(program *ssa.Program, names ...string) error {
	return pass.ApplySelected(program, names...)
}

// Passes lists registered SSA preparation passes.
func Passes() []pass.Pass { return pass.All() }
