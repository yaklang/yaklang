package yaklib

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/fp"
	"github.com/yaklang/yaklang/common/go-funk"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/synscan"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
	"github.com/yaklang/yaklang/common/utils/spacengine"
	"github.com/yaklang/yaklang/common/yak/yaklib/yakhttp"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"io/ioutil"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"time"
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
	"Info":          yakitInfo(emptyVirtualClient),
	"Warn":          yakitWarn(emptyVirtualClient),
	"Error":         yakitError(emptyVirtualClient),
	"Text":          yakitError(emptyVirtualClient),
	"Markdown":      yakitMarkdown(emptyVirtualClient),
	"Report":        yakitReport(emptyVirtualClient),
	"File":          yakitFile(emptyVirtualClient),
	"Output":        yakitOutput(emptyVirtualClient),
	"SetProgress":   yakitSetProgress(emptyVirtualClient),
	"SetProgressEx": yakitSetProgressEx(emptyVirtualClient),
}

func GetExtYakitLibByClient(client *YakitClient) map[string]interface{} {

	var YakitExports = map[string]interface{}{
		"Info":          yakitInfo(client),
		"Warn":          yakitWarn(client),
		"Error":         yakitError(client),
		"Text":          yakitTextBlock(client),
		"Markdown":      yakitMarkdown(client),
		"Report":        yakitReport(client),
		"File":          yakitFile(client),
		"Output":        yakitOutput(client),
		"SetProgress":   yakitSetProgress(client),
		"SetProgressEx": yakitSetProgressEx(client),
	}
	return YakitExports
}

//var yakitClientInstance YakitClient

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

func NewYakitStatusCardExecResult(status, data string, items ...string) *ypb.ExecResult {
	var card = &YakitStatusCard{
		Id:   status,
		Data: data,
		Tags: items,
	}
	raw, _ := YakitMessageGenerator(card)
	return &ypb.ExecResult{
		IsMessage: true,
		Message:   raw,
	}
}

func NewYakitLogExecResult(level string, data string, items ...interface{}) *ypb.ExecResult {
	var logItem = &YakitLog{
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
	if port <= 0 {
		port = utils.GetRandomAvailableTCPPort()
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
	case *yakit.Risk:
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
	case *YakitStatusCard:
		return "feature-status-card-data", string(raw)
	case string:
		return "info", utils.EscapeInvalidUTF8Byte([]byte(ret))
	case []byte:
		return "info", utils.EscapeInvalidUTF8Byte(ret)
	default:
		return "json", string(raw)
	}
}

func NewPortFromMatchResult(f *fp.MatchResult) *yakit.Port {
	return &yakit.Port{
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

func NewPortFromSpaceEngineResult(f *spacengine.NetSpaceEngineResult) *yakit.Port {
	host, port, _ := utils.ParseStringToHostPort(f.Addr)
	return &yakit.Port{
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

func NewPortFromSynScanResult(f *synscan.SynScanResult) *yakit.Port {
	return &yakit.Port{
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

func NewLineGraph() *YakitGraph {
	return &YakitGraph{
		Type: "line",
	}
}

func NewBarGraph() *YakitGraph {
	return &YakitGraph{
		Type: "bar",
	}
}

func NewPieGraph() *YakitGraph {
	return &YakitGraph{
		Type: "pie",
	}
}

func NewWordCloud() *YakitGraph {
	return &YakitGraph{
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

func yakitInfo(c *YakitClient) func(tmp string, items ...interface{}) {
	return func(tmp string, items ...interface{}) {
		c.Info(tmp, items...)
	}
}

func YakitInfo(c *YakitClient) func(tmp string, items ...interface{}) {
	return func(tmp string, items ...interface{}) {
		c.Info(tmp, items...)
	}
}

func yakitTextBlock(c *YakitClient) func(tmp interface{}) {
	return func(tmp interface{}) {
		c.OutputLog("text", utils.InterfaceToString(tmp))
	}
}

func yakitMarkdown(c *YakitClient) func(tmp interface{}) {
	return func(tmp interface{}) {
		c.OutputLog("markdown", utils.InterfaceToString(tmp))
	}
}

func yakitReport(c *YakitClient) func(i int) {
	return func(i int) {
		c.OutputLog("report", fmt.Sprint(i))
	}
}

func yakitFile(c *YakitClient) func(fileName string, desc ...interface{}) {
	return func(fileName string, desc ...interface{}) {
		var title = fileName
		var descStr = ""
		if len(desc) > 1 {
			title = utils.InterfaceToString(desc[0])
			descStr = utils.InterfaceToString(funk.Reduce(funk.Tail(desc), func(i interface{}, s interface{}) string {
				return utils.InterfaceToString(i) + "," + utils.InterfaceToString(s)
			}, ""))
			descStr = strings.Trim(descStr, " \r\n,")
		}

		existed, _ := utils.PathExists(fileName)
		var size uint64
		isDir := utils.IsDir(fileName)
		if existed && !isDir {
			if info, _ := os.Stat(fileName); info != nil {
				size = uint64(info.Size())
			}
		}
		dir := fileName
		if !isDir {
			dir = filepath.Dir(dir)
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
		c.OutputLog("file", string(raw))
	}
}

func yakitError(c *YakitClient) func(tmp string, items ...interface{}) {
	return func(tmp string, items ...interface{}) {
		c.Error(tmp, items...)
	}
}

func yakitOutput(c *YakitClient) func(i interface{}) error {
	return func(i interface{}) error {
		return c.Output(i)
	}
}

func yakitWarn(c *YakitClient) func(tmp string, items ...interface{}) {
	return func(tmp string, items ...interface{}) {
		c.Warn(tmp, items...)
	}
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
	addr := _cliString("yakit-webhook")
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
	var db = consts.GetGormProfileDatabase()
	if db == nil {
		return utils.Errorf("no database found")
	}

	return yakit.UpdateYakitStore(db, "")
}

func yakitSetProgressEx(c *YakitClient) func(id string, f float64) {
	return func(id string, f float64) {
		c.SetProgress(id, f)
	}
}

func yakitSetProgress(c *YakitClient) func(f float64) {
	return func(f float64) {
		yakitSetProgressEx(c)("main", f)
	}
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

func generateYakitMITMHookParams(method string, url string, opts ...yakhttp.HttpOption) ([]interface{}, error) {
	var isHttps = false
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
