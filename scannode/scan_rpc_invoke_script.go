package scannode

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	uuid "github.com/google/uuid"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/fp"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/mq"
	"github.com/yaklang/yaklang/common/spec"
	"github.com/yaklang/yaklang/common/synscan"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/yaklib"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"github.com/yaklang/yaklang/scannode/scanrpc"
)

var riskDebugCount int32

const defaultSSASTSRenewBeforeSec int64 = 600

const scannodeSSADataBaseRawParamKey = "_scannode_ssa_database_raw"
const scannodeSSASkipMigrateParamKey = "_scannode_ssa_skip_migrate"
const scannodeSSADBSkipMigrateEnvKey = "SSA_DB_SKIP_MIGRATE"

func readSSASTSRenewBeforeSec() int64 {
	v := strings.TrimSpace(os.Getenv("SCANNODE_SSA_STS_RENEW_BEFORE_SEC"))
	if v == "" {
		return defaultSSASTSRenewBeforeSec
	}
	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil || n <= 0 {
		return defaultSSASTSRenewBeforeSec
	}
	return n
}

func (s *ScanNode) ensureSSAUploadTicket(ctx context.Context, reporter *ScannerAgentReporter, force bool) error {
	if s == nil || reporter == nil {
		return utils.Errorf("invalid upload context")
	}
	cfg := reporter.ssaUploadCfg
	if cfg == nil {
		return utils.Errorf("ssa artifact upload config missing")
	}
	renewBeforeSec := readSSASTSRenewBeforeSec()
	if !force && !cfg.NeedSTSRefresh(renewBeforeSec) {
		return nil
	}

	fresh, err := s.fetchSSAArtifactUploadTicket(ctx, reporter.TaskId, cfg.ObjectKey)
	if err != nil {
		return err
	}
	if cfg.Codec != "" && fresh.Codec == "" {
		fresh.Codec = cfg.Codec
	}
	if cfg.ObjectKey != "" && fresh.ObjectKey == "" {
		fresh.ObjectKey = cfg.ObjectKey
	}
	if cfg.Endpoint != "" && fresh.Endpoint == "" {
		fresh.Endpoint = cfg.Endpoint
	}
	if cfg.Bucket != "" && fresh.Bucket == "" {
		fresh.Bucket = cfg.Bucket
	}
	if cfg.Region != "" && fresh.Region == "" {
		fresh.Region = cfg.Region
	}
	if !fresh.UseSSL && cfg.UseSSL {
		fresh.UseSSL = true
	}
	if fresh.STSAccessKey == "" {
		fresh.STSAccessKey = cfg.STSAccessKey
	}
	if fresh.STSSecretKey == "" {
		fresh.STSSecretKey = cfg.STSSecretKey
	}
	if fresh.STSSessionToken == "" {
		fresh.STSSessionToken = cfg.STSSessionToken
	}
	if fresh.STSExpiresAt <= 0 {
		fresh.STSExpiresAt = cfg.STSExpiresAt
	}
	if fresh.ObjectKey != "" && cfg.ObjectKey != "" && fresh.ObjectKey != cfg.ObjectKey {
		return utils.Errorf("upload object key mismatch old=%s new=%s", cfg.ObjectKey, fresh.ObjectKey)
	}
	if strings.TrimSpace(fresh.Endpoint) == "" || strings.TrimSpace(fresh.Bucket) == "" || strings.TrimSpace(fresh.ObjectKey) == "" {
		return utils.Errorf("invalid upload ticket: missing object storage fields")
	}
	if strings.TrimSpace(fresh.STSAccessKey) == "" || strings.TrimSpace(fresh.STSSecretKey) == "" {
		return utils.Errorf("invalid upload ticket: missing sts credentials")
	}
	reporter.ssaUploadCfg = fresh
	return nil
}

