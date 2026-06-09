//go:build ssa2llvm_pruned_runtime

package main

import "sync"

var (
	runtimeYaklibRegistryMu sync.RWMutex
	runtimeYaklibModules    = make(map[string]map[string]any)
	runtimeYaklibGlobals    = make(map[string]any)
)

func runtimeRegisterYaklibModule(name string, exports map[string]any) {
	if name == "" || len(exports) == 0 {
		return
	}
	runtimeYaklibRegistryMu.Lock()
	defer runtimeYaklibRegistryMu.Unlock()
	dst := runtimeYaklibModules[name]
	if dst == nil {
		dst = make(map[string]any, len(exports))
		runtimeYaklibModules[name] = dst
	}
	for key, value := range exports {
		if key == "" || value == nil {
			continue
		}
		dst[key] = value
	}
}

func runtimeRegisterYaklibGlobals(exports map[string]any) {
	if len(exports) == 0 {
		return
	}
	runtimeYaklibRegistryMu.Lock()
	defer runtimeYaklibRegistryMu.Unlock()
	for key, value := range exports {
		if key == "" || value == nil {
			continue
		}
		runtimeYaklibGlobals[key] = value
	}
}

func runtimeLookupYaklibCallable(pkg, method string) (any, bool) {
	runtimeYaklibRegistryMu.RLock()
	defer runtimeYaklibRegistryMu.RUnlock()
	if pkg == "" {
		fn, ok := runtimeYaklibGlobals[method]
		return fn, ok
	}
	exports := runtimeYaklibModules[pkg]
	if exports == nil {
		return nil, false
	}
	fn, ok := exports[method]
	return fn, ok
}
