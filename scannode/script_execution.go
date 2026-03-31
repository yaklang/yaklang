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
	s.manager.Add(taskID, newScriptTask(taskCtx, cancel, taskID))
	defer s.manager.Remove(taskID)

	reporter := NewScannerAgentReporter(
		input.TaskID,
		input.SubTaskID,
		input.RuntimeID,
		legionJobExecutionRefFromContext(taskCtx),
		s,
	)
	result := &ScriptExecutionResult{}
	yakitServer := s.createYakitServer(reporter, result)
	yakitServer.Start()
	defer yakitServer.Shutdown()

	scriptFile, err := s.createTempScriptFile(input.ScriptContent)
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(scriptFile)

	params := s.buildScriptParams(yakitServer.Addr(), input)
	scanNodePath, err := os.Executable()
	if err != nil {
		return nil, utils.Errorf("fetch node path err: %s", err)
	}
	if err := s.executeScript(taskCtx, scanNodePath, scriptFile, params, input.RuntimeID); err != nil {
		return nil, s.handleScriptFailure(err, result, taskID)
	}
	return result, nil
}

func newScriptTask(
	ctx context.Context,
	cancel context.CancelFunc,
	taskID string,
) *Task {
	return &Task{
		TaskType: "script-task",
		TaskId:   taskID,
		Ctx:      ctx,
		Cancel:   cancel,
	}
}

func (s *ScanNode) buildScriptParams(
	webhookAddr string,
	input ScriptExecutionRequest,
) []string {
	params := buildScriptBaseParams(webhookAddr, input.RuntimeID)
	keyValues := s.parseScriptParams(input.ScriptJSONParam)
	s.syncRulesIfNeeded(keyValues)
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

func (s *ScanNode) syncRulesIfNeeded(params map[string]any) {
	ruleSetHash, ok := params["ruleset-hash"].(string)
	if !ok || ruleSetHash == "" {
		return
	}

	client := GetRuleSyncClient()
	if client == nil || client.HasLocalSnapshot(ruleSetHash) {
		return
	}

	log.Infof("auto-syncing rules for hash: %s", ruleSetHash)
	ruleCount, err := client.SyncForHash(ruleSetHash)
	if err != nil {
		log.Warnf("auto-sync rules failed: %v, will continue with local rules", err)
		return
	}
	log.Infof("auto-synced %d rules from snapshot %s", ruleCount, ruleSetHash)
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
