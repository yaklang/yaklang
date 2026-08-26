package scannode

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/yaklang/yaklang/common/log"
)

// ssaDebugQueryResultPrefix is the core-NATS subject prefix a node uses to
// answer ssa.debug.query commands. The full subject is derived from the
// caller-provided query_id so concurrent queries never collide:
//
//	legion.realtime.ssa.debug.result.<query_id>
const ssaDebugQueryResultPrefix = "legion.realtime.ssa.debug.result"

// ssaDebugQueryPayload is the JSON body of an ssa.debug.query command. The
// platform sends it to the node that owns (or owned) a scan attempt; the node
// answers with whatever pprof/log data exists in the attempt's debug
// directory, regardless of the task state (running, paused, cancel-requested,
// succeeded, failed, cancelled, ...).
type ssaDebugQueryPayload struct {
	QueryID    string `json:"query_id"`
	JobID      string `json:"job_id"`
	AttemptID  string `json:"attempt_id"`
	TaskStatus string `json:"task_status,omitempty"`
}

// ssaDebugQueryResponse is the JSON body published back to the platform.
type ssaDebugQueryResponse struct {
	Found    bool            `json:"found"`
	Reason   string          `json:"reason,omitempty"`
	Analysis json.RawMessage `json:"analysis,omitempty"`
}

// debugDirRegistry tracks the debug directory of every active debug-enabled
// task so ssa.debug.query can find it no matter which state the run is in.
// Entries are removed once the run is finalized; the convention-path fallback
// (<base>/debug/<jobID>_<attemptID>) still answers for directories that are
// no longer registered (e.g. after a node restart).
type debugDirRegistry struct {
	mu      sync.Mutex
	baseDir string
	dirs    map[string]string // "<jobID>_<attemptID>" -> debug dir
}

var scanDebugDirs = &debugDirRegistry{dirs: make(map[string]string)}

func debugDirKey(jobID, attemptID string) string {
	return sanitizeLogName(jobID) + "_" + sanitizeLogName(attemptID)
}

func (r *debugDirRegistry) register(baseDir, jobID, attemptID, dir string) {
	if dir == "" {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if baseDir != "" {
		r.baseDir = baseDir
	}
	r.dirs[debugDirKey(jobID, attemptID)] = dir
}

func (r *debugDirRegistry) unregister(jobID, attemptID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.dirs, debugDirKey(jobID, attemptID))
}

func (r *debugDirRegistry) resolve(jobID, attemptID string) string {
	key := debugDirKey(jobID, attemptID)
	r.mu.Lock()
	dir := r.dirs[key]
	baseDir := r.baseDir
	r.mu.Unlock()
	if dir != "" {
		if info, err := os.Stat(dir); err == nil && info.IsDir() {
			return dir
		}
	}
	// Fallback: convention path <base>/debug/<jobID>_<attemptID>. Covers runs
	// whose registry entry was removed by finalization or a node restart.
	if baseDir == "" {
		return ""
	}
	fallback := filepath.Join(baseDir, "debug", key)
	if info, err := os.Stat(fallback); err == nil && info.IsDir() {
		return fallback
	}
	return ""
}

