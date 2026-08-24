package scannode

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"github.com/google/uuid"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssagitworkdir"
	pluginv1 "github.com/yaklang/yaklang/scannode/gen/legionpb/legion/plugin/v1"
)

type ScriptExecutionRequest struct {
	TaskID          string
	RuntimeID       string
	SubTaskID       string
	ScriptContent   string
	ScriptJSONParam string
	ScriptLabels    map[string]string
	PluginBundle    *pluginv1.PluginBundleRef
	DebugEnabled    bool
	DebugDir        string
}

type ScriptExecutionResult struct {
	Data any `json:"data,omitempty"`
}

func (s *ScanNode) executeScriptTask(
	ctx context.Context,
	input ScriptExecutionRequest,
) (*ScriptExecutionResult, error) {
	if strings.TrimSpace(input.ScriptContent) == "" {
		return nil, utils.Error("empty script_content")
	}

	taskID := taskIDForSubtask(input.SubTaskID)
	taskCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	if !s.manager.Add(taskID, newScriptTask(
		taskCtx,
		cancel,
		taskID,
		input.TaskID,
		input.SubTaskID,
		input.RuntimeID,
	)) {
		return nil, utils.Error("scan node is shutting down")
	}
	defer s.manager.Remove(taskID)

	reporter := NewScannerAgentReporter(
		input.TaskID,
		input.SubTaskID,
		input.RuntimeID,
		legionJobExecutionRefFromContext(taskCtx),
		s,
	)
	keyValues := s.parseScriptParams(input.ScriptJSONParam)
	// plugin_bundle_path is a platform-owned local capability. Never honor a
	// job-provided path, even when no immutable bundle reference was dispatched.
	delete(keyValues, "plugin_bundle_path")
	cleanupSourcePayload, err := s.prepareManagedSourcePayload(taskCtx, keyValues)
	if err != nil {
		return nil, err
	}
	defer cleanupSourcePayload()
	pluginBundlePath, err := s.preparePluginBundle(taskCtx, input.PluginBundle)
	if err != nil {
		return nil, err
	}
	if pluginBundlePath != "" {
		keyValues["plugin_bundle_path"] = pluginBundlePath
	}
	reporter.ssaUploadCfg = extractSSAArtifactUploadConfig(keyValues)
	reporter.ssaCollector = NewSSAArtifactCollector(input.TaskID, input.RuntimeID, input.SubTaskID)
	if reporter.ssaCollector != nil {
		defer reporter.ssaCollector.Cleanup()
	}
	ssaDBEnv := extractSSADatabaseEnv(keyValues)
	result := &ScriptExecutionResult{}
	yakitServer := s.createYakitServer(reporter, result)
	yakitServer.Start()
	defer yakitServer.Shutdown()

	s.syncRulesIfNeeded(taskCtx, keyValues, input.ScriptLabels)

	scriptFile, err := s.createTempScriptFile(input.ScriptContent)
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(scriptFile)

	params := s.buildScriptParams(yakitServer.Addr(), input.RuntimeID, keyValues)
	scanNodePath, err := os.Executable()
	if err != nil {
		return nil, utils.Errorf("fetch node path err: %s", err)
	}
	taskLogWriter, taskLogClose := openTaskLogWriter(s, input.TaskID, input.SubTaskID, input.RuntimeID)
	defer taskLogClose()

	// --- Debug directory setup ---
	// When debug is enabled, create a unique directory for this run.
	// The yak script (via ssa.withDebugDir) and the pprof collector will
	// write profiling data, logs, and SSA database to this directory.
	// We do NOT start a separate profiler here — the pprof collector
	// is started by syntaxflow_scan.Scan() when it consumes the debug_dir.
	debugDir := ""
	if input.DebugEnabled {
		if input.DebugDir != "" {
			debugDir = input.DebugDir
		} else {
			// Generate a unique directory under the node base dir
			baseDir := s.debugBaseDir()
			debugDir = filepath.Join(baseDir, "debug", fmt.Sprintf("%s_%s", sanitizeLogName(input.TaskID), sanitizeLogName(input.RuntimeID)))
		}
		if err := os.MkdirAll(debugDir, 0o755); err != nil {
			log.Warnf("[debug] failed to create debug dir: %v, continuing without debug", err)
			debugDir = ""
		} else {
			log.Infof("[debug] debug directory: %s", debugDir)
		}
	}

	// Inject debug_dir into the script params so the yak script can pass it to StartScan
	if debugDir != "" {
		keyValues["debug_dir"] = debugDir
		params = s.buildScriptParams(yakitServer.Addr(), input.RuntimeID, keyValues)
	}

	// Register a defer to finalize debug artifacts (analysis + zip) on both
	// success and failure paths. The pprof collector (started by Scan() inside
	// the child process) writes its final snapshot during script exit/cleanup.
	// We wait briefly for the child process to finish writing, then analyze.
	debugFinalized := false
	if debugDir != "" {
		defer func() {
			if debugFinalized {
				return
			}
			s.finalizeDebugRun(taskCtx, reporter, debugDir, "unknown")
		}()
	}

	if err := s.executeScript(taskCtx, scanNodePath, scriptFile, params, input.RuntimeID, ssaDBEnv, taskLogWriter); err != nil {
		logReporterEventError("final progress checkpoint", reporter.flushLatestJobProgress())
		// Finalize debug before returning the failure
		if debugDir != "" {
			s.finalizeDebugRun(taskCtx, reporter, debugDir, "failed")
			debugFinalized = true
		}
		return nil, s.handleScriptFailure(err, result, taskID)
	}
	logReporterEventError("final progress checkpoint", reporter.flushSuccessfulJobProgress())
	if err := s.finalizeSSAArtifactUpload(taskCtx, reporter, result); err != nil {
		if debugDir != "" {
			s.finalizeDebugRun(taskCtx, reporter, debugDir, "failed")
			debugFinalized = true
		}
		return nil, err
	}

	// Finalize debug artifacts on success path
	if debugDir != "" {
		s.finalizeDebugRun(taskCtx, reporter, debugDir, "succeeded")
		debugFinalized = true
	}
	return result, nil
}

