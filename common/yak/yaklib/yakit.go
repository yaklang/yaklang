package yaklib

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"time"

	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils/spacengine/base"

	uuid "github.com/google/uuid"
	"github.com/tidwall/gjson"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/fp"
	"github.com/yaklang/yaklang/common/go-funk"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/synscan"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/cli"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
	"github.com/yaklang/yaklang/common/utils/lowhttp/http_struct"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/sfreport"
	"github.com/yaklang/yaklang/common/yak/yaklib/yakhttp"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

var notifyEmtpyVirtualClientOnce = utils.NewOnce()

var emptyVirtualClient = NewVirtualYakitClient(func(i *ypb.ExecResult) error {
	notifyEmtpyVirtualClientOnce.Do(func() {
		log.Warn("empty yakit client (VirtualYakitClient)")
	})
	log.Info(i.String())
	return nil
})

var YakitExports = map[string]interface{}{
	"NewClient":       NewYakitClient,
	"NewTable":        NewTable,
	"NewLineGraph":    NewLineGraph,
	"NewBarGraph":     NewBarGraph,
	"NewPieGraph":     NewPieGraph,
	"NewWordCloud":    NewWordCloud,
	"NewHTTPFlowRisk": NewHTTPFlowRisk,

	"InitYakit":               InitYakit,
	"UpdateOnlineYakitStore":  updateOnlineYakitStore,
	"UpdateYakitStore":        updateYakitStore,
	"UpdateYakitStoreLocal":   yakit.LoadYakitFromLocalDir,
	"UpdateYakitStoreFromGit": yakit.LoadYakitThirdpartySourceScripts,

	"GenerateYakitMITMHooksParams": generateYakitMITMHookParams,
	"GetHomeDir":                   consts.GetDefaultYakitBaseDir,
	"GetHomeTempDir":               consts.GetDefaultYakitBaseTempDir,
	"GetOnlineBaseUrl":             consts.GetOnlineBaseUrl,
	"SetOnlineBaseUrl":             consts.SetOnlineBaseUrl,

	"MockHTTPFlowSlowSQL": func(seconds ...float64) {
		// 如果没有传入参数，默认使用3秒
		var duration float64 = 3.0
		if len(seconds) > 0 {
			duration = seconds[0]
		}
		// 如果传入的秒数小于2秒，设置为2.1秒，确保超过慢SQL阈值
		if duration < 2.0 {
			duration = 2.1
		}
		dur := time.Duration(float64(time.Second) * duration)
		// 同时触发慢插入和慢查询的模拟
		yakit.MockHTTPFlowSlowInsertSQL(dur)
		yakit.MockHTTPFlowSlowQuerySQL(dur)
	},

	"MockMITMSlowRuleHook": func(seconds ...float64) {
		// 如果没有传入参数，默认使用1秒（1000ms），确保超过300ms阈值
		var duration float64 = 1.0
		if len(seconds) > 0 {
			duration = seconds[0]
		}
		// 如果传入的秒数小于0.3秒，设置为0.4秒，确保超过300ms阈值
		if duration < 0.3 {
			duration = 0.4
		}
		dur := time.Duration(float64(time.Second) * duration)
		ruleCount := 10 // 默认规则数量
		if len(seconds) > 1 {
			ruleCount = int(seconds[1])
		}
		// 一次触发所有三种规则类型
		yakit.MockMITMSlowRuleHook(dur, "hook_color", ruleCount)
		yakit.MockMITMSlowRuleHook(dur, "hook_request", ruleCount)
		yakit.MockMITMSlowRuleHook(dur, "hook_response", ruleCount)
	},

	// dummy
	"Info":           emptyVirtualClient.YakitInfo,
	"Warn":           emptyVirtualClient.YakitWarn,
	"Debug":          emptyVirtualClient.YakitDebug,
	"Error":          emptyVirtualClient.YakitError,
	"Text":           emptyVirtualClient.YakitTextBlock,
	"Success":        emptyVirtualClient.YakitSuccess,
	"Code":           emptyVirtualClient.YakitCode,
	"Markdown":       emptyVirtualClient.YakitMarkdown,
	"Report":         emptyVirtualClient.YakitReport,
	"File":           emptyVirtualClient.YakitFile,
	"Stream":         emptyVirtualClient.Stream,
	"Output":         emptyVirtualClient.Output,
	"SetProgress":    emptyVirtualClient.YakitSetProgress,
	"SetProgressEx":  emptyVirtualClient.YakitSetProgressEx,
	"AIAgentSession": emptyVirtualClient.AIAgentSession,
}