func (s *ScanNode) rpc_startScript(ctx context.Context, node string, req *scanrpc.SCAN_StartScriptRequest, broker *mq.Broker) (*scanrpc.SCAN_StartScriptResponse, error) {
	if req.Content == "" {
		log.Error("empty content for rpc_startScript")
		return nil, utils.Error("empty content")
	}
	rsp, err := s.rpc_invokeScript(ctx, node, &scanrpc.SCAN_InvokeScriptRequest{
		TaskId:          uuid.New().String(),
		RuntimeId:       uuid.New().String(),
		SubTaskId:       uuid.New().String(),
		ScriptContent:   req.Content,
		ScriptJsonParam: "{}",
	}, broker)
	if err != nil {
		return nil, err
	}
	_ = rsp
	return &scanrpc.SCAN_StartScriptResponse{}, nil
}

func (s *ScanNode) rpc_invokeScript(ctx context.Context, node string, req *scanrpc.SCAN_InvokeScriptRequest, broker *mq.Broker) (*scanrpc.SCAN_InvokeScriptResponse, error) {
	runtimeId := req.RuntimeId

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	taskId := fmt.Sprintf("script-task-%v", req.SubTaskId)
	s.manager.Add(taskId, &Task{
		TaskType: "script-task",
		TaskId:   taskId,
		RootTaskID: req.TaskId,
		SubTaskID: req.SubTaskId,
		RuntimeID: req.RuntimeId,
		Status:   "queued",
		Ctx:      ctx,
		Cancel:   cancel,
	})
	defer s.manager.Remove(taskId)

	// Get executable path for script invocation
	scanNodePath, err := os.Executable()
	if err != nil {
		return nil, utils.Errorf("rpc call InvokeScript failed: fetch node path err: %s", err)
	}

	// Initialize reporter for sending scan results back
	reporter := NewScannerAgentReporter(req.TaskId, req.SubTaskId, req.RuntimeId, s)
	res := &scanrpc.SCAN_InvokeScriptResponse{}

	// Parse and convert user-provided JSON parameters to command-line arguments
	paramsKeyValue := s.parseScriptParams(req.ScriptJsonParam)
	reporter.ssaUploadCfg = extractSSAArtifactUploadConfig(paramsKeyValue)
	reporter.ssaCollector = NewSSAArtifactCollector(req.TaskId, req.RuntimeId, req.SubTaskId)
	if reporter.ssaCollector != nil && reporter.ssaUploadCfg != nil {
		_ = reporter.ssaCollector.EnableContinuousUpload(reporter.ssaUploadCfg.Codec, func(force bool) (*SSAArtifactUploadConfig, error) {
			if refreshErr := s.ensureSSAUploadTicket(context.Background(), reporter, force); refreshErr != nil {
				return nil, refreshErr
			}
			return reporter.ssaUploadCfg, nil
		})
	}

	waitStart := time.Now()
	log.Infof(
		"invoke limiter wait: task=%s subtask=%s runtime=%s active=%d/%d",
		req.TaskId,
		req.SubTaskId,
		req.RuntimeId,
		s.invokeLimiter.activeCount(),
		s.invokeLimiter.capacity(),
	)
	releaseLimiter, err := s.invokeLimiter.acquire(ctx)
	if err != nil {
		return nil, err
	}
	waitMs := time.Since(waitStart).Milliseconds()
	s.manager.MarkRunning(taskId, waitMs)
	log.Infof(
		"invoke limiter acquired: task=%s subtask=%s runtime=%s wait_ms=%d active=%d/%d",
		req.TaskId,
		req.SubTaskId,
		req.RuntimeId,
		waitMs,
		s.invokeLimiter.activeCount(),
		s.invokeLimiter.capacity(),
	)
	defer func() {
		activeBeforeRelease := s.invokeLimiter.activeCount()
		releaseLimiter()
		log.Infof(
			"invoke limiter released: task=%s subtask=%s runtime=%s active_before=%d/%d active_after=%d/%d",
			req.TaskId,
			req.SubTaskId,
			req.RuntimeId,
			activeBeforeRelease,
			s.invokeLimiter.capacity(),
			s.invokeLimiter.activeCount(),
			s.invokeLimiter.capacity(),
		)
	}()

	ssaDBRaw := strings.TrimSpace(utils.InterfaceToString(paramsKeyValue[scannodeSSADataBaseRawParamKey]))
	skipSSAMigrate := utils.InterfaceToBoolean(paramsKeyValue[scannodeSSASkipMigrateParamKey])

	extraEnv := []string{}
	cleanupDB := func() {}
	// When running multiple scripts concurrently, isolate the per-task project DB (SQLite) to avoid "database is locked".
	// SSA DB can be isolated (default) or overridden (e.g. shared SSA-IR DB for compile/scan-from-db mode).
	if s.needIsolateSSARuntimeDB() && (reporter.ssaUploadCfg != nil || ssaDBRaw != "") {
		env, cleanup := buildSSARuntimeDBEnv(runtimeId, ssaDBRaw)
		extraEnv = append(extraEnv, env...)
		cleanupDB = cleanup
	} else if ssaDBRaw != "" {
		// Override SSA DB for this invocation without isolating runtime DB files.
		extraEnv = append(extraEnv, fmt.Sprintf("%s=%s", consts.ENV_SSA_DATABASE_RAW, ssaDBRaw))
	}
	if skipSSAMigrate {
		extraEnv = append(extraEnv, fmt.Sprintf("%s=1", scannodeSSADBSkipMigrateEnvKey))
	}
	defer cleanupDB()

	// Setup Yakit server for receiving callbacks from script execution
	yakitServer := s.createYakitServer(reporter, res)
	yakitServer.Start()
	defer yakitServer.Shutdown()

	// Build base command-line parameters
	params := []string{"--yakit-webhook", yakitServer.Addr()}
	if runtimeId != "" {
		params = append(params, "--runtime-id", runtimeId)
	}

	// Automatic rule synchronization before script execution
	s.syncRulesIfNeeded(paramsKeyValue)

	// Convert key-value parameters to command-line format
	params = s.appendKeyValueParams(params, paramsKeyValue)

	// Create temporary file with script content
	scriptFile, err := s.createTempScriptFile(req.ScriptContent)
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(scriptFile)

	// Execute the Yak script
	if err := s.executeScript(ctx, scanNodePath, scriptFile, params, runtimeId, extraEnv); err != nil {
		log.Errorf("exec yakScript %v failed: %s", scriptFile, err)

		// 使用脚本通过 yakit.Output 返回的错误详情
		if detailedError := extractErrorFromResponse(res); detailedError != "" {
			return nil, utils.Errorf("%s", detailedError)
		}

		// 返回通用错误
		return nil, utils.Errorf("exec yakScript %v failed: %s", scriptFile, err)
	}

	if err := s.finalizeSSAArtifactUpload(reporter, res); err != nil {
		return nil, err
	}

	return res, nil
}