func newScriptTask(
	ctx context.Context,
	cancel context.CancelFunc,
	taskID string,
	jobID string,
	subtaskID string,
	attemptID string,
) *Task {
	return &Task{
		TaskType:  "script-task",
		TaskId:    taskID,
		JobID:     jobID,
		SubtaskID: subtaskID,
		AttemptID: attemptID,
		Ctx:       ctx,
		Cancel:    cancel,
	}
}

func (s *ScanNode) buildScriptParams(
	webhookAddr string,
	runtimeID string,
	keyValues map[string]any,
) []string {
	params := buildScriptBaseParams(webhookAddr, runtimeID)
	return s.appendKeyValueParams(params, keyValues)
}

func (s *ScanNode) handleScriptFailure(
	err error,
	result *ScriptExecutionResult,
	taskID string,
) error {
	if err == nil {
		return nil
	}
	if reason := s.cancelReasonForTask(taskID); reason != "" {
		return &TaskCancelledError{Reason: reason}
	}
	if errors.Is(err, context.Canceled) {
		return &TaskCancelledError{}
	}
	// If the error is a scriptExecError, it already carries stderr/stdout
	// tail — use it directly so the failure_message has actionable content.
	var scriptErr *scriptExecError
	if errors.As(err, &scriptErr) {
		return scriptErr
	}
	if detailedError := extractScriptError(result); detailedError != "" {
		return utils.Errorf("%s", detailedError)
	}
	return utils.Errorf("exec yak script failed: %s", err)
}

func (s *ScanNode) cancelReasonForTask(taskID string) string {
	task, err := s.manager.GetTaskById(taskID)
	if err != nil {
		return ""
	}
	return task.CancelReason()
}

func extractScriptError(result *ScriptExecutionResult) string {
	if result == nil || result.Data == nil {
		return ""
	}

	dataMap, ok := result.Data.(map[string]any)
	if !ok {
		return ""
	}
	errMsg, ok := dataMap["error"].(string)
	if !ok || errMsg == "" {
		return ""
	}
	return errMsg
}

func (s *ScanNode) parseScriptParams(jsonParam string) map[string]any {
	params := make(map[string]any)
	if strings.TrimSpace(jsonParam) == "" {
		return params
	}

	var raw any
	if err := json.Unmarshal([]byte(jsonParam), &raw); err != nil {
		return params
	}

	for key, value := range utils.InterfaceToGeneralMap(raw) {
		if key == "__DEFAULT__" {
			continue
		}
		params[key] = value
	}
	return params
}