func GetExtYakitLibByOutput(Output func(d any) error) map[string]interface{} {
	exports := map[string]interface{}{}
	exports["EnableWebsiteTrees"] = func(targets string) {
		Output(&YakitFeature{
			Feature: "website-trees",
			Params: map[string]interface{}{
				"targets":          targets,
				"refresh_interval": 3,
			},
		})
	}
	exports["EnableTable"] = func(tableName string, columns []string) {
		Output(&YakitFeature{
			Feature: "fixed-table",
			Params: map[string]interface{}{
				"table_name": tableName,
				"columns":    columns,
			},
		})
	}
	exports["TableData"] = func(tableName string, data any) *YakitFixedTableData {
		tableData := &YakitFixedTableData{
			TableName: tableName,
			Data:      utils.InterfaceToGeneralMap(data),
		}
		if tableData.Data == nil {
			tableData.Data = map[string]interface{}{}
		}
		if tableData.Data["uuid"] == nil {
			tableData.Data["uuid"] = uuid.New().String()
		}
		Output(tableData)
		return nil
	}

	exports["EnableDotGraphTab"] = func(tabName string) {
		Output(&YakitFeature{
			Feature: "dot-graph-tab",
			Params: map[string]interface{}{
				"tab_name": tabName,
			},
		})
	}

	exports["OutputDotGraph"] = func(tabName string, data string) *YakitDotGraphData {
		tabData := &YakitDotGraphData{
			TabName: tabName,
			Data:    data,
		}
		Output(tabData)
		return tabData
	}

	exports["StatusCard"] = func(id string, data interface{}, tags ...string) {
		Output(&YakitStatusCard{
			Id: id, Data: fmt.Sprint(data), Tags: tags,
		})
	}
	return exports
}

func GetExtYakitLibByClient(client *YakitClient) map[string]interface{} {
	YakitExports := map[string]interface{}{
		"Info":           client.YakitInfo,
		"Warn":           client.YakitWarn,
		"Error":          client.YakitError,
		"Text":           client.YakitTextBlock,
		"Success":        client.YakitSuccess,
		"Code":           client.YakitCode,
		"Markdown":       client.YakitMarkdown,
		"Report":         client.YakitReport,
		"File":           client.YakitFile,
		"Output":         client.Output,
		"AIOutput":       client.AIOutput,
		"AIAgentSession": client.AIAgentSession,
		"SetProgress":    client.YakitSetProgress,
		"SetProgressEx":  client.YakitSetProgressEx,
		"Stream":         client.Stream,
		// SSA stream events: a dedicated channel that ScanNode can hook to, avoiding
		// "risk.NewRisk(type=ssa-risk)" as a transport hack.
		//
		// Expect raw JSON string of sfreport.SSAResultParts.
		// Keep it as "raw JSON" to avoid extra marshal/unmarshal cycles in yak runtime.
		"SSAStream": func(partsJSON string) {
			_ = client.YakitLog("ssa-stream", string(partsJSON))
		},
		// EmitSSAResult converts one SyntaxFlowResult to SSA stream payload
		// and sends it through the internal "ssa-stream" channel.
		//
		// Returns (riskCount, fileCount, flowCount, error).
		// Dedup state is kept in current yak process memory (not global package state).
		"EmitSSAResult": func(result *ssaapi.SyntaxFlowResult) (int, int, int, error) {
			opts := sfreport.NewStreamPartsOptions(
				sfreport.WithStreamReportType(sfreport.IRifyFullReportType),
				sfreport.WithStreamShowDataflowPath(true),
				sfreport.WithStreamShowFileContent(true),
				sfreport.WithStreamWithFile(true),
			)
			parts, err := sfreport.ConvertSingleResultToSSAResultParts(result, opts)
			if err != nil {
				return 0, 0, 0, err
			}
			if parts == nil {
				return 0, 0, 0, nil
			}
			if len(parts.Risks) == 0 && len(parts.Files) == 0 && len(parts.Dataflows) == 0 {
				return 0, 0, 0, nil
			}
			partsJSON, err := json.Marshal(parts)
			if err != nil {
				return 0, 0, 0, err
			}
			_ = client.YakitLog("ssa-stream", string(partsJSON))
			return len(parts.Risks), len(parts.Files), len(parts.Dataflows), nil
		},
	}
	if os.Getenv("YAK_DISABLE") == "output" {
		// YakitExports["Info"] = func(a string, b ...interface{}) {}
		YakitExports["Warn"] = func(a string, b ...interface{}) {}
		YakitExports["Debug"] = func(a string, b ...interface{}) {}
		YakitExports["Error"] = func(a string, b ...interface{}) {}
	}

	exports := GetExtYakitLibByOutput(client.Output)
	for k, v := range exports {
		YakitExports[k] = v
	}
	return YakitExports
}

// var yakitClientInstance YakitClient
type YakitMessage struct {
	Type    string          `json:"type"`
	Content json.RawMessage `json:"content"`
}

type YakitProgress struct {
	Id       string  `json:"id"`
	Progress float64 `json:"progress"`
}

type YakitLog struct {
	Level     string `json:"level"`
	Data      string `json:"data"`
	Timestamp int64  `json:"timestamp"`
}

type YakitAIOutput struct {
	Data      string `json:"data"`
	Timestamp int64  `json:"timestamp"`
}

// 格式化输出 YakitLog
func (y *YakitLog) String() string {
	timestamp := time.Unix(y.Timestamp, 0).Format("2006-01-02 15:04:05")
	return fmt.Sprintf("[%s] %s %s\n", y.Level, timestamp, y.Data)
}

