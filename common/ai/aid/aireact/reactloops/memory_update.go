package reactloops

import (
	"bytes"
	"github.com/yaklang/yaklang/common/schema"

	"github.com/yaklang/yaklang/common/ai/aid/aimem"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

func (r *ReActLoop) currentMemorySize() int {
	var size = 0
	for _, i := range r.currentMemories.Values() {
		size += len(i.Content)
	}
	return size
}

func (r *ReActLoop) PushMemory(result *aimem.SearchMemoryResult) {
	if utils.IsNil(result) {
		return
	}
	mems := result.Memories
	for _, m := range mems {
		log.Infof("start to handle memory content bytes: %v", utils.ShrinkString(m.Content, 256))
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

		for r.currentMemorySize() > r.memorySizeLimit {
			// 删除最早的记忆
			var removed *aimem.MemoryEntity
			removed = r.currentMemories.Shift()
			if utils.IsNil(removed) {
				continue
			}
			if e := r.GetEmitter(); e != nil {
				r.GetEmitter().EmitJSON(schema.EVENT_TYPE_MEMORY_REMOVED_CONTEXT, "memory-triage", map[string]any{
					"reason": "memory size limit exceeded",
					"memory": removed,
				})
			}
		}
	}
}

func (r *ReActLoop) GetCurrentMemoriesContent() string {
	if r.currentMemories.Len() <= 0 {
		return ""
	}

	var buf bytes.Buffer
	for _, v := range r.currentMemories.Values() {
		buf.WriteString(v.Content)
		buf.WriteString("\n")
	}
	return buf.String()
}