func (s *ScanNode) syncRulesIfNeeded(
	ctx context.Context,
	params map[string]any,
	labels map[string]string,
) {
	snapshotID := resolveRuleSyncSnapshotID(params, labels)
	if snapshotID == "" {
		return
	}

	if s == nil || s.ruleSyncClient == nil || s.ruleSyncClient.HasLocalSnapshot(snapshotID) {
		return
	}

	log.Infof("auto-syncing rules for snapshot: %s", snapshotID)
	ruleCount, err := s.ruleSyncClient.SyncSnapshot(ctx, snapshotID)
	if err != nil {
		log.Warnf("auto-sync rules failed: %v, will continue with local rules", err)
		return
	}
	log.Infof("auto-synced %d rules from snapshot %s", ruleCount, snapshotID)
}

func resolveRuleSyncSnapshotID(params map[string]any, labels map[string]string) string {
	if labels != nil {
		if snapshotID := strings.TrimSpace(labels["rule_snapshot_id"]); snapshotID != "" {
			return snapshotID
		}
	}
	if snapshotID, ok := params["rule_snapshot_id"].(string); ok {
		return strings.TrimSpace(snapshotID)
	}
	return ""
}

func buildScriptBaseParams(webhookAddr string, runtimeID string) []string {
	params := []string{"--yakit-webhook", webhookAddr}
	if runtimeID != "" {
		params = append(params, "--runtime_id", runtimeID)
	}
	return params
}

func (s *ScanNode) appendKeyValueParams(params []string, keyValues map[string]any) []string {
	for key, value := range keyValues {
		if strings.HasPrefix(strings.TrimSpace(key), scannodeInternalParamPrefix) {
			continue
		}
		name := strings.TrimLeft(key, "-")
		params = appendCLIParamValue(params, "--"+name, value)
	}
	return params
}

func appendCLIParamValue(params []string, flag string, value any) []string {
	switch typed := value.(type) {
	case []string:
		for _, item := range typed {
			params = append(params, flag, item)
		}
	case []any:
		for _, item := range typed {
			params = append(params, flag, utils.InterfaceToString(item))
		}
	default:
		params = append(params, flag, utils.InterfaceToString(value))
	}
	return params
}

// debugBaseDir returns the base directory for debug run data.
// Defaults to the node base directory; can be overridden via
// SCANNODE_DEBUG_BASE_DIR environment variable.
func (s *ScanNode) debugBaseDir() string {
	if env := os.Getenv("SCANNODE_DEBUG_BASE_DIR"); env != "" {
		return env
	}
	if s != nil && s.node != nil {
		return filepath.Join(s.node.BaseDir(), "debug-runs")
	}
	return filepath.Join(os.TempDir(), "legion-debug-runs")
}

func (s *ScanNode) createTempScriptFile(content string) (string, error) {
	f, err := createDistributedScriptTempFile()
	if err != nil {
		return "", err
	}
	defer f.Close()

	if _, err := f.WriteString(content); err != nil {
		return "", err
	}
	return f.Name(), nil
}

func createDistributedScriptTempFile() (*os.File, error) {
	const pattern = "distributed-yakcode-*.yak"

	f, err := consts.TempFile(pattern)
	if err == nil {
		return f, nil
	}
	yakitTempErr := err

	f, err = os.CreateTemp("", pattern)
	if err != nil {
		return nil, utils.Errorf(
			"create distributed script temp file failed: yakit temp: %v; system temp: %v",
			yakitTempErr,
			err,
		)
	}
	log.Warnf("fallback to system temp for distributed script: yakit temp unavailable: %v", yakitTempErr)
	return f, nil
}

// scriptExecError wraps an exec.ExitError with captured stderr/stdout
// tail so the failure message carries actionable diagnostics instead of
// just "exit status 1".
type scriptExecError struct {
	*exec.ExitError
	stderrTail string
	stdoutTail string
}

func (e *scriptExecError) Error() string {
	msg := fmt.Sprintf("exec yak script failed: %s", e.ExitError.Error())
	if e.stderrTail != "" {
		msg += "\n--- stderr (last 2KB) ---\n" + e.stderrTail
	}
	if e.stdoutTail != "" {
		msg += "\n--- stdout (last 1KB) ---\n" + e.stdoutTail
	}
	return msg
}

