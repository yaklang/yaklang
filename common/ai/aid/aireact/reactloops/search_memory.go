package reactloops

import (
	"context"
	"strings"
	"time"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
)

// refreshFastMemoryAsync never delays an iteration and allows only one quick
// search in flight per loop, even when the backend is still initializing.
func (r *ReActLoop) refreshFastMemoryAsync(task aicommon.AIStatefulTask) {
	if utils.IsNil(r.memoryTriage) || task.GetContext().Err() != nil {
		return
	}
	r.fastMemorySearchMu.Lock()
	if r.fastMemorySearchInFlight {
		r.fastMemorySearchMu.Unlock()
		return
	}
	r.fastMemorySearchInFlight = true
	r.fastMemorySearchMu.Unlock()
	input, taskID, ctx := task.GetUserInput(), task.GetId(), task.GetContext()
	go func() {
		defer func() {
			r.fastMemorySearchMu.Lock()
			r.fastMemorySearchInFlight = false
			r.fastMemorySearchMu.Unlock()
		}()
		r.fastLoadSearchMemoryWithoutAI(ctx, input, taskID)
	}()
}

func (r *ReActLoop) fastLoadSearchMemoryWithoutAI(ctx context.Context, input, taskId string) {
	if r.memoryTriage == nil {
		return
	}
	emitter := r.GetEmitter()

	log.Info("start to handle searching memory for ReActLoop without AI")
	pr, pw := utils.NewPipe()
	defer pw.Close()
	emitter.EmitSystemStreamEvent("fast-memory-fetch", time.Now(), pr, taskId)
	pw.WriteString("快速检索记忆：Searching relevant memories quickly...")
	emitter.EmitJSON(schema.EVENT_TYPE_MEMORY_SEARCH_QUICKLY, "memory-search-quickly", map[string]any{
		"query": input,
	})
	searchResult, err := r.memoryTriage.SearchMemoryWithoutAI(input, r.memorySizeLimit)
	if ctx.Err() != nil {
		return
	}
	r.PushMemory(searchResult)
	if err != nil {
		aicommon.TypeWriterWrite(pw, "... 快速检索失败，Reason: "+err.Error(), 300)
	} else {
		var size int
		if !utils.IsNil(searchResult) && searchResult.ContentTokens > 0 {
			size = searchResult.ContentTokens
		}
		if size > 0 {
			aicommon.TypeWriterWrite(pw, "... 快速记忆检索结束，匹配到记忆大小为："+utils.InterfaceToString(size)+" tokens", 300)
		} else {
			aicommon.TypeWriterWrite(pw, "... 快速记忆检索结束，没能找到合适的过往记忆。", 300)
		}
	}
	pw.Close()
	if strings.TrimSpace(r.GetCurrentMemoriesContent()) != "" {
		log.Infof("memory updated via fast search memory - ========================== \n%v\n==========================", r.GetCurrentMemoriesContent())
	}
}