// handleSSADebugQuery analyzes the debug run directory of a scan attempt on
// demand and returns whatever exists (pprof samples, log, phases, summary).
// It answers from the local filesystem directly, so it works while the task
// is still running, when it was cancelled mid-run, and after it finished —
// as long as the debug directory still exists on this node.
func (b *legionJobBridge) handleSSADebugQuery(ctx context.Context, raw []byte) error {
	var payload ssaDebugQueryPayload
	if err := json.Unmarshal(raw, &payload); err != nil {
		return fmt.Errorf("unmarshal ssa debug query: %w", err)
	}
	if strings.TrimSpace(payload.QueryID) == "" {
		return fmt.Errorf("ssa debug query query_id is required")
	}
	if strings.TrimSpace(payload.JobID) == "" || strings.TrimSpace(payload.AttemptID) == "" {
		return fmt.Errorf("ssa debug query job_id and attempt_id are required")
	}

	response := ssaDebugQueryResponse{Found: false}
	dir := scanDebugDirs.resolve(payload.JobID, payload.AttemptID)
	if dir == "" && b != nil && b.agent != nil {
		// Registry misses (e.g. node restarted since the run): fall back to
		// the convention path under the node's current debug base dir.
		key := debugDirKey(payload.JobID, payload.AttemptID)
		fallback := filepath.Join(b.agent.debugBaseDir(), "debug", key)
		if info, err := os.Stat(fallback); err == nil && info.IsDir() {
			dir = fallback
		}
	}
	if dir == "" {
		response.Reason = "debug directory not found for this task attempt"
		log.Infof("[debug] ssa.debug.query answered: job=%s attempt=%s found=false (no debug dir)", payload.JobID, payload.AttemptID)
		return b.publishDebugQueryResponse(ctx, payload.QueryID, response)
	}

	// Serve the cached analysis when it is newer than every pprof/log input,
	// so repeated queries do not re-parse large profiles.
	if cached, ok := readCachedDebugAnalysis(dir); ok {
		response.Found = true
		response.Analysis = cached
		log.Infof("[debug] ssa.debug.query answered: job=%s attempt=%s found=true (cached) dir=%s", payload.JobID, payload.AttemptID, dir)
		return b.publishDebugQueryResponse(ctx, payload.QueryID, response)
	}

	analysis := AnalyzeDebugRunWithStatus(dir, payload.TaskStatus)
	hasContent := len(analysis.Samples) > 0 || analysis.Summary != nil ||
		analysis.StartedAt != nil || strings.TrimSpace(analysis.Status) != ""
	if !hasContent && len(analysis.Errors) > 0 {
		response.Reason = strings.Join(analysis.Errors, "; ")
	} else {
		analysisJSON, err := json.Marshal(analysis)
		if err != nil {
			return fmt.Errorf("marshal ssa debug analysis: %w", err)
		}
		if err := writeCachedDebugAnalysis(dir, analysisJSON); err != nil {
			log.Warnf("[debug] cache analysis json failed: %v", err)
		}
		response.Found = true
		response.Analysis = analysisJSON
	}
	log.Infof("[debug] ssa.debug.query answered: job=%s attempt=%s found=%v samples=%d dir=%s",
		payload.JobID, payload.AttemptID, response.Found, len(analysis.Samples), dir)
	return b.publishDebugQueryResponse(ctx, payload.QueryID, response)
}

// debugAnalysisCacheName is the JSON file used to cache a parsed debug run
// analysis next to the run directory.
const debugAnalysisCacheName = "analysis.cache.json"

func writeCachedDebugAnalysis(dir string, analysisJSON []byte) error {
	return os.WriteFile(filepath.Join(dir, debugAnalysisCacheName), analysisJSON, 0o644)
}

// readCachedDebugAnalysis returns the cached analysis JSON when it is at least
// as new as every profile/log input file of the debug run.
func readCachedDebugAnalysis(dir string) (json.RawMessage, bool) {
	cachePath := filepath.Join(dir, debugAnalysisCacheName)
	cacheInfo, err := os.Stat(cachePath)
	if err != nil {
		return nil, false
	}
	newestInput := time.Time{}
	inputs := []string{"cpu-pprof", "memory-pprof", "goroutine-pprof", "log"}
	for _, name := range inputs {
		path := filepath.Join(dir, name)
		info, err := os.Stat(path)
		if err != nil {
			continue
		}
		if info.IsDir() {
			entries, err := os.ReadDir(path)
			if err != nil {
				continue
			}
			for _, entry := range entries {
				entryInfo, err := entry.Info()
				if err != nil {
					continue
				}
				if entryInfo.ModTime().After(newestInput) {
					newestInput = entryInfo.ModTime()
				}
			}
			continue
		}
		if info.ModTime().After(newestInput) {
			newestInput = info.ModTime()
		}
	}
	if !newestInput.IsZero() && newestInput.After(cacheInfo.ModTime()) {
		return nil, false
	}
	data, err := os.ReadFile(cachePath)
	if err != nil || len(data) == 0 {
		return nil, false
	}
	return json.RawMessage(data), true
}

func (b *legionJobBridge) publishDebugQueryResponse(ctx context.Context, queryID string, response ssaDebugQueryResponse) error {
	responseRaw, err := json.Marshal(response)
	if err != nil {
		return fmt.Errorf("marshal ssa debug query response: %w", err)
	}
	publisher, ok := b.capabilityPublisher.(*capabilityEventPublisher)
	if !ok || publisher == nil {
		return fmt.Errorf("capability event publisher is not ready")
	}
	publishCtx, cancel := context.WithTimeout(b.agent.node.GetRootContext(), 5*time.Second)
	defer cancel()
	subject := ssaDebugQueryResultSubject(queryID)
	if err := publisher.PublishRaw(publishCtx, subject, responseRaw); err != nil {
		return fmt.Errorf("publish ssa debug query response: %w", err)
	}
	return nil
}

func ssaDebugQueryResultSubject(queryID string) string {
	return ssaDebugQueryResultPrefix + "." + queryID
}