// extractErrorFromResponse 从响应中提取脚本返回的错误信息
func extractErrorFromResponse(res *scanrpc.SCAN_InvokeScriptResponse) string {
	if res == nil || res.Data == nil {
		return ""
	}

	dataMap, ok := res.Data.(map[string]interface{})
	if !ok {
		return ""
	}

	errMsg, ok := dataMap["error"].(string)
	if !ok || errMsg == "" {
		return ""
	}

	return errMsg
}

func (s *ScanNode) finalizeSSAArtifactUpload(reporter *ScannerAgentReporter, res *scanrpc.SCAN_InvokeScriptResponse) error {
	if reporter == nil || reporter.agent == nil {
		return nil
	}
	if reporter.ssaCollector == nil {
		return nil
	}
	defer reporter.ssaCollector.Cleanup()
	meta := parseSSAResultMeta(res)
	cfg := reporter.ssaUploadCfg
	if cfg == nil {
		if reporter.ssaCollector.HasData() {
			return utils.Errorf("ssa artifact upload config missing")
		}
		// No SSA payload generated.
		return nil
	}
	if err := s.ensureSSAUploadTicket(context.Background(), reporter, false); err != nil {
		return err
	}
	cfg = reporter.ssaUploadCfg

	build, err := reporter.ssaCollector.FinalizeUploadWithProvider(cfg.Codec, func(force bool) (*SSAArtifactUploadConfig, error) {
		if refreshErr := s.ensureSSAUploadTicket(context.Background(), reporter, force); refreshErr != nil {
			return nil, refreshErr
		}
		return reporter.ssaUploadCfg, nil
	})
	if err != nil {
		return err
	}
	if build == nil {
		return nil
	}
	if build.ProgramName == "" {
		build.ProgramName = meta.ProgramName
	}
	if strings.TrimSpace(build.ObjectKey) == "" && cfg != nil {
		build.ObjectKey = cfg.ObjectKey
	}
	if strings.TrimSpace(build.Codec) == "" && cfg != nil {
		build.Codec = cfg.Codec
	}
	event := reporter.ssaCollector.BuildReadyEvent(build, meta.TotalLines, meta.RiskCount)
	raw, err := json.Marshal(event)
	if err != nil {
		return err
	}
	reporter.agent.feedback(&spec.ScanResult{
		Type:      spec.ScanResult_SSAArtifactReady,
		Content:   raw,
		TaskId:    reporter.TaskId,
		RuntimeId: reporter.RuntimeId,
		SubTaskId: reporter.SubTaskId,
	})
	log.Infof("ssa artifact uploaded task=%s key=%s codec=%s raw=%d compressed=%d risks=%d files=%d flows=%d",
		reporter.TaskId, build.ObjectKey, build.Codec,
		build.UncompressedSize, build.CompressedSize, event.RiskCount, event.FileCount, event.FlowCount)
	return nil
}

