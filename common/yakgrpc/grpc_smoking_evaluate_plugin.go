package yakgrpc

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"strings"
	"sync"

	"github.com/yaklang/yaklang/common/yak/static_analyzer/information"

	"github.com/yaklang/yaklang/common/fp"
	"github.com/yaklang/yaklang/common/mutate"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/syntaxflow/sfanalyzer"
	"github.com/yaklang/yaklang/common/syntaxflow/sfdb"
	"github.com/yaklang/yaklang/common/yak/static_analyzer"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
	"github.com/yaklang/yaklang/embed"
	"golang.org/x/exp/slices"

	"github.com/google/uuid"

	"github.com/davecgh/go-spew/spew"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/netx"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
	"github.com/yaklang/yaklang/common/yak"
	"github.com/yaklang/yaklang/common/yak/static_analyzer/result"
	_ "github.com/yaklang/yaklang/common/yak/static_analyzer/score_rules"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

type fakeStreamInstance struct {
	ctx     context.Context
	handler func(*ypb.ExecResult) error
}

func (f *fakeStreamInstance) Send(result *ypb.ExecResult) error {
	if f == nil {
		log.Error("fakeStreamInstance empty")
		return nil
	}
	if f.handler != nil {
		return f.handler(result)
	}
	log.Infof("*fakeStreamInstance.Send Called with: %v", spew.Sdump(result))
	return nil
}

func (f *fakeStreamInstance) Context() context.Context {
	return f.ctx
}

func NewFakeStream(ctx context.Context, handler func(result *ypb.ExecResult) error) *fakeStreamInstance {
	return &fakeStreamInstance{
		ctx:     ctx,
		handler: handler,
	}
}

func (s *Server) SmokingEvaluatePlugin(ctx context.Context, req *ypb.SmokingEvaluatePluginRequest) (*ypb.SmokingEvaluatePluginResponse, error) {
	pluginName := req.GetPluginName()
	var cancel func()
	ctx, cancel = context.WithCancel(ctx)
	defer cancel()

	var (
		pluginType = req.GetPluginType()
		pluginCode = req.GetCode()
	)
	if pluginCode == "" {
		// full plugin code by name
		switch pluginType {
		case schema.SCRIPT_TYPE_SYNTAXFLOW:
			rule, err := sfdb.QueryRuleByName(s.GetProfileDatabase(), pluginName)
			if err != nil {
				return nil, err
			}
			pluginCode = rule.Content
			pluginType = "syntaxflow"
		case schema.SCRIPT_TYPE_YAK:
			ins, err := yakit.GetYakScriptByName(s.GetProfileDatabase(), pluginName)
			if err != nil {
				return nil, err
			}
			pluginCode = ins.Content
			pluginType = ins.Type
		default:
			ins, err := yakit.GetYakScriptByName(s.GetProfileDatabase(), pluginName)
			if err != nil {
				return nil, err
			}
			pluginCode = ins.Content
			pluginType = ins.Type
		}
	}
	pluginTestingServer := NewPluginTestingEchoServer(ctx)
	return s.EvaluatePlugin(ctx, pluginCode, pluginType, pluginTestingServer)
}

type PluginTestingEchoServer struct {
	Host                string
	Port                int
	RequestsHistory     []byte
	RequestHistoryMutex *sync.Mutex
	RawHeader           []byte

	JunkData []byte
	Ctx      context.Context
}

func NewPluginTestingEchoServer(ctx context.Context) *PluginTestingEchoServer {
	defer func() {
		if err := recover(); err != nil {
			log.Errorf("lowhttp.DebugEchoServer panic: %v", err)
			utils.PrintCurrentGoroutineRuntimeStack()
		}
	}()
	echoServer := &PluginTestingEchoServer{
		RequestsHistory:     make([]byte, 0),
		RequestHistoryMutex: new(sync.Mutex),
		JunkData:            BuildPluginTestingJunkData(),
		Ctx:                 ctx,
		RawHeader: []byte(`HTTP/1.1 200 OK
Content-Type: text/html
Content-Type: text/plain
Content-Type: text/xml
Content-Type: image/gif
Content-Type: image/jpeg 
Content-Type: image/png
Content-Type: application/xhtml+xml
Content-Type: application/xml
Content-Type: application/atom+xml
Content-Type: application/pdf
Content-Type: application/msword
Content-Type: application/octet-stream
Content-Type: application/json`),
	}

	decodeReq := func(req []byte) []byte {
		method, uri, version := lowhttp.GetHTTPPacketFirstLine(req)
		decodeReq := lowhttp.ReplaceHTTPPacketFirstLine(req, strings.Join([]string{method, codec.ForceQueryUnescape(uri), version}, " "))
		decodeReq = lowhttp.ReplaceHTTPPacketBodyFast(decodeReq, []byte(codec.ForceQueryUnescape(string(lowhttp.GetHTTPPacketBody(req)))))
		if encoding := lowhttp.IGetHeader(req, "content-encoding"); len(encoding) > 0 {
			decodeReq, _ = lowhttp.ContentEncodingDecode(encoding[0], decodeReq)
		}
		return decodeReq
	}

	echoServer.Host, echoServer.Port = utils.DebugMockHTTPExContext(ctx, func(req []byte) []byte {
		echoServer.RequestHistoryMutex.Lock()
		defer echoServer.RequestHistoryMutex.Unlock()
		echoServer.RequestsHistory = append(echoServer.RequestsHistory, req...)
		echoServer.RequestsHistory = append(echoServer.RequestsHistory, decodeReq(req)...)
		body := append(echoServer.JunkData, echoServer.RequestsHistory...)

		return lowhttp.ReplaceHTTPPacketBodyFast(echoServer.RawHeader, body)
	})

	return echoServer
}

