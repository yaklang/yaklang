package yakgrpc

import (
	"bytes"
	"context"
	"encoding/json"
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
	host, port := setupEachServe(ctx)
	return s.EvaluatePlugin(ctx, pluginCode, pluginType, host, port)
}

func setupEachServe(ctx context.Context) (string, int) {
	var host string
	var port int
	var wg sync.WaitGroup
	// start each server
	wg.Add(1)
	go func() {
		defer func() {
			defer wg.Done()
			if err := recover(); err != nil {
				log.Errorf("lowhttp.DebugEchoServer panic: %v", err)
				utils.PrintCurrentGoroutineRuntimeStack()
			}
		}()
		host, port = lowhttp.DebugEchoServerContext(ctx)
	}()
	wg.Wait()
	return host, port
}

// 只在评分中使用
func (s *Server) EvaluatePlugin(ctx context.Context, pluginCode, pluginType string, host string, port int) (*ypb.SmokingEvaluatePluginResponse, error) {
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
		var fetchRisk bool
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
				switch utils.MapGetString(utils.MapGetMapRaw(m, "content"), "level") {
				case "json-risk":
					fetchRisk = true
				}
			}
			return nil
		}), []*ypb.KVPair{{Key: "State", Value: "Smoking"}}, runtimeId)
		if err != nil {
			score -= 60
			log.Errorf("debugScript failed: %v", err)
			pushSuggestion("冒烟测试失败[Smoking Test]", `请检查插件异常处理是否完备？查看 Console 以处理调试错误: `+err.Error(), nil, Error)
		}
		if fetchRisk {
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
