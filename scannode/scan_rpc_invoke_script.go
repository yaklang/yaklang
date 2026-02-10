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

	uuid "github.com/google/uuid"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/fp"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/mq"
	"github.com/yaklang/yaklang/common/spec"
	"github.com/yaklang/yaklang/common/synscan"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/ssaapi/sfreport"
	"github.com/yaklang/yaklang/common/yak/yaklib"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"github.com/yaklang/yaklang/scannode/scanrpc"
)

var riskDebugCount int32

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

	// Setup Yakit server for receiving callbacks from script execution
	yakitServer := s.createYakitServer(reporter, res)
	yakitServer.Start()
	defer yakitServer.Shutdown()

	// Build base command-line parameters
	params := []string{"--yakit-webhook", yakitServer.Addr()}
	if runtimeId != "" {
		params = append(params, "--runtime-id", runtimeId)
	}

	// Parse and convert user-provided JSON parameters to command-line arguments
	paramsKeyValue := s.parseScriptParams(req.ScriptJsonParam)

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
	if err := s.executeScript(ctx, scanNodePath, scriptFile, params, runtimeId); err != nil {
		log.Errorf("exec yakScript %v failed: %s", scriptFile, err)

		// 使用脚本通过 yakit.Output 返回的错误详情
		if detailedError := extractErrorFromResponse(res); detailedError != "" {
			return nil, utils.Errorf("%s", detailedError)
		}

		// 返回通用错误
		return nil, utils.Errorf("exec yakScript %v failed: %s", scriptFile, err)
	}

	// If stream-layered SSA events were emitted, send a final task_end marker after the script truly finishes.
	// This allows the server to finalize without relying on idle timeouts, while preserving audit completeness.
	if s.streamer != nil && s.streamer.Enabled() {
		var totalRisks, totalFiles, totalFlows int64
		if m, ok := res.Data.(map[string]interface{}); ok && m != nil {
			totalRisks = int64(utils.InterfaceToFloat64(utils.MapGetFirstRaw(m, "risk_count", "riskCount", "RiskCount")))
			totalFiles = int64(utils.InterfaceToFloat64(utils.MapGetFirstRaw(m, "file_count", "fileCount", "FileCount")))
			totalFlows = int64(utils.InterfaceToFloat64(utils.MapGetFirstRaw(m, "flow_count", "flowCount", "FlowCount")))
		}
		s.streamer.EmitTaskEnd(req.TaskId, req.RuntimeId, req.SubTaskId, totalRisks, totalFiles, totalFlows)
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
		// This avoids using risk.NewRisk(type=ssa-risk) as a transport hack, and removes the need
		// for the "SSA Risk Chunk ..." first-layer chunking entirely.
		case "ssa-stream-task-start":
			s.handleSSAStreamTaskStart(reporter, info)
		case "ssa-stream-file":
			s.handleSSAStreamFile(reporter, info)
		case "ssa-stream-dataflow":
			s.handleSSAStreamDataflow(reporter, info)
		case "ssa-stream-risk":
			s.handleSSAStreamRisk(reporter, info)
		case "ssa-stream-parts":
			s.handleSSAStreamParts(reporter, info)
		case "ssa-stream-parts-raw":
			s.handleSSAStreamPartsRaw(reporter, info)
		}
	}
}

func (s *ScanNode) handleSSAStreamTaskStart(reporter *ScannerAgentReporter, info string) {
	if reporter == nil || reporter.agent == nil || reporter.agent.streamer == nil || !reporter.agent.streamer.Enabled() {
		return
	}
	var ev struct {
		Program    string `json:"program"`
		ReportType string `json:"report_type"`
	}
	if err := json.Unmarshal([]byte(info), &ev); err != nil {
		log.Errorf("unmarshal ssa-stream-task-start failed: %v", err)
		return
	}
	// Always send task_start with forced seq to preserve ordering.
	reporter.agent.streamer.EmitSSATaskStart(reporter.TaskId, reporter.RuntimeId, reporter.SubTaskId, ev.Program, ev.ReportType)
}

func (s *ScanNode) handleSSAStreamFile(reporter *ScannerAgentReporter, info string) {
	if reporter == nil || reporter.agent == nil || reporter.agent.streamer == nil || !reporter.agent.streamer.Enabled() {
		return
	}
	var f sfreport.File
	if err := json.Unmarshal([]byte(info), &f); err != nil {
		log.Errorf("unmarshal ssa-stream-file failed: %v", err)
		return
	}
	if strings.TrimSpace(f.IrSourceHash) == "" {
		return
	}
	_ = reporter.agent.streamer.EmitSSAFile(reporter.TaskId, reporter.RuntimeId, reporter.SubTaskId, &f)
}

func (s *ScanNode) handleSSAStreamDataflow(reporter *ScannerAgentReporter, info string) {
	if reporter == nil || reporter.agent == nil || reporter.agent.streamer == nil || !reporter.agent.streamer.Enabled() {
		return
	}
	var ev struct {
		DataflowHash string          `json:"dataflow_hash"`
		Hash         string          `json:"hash"`
		Payload      json.RawMessage `json:"payload"`
	}
	if err := json.Unmarshal([]byte(info), &ev); err != nil {
		log.Errorf("unmarshal ssa-stream-dataflow failed: %v", err)
		return
	}
	h := strings.TrimSpace(ev.DataflowHash)
	if h == "" {
		h = strings.TrimSpace(ev.Hash)
	}
	if h == "" || len(ev.Payload) == 0 {
		return
	}
	_ = reporter.agent.streamer.EmitSSADataflow(reporter.TaskId, reporter.RuntimeId, reporter.SubTaskId, h, []byte(ev.Payload))
}

