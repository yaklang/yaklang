package scannode

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/yaklang/yaklang/common/log"
	ssav1 "github.com/yaklang/yaklang/scannode/gen/legionpb/legion/ssa/v1"
	"google.golang.org/protobuf/proto"
)

// ssaLogTailResultPrefix is the core-NATS subject prefix a node uses to
// answer ssa.log.tail commands:
//
//	legion.realtime.ssa.log.tail.v2.<query_id>
const ssaLogTailResultPrefix = "legion.realtime.ssa.log.tail.v2"

const (
	ssaLogTailDefaultMaxBytes = 64 * 1024
	ssaLogTailMaxBytesLimit   = 768 * 1024 // keep protobuf envelope below NATS max payload
)

// handleSSALogTail reads the tail of the per-task log file of a scan attempt.
// Task logs are written unconditionally for every scan (debug or not) by
// openTaskLogWriter into <node>/logs/<jobID>_<subtaskID>_<attemptID>.log, so
// this command works for any task state: running, cancelled or finished.
func (b *legionJobBridge) handleSSALogTail(ctx context.Context, raw []byte) error {
	var payload ssav1.LogTailCommand
	if err := proto.Unmarshal(raw, &payload); err != nil {
		return fmt.Errorf("unmarshal ssa log tail: %w", err)
	}
	if strings.TrimSpace(payload.QueryId) == "" {
		return fmt.Errorf("ssa log tail query_id is required")
	}
	if strings.TrimSpace(payload.JobId) == "" || strings.TrimSpace(payload.AttemptId) == "" {
		return fmt.Errorf("ssa log tail job_id and attempt_id are required")
	}
	if payload.Offset < 0 {
		return fmt.Errorf("ssa log tail offset must be >= 0")
	}
	maxBytes := payload.MaxBytes
	if maxBytes <= 0 {
		maxBytes = ssaLogTailDefaultMaxBytes
	}
	if maxBytes > ssaLogTailMaxBytesLimit {
		maxBytes = ssaLogTailMaxBytesLimit
	}

	response := &ssav1.LogTailResult{QueryId: payload.QueryId, Offset: payload.Offset}
	logPath, reason := b.resolveLogTailPath(payload.JobId, payload.AttemptId, payload.LogKind)
	if logPath == "" {
		if reason == "" {
			reason = "task log not found for this attempt"
		}
		response.Reason = reason
		log.Infof("[log-tail] answered: job=%s attempt=%s kind=%s found=false (%s)", payload.JobId, payload.AttemptId, payload.LogKind, reason)
		return b.publishLogTailResponse(ctx, payload.QueryId, response)
	}

	content, totalBytes, start, hasMore, err := tailLogFile(logPath, payload.Offset, maxBytes)
	if err != nil {
		response.Reason = fmt.Sprintf("read task log: %v", err)
		log.Warnf("[log-tail] job=%s attempt=%s read failed: %v", payload.JobId, payload.AttemptId, err)
		return b.publishLogTailResponse(ctx, payload.QueryId, response)
	}
	response.Found = true
	response.TotalBytes = totalBytes
	response.HasMore = hasMore
	response.Content = content
	response.Offset = totalBytes - start // bytes skipped from the end for this chunk

	log.Infof("[log-tail] answered: job=%s attempt=%s total=%d chunk=%d offset=%d has_more=%v",
		payload.JobId, payload.AttemptId, totalBytes, len(content), response.Offset, hasMore)
	return b.publishLogTailResponse(ctx, payload.QueryId, response)
}

func (b *legionJobBridge) publishLogTailResponse(ctx context.Context, queryID string, response *ssav1.LogTailResult) error {
	raw, err := proto.Marshal(response)
	if err != nil {
		return fmt.Errorf("marshal ssa log tail response: %w", err)
	}
	publisher, ok := b.capabilityPublisher.(*capabilityEventPublisher)
	if !ok || publisher == nil {
		return fmt.Errorf("capability event publisher is not ready")
	}
	publishCtx, cancel := context.WithTimeout(b.agent.node.GetRootContext(), 5*time.Second)
	defer cancel()
	subject := ssaLogTailResultSubject(queryID)
	if err := publisher.PublishRaw(publishCtx, subject, raw); err != nil {
		return fmt.Errorf("publish ssa log tail response: %w", err)
	}
	return nil
}

func ssaLogTailResultSubject(queryID string) string {
	return ssaLogTailResultPrefix + "." + queryID
}

