package scannode

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"

	uuid "github.com/google/uuid"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/fp"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/mq"
	"github.com/yaklang/yaklang/common/synscan"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/yaklib"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"github.com/yaklang/yaklang/scannode/scanrpc"
)

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
		return nil, utils.Errorf("exec yakScript %v failed: %s", scriptFile, err)
	}

	return res, nil
}

// createYakitServer initializes a Yakit server with progress and log handlers
func (s *ScanNode) createYakitServer(reporter *ScannerAgentReporter, res *scanrpc.SCAN_InvokeScriptResponse) *yaklib.YakitServer {
	return yaklib.NewYakitServer(
		0,
		yaklib.SetYakitServer_ProgressHandler(func(id string, progress float64) {
			reporter.ReportProcess(progress)
		}),
		yaklib.SetYakitServer_LogHandler(s.createLogHandler(reporter, res)),
	)
}

// createLogHandler creates a log handler for processing various types of scan results
func (s *ScanNode) createLogHandler(reporter *ScannerAgentReporter, res *scanrpc.SCAN_InvokeScriptResponse) func(string, string) {
	return func(level string, info string) {
		shrink := info
		if len(info) > 256 {
			shrink = string([]rune(info)[:100]) + "..."
		}
		log.Infof("LEVEL: %v INFO: %v", level, shrink)

		switch strings.ToLower(level) {
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
		}
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