func (s *ScanNode) executeScript(
	ctx context.Context,
	scanNodePath string,
	scriptFile string,
	params []string,
	runtimeID string,
	extraEnv []string,
	taskLogWriter io.Writer,
) error {
	baseCmd := []string{"distyak", scriptFile}
	log.Infof("yak %v %v", scriptFile, params)

	cmd := exec.CommandContext(ctx, scanNodePath, append(baseCmd, params...)...)
	env := append(os.Environ(),
		fmt.Sprintf("YAKIT_HOME=%v", os.Getenv("YAKIT_HOME")),
		fmt.Sprintf("YAK_RUNTIME_ID=%v", runtimeID),
	)
	env = append(env, extraEnv...)
	workspaceOwner := s.nextSSAGitWorkspaceOwner()
	env = replaceEnvironmentValue(env, ssagitworkdir.OwnerEnv, workspaceOwner)
	cmd.Env = env

	// Use a combined output writer: stdout/stderr go to both the parent
	// process (for docker logs) and the per-task log file. We also capture
	// the tail of stderr for inclusion in the failure message.
	stderrBuf := newTailBuffer(2048)
	stdoutBuf := newTailBuffer(1024)

	cmd.Stdout = io.MultiWriter(os.Stdout, stdoutBuf)
	cmd.Stderr = io.MultiWriter(os.Stderr, stderrBuf)
	if taskLogWriter != nil {
		cmd.Stdout = io.MultiWriter(os.Stdout, stdoutBuf, taskLogWriter)
		cmd.Stderr = io.MultiWriter(os.Stderr, stderrBuf, taskLogWriter)
	}

	if err := cmd.Start(); err != nil {
		return err
	}
	childPID := cmd.Process.Pid
	waitErr := cmd.Wait()
	if waitErr != nil {
		// Preserve the latest-main diagnostics while still waiting before the
		// parent-owned workspace sweep below.
		var exitErr *exec.ExitError
		if errors.As(waitErr, &exitErr) {
			waitErr = &scriptExecError{
				ExitError:  exitErr,
				stderrTail: stderrBuf.String(),
				stdoutTail: stdoutBuf.String(),
			}
		}
	}
	cleanupErr := ssagitworkdir.CleanupForOwner(workspaceOwner)
	if cleanupErr != nil {
		if waitErr == nil {
			return utils.Errorf("cleanup SSA Git workspaces for child process %d: %v", childPID, cleanupErr)
		}
		log.Errorf("cleanup SSA Git workspaces for failed child process %d: %v", childPID, cleanupErr)
	}
	return waitErr
}

func (s *ScanNode) nextSSAGitWorkspaceOwner() string {
	scope := "process-" + strconv.Itoa(os.Getpid())
	if s != nil && strings.TrimSpace(s.ssaGitOwnerScope) != "" {
		scope = s.ssaGitOwnerScope
	}
	return scope + "-task-" + strings.ReplaceAll(uuid.NewString(), "-", "")
}

func replaceEnvironmentValue(env []string, key string, value string) []string {
	prefix := key + "="
	replaced := make([]string, 0, len(env)+1)
	for _, item := range env {
		if strings.HasPrefix(item, prefix) {
			continue
		}
		replaced = append(replaced, item)
	}
	return append(replaced, prefix+value)
}

// tailBuffer is a ring buffer that keeps the last N bytes written to it.
type tailBuffer struct {
	buf []byte
	max int
}

func newTailBuffer(max int) *tailBuffer {
	return &tailBuffer{max: max}
}

func (b *tailBuffer) Write(p []byte) (int, error) {
	b.buf = append(b.buf, p...)
	if len(b.buf) > b.max {
		b.buf = b.buf[len(b.buf)-b.max:]
	}
	return len(p), nil
}

func (b *tailBuffer) String() string {
	return string(b.buf)
}