// resolveTaskLogPath locates the per-task log file for a job/attempt. File
// layout: <jobBaseDir>/logs/<JobID>_<SubTaskID>_<AttemptID>.log.
func (b *legionJobBridge) resolveTaskLogPath(jobID, attemptID string) string {
	var dirs []string
	if b != nil && b.agent != nil && b.agent.node != nil {
		dirs = append(dirs, filepath.Join(b.agent.node.BaseDir(), "logs"))
	}
	if env := os.Getenv("SCANNODE_TASK_LOG_DIR"); env != "" {
		dirs = append(dirs, env)
	}
	dirs = append(dirs, filepath.Join(os.TempDir(), "legion-node-logs"))
	return resolveTaskLogPathInDirs(dirs, jobID, attemptID)
}

func (b *legionJobBridge) resolveLogTailPath(jobID, attemptID, logKind string) (string, string) {
	switch strings.ToLower(strings.TrimSpace(logKind)) {
	case "", "task":
		path := b.resolveTaskLogPath(jobID, attemptID)
		if path == "" {
			return "", "task log not found for this attempt"
		}
		return path, ""
	case "db":
		path := b.resolveDBLogPath(jobID, attemptID)
		if path == "" {
			return "", "db log not found for this attempt"
		}
		return path, ""
	default:
		return "", "unsupported log_kind"
	}
}

func (b *legionJobBridge) resolveDBLogPath(jobID, attemptID string) string {
	dir := scanDebugDirs.resolve(jobID, attemptID)
	if dir == "" && b != nil && b.agent != nil {
		key := debugDirKey(jobID, attemptID)
		fallback := filepath.Join(b.agent.debugBaseDir(), "debug", key)
		if info, err := os.Stat(fallback); err == nil && info.IsDir() {
			dir = fallback
		}
	}
	if dir == "" {
		return ""
	}
	path := filepath.Join(dir, "db.log")
	if _, err := os.Stat(path); err != nil {
		return ""
	}
	return path
}

// resolveTaskLogPathWithDirs is the testable core of resolveTaskLogPath.
func (b *legionJobBridge) resolveTaskLogPathWithDirs(dirs []string, jobID, attemptID string) string {
	return resolveTaskLogPathInDirs(dirs, jobID, attemptID)
}

func resolveTaskLogPathInDirs(dirs []string, jobID, attemptID string) string {
	jobID = sanitizeLogName(jobID)
	attemptID = sanitizeLogName(attemptID)
	if jobID == "" || attemptID == "" {
		return ""
	}
	for _, dir := range dirs {
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, entry := range entries {
			name := entry.Name()
			if entry.IsDir() || !strings.HasSuffix(name, ".log") {
				continue
			}
			if !strings.HasPrefix(name, jobID+"_") {
				continue
			}
			if !strings.HasSuffix(name, "_"+attemptID+".log") {
				continue
			}
			return filepath.Join(dir, name)
		}
	}
	return ""
}

// tailLogFile reads the last portion of a log file. offset is the number of
// bytes to skip from the end; the returned chunk is aligned to line boundaries
// so the frontend can render complete lines. It returns the chunk, the full
// file size, the byte position of the chunk start, and whether older content
// exists before the chunk.
func tailLogFile(path string, offset int64, maxBytes int64) (content string, total int64, start int64, hasMore bool, err error) {
	info, err := os.Stat(path)
	if err != nil {
		return "", 0, 0, false, err
	}
	total = info.Size()
	if offset >= total {
		return "", total, total, false, nil
	}
	end := total - offset
	start = end - maxBytes
	if start < 0 {
		start = 0
	}
	data := make([]byte, end-start)
	f, err := os.Open(path)
	if err != nil {
		return "", total, 0, false, err
	}
	defer f.Close()
	if _, err := f.ReadAt(data, start); err != nil {
		return "", total, 0, false, err
	}
	// Align to line boundaries: skip the partial first line unless the chunk
	// already starts at the beginning of the file.
	if start > 0 {
		if idx := indexByte(data, '\n'); idx >= 0 {
			start += int64(idx + 1)
			data = data[idx+1:]
		}
	}
	hasMore = start > 0
	return string(data), total, start, hasMore, nil
}

func indexByte(data []byte, b byte) int {
	for i, c := range data {
		if c == b {
			return i
		}
	}
	return -1
}
