package sfvm

import (
	"runtime"
	"sync"

	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
)

// CompileRules compiles independent texts with bounded parallelism. Results
// retain input order, including errors; one invalid rule does not stop others.
// Every worker uses its own VM (a VM's parser and frame list are mutable).
func CompileRules(texts []string) ([]*schema.SyntaxFlowRule, []error) {
	rules := make([]*schema.SyntaxFlowRule, len(texts))
	errs := make([]error, len(texts))
	workers := min(8, runtime.GOMAXPROCS(0), len(texts))
	var next int
	var mu sync.Mutex
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				mu.Lock()
				index := next
				next++
				mu.Unlock()
				if index >= len(texts) {
					return
				}
				frame, err := CompileRule(texts[index])
				errs[index] = err
				if err == nil {
					rules[index] = frame.GetRule()
				}
			}
		}()
	}
	wg.Wait()
	return rules, errs
}

// RuleLoader is a scan-local cache of immutable decoded instructions, not of
// execution frames/results. Each query/unit still gets a fresh VM and frame.
// Rules are owned by this scan and must not be mutated concurrently by callers.
// The map is a SafeMap; each entry still has its own lock so different rules
// compile in parallel while one rule compiles only once.
type RuleLoader struct {
	once    sync.Once
	entries *utils.SafeMapWithKey[*schema.SyntaxFlowRule, *loadedRule]
}

func (loader *RuleLoader) cache() *utils.SafeMapWithKey[*schema.SyntaxFlowRule, *loadedRule] {
	loader.once.Do(func() {
		loader.entries = utils.NewSafeMapWithKey[*schema.SyntaxFlowRule, *loadedRule]()
	})
	return loader.entries
}

type loadedRule struct {
	mu                     sync.Mutex
	ready                  bool
	content, opcode, build string
	codes                  []*SFI
	err                    error
}

func (loader *RuleLoader) Load(vm *SyntaxFlowVirtualMachine, rule *schema.SyntaxFlowRule) (*SFFrame, bool, error) {
	if rule == nil {
		return nil, false, utils.Error("cannot load a nil SyntaxFlow rule")
	}
	if loader == nil {
		return vm.Load(rule)
	}
	entry, loadErr := loader.cache().GetOrLoad(rule, func() (*loadedRule, error) {
		return &loadedRule{}, nil
	})
	if loadErr != nil {
		return nil, false, loadErr
	}
	entry.mu.Lock()
	defer entry.mu.Unlock()
	build := consts.GetYakVersion()
	if entry.ready && entry.content == rule.Content && entry.opcode == rule.OpCodes && entry.build == build {
		if entry.err != nil {
			return nil, false, entry.err
		}
		frame := newSfFrameEx(vm.vars, rule.Content, entry.codes, rule, vm.config)
		frame.vm = vm
		vm.frames = append(vm.frames, frame)
		return frame, false, nil
	}
	// If the source was replaced without replacing its derived opcodes, the
	// previous program is no longer a valid cache entry even on the same build.
	if entry.ready && entry.content != rule.Content && entry.opcode == rule.OpCodes {
		rule.OpCodes = ""
	}
	frame, resave, err := vm.Load(rule)
	entry.ready = true
	entry.content, entry.opcode, entry.build = rule.Content, rule.OpCodes, build
	entry.err = err
	if err == nil {
		entry.codes = frame.Codes
	}
	return frame, resave, err
}
