package yakgrpc

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/yaklang/yaklang/common/mutate"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
	"github.com/yaklang/yaklang/embed"
	"strings"
	"sync"

	"golang.org/x/exp/slices"

	"github.com/google/uuid"

	"github.com/davecgh/go-spew/spew"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
	"github.com/yaklang/yaklang/common/yak"
	"github.com/yaklang/yaklang/common/yak/static_analyzer/result"
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
		ins, err := yakit.GetYakScriptByName(s.GetProfileDatabase(), pluginName)
		if err != nil {
			return nil, err
		}
		pluginCode = ins.Content
		pluginType = ins.Type
	}
	pluginTestingServer := NewPluginTestingEchoServer(ctx)
	return s.EvaluatePlugin(ctx, pluginCode, pluginType, pluginTestingServer)
}

type PluginTestingEchoServer struct {
	Host                string
	Port                int
	RequestsHistory     []byte
	RequestHistoryMutex *sync.Mutex

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
	var echoServer = &PluginTestingEchoServer{
		RequestsHistory:     make([]byte, 0),
		RequestHistoryMutex: new(sync.Mutex),
		JunkData:            BuildPluginTestingJunkData(),
		Ctx:                 ctx,
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

		return lowhttp.ReplaceHTTPPacketBodyFast([]byte(`HTTP/1.1 200 OK
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
Content-Type: application/json
`), body)
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
	commonWord, _ := embed.Asset("data/plugin-testing-data/common_word.txt.gz") //website common word
	junkData = append(junkData, commonWord...)
	return junkData
}

// 只在评分中使用
func (s *Server) EvaluatePlugin(ctx context.Context, pluginCode, pluginType string, pluginTestingServer *PluginTestingEchoServer) (*ypb.SmokingEvaluatePluginResponse, error) {
	host, port := pluginTestingServer.Host, pluginTestingServer.Port
	defer pluginTestingServer.ClearRequestsHistory()
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

	score := 100

	const (
		Error   = string(result.Error)
		Warning = string(result.Warn)
	)

	// static analyze
	if slices.Contains([]string{
		"mitm", "port-scan", "codec", "yak",
	}, pluginType) {
		staticResults := yak.StaticAnalyzeYaklang(pluginCode,
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
			log.Error("static analyze failed")
		}
	}

	if slices.Contains([]string{
		"mitm", "port-scan", "nuclei",
	}, pluginType) { // echo debug script

		if host == "" || port <= 0 {
			return nil, utils.Error("debug echo server start failed")
		}

		log.Info("start to echo debug script")
		runtimeId := uuid.New().String()
		err := s.debugScript("http://"+utils.HostPort(host, port), pluginType, pluginCode, NewFakeStream(ctx, func(result *ypb.ExecResult) error {
			if result.IsMessage {
				m := make(map[string]any)
				err := json.Unmarshal(result.Message, &m)
				if err != nil {
					return err
				}
				spew.Dump(m)
				spew.Dump(m["request"])
				spew.Dump(m["response"])
				log.Info("debugScript recv: ", string(result.Message))
			}
			return nil
		}), []*ypb.KVPair{{Key: "State", Value: "Smoking"}}, runtimeId)
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
