package linkprep

import (
	"github.com/yaklang/yaklang/common/yak/ssa2llvm/runtime/abi"
)

// CanonicalRuntimeSymbols lists C-link-visible symbols the LLVM backend may
// reference into libyak / obf runtime archives.
func CanonicalRuntimeSymbols() []string {
	return []string{
		abi.InvokeSymbol,
		abi.MakeSliceSymbol,
		"yak_internal_print_int",
		"yak_internal_malloc",
		"yak_runtime_to_cstring",
		"yak_host_release_handle",
		"yak_internal_release_shadow",
		"yak_runtime_new_shadow",
		"yak_runtime_get_field",
		"yak_runtime_set_field",
		"yak_runtime_dump",
		"yak_runtime_dump_handle",
		"yak_runtime_gc",
		"yak_runtime_wait_async",
		"yak_runtime_load_panic_value",
		"yak_runtime_invoke_vm",
		"yak_runtime_test_add1_ctx",
	}
}
