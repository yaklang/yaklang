package yakgrpc

import (
	"bytes"
	"context"
	"github.com/google/uuid"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/go-funk"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
	"github.com/yaklang/yaklang/common/yak"
	"github.com/yaklang/yaklang/common/yak/antlr4yak"
	"github.com/yaklang/yaklang/common/yak/yaklib"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	netURL "net/url"
	"strings"
)

type sender interface {
	Send(*ypb.ExecResult) error
	Context() context.Context
}

func (s *Server) execScript(scriptName string, targetInput string, stream sender, params ...*ypb.HTTPRequestBuilderParams) error {
	if targetInput == "" {
		return utils.Error("input is empty")
	}

	if scriptName == "" {
		return utils.Error("code N scriptName is empty")
	}

	runtimeId := uuid.New().String()
	var builderParams *ypb.HTTPRequestBuilderParams
	if len(params) > 0 {
		builderParams = params[0]
	}

	if builderParams == nil {
		builderParams = &ypb.HTTPRequestBuilderParams{
			Method: "GET",
			Path:   []string{"/"},
		}
	}
	uIns, err := netURL.Parse(targetInput)
	var builderParamsForTarget *ypb.HTTPRequestBuilderParams
	if err == nil {
		if !utils.StringArrayContains(builderParams.Path, uIns.Path) {
			builderParams.Path = append(builderParams.Path, uIns.Path)
		}
		queryMap := uIns.Query()
		if len(queryMap) != 0 {
			builderParamsForTargetIns := *builderParams
			builderParamsForTarget = &builderParamsForTargetIns
			builderParamsForTarget.GetParams = nil
			for k, vlist := range queryMap {
				for _, v := range vlist {
					builderParamsForTarget.GetParams = append(builderParamsForTarget.GetParams, &ypb.KVPair{
						Key:   k,
						Value: v,
					})
				}
			}
		}
	}
	scriptInstance, err := yakit.GetYakScriptByName(s.GetProfileDatabase(), scriptName)
	if err != nil {
		return err
	}
	var (
		debugType = scriptInstance.Type
		isTemp    = scriptInstance.Ignored && strings.HasPrefix(scriptInstance.ScriptName, "tmp-")
	)
	isUrlParam := false
	switch strings.ToLower(debugType) {
	case "mitm", "port-scan":
	case "nuclei":
		isUrlParam = true
		break
	default:
		return utils.Error("unsupported plugin type: " + debugType)
	}

	var reqs []any
	feed := func(req []byte, isHttps bool) {
		reqs = append(reqs, map[string]any{
			"RawHTTPRequest": req,
			"IsHttps":        isHttps,
		})
	}
	builderResponse, err := s.HTTPRequestBuilder(stream.Context(), builderParams)
	if err != nil {
		log.Errorf("failed to build http request: %v", err)
	}
	var results = builderResponse.GetResults()
	if builderParamsForTarget != nil {
		builderResponseForTarget, err := s.HTTPRequestBuilder(stream.Context(), builderParamsForTarget)
		if err != nil {
			log.Errorf("failed to build http request: %v", err)
		}
		results = append(results, builderResponseForTarget.GetResults()...)
	}
	if len(results) <= 0 { // 请求模板构造失败时直接用get请求目标
		var templates = []byte("GET / HTTP/1.1\r\nHost: {{Hostname}}\r\n\r\n")
		for _, res := range utils.PrettifyListFromStringSplitEx(targetInput, "\n", "|", ",") {
			res = strings.TrimSpace(res)
			if strings.HasPrefix(res, "http://") || strings.HasPrefix(res, "https://") {
				isHttps, raw, err := lowhttp.ParseUrlToHttpRequestRaw("GET", res)
				if err == nil {
					feed(raw, isHttps)
				}
				continue
			}

			host, port, _ := utils.ParseStringToHostPort(res)
			if host == "" {
				host = res
			}
			if host == "" {
				continue
			}
			if port > 0 {
				if port == 443 {
					feed(bytes.ReplaceAll(templates, []byte(`{{Hostname}}`), []byte(host)), true)
					continue
				}

				if port == 80 {
					feed(bytes.ReplaceAll(templates, []byte(`{{Hostname}}`), []byte(host)), false)
					continue
				}

				feed(bytes.ReplaceAll(templates, []byte(`{{Hostname}}`), []byte(utils.HostPort(host, port))), strings.HasPrefix(res, "https://"))
			} else {
				feed(bytes.ReplaceAll(templates, []byte(`{{Hostname}}`), []byte(host)), strings.HasPrefix(res, "https://"))
			}
		}
	} else {
		funk.ForEach(results, func(i *ypb.HTTPRequestBuilderResult) {
			for _, res := range utils.PrettifyListFromStringSplitEx(targetInput, "\n", "|", ",") {
				res = strings.TrimSpace(res)
				host, port, _ := utils.ParseStringToHostPort(res)
				if host == "" {
					host = res
				}
				if host == "" {
					continue
				}

				var targetAddr string
				https := i.GetIsHttps()
				if strings.HasPrefix(res, "http://") || strings.HasPrefix(res, "https://") { // 优先级高于模板packet
					https = strings.HasPrefix(res, "https://")
				}
				if port != 0 && (https && port != 443 || !https && port != 80) {
					targetAddr = utils.HostPort(host, port)
				} else {
					targetAddr = host
				}

				packet := bytes.ReplaceAll(i.HTTPRequest, []byte(`{{Hostname}}`), []byte(targetAddr))
				feed(packet, https)
			}
		})
	}
	if len(reqs) <= 0 {
		return utils.Error("build http request failed: no results")
	}

	defer func() {
		if err := recover(); err != nil {
			log.Warn(err)
			utils.PrintCurrentGoroutineRuntimeStack()
		}

		if isTemp {
			log.Infof("start to remove temp plugin: %v", scriptName)
			err = yakit.DeleteYakScriptByName(consts.GetGormProfileDatabase(), scriptName)
			if err != nil {
				log.Errorf("remove temp plugin failed: %v", err)
			}
		}
	}()

	// 不同的插件类型，需要不同的处理
	switch strings.ToLower(debugType) {
	case "mitm":
	case "nuclei":
	case "port-scan":
	default:
		return utils.Error("unsupported plugin type: " + debugType)
	}

	var feedbackClient = yaklib.NewVirtualYakitClient(func(result *ypb.ExecResult) error {
		result.RuntimeID = runtimeId
		return stream.Send(result)
	})
	engine := yak.NewYakitVirtualClientScriptEngine(feedbackClient)

	log.Infof("engine.ExecuteExWithContext(stream.Context(), debugScript ... \n")
	engine.RegisterEngineHooks(func(engine *antlr4yak.Engine) error {
		engine.SetVar("RUNTIME_ID", runtimeId)
		yak.BindYakitPluginContextToEngine(engine, &yak.YakitPluginContext{
			PluginName: scriptName,
			RuntimeId:  runtimeId,
		})
		return nil
	})
	subEngine, err := engine.ExecuteExWithContext(stream.Context(), debugScript, map[string]any{
		"REQUESTS":     reqs,
		"CTX":          stream.Context(),
		"PLUGIN_NAME":  scriptName,
		"IS_URL_PARAM": isUrlParam,
	})
	if err != nil {
		log.Warnf("execute debug script failed: %v", err)
		return err
	}
	_ = subEngine

	return nil
}

func (s *Server) debugScript(
	inputScanTarget string,
	debugType string,
	debugCode string,
	stream sender,
	params ...*ypb.HTTPRequestBuilderParams) error {
	tempName, err := yakit.CreateTemporaryYakScript(debugType, debugCode)
	if err != nil {
		return err
	}
	return s.execScript(tempName, inputScanTarget, stream, params...)
}
