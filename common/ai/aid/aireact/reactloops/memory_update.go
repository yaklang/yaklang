package reactloops

import (
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/ytoken"
	"github.com/yaklang/yaklang/common/schema"

	"github.com/yaklang/yaklang/common/utils"
)

func (r *ReActLoop) currentMemorySize() int {
	var size = 0
	for _, i := range r.currentMemories.Values() {
		size += ytoken.CalcTokenCount(i.Content)
	}
	return size
}

func (r *ReActLoop) PushMemory(result *aicommon.SearchMemoryResult) {
	if utils.IsNil(result) {
		return
	}
	r.memoryUpdateMu.Lock()
	defer r.memoryUpdateMu.Unlock()
	mems := result.Memories
	for _, m := range mems {
		if m == nil {
			continue
		}
		if _, ok := r.currentMemories.Get(m.Id); ok {
			r.currentMemories.Delete(m.Id)
			r.currentMemories.Set(m.Id, m)
			continue
		}
		if e := r.GetEmitter(); e != nil {
			e.EmitJSON(schema.EVENT_TYPE_MEMORY_ADD_CONTEXT, "memory-triage", map[string]any{
				"memory": m,
			})
		}
		r.currentMemories.Set(m.Id, m)

		// Compute once per insertion rather than re-tokenizing every remaining
		// entry after each eviction. Keep it local as entity contents may change.
		size := r.currentMemorySize()
		for size > r.memorySizeLimit {
			// 删除最早的记忆
			var removed *aicommon.MemoryEntity
			removed = r.currentMemories.Shift()
			if utils.IsNil(removed) {
				continue
			}
			size -= ytoken.CalcTokenCount(removed.Content)
			if e := r.GetEmitter(); e != nil {
				r.GetEmitter().EmitJSON(schema.EVENT_TYPE_MEMORY_REMOVE_CONTEXT, "memory-triage", map[string]any{
					"reason": "memory size limit exceeded",
					"memory": removed,
				})
			}
		}
	}
}

func (r *ReActLoop) GetCurrentMemoriesContent() string {
	r.memoryUpdateMu.Lock()
	defer r.memoryUpdateMu.Unlock()
	if r.intentMemoryActive {
		return r.intentMemorySnapshot
	}
	if r.currentMemories == nil || r.currentMemories.Len() <= 0 {
		return ""
	}
	return aicommon.BuildPromptMemoriesMarkdownFromEntities(r.currentMemories.Values(), aicommon.MemoryIntentGeneric)
}