func NewYakitStatusCardExecResult(status string, data any, items ...string) *ypb.ExecResult {
	card := &YakitStatusCard{
		Id:   status,
		Data: fmt.Sprint(data),
		Tags: items,
	}
	level, data := MarshalYakitOutput(card)
	return NewYakitLogExecResult(level, data)
}

func ConvertExecResultIntoAIToolCallStdoutLog(i *ypb.ExecResult, onlyAIOutputOpt ...bool) string {
	onlyAIOutput := false
	if len(onlyAIOutputOpt) > 0 {
		onlyAIOutput = onlyAIOutputOpt[0]
	}

	if onlyAIOutput {
		if !i.IsMessage {
			return ""
		}
		var yakitMsg YakitMessage
		err := json.Unmarshal(i.Message, &yakitMsg)
		if err != nil {
			return ""
		}
		if yakitMsg.Type != "log" {
			return ""
		}
		var logInfo YakitLog
		err = json.Unmarshal(yakitMsg.Content, &logInfo)
		if err != nil {
			return ""
		}
		if logInfo.Level != "ai-output" {
			return ""
		}
		var aiOutput YakitAIOutput
		err = json.Unmarshal([]byte(logInfo.Data), &aiOutput)
		if err != nil {
			return ""
		}
		// timestamp := time.Unix(aiOutput.Timestamp, 0).Format(time.RFC3339)
		// return fmt.Sprintf("[%s] %s", timestamp, aiOutput.Data)
		return aiOutput.Data
	}

	if utils.IsNil(i) {
		return ""
	}
	if !i.IsMessage {
		return string(i.Raw)
	}
	var yakitMsg YakitMessage
	err := json.Unmarshal(i.Message, &yakitMsg)
	if err != nil {
		return i.String()
	}
	// progress/status-card should not be printed to AI stdout
	switch yakitMsg.Type {
	case "progress", "status-card":
		return ""
	}
	if yakitMsg.Type == "log" {
		var logInfo YakitLog
		err := json.Unmarshal(yakitMsg.Content, &logInfo)
		if err != nil {
			return i.String()
		}
		// do NOT show noisy / UI-only logs to AI stdout
		switch logInfo.Level {
		case "file", "debug", "progress":
			return ""
		case "json-risk":
			return convertRiskLogToReadable(logInfo.Data)
		}
		return fmt.Sprintf("[%s] %s", logInfo.Level, logInfo.Data)
	}
	return i.String()
}

func convertRiskLogToReadable(data string) string {
	var riskData map[string]any
	if err := json.Unmarshal([]byte(data), &riskData); err != nil {
		return "[risk] " + data
	}

	severity, _ := riskData["Severity"].(string)
	if severity == "" {
		severity = "info"
	}
	title, _ := riskData["Title"].(string)
	if title == "" {
		if tv, ok := riskData["TitleVerbose"].(string); ok {
			title = tv
		}
	}
	url, _ := riskData["Url"].(string)
	riskType, _ := riskData["RiskType"].(string)

	var parts []string
	parts = append(parts, fmt.Sprintf("[risk][%s] %s", strings.ToUpper(severity), title))
	if url != "" {
		parts = append(parts, fmt.Sprintf("URL: %s", url))
	}
	if riskType != "" {
		parts = append(parts, fmt.Sprintf("Type: %s", riskType))
	}
	return strings.Join(parts, " | ")
}

// convertYakitFileLogToSummary extracts a brief summary from yakit.File log data
// to prevent flooding stdout with large file content
func convertYakitFileLogToSummary(data string) string {
	var fileData map[string]any
	if err := json.Unmarshal([]byte(data), &fileData); err != nil {
		return ""
	}

	path, _ := fileData["path"].(string)
	action, _ := fileData["action"].(string)

	if path == "" {
		return ""
	}

	switch action {
	case "READ":
		return fmt.Sprintf("[file] read file: %s", path)
	case "WRITE":
		return fmt.Sprintf("[file] write file: %s", path)
	case "STATUS":
		return fmt.Sprintf("[file] stat file: %s", path)
	case "FIND":
		return fmt.Sprintf("[file] find in: %s", path)
	case "CREATE":
		return fmt.Sprintf("[file] create file: %s", path)
	case "DELETE":
		return fmt.Sprintf("[file] delete file: %s", path)
	case "CHMOD":
		return fmt.Sprintf("[file] chmod file: %s", path)
	default:
		return fmt.Sprintf("[file] operation on: %s", path)
	}
}

func NewYakitLogExecResult(level string, input any, items ...interface{}) *ypb.ExecResult {
	logItem := &YakitLog{
		Level:     level,
		Timestamp: time.Now().Unix(),
	}
	data := utils.InterfaceToString(input)
	if len(items) > 0 {
		logItem.Data = fmt.Sprintf(data, items...)
	} else {
		logItem.Data = data
	}

	raw, _ := YakitMessageGenerator(logItem)
	return &ypb.ExecResult{
		IsMessage: true,
		Message:   raw,
	}
}