func (s *ScanNode) handleSSAStreamRisk(reporter *ScannerAgentReporter, info string) {
	if reporter == nil || reporter.agent == nil || reporter.agent.streamer == nil || !reporter.agent.streamer.Enabled() {
		return
	}
	var ev spec.StreamRiskEvent
	if err := json.Unmarshal([]byte(info), &ev); err != nil {
		log.Errorf("unmarshal ssa-stream-risk failed: %v", err)
		return
	}
	if strings.TrimSpace(ev.RiskHash) == "" || len(ev.RiskJSON) == 0 {
		return
	}
	_ = reporter.agent.streamer.EmitSSARisk(reporter.TaskId, reporter.RuntimeId, reporter.SubTaskId, &ev)
}

func (s *ScanNode) handleSSAStreamParts(reporter *ScannerAgentReporter, info string) {
	if reporter == nil || reporter.agent == nil || reporter.agent.streamer == nil || !reporter.agent.streamer.Enabled() {
		return
	}
	var parts sfreport.StreamSingleResultParts
	if err := json.Unmarshal([]byte(info), &parts); err != nil {
		log.Errorf("unmarshal ssa-stream-parts failed: %v", err)
		return
	}
	reporter.agent.streamer.EmitSSATaskStart(reporter.TaskId, reporter.RuntimeId, reporter.SubTaskId, parts.ProgramName, parts.ReportType)

	if len(parts.Files) > 0 {
		for _, f := range parts.Files {
			if f == nil {
				continue
			}
			_ = reporter.agent.streamer.EmitSSAFile(reporter.TaskId, reporter.RuntimeId, reporter.SubTaskId, f)
		}
	}
	if len(parts.Dataflows) > 0 {
		for _, df := range parts.Dataflows {
			if df == nil || strings.TrimSpace(df.DataflowHash) == "" || len(df.Payload) == 0 {
				continue
			}
			_ = reporter.agent.streamer.EmitSSADataflow(reporter.TaskId, reporter.RuntimeId, reporter.SubTaskId, df.DataflowHash, []byte(df.Payload))
		}
	}
	if len(parts.Risks) > 0 {
		for _, r := range parts.Risks {
			if r == nil || strings.TrimSpace(r.RiskHash) == "" || len(r.RiskJSON) == 0 {
				continue
			}
			ev := &spec.StreamRiskEvent{
				RiskHash:       r.RiskHash,
				ProgramName:    parts.ProgramName,
				ReportType:     parts.ReportType,
				RiskJSON:       r.RiskJSON,
				FileHashes:     r.FileHashes,
				DataflowHashes: r.DataflowHashes,
			}
			_ = reporter.agent.streamer.EmitSSARisk(reporter.TaskId, reporter.RuntimeId, reporter.SubTaskId, ev)
		}
	}
}

func (s *ScanNode) handleSSAStreamPartsRaw(reporter *ScannerAgentReporter, info string) {
	if reporter == nil || reporter.agent == nil || reporter.agent.streamer == nil || !reporter.agent.streamer.Enabled() {
		return
	}
	// info is expected to be a raw JSON string of StreamSingleResultParts.
	raw := strings.TrimSpace(info)
	if raw == "" {
		return
	}
	var parts sfreport.StreamSingleResultParts
	if err := json.Unmarshal([]byte(raw), &parts); err != nil {
		log.Errorf("unmarshal ssa-stream-parts-raw failed: %v", err)
		return
	}
	reporter.agent.streamer.EmitSSATaskStart(reporter.TaskId, reporter.RuntimeId, reporter.SubTaskId, parts.ProgramName, parts.ReportType)

	for _, f := range parts.Files {
		if f == nil {
			continue
		}
		_ = reporter.agent.streamer.EmitSSAFile(reporter.TaskId, reporter.RuntimeId, reporter.SubTaskId, f)
	}
	for _, df := range parts.Dataflows {
		if df == nil || strings.TrimSpace(df.DataflowHash) == "" || len(df.Payload) == 0 {
			continue
		}
		_ = reporter.agent.streamer.EmitSSADataflow(reporter.TaskId, reporter.RuntimeId, reporter.SubTaskId, df.DataflowHash, []byte(df.Payload))
	}
	for _, r := range parts.Risks {
		if r == nil || strings.TrimSpace(r.RiskHash) == "" || len(r.RiskJSON) == 0 {
			continue
		}
		ev := &spec.StreamRiskEvent{
			RiskHash:       r.RiskHash,
			ProgramName:    parts.ProgramName,
			ReportType:     parts.ReportType,
			RiskJSON:       r.RiskJSON,
			FileHashes:     r.FileHashes,
			DataflowHashes: r.DataflowHashes,
		}
		_ = reporter.agent.streamer.EmitSSARisk(reporter.TaskId, reporter.RuntimeId, reporter.SubTaskId, ev)
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

// executeScript executes the Yak script with the specified parameters and environment
func (s *ScanNode) executeScript(ctx context.Context, scanNodePath, scriptFile string, params []string, runtimeId string) error {
	// Build command with base arguments
	baseCmd := []string{"distyak", scriptFile}
	log.Infof("yak %v %v", scriptFile, params)

	cmd := exec.CommandContext(ctx, scanNodePath, append(baseCmd, params...)...)

	// Setup environment variables
	cmd.Env = append(os.Environ(),
		fmt.Sprintf("YAKIT_HOME=%v", os.Getenv("YAKIT_HOME")),
		fmt.Sprintf("YAK_RUNTIME_ID=%v", runtimeId),
	)

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
