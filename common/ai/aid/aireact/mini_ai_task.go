package aireact

import (
	"context"
	"sync"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
)

// MiniTaskDescriptor describes a registered mini AI task for frontend discovery.
type MiniTaskDescriptor struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
}

// MiniAITaskContext 是每个 mini AI handler 拿到的运行时上下文。
// handler 通过它访问 ReAct 的完整能力，核心是 ReAct.InvokeSpeedPriorityLiteForge。
type MiniAITaskContext struct {
	// ReAct 是完整的 ReAct 运行时实例，handler 通过它调用 InvokeSpeedPriorityLiteForge 等。
	ReAct *ReAct
	// Config 提供 GetTimeline / GetContext / GetEmitter 等基础能力。
	Config *aicommon.Config
	// Timeline 是 Config.GetTimeline() 的便捷引用，handler 可直接操作时间线。
	Timeline *aicommon.Timeline
	// Task 是当前正在执行的任务（可能为 nil）。
	Task aicommon.AIStatefulTask
	// SyncID 是本次 sync 事件的 ID，用于 EmitSyncJSON 回传响应。
	SyncID string
}

// MiniAITaskHandler 是一个可插拔的 mini AI 操作处理器。
// params 是从 SyncJsonInput 解析出来的参数 map（已移除 task_name）。
// 返回的 result 会被 JSON 序列化后通过 EmitSyncJSON 回传。
type MiniAITaskHandler func(ctx context.Context, taskCtx *MiniAITaskContext, params map[string]any) (result any, err error)

// MiniAITaskRegistry 管理已注册的 mini AI handler。
type MiniAITaskRegistry struct {
	mu       sync.RWMutex
	handlers map[string]MiniAITaskHandler
}

// NewMiniAITaskRegistry 创建一个新的 mini AI task 注册表。
func NewMiniAITaskRegistry() *MiniAITaskRegistry {
	return &MiniAITaskRegistry{
		handlers: make(map[string]MiniAITaskHandler),
	}
}

// Register 注册一个 mini AI handler。
func (r *MiniAITaskRegistry) Register(name string, handler MiniAITaskHandler) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.handlers == nil {
		r.handlers = make(map[string]MiniAITaskHandler)
	}
	r.handlers[name] = handler
}

// Unregister 注销一个 mini AI handler。
func (r *MiniAITaskRegistry) Unregister(name string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.handlers, name)
}

// Get 查找已注册的 handler。
func (r *MiniAITaskRegistry) Get(name string) (MiniAITaskHandler, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	h, ok := r.handlers[name]
	return h, ok
}

// Names 返回所有已注册的 handler 名称。
func (r *MiniAITaskRegistry) Names() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	names := make([]string, 0, len(r.handlers))
	for name := range r.handlers {
		names = append(names, name)
	}
	return names
}

// Descriptors returns MiniTaskDescriptor entries for all registered handlers.
func (r *MiniAITaskRegistry) Descriptors() []MiniTaskDescriptor {
	r.mu.RLock()
	defer r.mu.RUnlock()
	result := make([]MiniTaskDescriptor, 0, len(r.handlers))
	for name := range r.handlers {
		desc, ok := builtinMiniTaskDescriptions[name]
		if !ok {
			desc = name
		}
		result = append(result, MiniTaskDescriptor{
			Name:        name,
			Description: desc,
		})
	}
	return result
}

// RegisterMiniAITask 在 ReAct 上注册一个 mini AI handler。
func (r *ReAct) RegisterMiniAITask(name string, handler MiniAITaskHandler) {
	if r.miniAITaskRegistry == nil {
		r.miniAITaskRegistry = NewMiniAITaskRegistry()
	}
	r.miniAITaskRegistry.Register(name, handler)
}

// UnregisterMiniAITask 在 ReAct 上注销一个 mini AI handler。
func (r *ReAct) UnregisterMiniAITask(name string) {
	if r.miniAITaskRegistry == nil {
		return
	}
	r.miniAITaskRegistry.Unregister(name)
}