func NewYakitProgressExecResult(id string, progress float64) *ypb.ExecResult {
	p := &YakitProgress{
		Id:       id,
		Progress: progress,
	}
	raw, _ := YakitMessageGenerator(p)
	return &ypb.ExecResult{
		IsMessage: true,
		Message:   raw,
	}
}

type YakitServer struct {
	port   int
	server *lowhttp.WebHookServer

	// handleProgress
	progressHandler func(id string, progress float64)
	logHandler      func(level string, info string)
}

func SetYakitServer_ProgressHandler(h func(id string, progress float64)) func(s *YakitServer) {
	return func(s *YakitServer) {
		s.progressHandler = h
	}
}

func SetYakitServer_LogHandler(h func(level string, info string)) func(s *YakitServer) {
	return func(s *YakitServer) {
		s.logHandler = h
	}
}

func (s *YakitServer) handleRaw(raw []byte) {
	var msg YakitMessage
	_ = json.Unmarshal(raw, &msg)
	switch strings.ToLower(msg.Type) {
	case "progress", "prog":
		if s.progressHandler == nil {
			return
		}
		var prog YakitProgress
		err := json.Unmarshal(msg.Content, &prog)
		if err != nil {
			log.Errorf("unmarshal progress failed: %s", err)
			return
		}
		s.progressHandler(prog.Id, prog.Progress)
	case "log":
		if s.logHandler == nil {
			return
		}
		var logInfo YakitLog
		err := json.Unmarshal(msg.Content, &logInfo)
		if err != nil {
			log.Errorf("unmarshal log failed: %s", err)
			return
		}
		s.logHandler(logInfo.Level, logInfo.Data)
	}
}

func NewYakitServer(port int, opts ...func(server *YakitServer)) *YakitServer {
	var err error
	if port <= 0 {
		port, err = utils.GetRangeAvailableTCPPort(50000, 65535, 3)
		if err != nil {
			port = utils.GetRandomAvailableTCPPort()
		}
	}

	s := &YakitServer{
		port: port,
	}
	for _, opt := range opts {
		opt(s)
	}
	s.server = lowhttp.NewWebHookServerEx(port, func(data interface{}) {
		switch ret := data.(type) {
		case *http.Request:
			if ret == nil {
				return
			}
			if ret.RemoteAddr != "" {
				log.Infof("remote addr: %s", ret.RemoteAddr)
			}

			if ret.Body != nil {
				raw, _ := ioutil.ReadAll(ret.Body)
				if raw != nil {
					s.handleRaw(raw)
				}
			}
		}
	})
	return s
}

func (s *YakitServer) Start() {
	s.server.Start()
	return
}

func (s *YakitServer) Addr() string {
	if s.server == nil {
		return ""
	}
	return s.server.Addr()
}

func (s *YakitServer) Shutdown() {
	if s.server == nil {
		return
	}
	s.server.Shutdown()
}

// yaktable
type YakitTable struct {
	Head []string   `json:"head"`
	Data [][]string `json:"data"`
}

// NewTable 创建一个 Yakit 表格对象（导出名为 yakit.NewTable）
// 参数:
//   - head: 表头列名
//
// 返回值:
//   - 表格对象
//
// Example:
// ```
// table = yakit.NewTable("name", "age")
// table.Append("alice", 18)
// dump(table)
// ```
func NewTable(head ...string) *YakitTable {
	return &YakitTable{
		Head: head,
		Data: nil,
	}
}

func (y *YakitTable) SetHead(head ...string) {
	y.Head = head
}

func (y *YakitTable) Append(data ...interface{}) {
	var res []string
	for _, r := range data {
		res = append(res, fmt.Sprint(r))
	}
	y.Data = append(y.Data, res)
}

func MarshalYakitOutput(t interface{}) (string, string) {
	raw, err := json.Marshal(t)
	if err != nil {
		return "", ""
	}

	switch ret := t.(type) {
	case *fp.MatchResult:
		return "fingerprint", string(raw)
	case *synscan.SynScanResult:
		return "synscan-result", string(raw)
	case *schema.Risk:
		ret.CreatedAt = time.Now()
		ret.UpdatedAt = time.Now()
		output := ret.ToGRPCModel()
		a, err := utils.ToMapParams(output)
		if err != nil {
			return "", ""
		}
		a["Request"] = funk.Map(output.Request, func(i byte) uint {
			return uint(i)
		}).([]uint)
		a["Response"] = funk.Map(output.Response, func(i byte) uint {
			return uint(i)
		}).([]uint)
		raw, err := json.Marshal(a)
		if err != nil {
			return "", ""
		}
		return "json-risk", string(raw)
	case *schema.HTTPFlow:
		return "json-httpflow", string(raw)
	case *YakitTable:
		return "json-table", string(raw)
	case *YakitGraph:
		return "json-graph", string(raw)
	case *YakitFeature:
		return "json-feature", string(raw)
	case *YakitHTTPFlowRisk:
		return "json-httpflow-risk", string(raw)
	case *YakitFixedTableData:
		return "feature-table-data", string(raw)
	case *YakitDotGraphData:
		return "dot-graph-data", string(raw)
	case *YakitAIOutput:
		return "ai-output", string(raw)
	case *YakitTextTabData:
		return "feature-text-data", string(raw)
	case *YakitStatusCard:
		return "feature-status-card-data", string(raw)
	case *ypb.ExecResult:
		if ret.IsMessage {
			contentResult := gjson.Parse(string(ret.Message)).Get("content")
			level := contentResult.Get("level").String()
			data := contentResult.Get("data").String()
			return level, data
		}
		return "json", string(raw)
	case string:
		return "info", utils.EscapeInvalidUTF8Byte([]byte(ret))
	case []byte:
		return "info", utils.EscapeInvalidUTF8Byte(ret)
	default:
		return "json", string(raw)
	}
}

