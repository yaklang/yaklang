package scannode

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/fp"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/synscan"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/yaklib"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
)

func (s *ScanNode) createYakitServer(
	reporter *ScannerAgentReporter,
	result *ScriptExecutionResult,
) *yaklib.YakitServer {
	return yaklib.NewYakitServer(
		0,
		yaklib.SetYakitServer_ProgressHandler(func(id string, progress float64) {
			logReporterEventError("job progress", reporter.ReportProcess(progress))
		}),
		yaklib.SetYakitServer_LogHandler(s.createLogHandler(reporter, result)),
	)
}

func (s *ScanNode) createLogHandler(
	reporter *ScannerAgentReporter,
	result *ScriptExecutionResult,
) func(string, string) {
	return func(level string, info string) {
		logScriptMessage(level, info)

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
			s.handleJSONLog(result, info)
		case "feature-status-card-data":
			s.handleStatusCardLog(reporter, info)
		}
	}
}

func logScriptMessage(level string, info string) {
	message := info
	if len(info) > 256 {
		message = string([]rune(info)[:100]) + "..."
	}
	log.Infof("LEVEL: %v INFO: %v", level, message)
}

func (s *ScanNode) handleFingerprintLog(
	reporter *ScannerAgentReporter,
	info string,
) {
	var result fp.MatchResult
	if err := json.Unmarshal([]byte(info), &result); err != nil {
		log.Errorf("unmarshal fingerprint failed: %v", err)
		return
	}
	logReporterEventError("job asset service_fingerprint", reporter.ReportFingerprint(&result))
}

func (s *ScanNode) handleSynScanLog(
	reporter *ScannerAgentReporter,
	info string,
) {
	var result synscan.SynScanResult
	if err := json.Unmarshal([]byte(info), &result); err != nil {
		log.Errorf("unmarshal synscan-result failed: %v", err)
		return
	}
	logReporterEventError(
		"job asset tcp_open_port",
		reporter.ReportTCPOpenPort(result.Host, result.Port),
	)
}

func (s *ScanNode) handleRiskLog(
	reporter *ScannerAgentReporter,
	info string,
) {
	var rawData map[string]any
	if err := json.Unmarshal([]byte(info), &rawData); err != nil {
		log.Errorf("unmarshal risk failed: %s", err)
		return
	}

	title := utils.MapGetFirstRaw(rawData, "TitleVerbose", "Title")
	if title == "" {
		title = "Untitled Risk"
	}
	target := utils.MapGetFirstRaw(rawData, "Url", "url")
	if target == "" {
		target = utils.HostPort(
			utils.MapGetString(rawData, "Host"),
			utils.MapGetString(rawData, "Port"),
		)
	}
	logReporterEventError(
		"job risk",
		reporter.ReportRisk(fmt.Sprint(title), fmt.Sprint(target), rawData),
	)
}

func (s *ScanNode) handleReportLog(
	reporter *ScannerAgentReporter,
	info string,
) {
	reportID, _ := strconv.ParseInt(info, 10, 64)
	if reportID <= 0 {
		return
	}

	db := consts.GetGormProjectDatabase()
	if db == nil {
		return
	}
	reportIns, err := yakit.GetReportRecord(db, reportID)
	if err != nil {
		log.Errorf("query report failed: %s", err)
		return
	}
	reportOutput, err := reportIns.ToReport()
	if err != nil {
		log.Errorf("report marshal from database failed: %s", err)
		return
	}
	logReporterEventError("job report", reporter.Report(reportOutput))
}

func (s *ScanNode) handleJSONLog(
	result *ScriptExecutionResult,
	info string,
) {
	var rawData map[string]any
	if err := json.Unmarshal([]byte(info), &rawData); err != nil {
		return
	}

	flag := utils.MapGetFirstRaw(rawData, "Flag", "flag")
	if flag != "ReturnData" {
		return
	}

	data := utils.MapGetFirstRaw(rawData, "Data", "data")
	if data != nil {
		result.Data = data
	}
}

func (s *ScanNode) handleStatusCardLog(
	_ *ScannerAgentReporter,
	info string,
) {
	if utils.InDebugMode() {
		log.Infof("skip feature-status-card-data for legion event projection: %s", info)
	}
}
