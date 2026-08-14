// genfull generates a runtime_imports_generated.go at the path given as its
// first argument. By default it emits a blanket-registering init() for the given
// yaklib modules (or "all"). With --permodule it emits an empty init() plus one
// C-exported yak_register_module_<m>() per module (the C' approach: the compiler
// calls only the used modules' register functions, so lld --gc-sections can drop
// the unused ones from a single full libyak.a). With --aot the per-module
// export tables come from PrunedShim (lightweight AOT tables) instead of the
// monolithic yaklib package.
package main

import (
	"fmt"
	"os"

	embed "github.com/yaklang/yaklang/common/yak/ssa2llvm/runtime/embed"
)

func main() {
	args := os.Args[1:]
	perModule := false
	aot := false
	if len(args) > 0 && args[0] == "--permodule" {
		perModule = true
		args = args[1:]
	}
	if len(args) > 0 && args[0] == "--aot" {
		aot = true
		args = args[1:]
	}
	if len(args) < 1 {
		fmt.Fprintln(os.Stderr, "usage: genfull [--permodule] [--aot] <output-path> [module... | all]")
		os.Exit(2)
	}
	outputPath := args[0]
	modules := args[1:]
	if len(modules) == 0 {
		modules = []string{"all"}
	}

	var err error
	if perModule {
		err = embed.WriteRuntimeImportsPerModule(outputPath, modules, aot)
	} else {
		err = embed.WriteRuntimeImports(outputPath, modules)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "genfull:", err)
		os.Exit(1)
	}
	mode := "init"
	if perModule {
		mode = "per-module-export"
	}
	fmt.Printf("genfull: wrote %s (mode=%s, %d module(s))\n", outputPath, mode, len(modules))
}