func NewPortFromMatchResult(f *fp.MatchResult) *schema.Port {
	return &schema.Port{
		Host:        f.Target,
		Port:        f.Port,
		Proto:       string(f.GetProto()),
		ServiceType: f.GetServiceName(),
		State:       f.State.String(),
		Reason:      f.Reason,
		Fingerprint: f.GetBanner(),
		CPE:         strings.Join(f.GetCPEs(), "|"),
		From:        "servicescan",
		HtmlTitle:   f.GetHtmlTitle(),
	}
}

func NewPortFromSpaceEngineResult(f *base.NetSpaceEngineResult) *schema.Port {
	host, port, _ := utils.ParseStringToHostPort(f.Addr)
	return &schema.Port{
		Host:        host,
		Port:        port,
		Proto:       "tcp",
		ServiceType: f.Fingerprints,
		State:       "open",
		Fingerprint: f.Banner,
		HtmlTitle:   f.HtmlTitle,
		From:        "spacengine",
	}
}

func NewPortFromSynScanResult(f *synscan.SynScanResult) *schema.Port {
	return &schema.Port{
		Host:  f.Host,
		Port:  f.Port,
		Proto: "tcp",
		State: "open",
	}
}

func YakitMessageGenerator(i interface{}) ([]byte, error) {
	raw, err := json.Marshal(i)
	if err != nil {
		return nil, err
	}

	msg := &YakitMessage{}
	switch i.(type) {
	case *YakitStatusCard:
		msg.Type = "status-card"
		msg.Content = raw
	case *YakitProgress:
		msg.Type = "progress"
		msg.Content = raw
	case *YakitLog:
		msg.Type = "log"
		msg.Content = raw
	default:
		return nil, utils.Errorf("unknown type: %v", reflect.TypeOf(i))
	}

	return json.Marshal(msg)
}

// 设置基本图形
type YakitGraph struct {
	// line / bar / pie
	Name string             `json:"name"`
	Type string             `json:"type"`
	Data []*yakitGraphValue `json:"data"`
}

type yakitGraphValue struct {
	Id    string      `json:"id"`
	Key   string      `json:"key"`
	Value interface{} `json:"value"`
}

func (y *YakitGraph) Add(k string, v interface{}, id ...string) {
	y.Data = append(y.Data, &yakitGraphValue{
		Id:    strings.Join(id, ""),
		Key:   k,
		Value: v,
	})
}

var graphBaseName = "数据图表"

// NewLineGraph 创建一个折线图对象（导出名为 yakit.NewLineGraph）
// 参数:
//   - graphName: 可选的图表名称
//
// 返回值:
//   - 图表对象
//
// Example:
// ```
// graph = yakit.NewLineGraph("trend")
// graph.Add("day1", 10)
// dump(graph)
// ```
func NewLineGraph(graphName ...string) *YakitGraph {
	name := graphBaseName
	if len(graphName) > 0 {
		name = graphName[0]
	}
	return &YakitGraph{
		Name: name,
		Type: "line",
	}
}

// NewBarGraph 创建一个柱状图对象（导出名为 yakit.NewBarGraph）
// 参数:
//   - graphName: 可选的图表名称
//
// 返回值:
//   - 图表对象
//
// Example:
// ```
// graph = yakit.NewBarGraph("count")
// graph.Add("a", 3)
// dump(graph)
// ```
func NewBarGraph(graphName ...string) *YakitGraph {
	name := graphBaseName
	if len(graphName) > 0 {
		name = graphName[0]
	}
	return &YakitGraph{
		Name: name,
		Type: "bar",
	}
}

// NewPieGraph 创建一个饼图对象（导出名为 yakit.NewPieGraph）
// 参数:
//   - graphName: 可选的图表名称
//
// 返回值:
//   - 图表对象
//
// Example:
// ```
// graph = yakit.NewPieGraph("ratio")
// graph.Add("a", 30)
// dump(graph)
// ```
func NewPieGraph(graphName ...string) *YakitGraph {
	name := graphBaseName
	if len(graphName) > 0 {
		name = graphName[0]
	}
	return &YakitGraph{
		Name: name,
		Type: "pie",
	}
}

// NewWordCloud 创建一个词云图对象（导出名为 yakit.NewWordCloud）
// 参数:
//   - graphName: 可选的图表名称
//
// 返回值:
//   - 图表对象
//
// Example:
// ```
// graph = yakit.NewWordCloud("words")
// graph.Add("security", 100)
// dump(graph)
// ```
func NewWordCloud(graphName ...string) *YakitGraph {
	name := graphBaseName
	if len(graphName) > 0 {
		name = graphName[0]
	}
	return &YakitGraph{
		Name: name,
		Type: "wordcloud",
	}
}

