package scannode

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime/pprof"
	"strings"
	"time"

	"github.com/yaklang/yaklang/common/log"
)

// DebugCollector is DEPRECATED. The scan node no longer uses a separate
// profiler — debug profiling is handled by yak code-scan's built-in
// pprof collector (started via syntaxflow_scan.Scan() when debug_dir is set).
// This type is retained for test compatibility but its Start/StopProfiling
// methods should NOT be called in production code as they would start a
// second CPU profiler that conflicts with the built-in one.
//
// DebugCollector orchestrates per-task debug profiling: CPU profile,
// heap profile, task log capture, and a timing summary. It uploads all
// debug artifacts to MinIO and reports them via JobArtifactReady events.
type DebugCollector struct {
	taskID    string
	runtimeID string
	subTaskID string

	dir        string
	cpuFile    *os.File
	heapFile   *os.File
	logFile    *os.File
	logWriter  io.Writer
	startedAt  time.Time
	finishedAt time.Time

	closed bool
}

// debugTimingSummary is the JSON structure written to the timing artifact.
type debugTimingSummary struct {
	TaskID     string `json:"task_id"`
	SubTaskID  string `json:"subtask_id"`
	AttemptID  string `json:"attempt_id"`
	StartedAt  string `json:"started_at"`
	FinishedAt string `json:"finished_at"`
	DurationMS int64  `json:"duration_ms"`
	Phase      string `json:"phase,omitempty"`
}

// NewDebugCollector creates a DebugCollector and prepares a temporary
// directory for debug artifacts. Call Close to release resources.
func NewDebugCollector(taskID, runtimeID, subTaskID string) (*DebugCollector, error) {
	dir, err := os.MkdirTemp("", "legion-debug-*")
	if err != nil {
		return nil, fmt.Errorf("create debug dir: %w", err)
	}
	return &DebugCollector{
		taskID:    taskID,
		runtimeID: runtimeID,
		subTaskID: subTaskID,
		dir:       dir,
	}, nil
}

// Start begins CPU profiling and opens the task log file. It must be
// called before the scan script executes.
func (dc *DebugCollector) Start() error {
	if dc == nil {
		return nil
	}
	cpuPath := filepath.Join(dc.dir, "cpu.prof")
	cpuFile, err := os.Create(cpuPath)
	if err != nil {
		return fmt.Errorf("create cpu profile: %w", err)
	}
	if err := pprof.StartCPUProfile(cpuFile); err != nil {
		_ = cpuFile.Close()
		return fmt.Errorf("start cpu profile: %w", err)
	}
	dc.cpuFile = cpuFile

	logPath := filepath.Join(dc.dir, "task.log")
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		return fmt.Errorf("create debug log file: %w", err)
	}
	dc.logFile = logFile
	dc.logWriter = logFile

	dc.startedAt = time.Now()
	log.Infof("[debug] profiling started for task=%s attempt=%s dir=%s", dc.taskID, dc.runtimeID, dc.dir)
	return nil
}

// LogWriter returns the writer for capturing task logs. It is nil before
// Start is called.
func (dc *DebugCollector) LogWriter() io.Writer {
	if dc == nil {
		return nil
	}
	return dc.logWriter
}

// StopProfiling stops CPU profiling and writes a heap profile. It should
// be called immediately after the scan script finishes.
func (dc *DebugCollector) StopProfiling() error {
	if dc == nil || dc.closed {
		return nil
	}
	dc.closed = true
	dc.finishedAt = time.Now()

	if dc.cpuFile != nil {
		pprof.StopCPUProfile()
		_ = dc.cpuFile.Close()
	}

	heapPath := filepath.Join(dc.dir, "heap.prof")
	heapFile, err := os.Create(heapPath)
	if err != nil {
		return fmt.Errorf("create heap profile: %w", err)
	}
	defer heapFile.Close()
	if err := pprof.WriteHeapProfile(heapFile); err != nil {
		return fmt.Errorf("write heap profile: %w", err)
	}
	dc.heapFile = heapFile

	if dc.logFile != nil {
		_ = dc.logFile.Sync()
		_ = dc.logFile.Close()
	}

	log.Infof("[debug] profiling finished for task=%s duration=%v", dc.taskID, dc.finishedAt.Sub(dc.startedAt))
	return nil
}

// WriteTimingSummary writes the timing summary JSON file.
func (dc *DebugCollector) WriteTimingSummary(phase string) error {
	if dc == nil {
		return nil
	}
	summary := debugTimingSummary{
		TaskID:     dc.taskID,
		SubTaskID:  dc.subTaskID,
		AttemptID:  dc.runtimeID,
		StartedAt:  dc.startedAt.UTC().Format(time.RFC3339Nano),
		FinishedAt: dc.finishedAt.UTC().Format(time.RFC3339Nano),
		DurationMS: dc.finishedAt.Sub(dc.startedAt).Milliseconds(),
		Phase:      strings.TrimSpace(phase),
	}
	data, err := json.MarshalIndent(summary, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal timing summary: %w", err)
	}
	path := filepath.Join(dc.dir, "timing.json")
	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("write timing summary: %w", err)
	}
	return nil
}

// Artifact describes a single debug artifact file ready for upload.
type debugArtifact struct {
	Kind     string // debug_pprof_cpu | debug_pprof_heap | debug_log | debug_timing
	FilePath string
	FileName string
}

