package yakgrpc

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/davecgh/go-spew/spew"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
	"github.com/yaklang/yaklang/common/yak"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"sync"
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
	var (
		pluginType = req.GetPluginType()
		pluginCode = req.GetCode()
		err        error
	)
	if pluginCode == "" {
		ins, err := yakit.GetYakScriptByName(s.GetProfileDatabase(), pluginName)
		if err != nil {
			return nil, err
		}
		pluginCode = ins.Content
		pluginType = ins.Type
	}

	var host string
	var port int
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer func() {
			defer wg.Done()
			if err := recover(); err != nil {
				log.Errorf("lowhttp.DebugEchoServer panic: %v", err)
				utils.PrintCurrentGoroutineRuntimeStack()
			}
		}()
		host, port = lowhttp.DebugEchoServer()
	}()
	wg.Wait()
	if host == "" || port <= 0 {
		return nil, utils.Error("debug echo server start failed")
	}

	var results []*ypb.SmokingEvaluateResult
	var pushSuggestion = func(item string, suggestion string, i ...[]byte) {
		var buf bytes.Buffer
		for _, d := range i {
			buf.Write(d)
		}
		results = append(results, &ypb.SmokingEvaluateResult{
			Item:       item,
			Suggestion: suggestion,
			ExtraInfo:  buf.Bytes(),
		})
	}

	staticCheckingFailed := false
	staticResults := yak.AnalyzeStaticYaklang(pluginCode)
	if len(staticResults) > 0 {
		for _, sRes := range staticResults {
			staticCheckingFailed = true
			pushSuggestion(`静态代码检测失败[`+sRes.Severity+`]`, sRes.Message)
		}
		log.Error("static analyze failed")
	}

	if staticCheckingFailed {
		return &ypb.SmokingEvaluatePluginResponse{
			Score:   0,
			Results: results,
		}, nil
	}

	var score int
	log.Info("start to echo debug script")
	var fetchRisk bool
	err = s.debugScript("http://"+utils.HostPort(host, port), pluginType, pluginCode, NewFakeStream(ctx, func(result *ypb.ExecResult) error {
		if result.IsMessage {
			var m = make(map[string]any)
			err := json.Unmarshal(result.Message, &m)
			if err != nil {
				return err
			}
			log.Info("debugScript recv: ", string(result.Message))
			switch utils.MapGetString(utils.MapGetMapRaw(m, "content"), "level") {
			case "json-risk":
				fetchRisk = true
			}
		}
		return nil
	}))
	if err != nil {
		log.Errorf("debugScript failed: %v", err)
		pushSuggestion("冒烟测试失败[Smoking Test]", `请检查插件异常处理是否完备？查看 Console 以处理调试错误: `+err.Error())
	} else {
		score += 40
	}

	if !fetchRisk {
		score += 20
	} else {
		pushSuggestion("误报[Negative Alarm]", `本插件的漏洞判定可能过于宽松，请检查漏洞判定逻辑`)
	}

	return &ypb.SmokingEvaluatePluginResponse{
		Score:   int64(score),
		Results: results,
	}, nil
}
