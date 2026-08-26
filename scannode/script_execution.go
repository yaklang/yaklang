package scannode

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	ssaconfig "github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssagitworkdir"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

type ScriptExecutionRequest struct {
	TaskID               string
	RuntimeID            string
	SubTaskID            string
	ScriptContent        string
	ScriptJSONParam      string
	ScriptLabels         map[string]string
	DebugEnabled         bool
	DebugDir             string
	RuleSnapshot         *RuleSnapshotExpectation
	RuleSnapshotPrepared func(context.Context, RuleSnapshotPreparationReceipt) error
}

type ScriptExecutionResult struct {
	Data                 any                             `json:"data,omitempty"`
	RuleSnapshotPrepared *RuleSnapshotPreparationReceipt `json:"rule_snapshot_prepared,omitempty"`
}

type ruleSnapshotPreparationError struct {
	Expectation RuleSnapshotExpectation
	Err         error
}

func (e *ruleSnapshotPreparationError) Error() string {
	if e == nil || e.Err == nil {
		return "rule snapshot preparation failed"
	}
	return "rule snapshot preparation failed: " + e.Err.Error()
}

func (e *ruleSnapshotPreparationError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

func (s *ScanNode) executeScriptTask(
	task *Task,
	input ScriptExecutionRequest,
) (*ScriptExecutionResult, error) {
	if strings.TrimSpace(input.ScriptContent) == "" {
		return nil, utils.Error("empty script_content")
	}
	if task == nil || task.Ctx == nil {
		return nil, utils.Error("claimed script task is required")
	}
	taskCtx := task.Ctx

	reporter := NewScannerAgentReporter(
		input.TaskID,
		input.SubTaskID,
		input.RuntimeID,
		legionJobExecutionRefFromContext(taskCtx),
		s,
	)
	keyValues := s.parseScriptParams(input.ScriptJSONParam)
	preparedSnapshot, err := s.prepareRuleSnapshotForScriptExecution(
		taskCtx,
		keyValues,
		input.ScriptLabels,
		input.RuleSnapshot,
		input.RuleSnapshotPrepared,
	)
	if err != nil {
		return nil, err
	}
	if preparedSnapshot != nil {
		defer preparedSnapshot.Cleanup()
	}
	cleanupSourcePayload, err := s.prepareManagedSourcePayload(taskCtx, keyValues)
	if err != nil {
		return nil, err
	}
	defer cleanupSourcePayload()
	reporter.ssaUploadCfg = extractSSAArtifactUploadConfig(keyValues)
	reporter.ssaCollector = NewSSAArtifactCollector(input.TaskID, input.RuntimeID, input.SubTaskID)
	if reporter.ssaCollector != nil {
		defer reporter.ssaCollector.Cleanup()
	}
	result := &ScriptExecutionResult{}
	if preparedSnapshot != nil {
		receipt := preparedSnapshot.Receipt
		result.RuleSnapshotPrepared = &receipt
	}
	yakitServer := s.createYakitServer(reporter, result)
	yakitServer.Start()
	defer yakitServer.Shutdown()

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
			// Register the directory so ssa.debug.query can serve live pprof/
			// log data while the task is still running (or after a cancel).
			scanDebugDirs.register(s.debugBaseDir(), input.TaskID, input.RuntimeID, debugDir)
		}
	}

	// Inject debug_dir into the script params so the yak script can pass it to StartScan
	if debugDir != "" {
		keyValues["debug_dir"] = debugDir
		params = s.buildScriptParams(yakitServer.Addr(), input.RuntimeID, keyValues)
	}

	ssaDBEnv, sqliteLivePath := resolveSSADatabaseEnv(s, keyValues, debugDir, input.RuntimeID)
	ssaDBCleanup := func() {}
	if s.needIsolateSSARuntimeDB() {
		ssaOverride := environmentValueFromEntries(ssaDBEnv, consts.ENV_SSA_DATABASE_RAW)
		isolatedEnv, cleanup := buildSSARuntimeDBEnv(input.RuntimeID, ssaOverride)
		if environmentValueFromEntries(ssaDBEnv, consts.ENV_SSA_DB_SKIP_MIGRATE) != "" {
			isolatedEnv = append(isolatedEnv, fmt.Sprintf("%s=1", consts.ENV_SSA_DB_SKIP_MIGRATE))
		}
		ssaDBEnv = isolatedEnv
		ssaDBCleanup = cleanup
	}
	defer ssaDBCleanup()
	if preparedSnapshot != nil {
		ssaDBEnv = append(ssaDBEnv, "YAKIT_HOME="+preparedSnapshot.taskYakitHome)
	}

	// Register a defer to finalize debug artifacts (analysis + zip) on both
	// success and failure paths. The pprof collector (started by Scan() inside
	// the child process) writes its final snapshot during script exit/cleanup.
	// We wait briefly for the child process to finish writing, then analyze.
	debugFinalized := false
	if debugDir != "" {
		defer func() {
			scanDebugDirs.unregister(input.TaskID, input.RuntimeID)
			if debugFinalized {
				return
			}
			copySQLiteIRIntoDebugDir(debugDir, sqliteLivePath)
			s.finalizeDebugRun(taskCtx, reporter, debugDir, "unknown")
		}()
	}

	// Debug mode: lower yaklog threshold so the task log carries Debug lines the
	// console can filter (Info/Warn/Error remain available).
	scriptEnv := scriptEnvWithDebugLogLevel(ssaDBEnv, input.DebugEnabled)

	if err := s.executeScript(taskCtx, scanNodePath, scriptFile, params, input.RuntimeID, scriptEnv, taskLogWriter); err != nil {
		logReporterEventError("final progress checkpoint", reporter.flushLatestJobProgress())
		// Finalize debug before returning the failure. Cancel / shutdown leaves
		// taskCtx cancelled; finalize must still upload and write local cache.
		if debugDir != "" {
			copySQLiteIRIntoDebugDir(debugDir, sqliteLivePath)
			s.finalizeDebugRun(taskCtx, reporter, debugDir, debugStatusForScriptError(s, task.AttemptID, err))
			scanDebugDirs.unregister(input.TaskID, input.RuntimeID)
			debugFinalized = true
		}
		return nil, s.handleScriptFailure(err, result, task.AttemptID)
	}
	logReporterEventError("final progress checkpoint", reporter.flushSuccessfulJobProgress())
	if err := s.finalizeSSAArtifactUpload(taskCtx, reporter, result); err != nil {
		if debugDir != "" {
			copySQLiteIRIntoDebugDir(debugDir, sqliteLivePath)
			s.finalizeDebugRun(taskCtx, reporter, debugDir, debugStatusForScriptError(s, task.AttemptID, err))
			scanDebugDirs.unregister(input.TaskID, input.RuntimeID)
			debugFinalized = true
		}
		return nil, err
	}

	// Finalize debug artifacts on success path
	if debugDir != "" {
		copySQLiteIRIntoDebugDir(debugDir, sqliteLivePath)
		s.finalizeDebugRun(taskCtx, reporter, debugDir, "succeeded")
		scanDebugDirs.unregister(input.TaskID, input.RuntimeID)
		debugFinalized = true
	}
	return result, nil
}

