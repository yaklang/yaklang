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

	"MockHTTPFlowSlowSQL": mockHTTPFlowSlowSQL,

	"MockMITMSlowRuleHook": mockMITMSlowRuleHook,

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

// MockHTTPFlowSlowSQL 模拟 HTTP 流量入库时的慢 SQL（导出名为 yakit.MockHTTPFlowSlowSQL）
// 用于测试/演示慢 SQL 监控：会同时触发慢插入与慢查询，持续时间确保超过慢 SQL 阈值
//
// 参数:
//   - seconds: 可选的持续秒数，默认 3 秒；小于 2 秒会被提升到 2.1 秒以确保越过阈值
//
// Example:
// ```
// // 触发一次约 3 秒的慢 SQL 模拟（用于验证慢 SQL 监控，示意性示例）
// yakit.MockHTTPFlowSlowSQL(3)
// ```
func mockHTTPFlowSlowSQL(seconds ...float64) {
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
}

// MockMITMSlowRuleHook 模拟 MITM 规则 Hook 的慢执行（导出名为 yakit.MockMITMSlowRuleHook）
// 用于测试/演示慢 Hook 监控：会一次性触发 hook_color/hook_request/hook_response 三类规则
//
// 参数:
//   - seconds: 可选参数，第一个为持续秒数（默认 1 秒，小于 0.3 秒会提升到 0.4 秒以越过 300ms 阈值），第二个为规则数量（默认 10）
//
// Example:
// ```
// // 触发一次约 1 秒、每类 10 条规则的慢 Hook 模拟（用于验证慢 Hook 监控，示意性示例）
// yakit.MockMITMSlowRuleHook(1, 10)
// ```
func mockMITMSlowRuleHook(seconds ...float64) {
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
		"SSAStream":     client.SSAStream,
		"EmitSSAResult": client.EmitSSAResult,
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
	// 用带文档的具名方法覆盖上面来自闭包的同名导出，便于文档生成器提取参数/返回值说明
	YakitExports["EnableWebsiteTrees"] = client.EnableWebsiteTrees
	YakitExports["EnableTable"] = client.EnableTable
	YakitExports["TableData"] = client.TableData
	YakitExports["EnableDotGraphTab"] = client.EnableDotGraphTab
	YakitExports["OutputDotGraph"] = client.OutputDotGraph
	YakitExports["StatusCard"] = client.StatusCard
	return YakitExports
}

// EnableWebsiteTrees 在 Yakit UI 中启用「网站树」展示标签（导出名为 yakit.EnableWebsiteTrees）
// 用于在插件运行时让 Yakit 展示指定目标的网站结构树
//
// 参数:
//   - targets: 目标（如域名/URL），多个可用逗号等分隔
//
// Example:
// ```
// // 在 Yakit 中启用网站树展示（需在 Yakit 引擎环境下生效，示意性示例）
// yakit.EnableWebsiteTrees("example.com")
// ```
func (c *YakitClient) EnableWebsiteTrees(targets string) {
	c.Output(&YakitFeature{
		Feature: "website-trees",
		Params: map[string]interface{}{
			"targets":          targets,
			"refresh_interval": 3,
		},
	})
}

// EnableTable 在 Yakit UI 中启用一个动态固定表格用于实时展示数据（导出名为 yakit.EnableTable）
//
// 与 yakit.NewTable（静态、一次性输出）不同：EnableTable 先声明一张“持续可写”的表格，
// 之后在扫描过程中用 yakit.TableData 不断往里增量加行，界面实时刷新。适合“边扫边出结果”的体验。
// 用法：EnableTable 声明列 -> 循环中多次调用 TableData 写行（每行需有唯一 uuid，TableData 会自动补全）。
//
// 参数:
//   - tableName: 表格名称（后续 TableData 用同名表格写入）
//   - columns: 表格列名列表
//
// Example:
// ```
// // 实时端口扫描结果表：声明表 -> 边扫边写行（需在 Yakit 引擎环境下展示）
// yakit.EnableTable("Port Result", ["host", "port", "service"])
// findings = [["10.0.0.1", "80", "http"], ["10.0.0.1", "443", "https"], ["10.0.0.2", "22", "ssh"]]
// for f in findings {
//     yakit.TableData("Port Result", {"host": f[0], "port": f[1], "service": f[2]})
//     yakit.Info("found %s:%s", f[0], f[1])
// }
// ```
func (c *YakitClient) EnableTable(tableName string, columns []string) {
	c.Output(&YakitFeature{
		Feature: "fixed-table",
		Params: map[string]interface{}{
			"table_name": tableName,
			"columns":    columns,
		},
	})
}

