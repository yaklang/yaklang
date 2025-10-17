package reactloops

import (
	"bytes"

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
	if !utils.IsNil(result) {
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
		r.currentMemories.Set(m.Id, m)

		for r.currentMemorySize() > r.memorySizeLimit {
			// 删除最早的记忆
			r.currentMemories.Shift()
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
