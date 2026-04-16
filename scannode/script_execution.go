package scannode

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

type ScriptExecutionRequest struct {
	TaskID          string
	RuntimeID       string
	SubTaskID       string
	ScriptContent   string
	ScriptJSONParam string
	ScriptLabels    map[string]string
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
	s.manager.Add(taskID, newScriptTask(
		taskCtx,
		cancel,
		taskID,
		input.TaskID,
		input.SubTaskID,
		input.RuntimeID,
	))
	defer s.manager.Remove(taskID)

	reporter := NewScannerAgentReporter(
		input.TaskID,
		input.SubTaskID,
		input.RuntimeID,
		legionJobExecutionRefFromContext(taskCtx),
		s,
	)
	keyValues := s.parseScriptParams(input.ScriptJSONParam)
	reporter.ssaUploadCfg = extractSSAArtifactUploadConfig(keyValues)
	reporter.ssaCollector = NewSSAArtifactCollector(input.TaskID, input.RuntimeID, input.SubTaskID)
	if reporter.ssaCollector != nil {
		defer reporter.ssaCollector.Cleanup()
	}
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
	if err := s.executeScript(taskCtx, scanNodePath, scriptFile, params, input.RuntimeID); err != nil {
		logReporterEventError("final progress checkpoint", reporter.flushLatestJobProgress())
		return nil, s.handleScriptFailure(err, result, taskID)
	}
	logReporterEventError("final progress checkpoint", reporter.flushSuccessfulJobProgress())
	if err := s.finalizeSSAArtifactUpload(reporter, result); err != nil {
		return nil, err
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
		params = append(params, "--"+name, utils.InterfaceToString(value))
	}
	return params
}

func (s *ScanNode) createTempScriptFile(content string) (string, error) {
	f, err := consts.TempFile("distributed-yakcode-*.yak")
	if err != nil {
		return "", err
	}
	defer f.Close()

	if _, err := f.WriteString(content); err != nil {
		return "", err
	}
	return f.Name(), nil
}

func (s *ScanNode) executeScript(
	ctx context.Context,
	scanNodePath string,
	scriptFile string,
	params []string,
	runtimeID string,
) error {
	baseCmd := []string{"distyak", scriptFile}
	log.Infof("yak %v %v", scriptFile, params)

	cmd := exec.CommandContext(ctx, scanNodePath, append(baseCmd, params...)...)
	cmd.Env = append(os.Environ(),
		fmt.Sprintf("YAKIT_HOME=%v", os.Getenv("YAKIT_HOME")),
		fmt.Sprintf("YAK_RUNTIME_ID=%v", runtimeID),
	)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func (s *ScanNode) finalizeSSAArtifactUpload(
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

	build, err := reporter.ssaCollector.FinalizeUpload(cfg)
	if err != nil {
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
	return json.Marshal(map[string]int64{
		"risk_count":     event.RiskCount,
		"file_count":     event.FileCount,
		"dataflow_count": event.FlowCount,
	})
}