// openTaskLogWriter opens a per-task log file for capturing stdout/stderr
// of the yak script subprocess. By default it writes to
// <node-base-dir>/logs/ unless SCANNODE_TASK_LOG_DIR overrides it.
// This ensures every task execution has a persistent log file for
// post-mortem diagnosis, not just when the env var is manually set.
//
// File name format: <JobID>_<SubTaskID>_<AttemptID>.log
// On failure (dir not writable), it degrades to nil and logs a warning.
func openTaskLogWriter(s *ScanNode, jobID, subTaskID, runtimeID string) (io.Writer, func()) {
	dir := strings.TrimSpace(os.Getenv("SCANNODE_TASK_LOG_DIR"))
	if dir == "" {
		// Default: node base dir / logs
		if s != nil && s.node != nil {
			dir = filepath.Join(s.node.BaseDir(), "logs")
		} else {
			dir = filepath.Join(os.TempDir(), "legion-node-logs")
		}
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		log.Warnf("create task log dir %s failed: %v", dir, err)
		return nil, func() {}
	}
	name := fmt.Sprintf("%s_%s_%s.log",
		sanitizeLogName(jobID),
		sanitizeLogName(subTaskID),
		sanitizeLogName(runtimeID),
	)
	path := filepath.Join(dir, name)
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		log.Warnf("open per-task log file failed (fallback to stdout only): %s: %v", path, err)
		return nil, func() {}
	}
	log.Infof("[task-log] writing to: %s", path)
	return f, func() { _ = f.Close() }
}

// sanitizeLogName 把任务标识里的路径分隔符/空字符等替换为 "_"，避免注入
// 到 per-task 日志文件名中造成路径穿越或文件名异常。
func sanitizeLogName(s string) string {
	if s == "" {
		return "_"
	}
	r := strings.NewReplacer("/", "_", "\\", "_", string(os.PathSeparator), "_", "\x00", "_")
	out := r.Replace(s)
	out = strings.TrimSpace(out)
	if out == "" {
		return "_"
	}
	return out
}

// classifyUploadError maps an upload error message to a structured error code
// for the SSAArtifactUploadFailed event.
func classifyUploadError(errMsg string) string {
	lower := strings.ToLower(errMsg)
	if strings.Contains(lower, "deadline exceeded") || strings.Contains(lower, "timeout") {
		if strings.Contains(lower, "ticket") || strings.Contains(lower, "artifact-ticket") {
			return "ticket_timeout"
		}
	}
	credMarkers := []string{"expiredtoken", "expired token", "accessdenied", "access denied", "invalidsecuritytoken", "invalid security"}
	for _, m := range credMarkers {
		if strings.Contains(lower, m) {
			return "sts_expired"
		}
	}
	if strings.Contains(lower, "multipart") || strings.Contains(lower, "complete") {
		return "multipart_failed"
	}
	return "put_failed"
}

func (s *ScanNode) finalizeSSAArtifactUpload(
	ctx context.Context,
	reporter *ScannerAgentReporter,
	result *ScriptExecutionResult,
) error {
	if reporter == nil || reporter.ssaCollector == nil {
		return nil
	}

	meta := parseSSAResultMeta(result)
	cfg := reporter.ssaUploadCfg
	if cfg == nil {
		if reporter.ssaCollector.HasData() {
			return utils.Errorf("ssa artifact upload config missing")
		}
		return nil
	}

	provider := s.buildSSAArtifactUploadConfigProvider(ctx, reporter, cfg)
	build, err := reporter.ssaCollector.FinalizeUploadWithProvider(
		normalizeArtifactCodec(cfg.Codec),
		provider,
	)
	if err != nil {
		s.emitSSAArtifactUploadFailed(reporter, err, "")
		return err
	}
	if build == nil {
		return nil
	}
	if build.ProgramName == "" {
		build.ProgramName = meta.ProgramName
	}

	event := reporter.ssaCollector.BuildReadyEvent(build, meta.TotalLines, meta.RiskCount)
	if event == nil {
		return nil
	}
	if err := reporter.PublishSSAArtifactReady(event); err != nil {
		return err
	}

	log.Infof(
		"ssa artifact uploaded task=%s key=%s codec=%s raw=%d stored=%d risks=%d files=%d flows=%d",
		reporter.TaskId,
		build.ObjectKey,
		build.Codec,
		build.UncompressedSize,
		build.CompressedSize,
		event.RiskCount,
		event.FileCount,
		event.FlowCount,
	)
	return nil
}