var (
	yakitClientInstance  *YakitClient
	yakitClientInstanceP = &yakitClientInstance
)

func GetYakitClientInstance() *YakitClient {
	return yakitClientInstance
}

// YakitTextBlock 向 Yakit 输出一个文本块（导出名为 yakit.Text）
// 参数:
//   - tmp: 文本内容
//
// 返回值:
//   - 无
//
// Example:
// ```
// yakit.Text("scan finished")
// ```
func (c *YakitClient) YakitTextBlock(tmp interface{}) {
	c.YakitDraw("text", tmp)
}

// YakitSuccess 向 Yakit 输出一条成功信息（导出名为 yakit.Success）
// 参数:
//   - tmp: 成功信息内容
//
// 返回值:
//   - 无
//
// Example:
// ```
// yakit.Success("task done")
// ```
func (c *YakitClient) YakitSuccess(tmp interface{}) {
	c.YakitDraw("success", tmp)
}

// YakitCode 向 Yakit 输出一段代码块（导出名为 yakit.Code）
// 参数:
//   - tmp: 代码内容
//
// 返回值:
//   - 无
//
// Example:
// ```
// yakit.Code("println(\"hello\")")
// ```
func (c *YakitClient) YakitCode(tmp interface{}) {
	c.YakitDraw("code", tmp)
}

// YakitMarkdown 向 Yakit 输出一段 Markdown（导出名为 yakit.Markdown）
// 参数:
//   - tmp: Markdown 内容
//
// 返回值:
//   - 无
//
// Example:
// ```
// yakit.Markdown("# Title\nsome content")
// ```
func (c *YakitClient) YakitMarkdown(tmp interface{}) {
	c.YakitDraw("markdown", tmp)
}

// YakitReport 向 Yakit 输出一个报告引用（按报告 ID，导出名为 yakit.Report）
// 参数:
//   - i: 报告 ID
//
// 返回值:
//   - 无
//
// Example:
// ```
// yakit.Report(1)
// ```
func (c *YakitClient) YakitReport(i int) {
	c.YakitDraw("report", fmt.Sprint(i))
}

// YakitFile 向 Yakit 输出一个文件信息卡片或文件操作记录（导出名为 yakit.File）
// 参数:
//   - fileName: 文件路径
//   - option: 可选项，可为标题/描述字符串或 YakitFileAction 文件操作
//
// 返回值:
//   - 无
//
// Example:
// ```
// yakit.File("/tmp/result.txt", "Scan Result", "the result of this scan")
// ```
func (c *YakitClient) YakitFile(fileName string, option ...interface{}) {
	var rawDesc []string
	var yakitFileAction []*YakitFileAction
	for _, o := range option {
		switch o.(type) {
		case string:
			rawDesc = append(rawDesc, o.(string))
		case YakitFileAction:
			action := o.(YakitFileAction)
			yakitFileAction = append(yakitFileAction, &action)
		case *YakitFileAction:
			action := o.(*YakitFileAction)
			yakitFileAction = append(yakitFileAction, action)
		}
	}

	isDir := utils.IsDir(fileName)
	dir := fileName
	if !isDir {
		dir = filepath.Dir(dir)
	}

	if len(rawDesc) > 0 {
		descStr := ""
		title := rawDesc[0]
		if len(rawDesc) > 1 {
			descStr = utils.InterfaceToString(funk.Reduce(rawDesc[1:], func(i interface{}, s interface{}) string {
				return utils.InterfaceToString(i) + "," + utils.InterfaceToString(s)
			}, ""))
			descStr = strings.Trim(descStr, " \r\n,")
		}
		existed, _ := utils.PathExists(fileName)
		var size uint64
		if existed && !isDir {
			if info, _ := os.Stat(fileName); info != nil {
				size = uint64(info.Size())
			}
		}
		raw, err := json.Marshal(map[string]interface{}{
			"title":       title,
			"description": descStr,
			"path":        fileName,
			"is_dir":      utils.IsDir(fileName),
			"dir":         dir,
			"is_existed":  existed,
			"file_size":   utils.ByteSize(size),
		})
		if err != nil {
			log.Errorf("error for build file struct data: %v", err)
			return
		}
		c.YakitDraw("file", string(raw))
	}

	for _, action := range yakitFileAction {
		raw, err := json.Marshal(map[string]interface{}{
			"title":          fmt.Sprintf("operation file [%s] use asction [%s]", fileName, action.Action),
			"action":         action.Action,
			"path":           fileName,
			"is_dir":         utils.IsDir(fileName),
			"dir":            dir,
			"action_message": action.Message,
		})
		if err != nil {
			log.Errorf("error for build file struct data: %v", err)
			return
		}
		c.YakitDraw("file", string(raw))
	}

}