type ssaResultMeta struct {
	ProgramName string
	TotalLines  int64
	RiskCount   int64
}

func parseSSAResultMeta(res *scanrpc.SCAN_InvokeScriptResponse) ssaResultMeta {
	meta := ssaResultMeta{}
	if res == nil {
		return meta
	}
	m, ok := res.Data.(map[string]interface{})
	if !ok || m == nil {
		return meta
	}
	meta.ProgramName = strings.TrimSpace(utils.InterfaceToString(utils.MapGetFirstRaw(m, "program_name", "programName", "ProgramName")))
	meta.TotalLines = int64(utils.InterfaceToFloat64(utils.MapGetFirstRaw(m, "total_lines", "totalLines", "TotalLines")))
	meta.RiskCount = int64(utils.InterfaceToFloat64(utils.MapGetFirstRaw(m, "risk_count", "riskCount", "RiskCount")))
	return meta
}

// createYakitServer initializes a Yakit server with progress and log handlers
func (s *ScanNode) createYakitServer(reporter *ScannerAgentReporter, res *scanrpc.SCAN_InvokeScriptResponse) *yaklib.YakitServer {
	return yaklib.NewYakitServer(
		0,
		yaklib.SetYakitServer_ProgressHandler(func(id string, progress float64) {
			// Scripts and libraries may emit multiple progress streams via SetProgressEx(id, progress).
			// If we treat all of them as the "main" progress, a non-main stream that restarts from 0
			// will shrink the frontend progress bar (e.g. around phase transitions).
			id = strings.TrimSpace(id)
			if id == "" || id == "main" {
				_ = reporter.ReportProcess(progress)
				return
			}
			_ = reporter.ReportProcessWithSubTask(id, progress)
		}),
		yaklib.SetYakitServer_LogHandler(s.createLogHandler(reporter, res)),
	)
}

