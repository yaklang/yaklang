package syntaxflow_scan

import (
	"strings"
	"sync"
	"time"

	"github.com/yaklang/yaklang/common/schema"
)

const (
	defaultScanTaskMemTTL        = 30 * time.Minute
	defaultScanTaskMemMaxEntries = 256
)

type scanTaskMemEntry struct {
	task      *schema.SyntaxFlowScanTask
	expiresAt time.Time
	lastUsed  time.Time
}

var (
	scanTaskMemMu sync.RWMutex
	scanTaskMem   = make(map[string]*scanTaskMemEntry)
)

// PutScanTaskMemoryCache stores a snapshot for task_id. Used when the DB row is not written yet (first SaveTask
// runs per finished rule) and refreshed after each successful SaveTask. Bounded by TTL and max size (LRU).
func PutScanTaskMemoryCache(task *schema.SyntaxFlowScanTask) {
	if task == nil {
		return
	}
	id := strings.TrimSpace(task.TaskId)
	if id == "" {
		return
	}
	now := time.Now()
	snap := cloneSyntaxFlowScanTaskForCache(task)
	if snap == nil {
		return
	}

	scanTaskMemMu.Lock()
	defer scanTaskMemMu.Unlock()

	scanTaskMemEvictExpiredLocked(now)
	scanTaskMem[id] = &scanTaskMemEntry{
		task:      snap,
		expiresAt: now.Add(defaultScanTaskMemTTL),
		lastUsed:  now,
	}
	if len(scanTaskMem) > defaultScanTaskMemMaxEntries {
		scanTaskMemEvictLRULocked()
	}
}

// GetScanTaskMemoryCache returns a copy of the cached task when present and not expired.
func GetScanTaskMemoryCache(taskID string) (*schema.SyntaxFlowScanTask, bool) {
	id := strings.TrimSpace(taskID)
	if id == "" {
		return nil, false
	}
	now := time.Now()

	scanTaskMemMu.Lock()
	defer scanTaskMemMu.Unlock()

	ent, ok := scanTaskMem[id]
	if !ok {
		return nil, false
	}
	if !ent.expiresAt.After(now) {
		delete(scanTaskMem, id)
		return nil, false
	}
	ent.lastUsed = now
	return cloneSyntaxFlowScanTaskForCache(ent.task), true
}

func scanTaskMemEvictExpiredLocked(now time.Time) {
	for k, v := range scanTaskMem {
		if v == nil || !v.expiresAt.After(now) {
			delete(scanTaskMem, k)
		}
	}
}

func scanTaskMemEvictLRULocked() {
	var victim string
	var victimTime time.Time
	first := true
	for k, v := range scanTaskMem {
		if v == nil {
			delete(scanTaskMem, k)
			continue
		}
		if first || v.lastUsed.Before(victimTime) {
			victim = k
			victimTime = v.lastUsed
			first = false
		}
	}
	if victim != "" {
		delete(scanTaskMem, victim)
	}
}

func cloneSyntaxFlowScanTaskForCache(s *schema.SyntaxFlowScanTask) *schema.SyntaxFlowScanTask {
	if s == nil {
		return nil
	}
	c := *s
	if len(s.Config) > 0 {
		c.Config = append([]byte(nil), s.Config...)
	}
	return &c
}
