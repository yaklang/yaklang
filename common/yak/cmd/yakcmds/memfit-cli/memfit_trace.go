package memfitcli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/schema"
)

const (
	memfitTraceProtocolFile  = "protocol.jsonl"
	memfitTraceAIEventsFile  = "ai-events.jsonl"
	memfitTraceAICallsFile   = "ai-calls.jsonl"
	memfitTraceWorkerLogFile = "worker.log"
	memfitTraceLifecycleFile = "lifecycle.jsonl"
)

// memfitTrace writes an explicitly requested local diagnostic bundle. It is
// intentionally small and append-only so a crashed or cancelled run still
// leaves useful evidence.
type memfitTrace struct {
	dir    string
	secret string

	mu     sync.Mutex
	files  map[string]*os.File
	closed bool
}

func newMemfitTrace(dir, secret string) (*memfitTrace, error) {
	dir = strings.TrimSpace(dir)
	if dir == "" {
		return nil, nil
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		return nil, fmt.Errorf("resolve memfit trace dir: %w", err)
	}
	if err := os.MkdirAll(abs, 0o700); err != nil {
		return nil, fmt.Errorf("create memfit trace dir: %w", err)
	}
	trace := &memfitTrace{
		dir:    abs,
		secret: secret,
		files:  make(map[string]*os.File),
	}
	trace.recordJSONL(memfitTraceLifecycleFile, "trace_started", map[string]any{
		"pid":  os.Getpid(),
		"note": "This directory contains complete local AI diagnostics. Treat it as sensitive.",
	})
	return trace, nil
}

func (t *memfitTrace) Dir() string {
	if t == nil {
		return ""
	}
	return t.dir
}

func (t *memfitTrace) Close() {
	if t == nil {
		return
	}
	t.recordJSONL(memfitTraceLifecycleFile, "trace_closed", nil)
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.closed {
		return
	}
	t.closed = true
	for name, file := range t.files {
		if file != nil {
			_ = file.Close()
		}
		delete(t.files, name)
	}
}

func (t *memfitTrace) openLocked(name string) (*os.File, error) {
	if file := t.files[name]; file != nil {
		return file, nil
	}
	file, err := os.OpenFile(filepath.Join(t.dir, name), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return nil, err
	}
	t.files[name] = file
	return file, nil
}

func (t *memfitTrace) recordJSONL(name, kind string, payload any) {
	if t == nil {
		return
	}
	record := map[string]any{
		"timestamp": time.Now().Format(time.RFC3339Nano),
		"kind":      kind,
	}
	if payload != nil {
		record["payload"] = payload
	}
	raw, err := json.Marshal(record)
	if err != nil {
		t.recordLine(memfitTraceWorkerLogFile, fmt.Sprintf("trace marshal %s failed: %v", kind, err))
		return
	}
	raw = t.redactBytes(raw)

	t.mu.Lock()
	defer t.mu.Unlock()
	if t.closed {
		return
	}
	file, err := t.openLocked(name)
	if err != nil {
		return
	}
	_, _ = file.Write(append(raw, '\n'))
}

func (t *memfitTrace) recordLine(name, line string) {
	if t == nil || strings.TrimSpace(line) == "" {
		return
	}
	line = t.redact(line)

	t.mu.Lock()
	defer t.mu.Unlock()
	if t.closed {
		return
	}
	file, err := t.openLocked(name)
	if err != nil {
		return
	}
	_, _ = fmt.Fprintf(file, "%s %s\n", time.Now().Format(time.RFC3339Nano), line)
}

func (t *memfitTrace) redact(input string) string {
	if t == nil || t.secret == "" || input == "" {
		return input
	}
	return strings.ReplaceAll(input, t.secret, "[REDACTED]")
}

func (t *memfitTrace) redactBytes(input []byte) []byte {
	if t == nil || t.secret == "" || len(input) == 0 {
		return input
	}
	output := bytes.ReplaceAll(input, []byte(t.secret), []byte("[REDACTED]"))
	if escaped, err := json.Marshal(t.secret); err == nil && len(escaped) > 2 {
		output = bytes.ReplaceAll(output, escaped[1:len(escaped)-1], []byte("[REDACTED]"))
	}
	return output
}

func (t *memfitTrace) recordStartConfig(config memfitStartConfig) {
	if t == nil {
		return
	}
	t.recordJSONL(memfitTraceLifecycleFile, "start_config", map[string]any{
		"agent":          config.AgentName,
		"title":          config.AgentTitle,
		"description":    config.AgentDesc,
		"focus":          config.Focus,
		"workdir":        config.Workdir,
		"language":       config.Language,
		"review_policy":  config.ReviewPolicy,
		"max_iterations": config.MaxIterations,
		"timeout":        config.Timeout,
		"session_id":     config.SessionID,
		"debug":          config.Debug,
		"model":          config.Model,
		"ai_type":        config.AIType,
		"base_url":       config.BaseURL,
	})
}

func (t *memfitTrace) recordProtocol(direction, typ, id string, data any) {
	if t == nil {
		return
	}
	t.recordJSONL(memfitTraceProtocolFile, "protocol_frame", map[string]any{
		"direction": direction,
		"type":      typ,
		"id":        id,
		"data":      data,
	})
}

func (t *memfitTrace) recordAIEvent(event *schema.AiOutputEvent) {
	if t == nil || event == nil {
		return
	}
	t.recordJSONL(memfitTraceAIEventsFile, "ai_event", map[string]any{
		"type":             event.Type,
		"node_id":          event.NodeId,
		"event_uuid":       event.EventUUID,
		"task_id":          event.TaskId,
		"task_uuid":        event.TaskUUID,
		"task_index":       event.TaskIndex,
		"call_tool_id":     event.CallToolID,
		"is_system":        event.IsSystem,
		"is_stream":        event.IsStream,
		"is_reason":        event.IsReason,
		"is_json":          event.IsJson,
		"stream_delta":     string(event.StreamDelta),
		"content":          string(event.Content),
		"content_type":     event.ContentType,
		"viz_source":       event.VizSource,
		"ai_service":       event.AIService,
		"ai_model":         event.AIModelName,
		"model_verbose":    event.AIModelVerboseName,
		"session_id":       event.SessionId,
		"timestamp":        event.Timestamp,
		"disable_markdown": event.DisableMarkdown,
	})
}

func (t *memfitTrace) recordAICall(record aicommon.AICallTraceRecord) {
	if t == nil {
		return
	}
	t.recordJSONL(memfitTraceAICallsFile, "ai_call", record)
}
