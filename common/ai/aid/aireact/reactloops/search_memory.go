package reactloops

import (
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"strings"
)

func (r *ReActLoop) loadingSearchMemory(input string) {
	if r.memoryTriage == nil {
		return
	}
	emitter := r.GetEmitter()

	var taskId string
	if r.GetCurrentTask() != nil {
		taskId = r.GetCurrentTask().GetId()
	}

	log.Info("start to handle searching memory for ReActLoop without AI")
	pr, pw := utils.NewPipe()
	emitter.EmitThoughtTypeWriterStreamReader(taskId, pr)
	pw.WriteString("快速检索记忆：Searching relevant memories quickly...")
	emitter.EmitJSON(schema.EVENT_TYPE_MEMORY_SEARCH_QUICKLY, "memory-search-quickly", map[string]any{
		"query": input,
	})
	searchResult, err := r.memoryTriage.SearchMemoryWithoutAI(input, r.memorySizeLimit)
	if err != nil {
		aicommon.TypeWriterWrite(pw, "... 快速检索失败，Reason: "+err.Error(), 300)
	} else {
		var size int
		if !utils.IsNil(searchResult) && searchResult.ContentBytes > 0 {
			size = searchResult.ContentBytes
		}
		if size > 0 {
			aicommon.TypeWriterWrite(pw, "... 快速记忆检索结束，匹配到记忆大小为："+utils.ByteSize(uint64(size)), 300)
		} else {
			aicommon.TypeWriterWrite(pw, "... 快速记忆检索结束，没能找到合适的过往记忆"+utils.ByteSize(uint64(size)), 300)
		}
	}
	pw.Close()
	r.PushMemory(searchResult)
	if strings.TrimSpace(r.GetCurrentMemoriesContent()) != "" {
		log.Infof("memory updated via fast search memory - ========================== \n%v\n==========================", r.GetCurrentMemoriesContent())
	}
}
