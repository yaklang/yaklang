package pass

import (
	"fmt"
	"sync"

	"github.com/yaklang/yaklang/common/yak/ssa"
)

// Pass is one selectable SSA transformation.
type Pass struct {
	Name        string
	Description string
	Apply       func(*ssa.Program) error
}

var (
	registryMu sync.RWMutex
	registered = make(map[string]Pass)
	passOrder  = make([]string, 0)
)

// Register makes an SSA pass selectable by name.
func Register(pass Pass) {
	if pass.Name == "" || pass.Apply == nil {
		return
	}
	registryMu.Lock()
	defer registryMu.Unlock()
	if _, exists := registered[pass.Name]; exists {
		return
	}
	registered[pass.Name] = pass
	passOrder = append(passOrder, pass.Name)
}

// All returns registered SSA passes in registration order.
func All() []Pass {
	registryMu.RLock()
	defer registryMu.RUnlock()
	result := make([]Pass, 0, len(passOrder))
	for _, name := range passOrder {
		result = append(result, registered[name])
	}
	return result
}

// ApplySelected runs named SSA passes.
func ApplySelected(program *ssa.Program, names ...string) error {
	return ApplySelectedWith(program, nil, names...)
}

// ApplySelectedWith runs named SSA passes and supplies the string lookup used
// by the type-redefinition pass.
func ApplySelectedWith(program *ssa.Program, stringAt func(uint64) (string, bool), names ...string) error {
	for _, name := range names {
		if name == "redefine-type-value" {
			var options []Option
			if stringAt != nil {
				options = append(options, WithStringAt(stringAt))
			}
			RedefineTypeValue(program, options...)
			continue
		}
		pass, ok := registered[name]
		if !ok {
			return fmt.Errorf("unknown SSA pass %q", name)
		}
		if err := pass.Apply(program); err != nil {
			return fmt.Errorf("SSA pass %s: %w", name, err)
		}
	}
	return nil
}

func init() {
	Register(Pass{Name: "apply-function-define", Description: "apply known C function signatures", Apply: func(program *ssa.Program) error {
		ApplyFunctionDefine(program)
		return nil
	}})
	Register(Pass{Name: "infer-types", Description: "infer pointer and return types", Apply: func(program *ssa.Program) error {
		InferTypes(program)
		return nil
	}})
	Register(Pass{Name: "trim-internal-calls", Description: "trim calls to recovered functions", Apply: func(program *ssa.Program) error {
		TrimInternalCalls(program)
		return nil
	}})
	Register(Pass{Name: "mark-void-returns", Description: "mark unused internal returns as void", Apply: func(program *ssa.Program) error {
		MarkVoidReturns(program)
		return nil
	}})
	Register(Pass{Name: "refine-return-types", Description: "publish return types from SSA returns", Apply: func(program *ssa.Program) error {
		RefineReturnTypes(program)
		return nil
	}})
	Register(Pass{Name: "simplify-partial-writes", Description: "fold partial-register merges", Apply: func(program *ssa.Program) error {
		SimplifyPartialWrites(program)
		return nil
	}})
	Register(Pass{Name: "redefine-type-value", Description: "replace typed values and string arguments", Apply: func(program *ssa.Program) error {
		RedefineTypeValue(program)
		return nil
	}})
}