// emitSSAArtifactUploadFailed constructs and publishes an upload-failed event
// from the collector's accumulated metrics. The objectKey is the intended
// object key (may be empty if the failure happened before the key was known).
func (s *ScanNode) emitSSAArtifactUploadFailed(
	reporter *ScannerAgentReporter,
	uploadErr error,
	objectKey string,
) {
	if reporter == nil || reporter.ssaCollector == nil || uploadErr == nil {
		return
	}
	errorCode := classifyUploadError(uploadErr.Error())
	metrics := reporter.ssaCollector.snapshotUploadMetrics()
	failedEvent := reporter.ssaCollector.BuildUploadFailedEvent(
		errorCode,
		uploadErr.Error(),
		metrics.CompressedBytes,
	)
	if failedEvent == nil {
		return
	}
	if objectKey != "" {
		failedEvent.ObjectKey = objectKey
	}
	if err := reporter.PublishSSAArtifactUploadFailed(failedEvent); err != nil {
		log.Errorf("publish ssa artifact upload failed event: %v", err)
	}
	log.Infof(
		"ssa artifact upload failed task=%s error_code=%s error=%s uploaded_bytes=%d",
		reporter.TaskId, errorCode, uploadErr.Error(), metrics.CompressedBytes,
	)
}

func (s *ScanNode) buildSSAArtifactUploadConfigProvider(
	ctx context.Context,
	reporter *ScannerAgentReporter,
	baseCfg *SSAArtifactUploadConfig,
) ssaUploadConfigProvider {
	if baseCfg == nil {
		return nil
	}

	current := *baseCfg
	taskID := ""
	if reporter != nil {
		taskID = strings.TrimSpace(reporter.TaskId)
	}

	var mu sync.Mutex
	return func(force bool) (*SSAArtifactUploadConfig, error) {
		mu.Lock()
		defer mu.Unlock()

		if !force && !current.NeedSTSRefresh(600) {
			cp := current
			return &cp, nil
		}

		if s == nil {
			return nil, utils.Errorf("scannode not ready")
		}
		if taskID == "" {
			return nil, utils.Errorf("ssa artifact task id missing")
		}

		objectKey := strings.TrimSpace(current.ObjectKey)
		if objectKey == "" {
			return nil, utils.Errorf("ssa artifact object key missing")
		}

		refreshCtx := ctx
		if refreshCtx == nil {
			refreshCtx = context.Background()
		}
		fresh, err := s.fetchSSAArtifactUploadTicket(refreshCtx, taskID, objectKey)
		if err != nil {
			return nil, err
		}
		if fresh == nil {
			return nil, utils.Errorf("empty upload ticket")
		}
		if strings.TrimSpace(fresh.ObjectKey) == "" {
			fresh.ObjectKey = objectKey
		}
		if strings.TrimSpace(fresh.Codec) == "" {
			fresh.Codec = current.Codec
		}
		current = *fresh

		cp := current
		return &cp, nil
	}
}

type ssaResultMeta struct {
	ProgramName string
	TotalLines  int64
	RiskCount   int64
}

func parseSSAResultMeta(result *ScriptExecutionResult) ssaResultMeta {
	meta := ssaResultMeta{}
	if result == nil || result.Data == nil {
		return meta
	}

	dataMap, ok := result.Data.(map[string]any)
	if !ok || dataMap == nil {
		return meta
	}

	meta.ProgramName = strings.TrimSpace(utils.InterfaceToString(
		utils.MapGetFirstRaw(dataMap, "program_name", "programName", "ProgramName"),
	))
	meta.TotalLines = int64(utils.InterfaceToFloat64(
		utils.MapGetFirstRaw(dataMap, "total_lines", "totalLines", "TotalLines"),
	))
	meta.RiskCount = int64(utils.InterfaceToFloat64(
		utils.MapGetFirstRaw(dataMap, "risk_count", "riskCount", "RiskCount"),
	))
	return meta
}

func buildSSAArtifactMetricsPayload(event *SSAArtifactReadyEvent) ([]byte, error) {
	if event == nil {
		return json.Marshal(map[string]int64{})
	}
	merged := make(map[string]any)
	// Start with the upload metrics from the collector (upload_ms, ticket_fetch_ms, etc.)
	if len(event.Metrics) > 0 {
		_ = json.Unmarshal(event.Metrics, &merged)
	}
	// Add risk/file/flow counts
	merged["risk_count"] = event.RiskCount
	merged["file_count"] = event.FileCount
	merged["dataflow_count"] = event.FlowCount
	return json.Marshal(merged)
}

