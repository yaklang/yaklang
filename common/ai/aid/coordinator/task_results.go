package coordinator

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
)

func taskResultEvidence(cfg aicommon.AICallerConfigIf, observation taskResultRecord) (string, string) {
	key := fmt.Sprintf("%s:%s:%d:%d", cfg.GetRuntimeId(), observation.TaskID, observation.PlanVersion, observation.AttemptID)
	digest := sha256.Sum256([]byte(key))
	data, _ := json.Marshal(observation) // Only serializable task/result fields.
	return fmt.Sprintf("coordinator.task.%x", digest[:16]), "任务执行结果与验收记录（实时状态以 PLAN STATUS 为准）：\n" + string(data)
}

// Controller publishes settled/reviewed transitions, including failures
// and cancelled workers that never called submit_task_result. No model query is needed.
func persistTaskResults(cfg aicommon.AICallerConfigIf, results map[string]taskResultRecord, published map[string][32]byte) error {
	if len(results) == 0 {
		return nil
	}
	rendered := cfg.GetSessionEvidenceRendered()
	ids := make([]string, 0, len(results))
	for id := range results {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, taskID := range ids {
		result := results[taskID]
		id, content := taskResultEvidence(cfg, result)
		hash := sha256.Sum256([]byte(content))
		if previous, ok := published[id]; ok && previous == hash {
			continue
		}
		if strings.Contains(rendered, "[id: "+id+"]") && strings.Contains(rendered, content) {
			published[id] = hash
			continue
		}
		if _, err := reactloops.SaveSessionEvidence(cfg, id, content); err != nil {
			return err
		}
		published[id] = hash
	}
	return nil
}

func (c *Controller) attachResultTimeline(cfg aicommon.AICallerConfigIf) error {
	c.publishMu.Lock()
	defer c.publishMu.Unlock()
	c.mu.Lock()
	c.resultConfig = cfg
	c.resultHashes = make(map[string][32]byte)
	c.pendingResults = make(map[string]taskResultRecord)
	for _, a := range c.state.Attempts {
		c.queueResultLocked(a)
	}
	results := clone(c.pendingResults)
	c.mu.Unlock()
	err := persistTaskResults(cfg, results, c.resultHashes)
	c.mu.Lock()
	c.resultErr = err
	if err == nil {
		c.clearPublishedResultsLocked(results)
	}
	c.mu.Unlock()
	return err
}

// Only meaningful result transitions enter Evidence. Running/cancelling state
// belongs to PLAN STATUS; wait and unrelated revisions never scan all tasks.
func (c *Controller) queueResultLocked(a Attempt) {
	if c.resultConfig == nil || a.ID == 0 || a.State == Pending || a.State == Running || a.State == Cancelling {
		return
	}
	if c.pendingResults == nil {
		c.pendingResults = make(map[string]taskResultRecord)
	}
	c.pendingResults[a.Task.ID] = clone(taskResultRecords([]Attempt{a})[0])
}

func (c *Controller) clearPublishedResultsLocked(results map[string]taskResultRecord) {
	for id, result := range results {
		current, ok := c.pendingResults[id]
		// A concurrent review may already have queued a newer state/reason.
		oldData, _ := json.Marshal(result)
		newData, _ := json.Marshal(current)
		if ok && string(oldData) == string(newData) {
			delete(c.pendingResults, id)
		}
	}
}