// createLogHandler creates a log handler for processing various types of scan results
func (s *ScanNode) createLogHandler(reporter *ScannerAgentReporter, res *scanrpc.SCAN_InvokeScriptResponse) func(string, string) {
	return func(level string, info string) {
		level = strings.TrimSpace(level)
		lowerLevel := strings.ToLower(level)
		shrink := info
		if len(info) > 256 {
			shrink = string([]rune(info)[:100]) + "..."
		}
		log.Infof("LEVEL: %v INFO: %v", level, shrink)

		switch lowerLevel {
		case "fingerprint":
			s.handleFingerprintLog(reporter, info)

		case "synscan-result":
			s.handleSynScanLog(reporter, info)

		case "json-risk":
			s.handleRiskLog(reporter, info)

		case "report":
			s.handleReportLog(reporter, info)

		case "json":
			s.handleJSONLog(res, info)

		case "feature-status-card-data":
			s.handleStatusCardLog(reporter, info)

		// SSA stream producer path (yak -> scannode -> mq -> legion).
		// This avoids using risk.NewRisk(type=ssa-risk) as a transport hack.
		case "ssa-stream":
			s.handleSSAStream(reporter, info)
		}
	}
}

func (s *ScanNode) handleSSAStream(reporter *ScannerAgentReporter, info string) {
	if reporter == nil || reporter.ssaCollector == nil {
		return
	}
	if err := reporter.ssaCollector.AddStreamPayload(info); err != nil {
		log.Errorf("collect ssa stream payload failed: %v", err)
	}
}

// handleFingerprintLog processes fingerprint detection results
func (s *ScanNode) handleFingerprintLog(reporter *ScannerAgentReporter, info string) {
	var result fp.MatchResult
	if err := json.Unmarshal([]byte(info), &result); err != nil {
		log.Errorf("unmarshal fingerprint failed: %v", err)
		return
	}
	reporter.ReportFingerprint(&result)
}

// handleSynScanLog processes SYN scan results
func (s *ScanNode) handleSynScanLog(reporter *ScannerAgentReporter, info string) {
	var result synscan.SynScanResult
	if err := json.Unmarshal([]byte(info), &result); err != nil {
		log.Errorf("unmarshal synscan-result failed: %v", err)
		return
	}
	reporter.ReportTCPOpenPort(result.Host, result.Port)
}

// handleRiskLog processes risk/vulnerability detection results
func (s *ScanNode) handleRiskLog(reporter *ScannerAgentReporter, info string) {
	var rawData = make(map[string]interface{})
	if err := json.Unmarshal([]byte(info), &rawData); err != nil {
		log.Errorf("unmarshal risk failed: %s", err)
		return
	}
	if atomic.AddInt32(&riskDebugCount, 1) <= 1 {
		keys := make([]string, 0, len(rawData))
		for k := range rawData {
			keys = append(keys, k)
		}
		detailRaw := utils.MapGetFirstRaw(rawData, "Details", "Detail", "details", "detail")
		log.Infof("risk_log_debug keys=%v RiskType=%v DetailsType=%T DetailsLen=%d", keys, rawData["RiskType"], detailRaw, len(utils.InterfaceToString(detailRaw)))
	}

	// Extract risk title with fallback
	title := utils.MapGetFirstRaw(rawData, "TitleVerbose", "Title")
	if title == "" {
		title = "Untitled Risk"
	}

	// Extract target information
	target := utils.MapGetFirstRaw(rawData, "Url", "url")
	if target == "" {
		host := utils.MapGetString(rawData, "Host")
		port := utils.MapGetString(rawData, "Port")
		target = utils.HostPort(host, port)
	}

	reporter.ReportRisk(fmt.Sprint(title), fmt.Sprint(target), rawData)
}

