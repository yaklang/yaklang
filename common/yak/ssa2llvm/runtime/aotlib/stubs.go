package aotlib

// CorePluginStubExports is the compile-only export table for yaklib modules
// that core plugins reference but the AOT runtime does not execute. The
// coreplugin compile sweep (TestCorePlugin_CompileAll) builds every plugin to
// a native binary; those plugins use yaklib modules without AOT shims (ai,
// rag, syntaxflow, fuzz, risk, tls, ...). Registering a stub table keeps the
// monolithic common/yak/yaklib out of libyak.a while letting the sweep link.
//
// The table must be non-empty: runtimeRegisterYaklibModule ignores empty
// tables, and genfull only emits the per-module register symbol for modules
// with a pruned export source.
var CorePluginStubExports = map[string]any{
	"__aot_stub__": true,
}