// YakitError 向 Yakit 输出一条 error 级别日志（导出名为 yakit.Error）
// 参数:
//   - tmp: 日志格式字符串
//   - items: 格式化参数
//
// 返回值:
//   - 无
//
// Example:
// ```
// yakit.Error("scan failed: %v", "timeout")
// ```
func (c *YakitClient) YakitError(tmp string, items ...interface{}) {
	c.YakitLog("error", tmp, items...)
}

// YakitInfo 向 Yakit 输出一条 info 级别日志（导出名为 yakit.Info）
// 参数:
//   - tmp: 日志格式字符串
//   - items: 格式化参数
//
// 返回值:
//   - 无
//
// Example:
// ```
// yakit.Info("scanning target: %s", "example.com")
// ```
func (c *YakitClient) YakitInfo(tmp string, items ...interface{}) {
	c.YakitLog("info", tmp, items...)
}

// YakitDebug 向 Yakit 输出一条 debug 级别日志（导出名为 yakit.Debug）
// 参数:
//   - tmp: 日志格式字符串
//   - items: 格式化参数
//
// 返回值:
//   - 无
//
// Example:
// ```
// yakit.Debug("debug value: %v", 123)
// ```
func (c *YakitClient) YakitDebug(tmp string, items ...interface{}) {
	c.YakitLog("debug", tmp, items...)
}

// YakitWarn 向 Yakit 输出一条 warn 级别日志（导出名为 yakit.Warn）
// 参数:
//   - tmp: 日志格式字符串
//   - items: 格式化参数
//
// 返回值:
//   - 无
//
// Example:
// ```
// yakit.Warn("deprecated option: %s", "old-flag")
// ```
func (c *YakitClient) YakitWarn(tmp string, items ...interface{}) {
	c.YakitLog("warn", tmp, items...)
}

// AIAgentSession 向 Yakit 输出并注册一个 AI Agent 会话 ID（导出名为 yakit.AIAgentSession）
// 参数:
//   - sessionID: 会话 ID
//   - source: 可选的会话来源标识
//
// 返回值:
//   - 无
//
// Example:
// ```
// yakit.AIAgentSession("my-session-id")
// ```
func (c *YakitClient) AIAgentSession(sessionID string, source ...string) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return
	}
	_ = c.YakitLog("ai_agent_session", sessionID)
	if db := consts.GetGormProjectDatabase(); db != nil {
		if err := yakit.RegisterAIAgentSession(db, sessionID, source...); err != nil {
			log.Warnf("register ai agent session in yakit db failed: %v", err)
		}
	}
}

// AIOutput 向 Yakit 输出可被 AI 工具 stdout 过滤的 AI 专用输出（导出名为 yakit.AIOutput）
// AIOutput writes AI-focused output that can be filtered for AI tool stdout.
// aiLevel is an optional category for downstream consumers.
// 参数:
//   - tmp: 输出格式字符串
//   - items: 格式化参数
//
// 返回值:
//   - 无
//
// Example:
// ```
// yakit.AIOutput("analysis result: %s", "ok")
// ```
func (c *YakitClient) AIOutput(tmp string, items ...interface{}) {
	var data = tmp
	if len(items) > 0 {
		data = fmt.Sprintf(tmp, items...)
	}
	logItem := &YakitAIOutput{
		Data:      data,
		Timestamp: time.Now().Unix(),
	}
	c.Output(logItem)
}

func init() {
	AutoInitYakit()
}

// InitYakit 初始化全局 Yakit 客户端实例（导出名为 yakit.InitYakit）
// 参数:
//   - y: Yakit 客户端对象
//
// 返回值:
//   - 无
//
// Example:
// ```
// client = yakit.NewClient("http://127.0.0.1:8080/webhook")
// yakit.InitYakit(client)
// ```
func InitYakit(y *YakitClient) {
	*yakitClientInstanceP = y
}

func AutoInitYakit() *YakitClient {
	if yakitClientInstance != nil {
		return nil
	}
	addr := cli.DefaultCliApp.String("yakit-webhook")
	if addr != "" {
		client := NewYakitClient(addr)
		InitYakit(client)
		return client
	} else {
		InitYakit(emptyVirtualClient)
		return emptyVirtualClient
	}
}

// updateYakitStore 从本地数据库更新 Yakit 插件商店（导出名为 yakit.UpdateYakitStore）
// 参数:
//   - 无
//
// 返回值:
//   - 错误信息
//
// Example:
// ```
// // 依赖本地数据库（示意性示例）
// yakit.UpdateYakitStore()~
// ```
func updateYakitStore() error {
	db := consts.GetGormProfileDatabase()
	if db == nil {
		return utils.Errorf("no database found")
	}

	return yakit.UpdateYakitStore(db, "")
}

// YakitSetProgressEx 设置指定 ID 的进度条进度（导出名为 yakit.SetProgressEx）
// 参数:
//   - id: 进度条 ID
//   - f: 进度值（0.0-1.0）
//
// 返回值:
//   - 无
//
// Example:
// ```
// yakit.SetProgressEx("download", 0.5)
// ```
func (c *YakitClient) YakitSetProgressEx(id string, f float64) {
	c.send(&YakitProgress{
		Id:       id,
		Progress: f,
	})
}