func (s *PluginTestingEchoServer) ClearRequestsHistory() {
	s.RequestHistoryMutex.Lock()
	defer s.RequestHistoryMutex.Unlock()
	s.RequestsHistory = make([]byte, 0)
}

func BuildPluginTestingJunkData() []byte {
	var junkData []byte
	junkData = append(junkData, []byte(strings.Join(mutate.MutateQuick("{{int(10000-99999)}}"), ","))...) // number
	junkData = append(junkData, []byte(strings.Join(mutate.MutateQuick("{{rangechar(20,7e)}}"), ""))...)  // visible characters
	passwd, _ := embed.Asset("data/plugin-testing-data/top_100_passwd.txt.gz")                            //  top passwd and hash
	junkData = append(junkData, passwd...)
	commonWord, _ := embed.Asset("data/plugin-testing-data/common_word.txt.gz") // website common word
	junkData = append(junkData, commonWord...)
	commonWebSite, _ := embed.Asset("data/plugin-testing-data/common_website.txt.gz") // common website page baidu/bilibili/taobao
	junkData = append(junkData, commonWebSite...)
	return junkData
}

// 只在评分中使用
func (s *Server) EvaluatePlugin(ctx context.Context, pluginCode, pluginType string, pluginTestingServer *PluginTestingEchoServer) (*ypb.SmokingEvaluatePluginResponse, error) {
	if pluginType == schema.SCRIPT_TYPE_SYNTAXFLOW {
		sfAnalyzer := sfanalyzer.NewSyntaxFlowAnalyzer(pluginCode)
		sfAnalyzeRes := sfAnalyzer.Analyze()
		return sfAnalyzeRes.GetResponse(), nil
	}

	defer pluginTestingServer.ClearRequestsHistory()
	host, port := pluginTestingServer.Host, pluginTestingServer.Port
	testDomain := utils.RandStringBytes(60) + ".com"
	netx.AddHost(testDomain, host)
	defer netx.DeleteHost(testDomain)
	target := fmt.Sprintf("http://%s:%d", testDomain, port)
	var results []*ypb.SmokingEvaluateResult
	pushSuggestion := func(item string, suggestion string, R *ypb.Range, severity string, i ...[]byte) {
		var buf bytes.Buffer
		for _, d := range i {
			buf.Write(d)
		}
		results = append(results, &ypb.SmokingEvaluateResult{
			Item:       item,
			Suggestion: suggestion,
			ExtraInfo:  buf.Bytes(),
			Range:      R,
			Severity:   severity,
		})
	}
	fp.SetMatchResultCache(utils.HostPort(testDomain, port), MockPluginTestingFpResult(testDomain, pluginTestingServer))

	score := 100

	const (
		Error   = string(result.Error)
		Warning = string(result.Warn)
	)

	var hasParameter bool
	var hasHttpRequest bool
	if pluginType != schema.SCRIPT_TYPE_NUCLEI {
		prog, err := static_analyzer.SSAParse(pluginCode, pluginType)
		if err != nil {
			pushSuggestion(`静态代码检测失败`, "ssa 编译失败", nil, Error)
		} else {
			parameters, _, _ := information.ParseCliParameter(prog)
			if len(parameters) > 0 {
				hasParameter = true
			}
			hasHttpRequest = information.GetHTTPRequestCount(prog) > 0
		}
	}
	// static analyze
	if slices.Contains([]string{
		schema.SCRIPT_TYPE_MITM, schema.SCRIPT_TYPE_PORT_SCAN, schema.SCRIPT_TYPE_CODEC, schema.SCRIPT_TYPE_YAK,
	}, pluginType) {
		staticResults := yak.StaticAnalyze(pluginCode,
			yak.WithStaticAnalyzePluginType(pluginType),
			yak.WithStaticAnalyzeKindScore(),
		)
		if len(staticResults) > 0 {
			score = result.CalculateScoreFromResults(staticResults)
			for _, sRes := range staticResults {
				R := &ypb.Range{
					StartLine:   int64(sRes.StartLineNumber),
					StartColumn: int64(sRes.StartColumn),
					EndLine:     int64(sRes.EndLineNumber),
					EndColumn:   int64(sRes.EndColumn),
				}
				switch sRes.Severity {
				case result.Error:
					pushSuggestion(`静态代码检测失败`, sRes.Message, R, Error, []byte(sRes.From))
				case result.Warn:
					pushSuggestion(`静态代码检测警告`, sRes.Message, R, Warning, []byte(sRes.From))
				}
			}
			if score < 60 {
				return &ypb.SmokingEvaluatePluginResponse{
					Score:   0,
					Results: results,
				}, nil
			}
		}
	}

	if !hasParameter && slices.Contains([]string{
		schema.SCRIPT_TYPE_MITM, schema.SCRIPT_TYPE_PORT_SCAN, schema.SCRIPT_TYPE_NUCLEI,
	}, pluginType) { // echo debug script
		getMockParam := func() []*ypb.KVPair {
			var params []*ypb.KVPair
			for i := 0; i < rand.Intn(4)+1; i++ {
				params = append(params, &ypb.KVPair{
					Key:   utils.RandStringBytes(5),
					Value: utils.RandStringBytes(5),
				})
			}
			return params
		}
		if host == "" || port <= 0 {
			return nil, utils.Error("debug echo server start failed")
		}

		log.Info("start to echo debug script")
		runtimeId := uuid.New().String()
		err := s.debugScript(target, pluginType, pluginCode, NewFakeStream(ctx, func(result *ypb.ExecResult) error {
			if result.IsMessage {
				m := make(map[string]any)
				err := json.Unmarshal(result.Message, &m)
				if err != nil {
					return err
				}
				// spew.Dump(m)
				// spew.Dump(m["request"])
				// spew.Dump(m["response"])
				log.Info("debugScript recv: ", string(result.Message))
			}
			return nil
		}), []*ypb.KVPair{{Key: "Mode", Value: "Strict"}}, runtimeId, &ypb.HTTPRequestBuilderParams{
			GetParams: getMockParam(), PostParams: getMockParam(), Cookie: getMockParam(),
		})
		if err != nil {
			score -= 60
			log.Errorf("debugScript failed: %v", err)
			pushSuggestion("冒烟测试失败[Smoking Test]", `请检查插件异常处理是否完备？查看 Console 以处理调试错误: `+err.Error(), nil, Error)
		}
		riskCount, err := yakit.CountRiskByRuntimeId(s.GetProjectDatabase(), runtimeId)
		if err != nil {
			score -= 60
			log.Errorf("debugScript failed: %v", err)
		}
		if riskCount > 0 {
			score -= 50
			pushSuggestion("误报[Negative Alarm]", `本插件的漏洞判定可能过于宽松，请检查漏洞判定逻辑`, nil, Error)
			err := yakit.DeleteRisk(s.GetProjectDatabase(), &ypb.QueryRisksRequest{RuntimeId: runtimeId})
			if err != nil {
				log.Errorf("delete plugin testing risk error: %v", err)
			}
		} else if hasHttpRequest { //  if not negative alarm, check plugin sent request
			wantCount := 1
			if pluginType == "mitm" {
				wantCount = 2 // mitm plugin need rsp, so there will have a default request
			}
			count := yakit.CountHTTPFlowByRuntimeID(s.GetProjectDatabase(), runtimeId)
			if count < wantCount {
				score -= 50
				pushSuggestion("逻辑测试失败[logic Test]", `请检查插件是否正常发起请求`, nil, Error)
			}
		}
	}

	if score < 0 {
		score = 0
	}

	return &ypb.SmokingEvaluatePluginResponse{
		Score:   int64(score),
		Results: results,
	}, nil
}

func MockPluginTestingFpResult(testDomain string, pluginTestingServer *PluginTestingEchoServer) *fp.MatchResult {
	port := pluginTestingServer.Port
	return &fp.MatchResult{
		Target: testDomain,
		Port:   port,
		State:  fp.OPEN,
		Reason: "",
		Fingerprint: &fp.FingerprintInfo{
			IP:          testDomain,
			Port:        port,
			Proto:       "tcp",
			ServiceName: "http",
			Banner:      "",
			CPEFromUrls: make(map[string][]*schema.CPE),
			HttpFlows: []*fp.HTTPFlow{
				{
					StatusCode:     200,
					IsHTTPS:        false,
					RequestHeader:  []byte("GET / HTTP/1.1\r\nHost: " + testDomain + "\r\n\r\n"),
					RequestBody:    nil,
					ResponseHeader: pluginTestingServer.RawHeader,
					ResponseBody:   pluginTestingServer.JunkData,
				},
			},
		},
	}
}