// finalizeDebugRun analyzes the debug run directory, generates a ZIP archive,
// and publishes both as JobArtifactReady events. Failures are logged but do
// not affect the scan result. This is called on both success and failure paths.
func (s *ScanNode) finalizeDebugRun(
	ctx context.Context,
	reporter *ScannerAgentReporter,
	debugDir string,
	status string,
) {
	// Ensure the debug package contains the actual scan log even when the
	// child pprof collector did not write its own log file.
	mergeTaskLogIntoDebugDir(debugDir)
	s.publishDebugAnalysis(ctx, reporter, debugDir, status)
	s.publishDebugZip(ctx, reporter, debugDir)
}

// publishDebugAnalysis analyzes the debug run directory and publishes the
// structured result as a JobArtifactReady event with artifact_kind="debug_analysis".
// The analysis JSON is uploaded to MinIO and the event carries the object key.
// Failures are logged but do not affect the scan result.
func (s *ScanNode) publishDebugAnalysis(
	ctx context.Context,
	reporter *ScannerAgentReporter,
	debugDir string,
	status string,
) {
	analysis := AnalyzeDebugRunWithStatus(debugDir, status)
	analysisJSON, err := json.Marshal(analysis)
	if err != nil {
		log.Warnf("[debug] marshal analysis failed: %v", err)
		return
	}

	// Upload analysis JSON to MinIO
	cfg := reporter.ssaUploadCfg
	if cfg == nil {
		log.Warnf("[debug] no upload config available, skipping analysis upload")
		return
	}

	// Use debug_analysis/<taskID>/<runID>/analysis.json as object key
	taskID := strings.TrimSpace(reporter.TaskId)
	attemptID := strings.TrimSpace(reporter.RuntimeId)
	objKey := fmt.Sprintf("debug_analysis/%s/%s/analysis.json", taskID, attemptID)

	provider := s.buildDebugUploadConfigProvider(ctx, reporter, cfg, objKey)
	if err := uploadDebugArtifactBytes(analysisJSON, objKey, provider); err != nil {
		log.Warnf("[debug] upload analysis failed: %v", err)
		return
	}

	// Publish JobArtifactReady event
	sha, _ := computeSHA256FromBytes(analysisJSON)
	size := int64(len(analysisJSON))
	if err := reporter.PublishArtifactReady(ctx, "debug_analysis", "json", objKey, "", sha, uint64(size), uint64(size), nil); err != nil {
		log.Warnf("[debug] publish analysis artifact ready failed: %v", err)
		return
	}

	log.Infof("[debug] analysis published: key=%s size=%d samples=%d", objKey, size, len(analysis.Samples))
}

// publishDebugZip generates a ZIP archive of the debug run directory and
// uploads it to MinIO, then publishes a JobArtifactReady event with
// artifact_kind="debug_zip". Failures are logged but do not affect the scan.
func (s *ScanNode) publishDebugZip(
	ctx context.Context,
	reporter *ScannerAgentReporter,
	debugDir string,
) {
	zipPath, err := GenerateDebugZip(debugDir)
	if err != nil {
		log.Warnf("[debug] generate zip failed: %v", err)
		return
	}
	defer os.Remove(zipPath)

	cfg := reporter.ssaUploadCfg
	if cfg == nil {
		log.Warnf("[debug] no upload config available, skipping zip upload")
		return
	}

	taskID := strings.TrimSpace(reporter.TaskId)
	attemptID := strings.TrimSpace(reporter.RuntimeId)
	objKey := fmt.Sprintf("debug_zip/%s/%s/run.zip", taskID, attemptID)

	provider := s.buildDebugUploadConfigProvider(ctx, reporter, cfg, objKey)
	zipSize := fileSize(zipPath)
	if err := uploadDebugArtifactFile(zipPath, zipSize, objKey, provider); err != nil {
		log.Warnf("[debug] upload zip failed: %v", err)
		return
	}

	sha, _ := computeSHA256(zipPath)
	if err := reporter.PublishArtifactReady(ctx, "debug_zip", "zip", objKey, "", sha, uint64(zipSize), uint64(zipSize), nil); err != nil {
		log.Warnf("[debug] publish zip artifact ready failed: %v", err)
		return
	}

	log.Infof("[debug] zip published: key=%s size=%d", objKey, zipSize)
}