// Artifacts returns the list of debug artifact files produced by this collector.
func (dc *DebugCollector) Artifacts() []debugArtifact {
	if dc == nil || dc.dir == "" {
		return nil
	}
	var artifacts []debugArtifact
	entries := []struct {
		kind     string
		filename string
	}{
		{"debug_pprof_cpu", "cpu.prof"},
		{"debug_pprof_heap", "heap.prof"},
		{"debug_log", "task.log"},
		{"debug_timing", "timing.json"},
	}
	for _, e := range entries {
		path := filepath.Join(dc.dir, e.filename)
		if _, err := os.Stat(path); err == nil {
			artifacts = append(artifacts, debugArtifact{
				Kind:     e.kind,
				FilePath: path,
				FileName: e.filename,
			})
		}
	}
	return artifacts
}

// Cleanup removes the temporary debug directory. Safe to call after
// artifacts have been uploaded.
func (dc *DebugCollector) Cleanup() {
	if dc == nil {
		return
	}
	if dc.dir != "" {
		_ = os.RemoveAll(dc.dir)
	}
}

// computeSHA256 computes the SHA-256 hash of a file.
func computeSHA256(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// fileSize returns the size of a file in bytes.
func fileSize(path string) int64 {
	st, err := os.Stat(path)
	if err != nil {
		return 0
	}
	return st.Size()
}

// debugObjectKey builds an S3 object key for a debug artifact.
func debugObjectKey(taskID, attemptID, kind, filename string) string {
	return fmt.Sprintf("debug/%s/%s/%s/%s", taskID, attemptID, kind, filename)
}

// isDebugEnabled checks whether the debug_enabled label is set to "true".
func isDebugEnabled(labels map[string]string) bool {
	if labels == nil {
		return false
	}
	return strings.EqualFold(strings.TrimSpace(labels["debug_enabled"]), "true")
}

// uploadDebugArtifacts uploads all debug artifact files to MinIO and
// publishes JobArtifactReady events for each. Failures are logged but
// do not affect the scan result.
func (s *ScanNode) uploadDebugArtifacts(
	ctx context.Context,
	reporter *ScannerAgentReporter,
	dc *DebugCollector,
) {
	if dc == nil {
		return
	}
	artifacts := dc.Artifacts()
	if len(artifacts) == 0 {
		return
	}

	for _, art := range artifacts {
		objKey := debugObjectKey(dc.taskID, dc.runtimeID, art.Kind, art.FileName)
		sha, err := computeSHA256(art.FilePath)
		if err != nil {
			log.Warnf("[debug] compute sha256 for %s: %v", art.FileName, err)
			sha = ""
		}
		size := fileSize(art.FilePath)

		// Upload via the SSA upload config if available; otherwise log a warning.
		cfg := reporter.ssaUploadCfg
		if cfg == nil {
			log.Warnf("[debug] no upload config available, skipping upload of %s", art.FileName)
			continue
		}

		provider := s.buildDebugUploadConfigProvider(ctx, reporter, cfg, objKey)
		if err := uploadDebugArtifactFile(ctx, art.FilePath, size, objKey, provider); err != nil {
			log.Warnf("[debug] upload %s failed: %v", art.FileName, err)
			continue
		}

		// Publish JobArtifactReady event
		if err := reporter.PublishArtifactReady(ctx, art.Kind, "", objKey, "", sha, uint64(size), uint64(size), nil); err != nil {
			log.Warnf("[debug] publish artifact ready for %s: %v", art.FileName, err)
		}
		log.Infof("[debug] uploaded artifact kind=%s key=%s size=%d sha=%s", art.Kind, objKey, size, sha)
	}
}

// buildDebugUploadConfigProvider returns a provider that clones the SSA
// upload config but overrides the ObjectKey for debug artifacts.
func (s *ScanNode) buildDebugUploadConfigProvider(
	ctx context.Context,
	reporter *ScannerAgentReporter,
	baseCfg *SSAArtifactUploadConfig,
	objKey string,
) ssaUploadConfigProvider {
	baseProvider := s.buildSSAArtifactUploadConfigProvider(ctx, reporter, baseCfg)
	return func(force bool) (*SSAArtifactUploadConfig, error) {
		cfg, err := baseProvider(force)
		if err != nil {
			return nil, err
		}
		cp := *cfg
		cp.ObjectKey = objKey
		return &cp, nil
	}
}

// uploadDebugArtifactFile uploads a file with a specific object key.
func uploadDebugArtifactFile(ctx context.Context, path string, size int64, objKey string, provider ssaUploadConfigProvider) error {
	tmp := &SSAArtifactCollector{}
	return tmp.UploadBySTSWithProviderContext(ctx, path, size, func(force bool) (*SSAArtifactUploadConfig, error) {
		cfg, err := provider(force)
		if err != nil {
			return nil, err
		}
		cp := *cfg
		cp.ObjectKey = strings.TrimSpace(objKey)
		return &cp, nil
	})
}

// resolveDebugDir extracts a debug directory path from labels.
// If "debug_dir" label is set, it is used directly ( Legion-generated path ).
// Otherwise returns empty — the node will generate its own path.
func resolveDebugDir(labels map[string]string) string {
	if labels == nil {
		return ""
	}
	return strings.TrimSpace(labels["debug_dir"])
}

// computeSHA256FromBytes computes the SHA-256 hash of a byte slice.
func computeSHA256FromBytes(data []byte) (string, error) {
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:]), nil
}

// uploadDebugArtifactBytes uploads a byte slice with a specific object key.
func uploadDebugArtifactBytes(ctx context.Context, payload []byte, objKey string, provider ssaUploadConfigProvider) error {
	tmpFile, err := os.CreateTemp("", "debug-analysis-*.json")
	if err != nil {
		return err
	}
	path := tmpFile.Name()
	defer os.Remove(path)
	if _, err := tmpFile.Write(payload); err != nil {
		_ = tmpFile.Close()
		return err
	}
	if err := tmpFile.Close(); err != nil {
		return err
	}
	return uploadDebugArtifactFile(ctx, path, int64(len(payload)), objKey, provider)
}