// TableData 向 Yakit UI 中已启用的固定表格写入一行数据（导出名为 yakit.TableData）
//
// 与 yakit.EnableTable 配对使用：先 EnableTable 声明表格，再用本函数逐行写入。data 是一个 map，
// 其键应与 EnableTable 声明的列名对应。每行需要一个唯一标识 "uuid"，若 data 中未提供会自动生成；
// 用相同 uuid 再次写入可“更新”同一行（例如先写“扫描中”，拿到结果后用同 uuid 更新为“完成”）。
//
// 参数:
//   - tableName: 目标表格名称（需与 EnableTable 一致）
//   - data: 行数据 map，键为列名；可含 "uuid" 控制行标识
//
// 返回值:
//   - 始终返回 nil（数据通过 Yakit 输出通道异步发送）
//
// Example:
// ```
// // 用固定 uuid 实现“同一行”的状态更新：扫描中 -> 完成
// yakit.EnableTable("Task", ["target", "status"])
// rowId = "row-10.0.0.1"
// yakit.TableData("Task", {"uuid": rowId, "target": "10.0.0.1", "status": "scanning"})
// sleep(0.1)
// yakit.TableData("Task", {"uuid": rowId, "target": "10.0.0.1", "status": "done"})   // 更新同一行
// ```
func (c *YakitClient) TableData(tableName string, data any) *YakitFixedTableData {
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
	c.Output(tableData)
	return nil
}

// EnableDotGraphTab 在 Yakit UI 中启用一个 DOT 图标签页（导出名为 yakit.EnableDotGraphTab）
// 启用后可配合 yakit.OutputDotGraph 向该标签页输出 Graphviz DOT 图
//
// 参数:
//   - tabName: 标签页名称
//
// Example:
// ```
// // 启用 DOT 图标签页并输出一张图（需在 Yakit 引擎环境下生效，示意性示例）
// yakit.EnableDotGraphTab("Graph")
// yakit.OutputDotGraph("Graph", "digraph G { a -> b }")
// ```
func (c *YakitClient) EnableDotGraphTab(tabName string) {
	c.Output(&YakitFeature{
		Feature: "dot-graph-tab",
		Params: map[string]interface{}{
			"tab_name": tabName,
		},
	})
}

// OutputDotGraph 向 Yakit UI 中已启用的 DOT 图标签页输出一张 Graphviz DOT 图（导出名为 yakit.OutputDotGraph）
// 需先通过 yakit.EnableDotGraphTab 启用同名标签页
//
// 参数:
//   - tabName: 目标标签页名称（需与 EnableDotGraphTab 一致）
//   - data: Graphviz DOT 图字符串
//
// 返回值:
//   - 本次输出的 DOT 图数据对象
//
// Example:
// ```
// // 输出一张简单的 DOT 图（需在 Yakit 引擎环境下生效，示意性示例）
// yakit.EnableDotGraphTab("Graph")
// yakit.OutputDotGraph("Graph", "digraph G { a -> b }")
// ```
func (c *YakitClient) OutputDotGraph(tabName string, data string) *YakitDotGraphData {
	tabData := &YakitDotGraphData{
		TabName: tabName,
		Data:    data,
	}
	c.Output(tabData)
	return tabData
}

// SSAStream 通过内部 "ssa-stream" 通道发送一段原始 JSON 字符串（导出名为 yakit.SSAStream）
// 供 ScanNode 等订阅 SSA 扫描事件，避免使用 risk.NewRisk(type=ssa-risk) 作为传输 hack
//
// 参数:
//   - partsJSON: sfreport.SSAResultParts 的原始 JSON 字符串
//
// Example:
// ```
// // 发送一段 SSA 结果 JSON（需在 Yakit 引擎环境下生效，示意性示例）
// yakit.SSAStream("{}")
// ```
func (c *YakitClient) SSAStream(partsJSON string) {
	_ = c.YakitLog("ssa-stream", string(partsJSON))
}

