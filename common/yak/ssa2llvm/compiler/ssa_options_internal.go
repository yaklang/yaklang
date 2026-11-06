package compiler

import (
	"os"
	"reflect"

	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
	"github.com/yaklang/yaklang/common/yak/yaklang"

	// Register yaklang stdlib modules (ssa, syntaxflow, ...) for SSA extern resolution.
	_ "github.com/yaklang/yaklang/common/yak"
)

// yakCompileSSAOptions mirrors static_analyzer YakGetTypeSSAOpt extern wiring so
// ssa2llvm SSA parse sees the same import tables as the Yak interpreter.
func yakCompileSSAOptions() []ssaconfig.Option {
	opts := make([]ssaconfig.Option, 0)
	symbol := yaklang.New().GetFntable()
	valueTable := make(map[string]any)
	mapType := reflect.TypeOf(map[string]any{})
	for name, item := range symbol {
		if reflect.TypeOf(item) == mapType {
			opts = append(opts, ssaapi.WithExternLib(name, item.(map[string]any)))
			continue
		}
		valueTable[name] = item
	}
	valueTable["__yak_chan_send"] = func(ch any, v any) {}
	// Inject well-known runner globals from the environment: yak.Execute
	// receives them as vars (SetVars), and scripts reference them directly
	// (e.g. f`${VULINBOX}/gitserver/...`). Exporting them here keeps the
	// AOT compile/run flow equivalent for mustpass-style runners.
	if v := os.Getenv("VULINBOX"); v != "" {
		valueTable["VULINBOX"] = v
	}
	if v := os.Getenv("VULINBOX_HOST"); v != "" {
		valueTable["VULINBOX_HOST"] = v
	}
	if len(valueTable) > 0 {
		opts = append(opts, ssaapi.WithExternValue(valueTable))
	}
	return opts
}
