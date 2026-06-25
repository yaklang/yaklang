package loop_ssa_api_discovery

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/loop_ssa_api_discovery/store"
	"github.com/yaklang/yaklang/common/log"
)

const defaultPipelineLogMaxBytes = 20 * 1024 * 1024

// PipelineLogger appends structured execution logs to workDir/ssa_discovery/pipeline.log.
type PipelineLogger struct {
	workDir string
	mu      sync.Mutex
}

// NewPipelineLogger creates a PipelineLogger for the given workDir.
// It ensures the parent directory exists but does not truncate an existing log.
func NewPipelineLogger(workDir string) *PipelineLogger {
	if workDir == "" {
		workDir = "."
	}
	_ = os.MkdirAll(filepath.Join(workDir, store.SubDirName()), 0o755)
	return &PipelineLogger{workDir: workDir}
}

// path returns the log file path.
func (l *PipelineLogger) path() string {
	return store.PipelineLogPath(l.workDir)
}

// Writef writes a formatted message with the given level.
func (l *PipelineLogger) Writef(level, format string, args ...any) {
	if l == nil {
		return
	}
	msg := fmt.Sprintf(format, args...)
	line := fmt.Sprintf("%s [%s] %s\n", time.Now().Format(time.RFC3339), level, msg)
	if err := l.append(line); err != nil {
		log.Warnf("pipeline.log write failed: %v", err)
	}
}

// Infof writes an info-level log entry.
func (l *PipelineLogger) Infof(format string, args ...any) {
	l.Writef("INFO", format, args...)
}

// Warnf writes a warning-level log entry.
func (l *PipelineLogger) Warnf(format string, args ...any) {
	l.Writef("WARN", format, args...)
}

// Errorf writes an error-level log entry.
func (l *PipelineLogger) Errorf(format string, args ...any) {
	l.Writef("ERROR", format, args...)
}

// Debugf writes a debug-level log entry.
func (l *PipelineLogger) Debugf(format string, args ...any) {
	l.Writef("DEBUG", format, args...)
}

// StageBegin writes a stage begin marker.
func (l *PipelineLogger) StageBegin(stage, detail string) {
	l.Infof("== STAGE BEGIN: %s | %s ==", stage, detail)
}

// StageEnd writes a stage end marker with duration.
func (l *PipelineLogger) StageEnd(stage string, started time.Time, err error) {
	if err != nil {
		l.Errorf("== STAGE END: %s | FAILED after %s | %v ==", stage, time.Since(started).Round(time.Millisecond), err)
		return
	}
	l.Infof("== STAGE END: %s | OK after %s ==", stage, time.Since(started).Round(time.Millisecond))
}

// append writes a single line to the log file, rotating when it exceeds max size.
func (l *PipelineLogger) append(line string) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	p := l.path()
	if err := l.rotateIfNeeded(p); err != nil {
		return err
	}

	f, err := os.OpenFile(p, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()

	_, writeErr := f.WriteString(line)
	return writeErr
}

func (l *PipelineLogger) rotateIfNeeded(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return nil
	}
	if info.Size() < defaultPipelineLogMaxBytes {
		return nil
	}

	rotated := path + ".1"
	_ = os.Remove(rotated)
	return os.Rename(path, rotated)
}
