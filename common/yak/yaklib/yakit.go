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
	"github.com/yaklang/yaklang/common/yak/yaklib/yakhttp"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

var emptyVirtualClient = NewVirtualYakitClient(func(i *ypb.ExecResult) error {
	return fmt.Errorf("empty virtual client")
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

	// dummy
	"Info":          emptyVirtualClient.YakitInfo,
	"Warn":          emptyVirtualClient.YakitWarn,
	"Debug":         emptyVirtualClient.YakitDebug,
	"Error":         emptyVirtualClient.YakitError,
	"Text":          emptyVirtualClient.YakitTextBlock,
	"Success":       emptyVirtualClient.YakitSuccess,
	"Code":          emptyVirtualClient.YakitCode,
	"Markdown":      emptyVirtualClient.YakitMarkdown,
	"Report":        emptyVirtualClient.YakitReport,
	"File":          emptyVirtualClient.YakitFile,
	"Stream":        emptyVirtualClient.Stream,
	"Output":        emptyVirtualClient.Output,
	"SetProgress":   emptyVirtualClient.YakitSetProgress,
	"SetProgressEx": emptyVirtualClient.YakitSetProgressEx,
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
		"Info":          client.YakitInfo,
		"Warn":          client.YakitWarn,
		"Error":         client.YakitError,
		"Text":          client.YakitTextBlock,
		"Success":       client.YakitSuccess,
		"Code":          client.YakitCode,
		"Markdown":      client.YakitMarkdown,
		"Report":        client.YakitReport,
		"File":          client.YakitFile,
		"Output":        client.Output,
		"SetProgress":   client.YakitSetProgress,
		"SetProgressEx": client.YakitSetProgressEx,
		"Stream":        client.Stream,
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

func NewYakitStatusCardExecResult(status string, data any, items ...string) *ypb.ExecResult {
	card := &YakitStatusCard{
		Id:   status,
		Data: fmt.Sprint(data),
		Tags: items,
	}
	raw, _ := YakitMessageGenerator(card)
	return &ypb.ExecResult{
		IsMessage: true,
		Message:   raw,
	}
}

func ConvertExecResultIntoLog(i *ypb.ExecResult) string {
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
	if yakitMsg.Type == "log" {
		var logInfo YakitLog
		err := json.Unmarshal(yakitMsg.Content, &logInfo)
		if err != nil {
			return i.String()
		}
		return fmt.Sprintf("[%s] %s", logInfo.Level, logInfo.Data)
	}
	return i.String()
}

func NewYakitLogExecResult(level string, data string, items ...interface{}) *ypb.ExecResult {
	logItem := &YakitLog{
		Level:     level,
		Timestamp: time.Now().Unix(),
	}
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
	raw, _ := json.Marshal(&YakitProgress{
		Id:       id,
		Progress: progress,
	})
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

func (c *YakitClient) YakitTextBlock(tmp interface{}) {
	c.YakitDraw("text", tmp)
}

func (c *YakitClient) YakitSuccess(tmp interface{}) {
	c.YakitDraw("success", tmp)
}

func (c *YakitClient) YakitCode(tmp interface{}) {
	c.YakitDraw("code", tmp)
}

func (c *YakitClient) YakitMarkdown(tmp interface{}) {
	c.YakitDraw("markdown", tmp)
}

func (c *YakitClient) YakitReport(i int) {
	c.YakitDraw("report", fmt.Sprint(i))
}

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

func (c *YakitClient) YakitError(tmp string, items ...interface{}) {
	c.YakitLog("error", tmp, items...)
}

func (c *YakitClient) YakitInfo(tmp string, items ...interface{}) {
	c.YakitLog("info", tmp, items...)
}

func (c *YakitClient) YakitDebug(tmp string, items ...interface{}) {
	c.YakitLog("debug", tmp, items...)
}

func (c *YakitClient) YakitWarn(tmp string, items ...interface{}) {
	c.YakitLog("warn", tmp, items...)
}

func init() {
	AutoInitYakit()
}

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

func updateYakitStore() error {
	db := consts.GetGormProfileDatabase()
	if db == nil {
		return utils.Errorf("no database found")
	}

	return yakit.UpdateYakitStore(db, "")
}

func (c *YakitClient) YakitSetProgressEx(id string, f float64) {
	c.send(&YakitProgress{
		Id:       id,
		Progress: f,
	})
}

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