// YakitSetProgress 设置主进度条进度（导出名为 yakit.SetProgress）
// 参数:
//   - f: 进度值（0.0-1.0）
//
// 返回值:
//   - 无
//
// Example:
// ```
// yakit.SetProgress(0.5)
// ```
func (c *YakitClient) YakitSetProgress(f float64) {
	c.send(&YakitProgress{
		Id:       "main",
		Progress: f,
	})
}

// mitm risk
type YakitHTTPFlowRisk struct {
	RiskName  string   `json:"risk_name"`
	Url       string   `json:"url"`
	IsHTTPS   bool     `json:"is_https"`
	Highlight string   `json:"highlight"`
	Request   []byte   `json:"request"`
	Response  []byte   `json:"response"`
	Fragment  []string `json:"fragment"`

	// low / middle / high / critical
	Level string `json:"level"`
}

func (y *YakitHTTPFlowRisk) SetFragment(item ...string) {
	y.Fragment = item
}

func (y *YakitHTTPFlowRisk) SetLevel(l string) {
	switch strings.ToLower(l) {
	case "info", "debug", "low":
		y.Level = "low"
		return
	case "warning", "middle", "medium":
		y.Level = "middle"
	case "error", "high":
		y.Level = "high"
		return
	case "critical", "panic", "fatal":
		y.Level = "critical"
		return
	default:
		y.Level = "low"
		return
	}
}

// NewHTTPFlowRisk 创建一个 HTTP 流量风险对象（导出名为 yakit.NewHTTPFlowRisk）
// 参数:
//   - riskName: 风险名称
//   - isHttps: 是否为 HTTPS
//   - url: 关联 URL
//   - req: 原始请求字节
//   - rsp: 原始响应字节
//
// 返回值:
//   - HTTP 流量风险对象
//
// Example:
// ```
// risk = yakit.NewHTTPFlowRisk("XSS", true, "https://example.com", reqBytes, rspBytes)
// risk.SetLevel("high")
// dump(risk)
// ```
func NewHTTPFlowRisk(
	riskName string,
	isHttps bool, url string,
	req []byte, rsp []byte,
) *YakitHTTPFlowRisk {
	return &YakitHTTPFlowRisk{
		RiskName: riskName,
		Url:      url,
		IsHTTPS:  isHttps,
		Request:  req,
		Response: rsp,
		Level:    "low",
	}
}

// updateOnlineYakitStore 从在线商店下载并保存全部 Yakit 插件（导出名为 yakit.UpdateOnlineYakitStore）
// 参数:
//   - 无
//
// 返回值:
//   - 错误信息
//
// Example:
// ```
// // 需要联网及本地数据库（示意性示例）
// yakit.UpdateOnlineYakitStore()~
// ```
func updateOnlineYakitStore() error {
	client := NewOnlineClient(consts.GetOnlineBaseUrl())
	stream := client.DownloadYakitPluginAll(context.Background())
	if stream == nil || stream.Chan == nil {
		return utils.Errorf("download plugin failed: %s", "empty stream")
	}

	var total int64 = 0
	var current int64 = 0
	for i := range stream.Chan {
		if i.Total > 0 {
			total = i.Total
		}
		current++
		err := client.Save(consts.GetGormProfileDatabase(), i.Plugin)
		if err != nil {
			log.Errorf("save [%v/%v] plugin [%s] failed: %s", current, total, i.Plugin.ScriptName, err)
		} else {
			log.Infof("save [%v/%v] plugin [%s] failed: %s", current, total, i.Plugin.ScriptName, err)
		}
	}
	return nil
}

// generateYakitMITMHookParams 发起一次 HTTP 请求并生成可传给 MITM hook 的参数列表（导出名为 yakit.GenerateYakitMITMHooksParams）
// 参数:
//   - method: HTTP 方法
//   - url: 请求 URL
//   - opts: HTTP 请求可选项
//
// 返回值:
//   - 参数列表（isHttps, url, reqRaw, rspRaw, body）
//   - 错误信息
//
// Example:
// ```
// // 需要联网（示意性示例）
// params = yakit.GenerateYakitMITMHooksParams("GET", "http://example.com")~
// dump(params)
// ```
func generateYakitMITMHookParams(method string, url string, opts ...http_struct.HttpOption) ([]interface{}, error) {
	isHttps := false
	if strings.HasPrefix(url, "https://") {
		isHttps = true
	}

	req, err := yakhttp.NewHttpNewRequest(method, url, opts...)
	if err != nil {
		return nil, err
	}

	reqRaw, err := utils.HttpDumpWithBody(req.Request, true)
	if err != nil {
		return nil, err
	}

	rsp, err := yakhttp.Do(req)
	if err != nil {
		return nil, err
	}

	rspRaw, err := utils.HttpDumpWithBody(rsp, true)
	if err != nil {
		return nil, err
	}

	rspRaw, body, err := lowhttp.FixHTTPResponse(rspRaw)
	if err != nil {
		return nil, err
	}

	return []interface{}{
		isHttps, url, reqRaw, rspRaw, body,
	}, nil
}
