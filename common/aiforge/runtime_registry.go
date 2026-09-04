package aiforge

import (
	"context"
	"strings"
	"sync"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

// RuntimeForgeRegistry stores Forge executors that depend on one runtime
// instance, such as a specific gRPC server and its browser-extension bridge.
// It intentionally does not mutate the process-wide built-in Forge registry.
type RuntimeForgeRegistry struct {
	mu      sync.RWMutex
	entries map[string]runtimeForgeEntry
}

type runtimeForgeEntry struct {
	executor      ForgeExecutor
	reActPreparer RuntimeForgeReActPreparer
}

func NewRuntimeForgeRegistry() *RuntimeForgeRegistry {
	return &RuntimeForgeRegistry{
		entries: make(map[string]runtimeForgeEntry),
	}
}

func (r *RuntimeForgeRegistry) Register(name string, executor ForgeExecutor) error {
	return r.register(name, runtimeForgeEntry{executor: executor})
}

func (r *RuntimeForgeRegistry) RegisterWithReAct(
	name string,
	executor ForgeExecutor,
	reActPreparer RuntimeForgeReActPreparer,
) error {
	if reActPreparer == nil {
		return utils.Errorf("runtime forge %s has no ReAct preparer", strings.TrimSpace(name))
	}
	return r.register(name, runtimeForgeEntry{
		executor:      executor,
		reActPreparer: reActPreparer,
	})
}

func (r *RuntimeForgeRegistry) register(name string, entry runtimeForgeEntry) error {
	if r == nil {
		return utils.Error("runtime forge registry is nil")
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return utils.Error("runtime forge name is empty")
	}
	if entry.executor == nil {
		return utils.Errorf("runtime forge %s has no executor", name)
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.entries[name]; exists {
		return utils.Errorf("runtime forge %s already registered", name)
	}
	r.entries[name] = entry
	return nil
}

func (r *RuntimeForgeRegistry) Execute(
	name string,
	ctx context.Context,
	params []*ypb.ExecParamItem,
	opts ...aicommon.ConfigOption,
) (*ForgeResult, bool, error) {
	if r == nil {
		return nil, false, nil
	}

	r.mu.RLock()
	entry, exists := r.entries[strings.TrimSpace(name)]
	r.mu.RUnlock()
	if !exists {
		return nil, false, nil
	}

	result, err := entry.executor(ctx, params, opts...)
	return result, true, err
}

func (r *RuntimeForgeRegistry) PrepareReAct(
	name string,
	ctx context.Context,
	params []*ypb.ExecParamItem,
) (*RuntimeForgeReActPreparation, bool, error) {
	if r == nil {
		return nil, false, nil
	}

	trimmedName := strings.TrimSpace(name)
	r.mu.RLock()
	entry, exists := r.entries[trimmedName]
	r.mu.RUnlock()
	if !exists {
		return nil, false, nil
	}
	if entry.reActPreparer == nil {
		return nil, true, utils.Errorf(
			"runtime forge %s does not support the ReAct transport",
			trimmedName,
		)
	}

	preparation, err := entry.reActPreparer(ctx, params)
	if err != nil {
		return nil, true, err
	}
	if preparation == nil {
		return nil, true, utils.Errorf(
			"runtime forge %s returned an empty ReAct preparation",
			trimmedName,
		)
	}
	return preparation, true, nil
}
