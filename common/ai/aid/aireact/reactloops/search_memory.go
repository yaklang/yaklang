package reactloops

import (
	"strings"
	"time"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/ytoken"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
)

// resetIntentMemory detaches the previous task's snapshot before initialization.
// A task without intent recognition starts with no automatic memory injection.
func (r *ReActLoop) resetIntentMemory() {
	r.memoryUpdateMu.Lock()
	defer r.memoryUpdateMu.Unlock()
	r.intentMemoryGeneration++
	r.intentMemoryKey, r.intentMemorySnapshot = "", ""
	r.intentMemoryActive = true
}

// RecallMemoryForIntent runs only after an intent decision is applied.
// Iterations, task status changes and explicit searches do not refresh it.
// Empty results are cached too; a late result cannot replace a newer intent.
func (r *ReActLoop) RecallMemoryForIntent(task aicommon.AIStatefulTask, intent string) {
	if task == nil || utils.IsNil(r.memoryTriage) || task.GetContext().Err() != nil || r.isSimpleQueryWithoutWork() {
		return
	}
	input := strings.TrimSpace(task.GetUserInput())
	if input == "" || strings.TrimSpace(intent) == "" {
		return
	}
	query := input
	if info := task.GetTaskRetrievalInfo(); info != nil && strings.TrimSpace(info.Target) != "" {
		query = strings.TrimSpace(info.Target)
	}
	key := task.GetId() + "\x00" + input + "\x00" + query + "\x00" + intent
	r.memoryUpdateMu.Lock()
	if key == r.intentMemoryKey {
		r.memoryUpdateMu.Unlock()
		return
	}
	r.intentMemoryKey, r.intentMemorySnapshot, r.intentMemoryActive = key, "", true
	r.intentMemoryGeneration++
	generation := r.intentMemoryGeneration
	r.memoryUpdateMu.Unlock()
	go func() {
		var entities []*aicommon.MemoryEntity
		// Reuse the existing non-AI retrieval API once per applied intent.
		result, err := r.memoryTriage.SearchMemoryWithoutAI(query, 1200)
		if result != nil {
			entities = result.Memories
		}
		if err != nil {
			log.Warnf("intent memory recall failed for task %s: %v", task.GetId(), err)
			return
		}
		var selected []*aicommon.MemoryEntity
		now := time.Now()
		for _, entity := range entities {
			if entity == nil || entity.O_Score < 0.5 || entity.R_Score < 0.5 || (entity.ExpiresAt != nil && !entity.ExpiresAt.After(now)) {
				continue
			}
			selected = append(selected, entity)
			if len(selected) == 5 {
				break
			}
		}
		var snapshot string
		for len(selected) > 0 {
			snapshot = aicommon.BuildPromptMemoriesMarkdownFromEntities(selected, aicommon.MemoryIntentGeneric,
				aicommon.WithMemoryInjectNow(now), aicommon.WithMemoryInjectMaxTotal(5), aicommon.WithMemoryInjectMaxPerRoute(5), aicommon.WithMemoryInjectMaxContentRunes(300))
			if ytoken.CalcTokenCount(snapshot) <= 1200 {
				break
			}
			selected = selected[:len(selected)-1]
			snapshot = ""
		}
		r.memoryUpdateMu.Lock()
		defer r.memoryUpdateMu.Unlock()
		if generation != r.intentMemoryGeneration || task.GetContext().Err() != nil {
			return
		}
		r.intentMemorySnapshot = snapshot
		if emitter := r.GetEmitter(); emitter != nil {
			emitter.EmitJSON(schema.EVENT_TYPE_MEMORY_SEARCH_QUICKLY, "intent-memory-recall", map[string]any{"task_id": task.GetId(), "matches": len(selected), "snapshot_tokens": ytoken.CalcTokenCount(snapshot)})
		}
	}()
}