// handleReportLog processes report generation results
func (s *ScanNode) handleReportLog(reporter *ScannerAgentReporter, info string) {
	reportId, _ := strconv.ParseInt(info, 10, 64)
	if reportId <= 0 {
		return
	}

	db := consts.GetGormProjectDatabase()
	if db == nil {
		return
	}

	reportIns, err := yakit.GetReportRecord(db, reportId)
	if err != nil {
		log.Errorf("query report failed: %s", err)
		return
	}

	reportOutput, err := reportIns.ToReport()
	if err != nil {
		log.Errorf("report marshal from database failed: %s", err)
		return
	}

	if err := reporter.Report(reportOutput); err != nil {
		log.Errorf("report to palm-server failed: %s", err)
	}
}

// handleJSONLog processes generic JSON responses from scripts
func (s *ScanNode) handleJSONLog(res *scanrpc.SCAN_InvokeScriptResponse, info string) {
	var rawData = make(map[string]interface{})
	if err := json.Unmarshal([]byte(info), &rawData); err != nil {
		return
	}

	flag := utils.MapGetFirstRaw(rawData, "Flag", "flag")
	if flag == "ReturnData" {
		data := utils.MapGetFirstRaw(rawData, "Data", "data")
		if data != nil {
			res.Data = data
		}
	}
}

func (s *ScanNode) handleStatusCardLog(reporter *ScannerAgentReporter, info string) {
	result := &spec.ScanResult{
		Type:      spec.ScanResult_StatusCard,
		Content:   []byte(info),
		TaskId:    reporter.TaskId,
		RuntimeId: reporter.RuntimeId,
		SubTaskId: reporter.SubTaskId,
	}
	s.feedback(result)
	if utils.InDebugMode() {
		log.Infof("Reported status card: %s", info)
	}
}

// parseScriptParams parses JSON parameters into a key-value map
func (s *ScanNode) parseScriptParams(jsonParam string) map[string]interface{} {
	paramsKeyValue := make(map[string]interface{})
	var paramsRaw interface{}

	if err := json.Unmarshal([]byte(jsonParam), &paramsRaw); err != nil {
		return paramsKeyValue
	}

	values := utils.InterfaceToGeneralMap(paramsRaw)
	for k, v := range values {
		// Skip internal default parameter
		if k == "__DEFAULT__" {
			continue
		}
		paramsKeyValue[k] = v
	}

	return paramsKeyValue
}

// syncRulesIfNeeded checks for ruleset-hash parameter and synchronizes rules if necessary
func (s *ScanNode) syncRulesIfNeeded(params map[string]interface{}) {
	ruleSetHash, ok := params["ruleset-hash"].(string)
	if !ok || ruleSetHash == "" {
		return
	}

	client := GetRuleSyncClient()
	if client == nil {
		return
	}

	// Check if the ruleset is already cached locally
	if !client.HasLocalSnapshot(ruleSetHash) {
		log.Infof("auto-syncing rules for hash: %s", ruleSetHash)
		ruleCount, err := client.SyncForHash(ruleSetHash)
		if err != nil {
			log.Warnf("auto-sync rules failed: %v, will continue with local rules", err)
		} else {
			log.Infof("auto-synced %d rules from snapshot %s", ruleCount, ruleSetHash)
		}
	} else {
		log.Infof("rules already cached locally for hash: %s", ruleSetHash)
	}
}

// appendKeyValueParams converts key-value parameters to command-line format
func (s *ScanNode) appendKeyValueParams(params []string, keyValues map[string]interface{}) []string {
	for k, v := range keyValues {
		if strings.HasPrefix(k, scannodeInternalParamPrefix) {
			continue
		}
		k = strings.TrimLeft(k, "-")
		params = append(params, "--"+k)
		params = append(params, utils.InterfaceToString(v))
	}
	return params
}

// createTempScriptFile creates a temporary file containing the script content
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

