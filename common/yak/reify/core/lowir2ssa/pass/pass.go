package pass

import (
	"fmt"
	"sync"

	"github.com/yaklang/yaklang/common/yak/reify/core"
)

// Pass is one selectable LowIR transformation. A pass receives the frame model of
// the target architecture, so it can name the stack and frame registers without
// matching architecture-specific spellings.
type Pass struct {
	Name        string
	Description string
	Apply       func(*core.Function, core.ArchFacts) error
}

var (
	mu     sync.RWMutex
	byName = make(map[string]Pass)
	order  = make([]string, 0)
)

// Register makes a pass selectable by name. Registration is explicit and
// idempotent for identical names.
func Register(pass Pass) {
	if pass.Name == "" || pass.Apply == nil {
		return
	}
	mu.Lock()
	defer mu.Unlock()
	if _, exists := byName[pass.Name]; exists {
		return
	}
	byName[pass.Name] = pass
	order = append(order, pass.Name)
}

// Get returns a registered pass.
func Get(name string) (Pass, bool) {
	mu.RLock()
	defer mu.RUnlock()
	pass, ok := byName[name]
	return pass, ok
}

// All returns registered passes in registration order.
func All() []Pass {
	mu.RLock()
	defer mu.RUnlock()
	result := make([]Pass, 0, len(order))
	for _, name := range order {
		result = append(result, byName[name])
	}
	return result
}

// Apply runs the named passes in the given order.
func Apply(function *core.Function, facts core.ArchFacts, names ...string) error {
	for _, name := range names {
		pass, ok := Get(name)
		if !ok {
			return fmt.Errorf("unknown LowIR pass %q", name)
		}
		if err := pass.Apply(function, facts); err != nil {
			return fmt.Errorf("LowIR pass %s: %w", name, err)
		}
	}
	return nil
}

func init() {
	Register(Pass{Name: "remove-canary", Description: "remove stack-protector canary checks", Apply: func(function *core.Function, facts core.ArchFacts) error {
		RemoveStackCanary(function, facts)
		return nil
	}})
	Register(Pass{Name: "fold-constants", Description: "fold arithmetic on constants", Apply: func(function *core.Function, _ core.ArchFacts) error {
		FoldConstants(function)
		return nil
	}})
	Register(Pass{Name: "duplicate-return-blocks", Description: "duplicate shared return epilogues", Apply: func(function *core.Function, _ core.ArchFacts) error {
		DuplicateReturnBlocks(function)
		return nil
	}})
	Register(Pass{Name: "remove-stackframe", Description: "remove standard stack frame setup and teardown", Apply: func(function *core.Function, facts core.ArchFacts) error {
		RemoveFrame(function, facts)
		return nil
	}})
	Register(Pass{Name: "fold-return-tails", Description: "fold acyclic return-only tails", Apply: func(function *core.Function, _ core.ArchFacts) error {
		FoldReturnTails(function)
		return nil
	}})
	Register(Pass{Name: "drop-unreachable-blocks", Description: "remove blocks no path reaches", Apply: func(function *core.Function, _ core.ArchFacts) error {
		DropUnreachable(function)
		return nil
	}})
	Register(Pass{Name: "reverse-postorder", Description: "order blocks for loop construction", Apply: func(function *core.Function, _ core.ArchFacts) error {
		ReorderReversePostorder(function)
		return nil
	}})
}

// DefaultNames returns the default LowIR pass pipeline.
func DefaultNames() []string {
	return []string{"fold-constants", "remove-canary", "duplicate-return-blocks", "remove-stackframe", "fold-return-tails", "drop-unreachable-blocks", "reverse-postorder"}
}
