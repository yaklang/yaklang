package pack

import "github.com/yaklang/yaklang/common/yak/ssa2llvm/llvminterop/plugin"

var builtinRegistry = func() *Registry {
	reg := NewRegistry()
	_ = reg.Register(&Manifest{
		Name:           "instcombine-simplifycfg",
		Description:    "Run opt with instcombine + simplifycfg as a curated builtin pack",
		LLVMVersionMin: 13,
		Plugins: []plugin.Descriptor{
			{
				Name: "opt-instcombine-simplifycfg",
				Kind: plugin.KindTool,
				Path: "opt",
				Args: []string{"-S", "-passes=instcombine,simplifycfg"},
			},
		},
		KnownLimitations: []string{
			"Requires opt in PATH",
			"Uses the tool adapter path rather than a loadable pass plugin",
		},
	})
	return reg
}()

// Builtins returns the shared curated builtin pack registry.
func Builtins() *Registry {
	return builtinRegistry
}

// LookupBuiltin finds a builtin pack by name.
func LookupBuiltin(name string) (*Manifest, bool) {
	return builtinRegistry.Get(name)
}