// executeScript executes the Yak script with the specified parameters and environment.
// extraEnv should be in "KEY=VALUE" form.
func (s *ScanNode) executeScript(ctx context.Context, scanNodePath, scriptFile string, params []string, runtimeId string, extraEnv []string) error {
	// Build command with base arguments
	baseCmd := []string{"distyak", scriptFile}
	log.Infof("yak %v %v", scriptFile, params)

	cmd := exec.CommandContext(ctx, scanNodePath, append(baseCmd, params...)...)

	// Setup environment variables: inherit, then override selected keys (avoid duplicate keys).
	overrides := []string{
		fmt.Sprintf("YAKIT_HOME=%v", os.Getenv("YAKIT_HOME")),
		fmt.Sprintf("YAK_RUNTIME_ID=%v", runtimeId),
	}
	overrides = append(overrides, extraEnv...)
	cmd.Env = mergeEnviron(os.Environ(), overrides)

	// Configure remote bridge settings if needed (currently disabled)
	// This section is preserved for future use when remote bridge configuration is provided
	var remoteReverseIP string
	var remoteReversePort int
	var remoteAddr string
	var remoteSecret string
	if remoteReverseIP != "" && remoteReversePort > 0 {
		cmd.Env = append(cmd.Env,
			fmt.Sprintf("YAK_BRIDGE_REMOTE_REVERSE_ADDR=%v", utils.HostPort(remoteReverseIP, remoteReversePort)),
			fmt.Sprintf("YAK_BRIDGE_ADDR=%v", remoteAddr),
			fmt.Sprintf("YAK_BRIDGE_SECRET=%v", remoteSecret),
		)
	}

	// Redirect output to standard streams
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	return cmd.Run()
}

func mergeEnviron(base []string, overrides []string) []string {
	if len(overrides) == 0 {
		return base
	}
	keys := make(map[string]struct{}, len(overrides))
	cleanOverrides := make([]string, 0, len(overrides))
	for _, kv := range overrides {
		idx := strings.IndexByte(kv, '=')
		if idx <= 0 {
			continue
		}
		keys[kv[:idx]] = struct{}{}
		cleanOverrides = append(cleanOverrides, kv)
	}
	out := make([]string, 0, len(base)+len(cleanOverrides))
	for _, kv := range base {
		idx := strings.IndexByte(kv, '=')
		if idx <= 0 {
			continue
		}
		if _, ok := keys[kv[:idx]]; ok {
			continue
		}
		out = append(out, kv)
	}
	out = append(out, cleanOverrides...)
	return out
}

func (s *ScanNode) rpcQueryYakScript(ctx context.Context, node string, req *ypb.QueryYakScriptRequest, broker *mq.Broker) (*scanrpc.SCAN_QueryYakScriptResponse, error) {

	if req.GetNoResultReturn() {
		return &scanrpc.SCAN_QueryYakScriptResponse{
			Pagination: req.GetPagination(),
			Total:      0,
			Data:       nil,
			Groups:     nil,
		}, nil
	}
	p, data, err := yakit.QueryYakScript(consts.GetGormProfileDatabase(), req)
	if err != nil {
		return nil, err
	}

	rsp := &scanrpc.SCAN_QueryYakScriptResponse{
		Pagination: &ypb.Paging{
			Page:    int64(p.Page),
			Limit:   int64(p.Limit),
			OrderBy: req.Pagination.OrderBy,
			Order:   req.Pagination.Order,
		},
		Total: int64(p.TotalRecord),
	}
	for _, d := range data {
		rsp.Data = append(rsp.Data, d.ToGRPCModel())
	}
	var gs []string
	groups, err := yakit.QueryGroupCount(consts.GetGormProfileDatabase(), nil, 0)
	if err != nil {
		return nil, err
	}
	for _, group := range groups {
		if group.IsPocBuiltIn == true {
			continue
		}
		gs = append(gs, group.Value)
	}
	if len(gs) > 0 {
		rsp.Groups = gs
	}
	return rsp, nil
}
