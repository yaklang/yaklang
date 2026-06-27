package loop_ssa_api_discovery

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/loop_ssa_api_discovery/store"
)

// ExecutionLogEntry represents a single structured log entry in JSONL format.
type ExecutionLogEntry struct {
	Timestamp     time.Time `json:"timestamp"`
	Step          string    `json:"step"`
	Status        string    `json:"status"` // "start" | "end" | "error" | "info"
	ExecutionType string    `json:"execution_type"` // "programmatic" | "ai"
	DurationMs    int64     `json:"duration_ms,omitempty"`
	OutputFiles   []string  `json:"output_files,omitempty"`
	Message       string    `json:"message,omitempty"`
	Error         string    `json:"error,omitempty"`
}

// ExecutionLogger writes structured execution logs as JSONL to workDir/ssa_discovery/execution_log.jsonl.
type ExecutionLogger struct {
	workDir string
	mu      sync.Mutex
}

// ExecutionLogPath returns the path of the structured execution log file.
func ExecutionLogPath(workDir string) string {
	return filepath.Join(workDir, store.SubDirName(), "execution_log.jsonl")
}

// NewExecutionLogger creates an ExecutionLogger for the given workDir.
func NewExecutionLogger(workDir string) *ExecutionLogger {
	if workDir == "" {
		workDir = "."
	}
	_ = os.MkdirAll(filepath.Join(workDir, store.SubDirName()), 0o755)
	return &ExecutionLogger{workDir: workDir}
}

// ForRuntime creates an ExecutionLogger from a Runtime.
func ForRuntime(rt *Runtime) *ExecutionLogger {
	if rt == nil || rt.WorkDir == "" {
		return nil
	}
	return NewExecutionLogger(rt.WorkDir)
}

// path returns the log file path.
func (l *ExecutionLogger) path() string {
	return ExecutionLogPath(l.workDir)
}

// writeEntry appends a single JSONL entry to the log file.
func (l *ExecutionLogger) writeEntry(entry ExecutionLogEntry) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	data, err := json.Marshal(entry)
	if err != nil {
		return err
	}
	data = append(data, '\n')

	f, err := os.OpenFile(l.path(), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()

	_, writeErr := f.Write(data)
	return writeErr
}

// StepStart logs the beginning of a step.
func (l *ExecutionLogger) StepStart(step, executionType string) {
	if l == nil {
		return
	}
	_ = l.writeEntry(ExecutionLogEntry{
		Timestamp:     time.Now().UTC(),
		Step:          step,
		Status:        "start",
		ExecutionType: executionType,
	})
}

// StepEnd logs the successful completion of a step with duration and output files.
func (l *ExecutionLogger) StepEnd(step, executionType string, started time.Time, outputFiles []string) {
	if l == nil {
		return
	}
	_ = l.writeEntry(ExecutionLogEntry{
		Timestamp:     time.Now().UTC(),
		Step:          step,
		Status:        "end",
		ExecutionType: executionType,
		DurationMs:    time.Since(started).Milliseconds(),
		OutputFiles:   outputFiles,
	})
}

// StepError logs a failed step with duration and error message.
func (l *ExecutionLogger) StepError(step, executionType string, started time.Time, err error, outputFiles []string) {
	if l == nil {
		return
	}
	_ = l.writeEntry(ExecutionLogEntry{
		Timestamp:     time.Now().UTC(),
		Step:          step,
		Status:        "error",
		ExecutionType: executionType,
		DurationMs:    time.Since(started).Milliseconds(),
		OutputFiles:   outputFiles,
		Error:         err.Error(),
	})
}

// Info logs a general informational message.
func (l *ExecutionLogger) Info(step, executionType, message string) {
	if l == nil {
		return
	}
	_ = l.writeEntry(ExecutionLogEntry{
		Timestamp:     time.Now().UTC(),
		Step:          step,
		Status:        "info",
		ExecutionType: executionType,
		Message:       message,
	})
}

// SafeStepEnd is a convenience wrapper that recovers from panics and logs accordingly.
func (l *ExecutionLogger) SafeStepEnd(step, executionType string, started time.Time, outputFiles []string) {
	if r := recover(); r != nil {
		err := fmt.Errorf("panic: %v", r)
		l.StepError(step, executionType, started, err, outputFiles)
		panic(r)
	}
	l.StepEnd(step, executionType, started, outputFiles)
}
