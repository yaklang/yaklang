package virtualize

import "github.com/yaklang/yaklang/common/yak/ssa2llvm/obfuscation/core"

func (virtualizeObfuscator) RuntimeDeps() []core.RuntimeDep {
	return []core.RuntimeDep{
		{
			ObfName:        obfName,
			ArchiveName:    "virtualize",
			Symbols:        []string{"yak_runtime_invoke_vm"},
			FallbackToMain: true,
		},
	}
}