// EmitSSAResult 将单个 SyntaxFlowResult 转换为 SSA 流式负载，并通过内部 "ssa-stream" 通道发送（导出名为 yakit.EmitSSAResult）
// 去重状态保存在当前 yak 进程内存中（非全局包状态）
//
// 参数:
//   - result: 一个 SyntaxFlow 查询结果
//
// 返回值:
//   - 风险数量
//   - 文件数量
//   - 数据流数量
//   - 错误信息
//
// Example:
// ```
// // 发送一个 SyntaxFlow 结果（需在 Yakit 引擎环境下生效，示意性示例）
// riskCount, fileCount, flowCount, err = yakit.EmitSSAResult(result)
// ```
func (c *YakitClient) EmitSSAResult(result *ssaapi.SyntaxFlowResult) (int, int, int, error) {
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
	_ = c.YakitLog("ssa-stream", string(partsJSON))
	return len(parts.Risks), len(parts.Files), len(parts.Dataflows), nil
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

// NewTable 创建一个 Yakit 静态表格对象（导出名为 yakit.NewTable）
//
// 用于一次性汇总展示结构化数据：先用表头创建表格，再用 table.Append(列1, 列2, ...) 逐行追加，
// 最后用 yakit.Output(table) 推送到 Yakit 界面渲染。适合“收集完再统一展示”的场景。
// 若需要在扫描过程中实时往同一张表里增量加行，用 yakit.EnableTable + yakit.TableData。
//
// 参数:
//   - head: 表头列名（可变参数），Append 的列数应与表头一致
//
// 返回值:
//   - 表格对象，支持 .Append(...) 追加行、.SetHead(...) 重设表头
//
// Example:
// ```
// // 把一批端口扫描结果汇总成一张表并展示（建表->逐行追加->输出 联动）
// table = yakit.NewTable("Host", "Port", "Service")
// rows = [["10.0.0.1", "80", "http"], ["10.0.0.1", "443", "https"], ["10.0.0.2", "22", "ssh"]]
// for r in rows {
//     table.Append(r[0], r[1], r[2])
// }
// yakit.Output(table)
// yakit.Info("rendered %d rows", len(rows))
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
//
// 折线图适合展示“随时间/序列变化的趋势”。用 graph.Add(键, 值) 逐点添加数据，再用 yakit.Output(graph) 渲染。
// 四种图表构造器（NewLineGraph/NewBarGraph/NewPieGraph/NewWordCloud）用法完全一致，仅展示形态不同。
//
// 参数:
//   - graphName: 可选的图表名称（不传则使用默认名）
//
// 返回值:
//   - 图表对象，支持 .Add(key, value) 添加数据点
//
// Example:
// ```
// // 展示一天中各时段发现的开放端口数量趋势
// graph = yakit.NewLineGraph("open ports by hour")
// graph.Add("00:00", 120)
// graph.Add("06:00", 98)
// graph.Add("12:00", 203)
// graph.Add("18:00", 156)
// yakit.Output(graph)
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
//
// 柱状图适合“分类对比”：比较各类别的数量大小。用 graph.Add(类别, 数量) 添加，再用 yakit.Output(graph) 渲染。
//
// 参数:
//   - graphName: 可选的图表名称
//
// 返回值:
//   - 图表对象，支持 .Add(key, value) 添加数据点
//
// Example:
// ```
// // 联动：统计一批端口结果里各端口出现的次数，用柱状图对比
// scanned = ["80", "443", "80", "22", "80", "443"]
// counter = {}
// for port in scanned {
//     if port in counter { counter[port] = counter[port] + 1 } else { counter[port] = 1 }
// }
// graph = yakit.NewBarGraph("ports distribution")
// for p, c in counter { graph.Add(p, c) }
// yakit.Output(graph)
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
//
// 饼图适合展示“占比/构成”：各部分在总量中所占比例。用 graph.Add(类别, 数量) 添加，再用 yakit.Output(graph) 渲染。
//
// 参数:
//   - graphName: 可选的图表名称
//
// 返回值:
//   - 图表对象，支持 .Add(key, value) 添加数据点
//
// Example:
// ```
// // 展示漏洞按危险等级的构成占比
// graph = yakit.NewPieGraph("vuln severity")
// graph.Add("critical", 2)
// graph.Add("high", 5)
// graph.Add("medium", 12)
// graph.Add("low", 30)
// yakit.Output(graph)
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
//
// 词云适合展示“关键词频率”：词的大小正比于其权重值。用 graph.Add(词, 权重) 添加，再用 yakit.Output(graph) 渲染。
// 常用于展示漏洞类型分布、指纹关键词、高频参数名等。
//
// 参数:
//   - graphName: 可选的图表名称
//
// 返回值:
//   - 图表对象，支持 .Add(key, value) 添加数据点
//
// Example:
// ```
// // 展示漏洞类型关键词云，词越大代表出现次数越多
// graph = yakit.NewWordCloud("vuln keywords")
// graph.Add("SQL Injection", 35)
// graph.Add("XSS", 28)
// graph.Add("Weak Password", 30)
// graph.Add("SSRF", 15)
// yakit.Output(graph)
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
//
// 与 yakit.Info 的区别：Info 是“一行日志”，Text 是“一整块文本”，适合输出多行内容（如汇总报告、配置清单、
// banner 抓取结果）。注意 Text 接收已拼好的字符串，不做 printf 格式化（需格式化请先用 sprintf/f-string 拼好）。
// 输出代码用 yakit.Code，输出 Markdown 用 yakit.Markdown。
//
// 参数:
//   - tmp: 要展示的文本内容（可多行）
//
// 返回值:
//   - 无
//
// Example:
// ```
// // 把一段多行的扫描小结作为整块文本输出
// summary = sprintf("Scan Summary\n  targets: %d\n  open ports: %d\n  vulns: %d", 10, 23, 2)
// yakit.Text(summary)
// ```
func (c *YakitClient) YakitTextBlock(tmp interface{}) {
	c.YakitDraw("text", tmp)
}

// YakitSuccess 向 Yakit 输出一条成功信息（导出名为 yakit.Success）
//
// 用于标记关键步骤成功完成，在 Yakit 中以成功样式（绿色）展示，比普通 Info 更醒目。
// 常用于“发现漏洞”“爆破命中”“任务完成”等正向结果。注意：本函数接收已拼好的字符串，不做 printf 格式化。
//
// 参数:
//   - tmp: 成功信息内容（如需格式化请先用 sprintf/f-string 拼好）
//
// 返回值:
//   - 无
//
// Example:
// ```
// // 在命中结果时给出醒目的成功提示
// user = "admin"; pass = "admin123"
// yakit.Success(sprintf("credential hit: %s / %s", user, pass))
// yakit.Success("all 5 tasks finished")
// ```
func (c *YakitClient) YakitSuccess(tmp interface{}) {
	c.YakitDraw("success", tmp)
}

// YakitCode 向 Yakit 输出一段代码块（导出名为 yakit.Code）
//
// 在 Yakit 中以代码块样式（等宽字体、保留缩进）展示，适合输出原始 HTTP 报文、PoC 片段、配置文件、payload 等
// 需要保留格式的内容，比 yakit.Text 更适合展示“代码/报文”这类结构化文本。
//
// 参数:
//   - tmp: 代码或报文内容
//
// 返回值:
//   - 无
//
// Example:
// ```
// // 把一个用于复现的原始 HTTP 请求作为代码块展示，方便使用者复制
// poc = `POST /login HTTP/1.1
// Host: target.example.com
// Content-Type: application/x-www-form-urlencoded
//
// username=admin&password=' OR '1'='1`
// yakit.Code(poc)
// ```
func (c *YakitClient) YakitCode(tmp interface{}) {
	c.YakitDraw("code", tmp)
}

// YakitMarkdown 向 Yakit 输出一段 Markdown（导出名为 yakit.Markdown）
//
// 在 Yakit 中渲染 Markdown，支持标题、列表、表格、加粗等，适合输出结构化的扫描报告/结论。
// 是把统计结果做成“可读报告”的常用方式，常与 db 查询统计、yakit 图表配合，形成图文并茂的输出。
//
// 参数:
//   - tmp: Markdown 文本内容
//
// 返回值:
//   - 无
//
// Example:
// ```
// // 把扫描结论拼成 Markdown 报告输出（带标题、列表与表格）
// report = `# Scan Report
//
// ## Overview
// - hosts: 10
// - open ports: 23
// - findings: 2
//
// ## Findings
// | severity | title |
// |----------|-------|
// | high     | SQL Injection |
// | medium   | Reflected XSS |
// `
// yakit.Markdown(report)
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
//
// 用于输出错误信息，在 Yakit 中以错误样式（红色）展示。用法与 yakit.Info 一致，支持格式化。
// 常与 try-catch 或带错误返回的调用配合，把失败原因清晰反馈给使用者。
//
// 参数:
//   - tmp: 日志内容或 printf 风格的格式字符串
//   - items: 与格式字符串对应的格式化参数（可变参数）
//
// 返回值:
//   - 无
//
// Example:
// ```
// // 捕获异常并把错误信息上报到 Yakit
// try {
//     rsp, req = poc.Get("http://127.0.0.1:1/", poc.timeout(1))~   // 故意失败
//     yakit.Info("status: %v", rsp.RawPacket)
// } catch err {
//     yakit.Error("request failed: %v", err)
// }
// ```
func (c *YakitClient) YakitError(tmp string, items ...interface{}) {
	c.YakitLog("error", tmp, items...)
}

// YakitInfo 向 Yakit 输出一条 info 级别日志（导出名为 yakit.Info）
//
// 这是插件向 Yakit 界面输出运行信息的主力函数：在 Yakit 中以 info 级别日志展示，命令行运行时打印到控制台。
// 支持 printf 风格的格式化（%s/%d/%v 等），第一个参数为格式字符串、其余为对应参数。
// 配套的还有 yakit.Warn（警告）、yakit.Error（错误）、yakit.Success（成功）、yakit.Debug（调试），
// 它们仅日志级别/颜色不同，用法一致。按照规范，日志内容统一用英文输出。
//
// 参数:
//   - tmp: 日志内容或 printf 风格的格式字符串
//   - items: 与格式字符串对应的格式化参数（可变参数）
//
// 返回值:
//   - 无
//
// Example:
// ```
// // 普通输出与格式化输出
// yakit.Info("scanner started")
// target = "example.com"; port = 443
// yakit.Info("scanning target: %s:%d", target, port)
//
// // 联动：边查询资产边输出进度信息，是插件里最常见的写法
// total = 0
// for u in db.QueryUrlsByKeyword("example.com") {
//     total++
//     yakit.Info("found url: %s", u)
// }
// yakit.Info("collected %d urls in total", total)
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
//
// 用于输出“需要注意但不致命”的信息，在 Yakit 中以警告样式展示。用法与 yakit.Info 完全一致，支持格式化。
// 典型场景：某个目标不可达而跳过、命中可疑特征、使用了不推荐的配置等。
//
// 参数:
//   - tmp: 日志内容或 printf 风格的格式字符串
//   - items: 与格式字符串对应的格式化参数（可变参数）
//
// 返回值:
//   - 无
//
// Example:
// ```
// // 对单个目标做容错时，用 Warn 记录被跳过的原因而不中断整体流程
// for host in ["10.0.0.1", "10.0.0.2"] {
//     if host == "10.0.0.2" {
//         yakit.Warn("target %s unreachable, skipped", host)
//         continue
//     }
//     yakit.Info("processing %s", host)
// }
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

// AutoInitYakit 根据命令行参数自动初始化 Yakit 客户端（导出名为 yakit.AutoInitYakit）
// 若已初始化则直接返回 nil；否则读取 --yakit-webhook 参数：有地址则创建 webhook 客户端，
// 无地址则使用一个空的虚拟客户端（输出被丢弃），便于脚本在有无 Yakit 环境下都能运行
//
// 返回值:
//   - 初始化得到的 Yakit 客户端；若此前已初始化则返回 nil
//
// Example:
// ```
// // 自动初始化 Yakit 客户端（无 --yakit-webhook 时使用空客户端，示意性示例）
// client = yakit.AutoInitYakit()
// yakit.Info("hello from yak")
// ```
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
//
// 与 yakit.SetProgress 的区别：本函数带一个 id，可同时维护“多条独立进度条”，每个 id 对应一条。
// 适合并发/多阶段任务（如同时跑端口扫描和指纹识别，各自一条进度条）。相同 id 的后续调用会更新同一条进度条。
//
// 参数:
//   - id: 进度条 ID（不同 id 对应不同进度条）
//   - f: 进度值，取值范围 0.0~1.0
//
// 返回值:
//   - 无
//
// Example:
// ```
// // 维护两条独立进度条，分别表示两个阶段的进度
// for i = 0; i < 5; i++ {
//     yakit.SetProgressEx("port-scan",   float(i + 1) / 5.0)
//     yakit.SetProgressEx("fingerprint", float(i + 1) / 10.0)
//     sleep(0.05)
// }
// yakit.SetProgressEx("port-scan", 1.0)
// ```
func (c *YakitClient) YakitSetProgressEx(id string, f float64) {
	c.send(&YakitProgress{
		Id:       id,
		Progress: f,
	})
}

// YakitSetProgress 设置主进度条进度（导出名为 yakit.SetProgress）
//
// 在 Yakit 任务界面更新主进度条。进度值是 0.0~1.0 的小数（0.0 表示 0%，1.0 表示 100%）。
// 典型用法：在遍历目标的循环里用 已完成数/总数 实时刷新进度。多任务并行时用 yakit.SetProgressEx 区分不同进度条。
//
// 参数:
//   - f: 进度值，取值范围 0.0~1.0
//
// 返回值:
//   - 无
//
// Example:
// ```
// // 在循环中按 已完成/总数 刷新主进度条，结束时置满
// targets = ["10.0.0.1", "10.0.0.2", "10.0.0.3", "10.0.0.4"]
// for i = 0; i < len(targets); i++ {
//     yakit.Info("scanning %s", targets[i])
//     yakit.SetProgress(float(i + 1) / float(len(targets)))
// }
// yakit.SetProgress(1.0)
// yakit.Success("scan completed")
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