func environmentValueFromEntries(entries []string, key string) string {
	prefix := key + "="
	for i := len(entries) - 1; i >= 0; i-- {
		if strings.HasPrefix(entries[i], prefix) {
			return strings.TrimPrefix(entries[i], prefix)
		}
	}
	return ""
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
	attemptID string,
) error {
	if err == nil {
		return nil
	}
	if reason := s.cancelReasonForAttempt(attemptID); reason != "" {
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

func (s *ScanNode) cancelReasonForAttempt(attemptID string) string {
	task, err := s.manager.GetTaskByAttemptID(attemptID)
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

func (s *ScanNode) prepareRuleSnapshotForExecution(
	ctx context.Context,
	params map[string]any,
	labels map[string]string,
	explicit *RuleSnapshotExpectation,
) (*PreparedRuleSnapshot, error) {
	legacy, hasLegacy, err := resolveLegacyRuleSnapshotExpectation(params, labels)
	if err != nil {
		return nil, &ruleSnapshotPreparationError{Err: err}
	}

	var expectation RuleSnapshotExpectation
	switch {
	case explicit != nil:
		expectation = *explicit
		if hasLegacy {
			expectation, err = mergeRuleSnapshotExpectations(expectation, legacy)
			if err != nil {
				return nil, &ruleSnapshotPreparationError{Expectation: expectation, Err: err}
			}
		}
	case hasLegacy:
		expectation = legacy
	default:
		return nil, nil
	}

	expectation, err = normalizeRuleSnapshotExpectation(expectation)
	if err != nil {
		return nil, &ruleSnapshotPreparationError{Expectation: expectation, Err: err}
	}
	if s == nil || s.ruleSyncClient == nil {
		return nil, &ruleSnapshotPreparationError{
			Expectation: expectation,
			Err:         utils.Error("rule sync client is not configured"),
		}
	}

	prepared, err := s.ruleSyncClient.PrepareSnapshot(ctx, expectation)
	if err != nil {
		return nil, &ruleSnapshotPreparationError{Expectation: expectation, Err: err}
	}
	if prepared == nil {
		return nil, &ruleSnapshotPreparationError{
			Expectation: expectation,
			Err:         utils.Error("rule sync client returned an empty snapshot"),
		}
	}
	cleanup, err := injectPreparedRuleSnapshot(params, prepared.Bundle)
	if err != nil {
		return nil, &ruleSnapshotPreparationError{Expectation: expectation, Err: err}
	}
	taskYakitHome, cleanupTaskYakitHome, err := createRuleSnapshotTaskYakitHome()
	if err != nil {
		cleanup()
		return nil, &ruleSnapshotPreparationError{
			Expectation: expectation,
			Err:         utils.Wrap(err, "create isolated task rule runtime"),
		}
	}
	prepared.taskYakitHome = taskYakitHome
	prepared.cleanup = func() {
		cleanup()
		cleanupTaskYakitHome()
	}
	return prepared, nil
}

func (s *ScanNode) prepareRuleSnapshotForScriptExecution(
	ctx context.Context,
	params map[string]any,
	labels map[string]string,
	explicit *RuleSnapshotExpectation,
	preparedCallback func(context.Context, RuleSnapshotPreparationReceipt) error,
) (*PreparedRuleSnapshot, error) {
	prepared, err := s.prepareRuleSnapshotForExecution(ctx, params, labels, explicit)
	if err != nil || prepared == nil || preparedCallback == nil {
		return prepared, err
	}
	if err := preparedCallback(ctx, prepared.Receipt); err != nil {
		prepared.Cleanup()
		return nil, &ruleSnapshotPreparationError{
			Expectation: RuleSnapshotExpectation{
				SnapshotID:    prepared.Receipt.SnapshotID,
				ContentSHA256: prepared.Receipt.ContentSHA256,
				SchemaVersion: prepared.Receipt.SchemaVersion,
				BundleFormat:  prepared.Receipt.BundleFormat,
			},
			Err: utils.Wrap(err, "publish prepared receipt"),
		}
	}
	return prepared, nil
}

func resolveLegacyRuleSnapshotExpectation(
	params map[string]any,
	labels map[string]string,
) (RuleSnapshotExpectation, bool, error) {
	nested := map[string]any{}
	if raw, ok := params["rule_snapshot"]; ok {
		nested = utils.InterfaceToGeneralMap(raw)
	}

	resolveString := func(field string, candidates ...any) (string, error) {
		resolved := ""
		for _, candidate := range candidates {
			value := strings.TrimSpace(utils.InterfaceToString(candidate))
			if value == "" {
				continue
			}
			if resolved != "" && resolved != value {
				return "", utils.Errorf("conflicting legacy rule snapshot %s values", field)
			}
			resolved = value
		}
		return resolved, nil
	}

	expectation := RuleSnapshotExpectation{}
	var err error
	expectation.SnapshotID, err = resolveString(
		"snapshot_id",
		labels["rule_snapshot_id"],
		params["rule_snapshot_id"],
		nested["snapshot_id"],
	)
	if err != nil {
		return RuleSnapshotExpectation{}, false, err
	}
	expectation.ContentSHA256, err = resolveString(
		"content_sha256",
		labels["rule_snapshot_content_sha256"],
		params["rule_snapshot_content_sha256"],
		nested["content_sha256"],
	)
	if err != nil {
		return RuleSnapshotExpectation{}, false, err
	}
	expectation.SchemaVersion, err = resolveString(
		"schema_version",
		labels["rule_snapshot_schema_version"],
		params["rule_snapshot_schema_version"],
		nested["schema_version"],
	)
	if err != nil {
		return RuleSnapshotExpectation{}, false, err
	}
	expectation.BundleFormat, err = resolveString(
		"bundle_format",
		labels["rule_snapshot_bundle_format"],
		params["rule_snapshot_bundle_format"],
		nested["bundle_format"],
	)
	if err != nil {
		return RuleSnapshotExpectation{}, false, err
	}
	expectation.AssetIDs, err = resolveLegacyRuleSnapshotAssetIDs(
		params["rule_snapshot_asset_ids"],
		nested["asset_ids"],
	)
	if err != nil {
		return RuleSnapshotExpectation{}, false, err
	}

	hasAny := expectation.SnapshotID != "" || expectation.ContentSHA256 != "" ||
		expectation.SchemaVersion != "" || expectation.BundleFormat != "" ||
		len(expectation.AssetIDs) > 0
	if !hasAny {
		return RuleSnapshotExpectation{}, false, nil
	}
	if expectation.SnapshotID == "" {
		return RuleSnapshotExpectation{}, false, utils.Error("legacy rule snapshot metadata requires snapshot_id")
	}
	return expectation, true, nil
}

func resolveLegacyRuleSnapshotAssetIDs(values ...any) ([]string, error) {
	var resolved []string
	for _, value := range values {
		if value == nil {
			continue
		}
		var current []string
		switch typed := value.(type) {
		case []string:
			current = append(current, typed...)
		case []any:
			for _, item := range typed {
				current = append(current, strings.TrimSpace(utils.InterfaceToString(item)))
			}
		case string:
			if strings.TrimSpace(typed) != "" {
				if err := json.Unmarshal([]byte(typed), &current); err != nil {
					return nil, utils.Wrap(err, "decode legacy rule snapshot asset_ids")
				}
			}
		default:
			return nil, utils.Errorf("invalid legacy rule snapshot asset_ids type: %T", value)
		}
		if len(current) == 0 {
			continue
		}
		if len(resolved) > 0 && !equalStringSlices(resolved, current) {
			return nil, utils.Error("conflicting legacy rule snapshot asset_ids values")
		}
		resolved = current
	}
	return resolved, nil
}

func mergeRuleSnapshotExpectations(
	primary RuleSnapshotExpectation,
	legacy RuleSnapshotExpectation,
) (RuleSnapshotExpectation, error) {
	mergeString := func(field string, target *string, fallback string) error {
		if strings.TrimSpace(fallback) == "" {
			return nil
		}
		if strings.TrimSpace(*target) == "" {
			*target = fallback
			return nil
		}
		if strings.TrimSpace(*target) != strings.TrimSpace(fallback) {
			return utils.Errorf("protobuf and legacy rule snapshot %s mismatch", field)
		}
		return nil
	}
	if err := mergeString("snapshot_id", &primary.SnapshotID, legacy.SnapshotID); err != nil {
		return primary, err
	}
	if err := mergeString("content_sha256", &primary.ContentSHA256, legacy.ContentSHA256); err != nil {
		return primary, err
	}
	if err := mergeString("schema_version", &primary.SchemaVersion, legacy.SchemaVersion); err != nil {
		return primary, err
	}
	if err := mergeString("bundle_format", &primary.BundleFormat, legacy.BundleFormat); err != nil {
		return primary, err
	}
	if len(legacy.AssetIDs) > 0 {
		if len(primary.AssetIDs) == 0 {
			primary.AssetIDs = legacy.AssetIDs
		} else if !equalStringSlices(primary.AssetIDs, legacy.AssetIDs) {
			return primary, utils.Error("protobuf and legacy rule snapshot asset_ids mismatch")
		}
	}
	return primary, nil
}

func equalStringSlices(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	leftCanonical := append([]string(nil), left...)
	rightCanonical := append([]string(nil), right...)
	for index := range leftCanonical {
		leftCanonical[index] = strings.TrimSpace(leftCanonical[index])
		rightCanonical[index] = strings.TrimSpace(rightCanonical[index])
	}
	sort.Strings(leftCanonical)
	sort.Strings(rightCanonical)
	for index := range leftCanonical {
		if leftCanonical[index] != rightCanonical[index] {
			return false
		}
	}
	return true
}

func injectPreparedRuleSnapshot(params map[string]any, bundle RuleSnapshotBundle) (func(), error) {
	rawConfig, ok := params["config"]
	if !ok {
		return nil, utils.Error("rule snapshot execution requires config input")
	}
	configText, ok := rawConfig.(string)
	if !ok || strings.TrimSpace(configText) == "" {
		return nil, utils.Error("rule snapshot execution requires string config input")
	}

	config := make(map[string]any)
	if err := json.Unmarshal([]byte(configText), &config); err != nil {
		return nil, utils.Wrap(err, "decode rule snapshot execution config")
	}
	mode := ssaconfig.Mode(utils.InterfaceToInt(config["Mode"]))
	if mode&ssaconfig.ModeSyntaxFlowRule == 0 {
		return nil, utils.Error("rule snapshot execution config must enable SyntaxFlow rule mode")
	}
	ruleConfig := map[string]any{}
	if current, exists := config["SyntaxFlowRule"]; exists {
		if typed, ok := current.(map[string]any); ok {
			ruleConfig = typed
		}
	}
	delete(ruleConfig, "rule_names")
	delete(ruleConfig, "RuleNames")
	delete(ruleConfig, "rule_filter")
	delete(ruleConfig, "RuleFilter")

	ruleInputs := make([]*ypb.SyntaxFlowRuleInput, 0, len(bundle.Items))
	ruleMetadata := make(map[string]ssaconfig.TaskLocalRuleMetadata, len(bundle.Items))
	for _, item := range bundle.Items {
		ruleInput := &ypb.SyntaxFlowRuleInput{
			RuleName:    item.Name,
			Content:     item.Content,
			Language:    item.Language,
			Description: item.Description,
			GroupNames:  append([]string(nil), item.Groups...),
		}
		if tags := splitRuleSnapshotTags(item.Tag); len(tags) > 0 {
			ruleInput.Tags = tags
		}
		ruleInputs = append(ruleInputs, ruleInput)
		ruleMetadata[item.Name] = ssaconfig.TaskLocalRuleMetadata{
			AssetID: item.AssetID, SourceRuleID: item.SourceRuleID,
			Title: item.Title, TitleZh: item.TitleZh, Language: item.Language,
			Purpose: item.Purpose, Tag: item.Tag,
			CWE: append([]string(nil), item.CWE...), CVE: item.CVE, RiskType: item.RiskType,
			Type: item.Type, Severity: item.Severity, Description: item.Description,
			Solution: item.Solution,
			Version:  item.Version, ContentHash: item.ContentHash,
			IsBuiltin: item.IsBuiltin, Verified: item.Verified,
			AllowIncluded: item.AllowIncluded, IncludedName: item.IncludedName,
			Groups:    append([]string(nil), item.Groups...),
			AlertDesc: append(json.RawMessage(nil), item.AlertDesc...),
		}
	}
	payload, err := json.Marshal(ssaconfig.TaskLocalRuleInputFile{
		Version:  ssaconfig.TaskLocalRuleInputFileVersionV1,
		Rules:    ruleInputs,
		Metadata: ruleMetadata,
	})
	if err != nil {
		return nil, utils.Wrap(err, "encode task-local rule input file")
	}
	inputFile, err := createRuleSnapshotTaskInputFile()
	if err != nil {
		return nil, err
	}
	inputPath := inputFile.Name()
	cleanup := func() { _ = os.Remove(inputPath) }
	if err := inputFile.Chmod(0o600); err != nil {
		_ = inputFile.Close()
		cleanup()
		return nil, utils.Wrap(err, "set task-local rule input permissions")
	}
	if _, err := inputFile.Write(payload); err != nil {
		_ = inputFile.Close()
		cleanup()
		return nil, utils.Wrap(err, "write task-local rule input file")
	}
	if err := inputFile.Sync(); err != nil {
		_ = inputFile.Close()
		cleanup()
		return nil, utils.Wrap(err, "sync task-local rule input file")
	}
	if err := inputFile.Close(); err != nil {
		cleanup()
		return nil, utils.Wrap(err, "close task-local rule input file")
	}
	payloadSHA := sha256.Sum256(payload)
	delete(ruleConfig, "rule_input")
	delete(ruleConfig, "RuleInput")
	ruleConfig["task_local"] = true
	ruleConfig["task_local_input_file"] = inputPath
	ruleConfig["task_local_input_sha256"] = hex.EncodeToString(payloadSHA[:])
	ruleConfig["task_local_input_count"] = len(ruleInputs)
	config["SyntaxFlowRule"] = ruleConfig

	canonical, err := json.Marshal(config)
	if err != nil {
		cleanup()
		return nil, utils.Wrap(err, "encode task-local rule snapshot config")
	}
	params["config"] = string(canonical)
	return cleanup, nil
}

func createRuleSnapshotTaskInputFile() (*os.File, error) {
	const pattern = "rule-snapshot-input-*.json"
	file, err := consts.TempFile(pattern)
	if err == nil {
		return file, nil
	}
	file, fallbackErr := os.CreateTemp("", pattern)
	if fallbackErr != nil {
		return nil, utils.Errorf(
			"create task-local rule input file failed: yakit temp: %v; system temp: %v",
			err,
			fallbackErr,
		)
	}
	return file, nil
}

func createRuleSnapshotTaskYakitHome() (string, func(), error) {
	dir, err := os.MkdirTemp("", "rule-snapshot-task-home-*")
	if err != nil {
		return "", func() {}, err
	}
	cleanup := func() {
		if err := os.RemoveAll(dir); err != nil {
			log.Warnf("remove task-local rule runtime %s failed: %v", dir, err)
		}
	}
	return dir, cleanup, nil
}

func splitRuleSnapshotTags(value string) []string {
	parts := strings.FieldsFunc(value, func(r rune) bool { return r == '|' || r == ',' })
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
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
	env := replaceEnvironmentValue(os.Environ(), "YAKIT_HOME", os.Getenv("YAKIT_HOME"))
	env = replaceEnvironmentValue(env, "YAK_RUNTIME_ID", runtimeID)
	for _, item := range extraEnv {
		key, value, ok := strings.Cut(item, "=")
		if !ok || strings.TrimSpace(key) == "" {
			continue
		}
		env = replaceEnvironmentValue(env, key, value)
	}
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

// scriptEnvWithDebugLogLevel copies base env entries and forces LOG_LEVEL=debug
// when the scan attempt has debug mode enabled, so yaklog Debug lines land in
// the per-task log the console filters.
func scriptEnvWithDebugLogLevel(base []string, debugEnabled bool) []string {
	out := append([]string(nil), base...)
	if !debugEnabled {
		return out
	}
	return replaceEnvironmentValue(out, "LOG_LEVEL", "debug")
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
// not affect the scan result. This is called on success, failure, cancel, and
// shutdown paths. Upload uses a detached timeout so a cancelled task context
// cannot skip persistence.
func (s *ScanNode) finalizeDebugRun(
	ctx context.Context,
	reporter *ScannerAgentReporter,
	debugDir string,
	status string,
) {
	uploadCtx, cancel := debugFinalizeContext(ctx)
	defer cancel()

	// Ensure the debug package contains the actual scan log even when the
	// child pprof collector did not write its own log file.
	mergeTaskLogIntoDebugDir(debugDir)
	s.publishDebugAnalysis(uploadCtx, reporter, debugDir, status)
	s.publishDebugZip(uploadCtx, reporter, debugDir)
}

const debugFinalizeTimeout = 45 * time.Second

// debugFinalizeContext returns a timeout context that is not cancelled when
// the parent task context is cancelled (cancel / shutdown / lease loss paths).
func debugFinalizeContext(parent context.Context) (context.Context, context.CancelFunc) {
	base := context.Background()
	if parent != nil {
		base = context.WithoutCancel(parent)
	}
	return context.WithTimeout(base, debugFinalizeTimeout)
}

// debugStatusForScriptError maps a script/task error onto the debug analysis
// status string used in analysis JSON and the console.
func debugStatusForScriptError(s *ScanNode, attemptID string, err error) string {
	if s != nil && strings.TrimSpace(s.cancelReasonForAttempt(attemptID)) != "" {
		return "cancelled"
	}
	if err != nil && errors.Is(err, context.Canceled) {
		return "cancelled"
	}
	return "failed"
}

// publishDebugAnalysis analyzes the debug run directory and publishes the
// structured result as a JobArtifactReady event with artifact_kind="debug_analysis".
// The analysis JSON is always written to the local debug dir first so live
// queries still work after cancel / lost / node restart even when MinIO upload
// fails. Failures are logged but do not affect the scan result.
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
	if err := writeCachedDebugAnalysis(debugDir, analysisJSON); err != nil {
		log.Warnf("[debug] write local analysis cache failed: %v", err)
	}

	// Upload analysis JSON to MinIO
	if reporter == nil {
		log.Warnf("[debug] no reporter available, skipping analysis upload")
		return
	}
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
