package yakgrpc

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/google/uuid"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/cli"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
	"github.com/yaklang/yaklang/common/yak"
	"github.com/yaklang/yaklang/common/yak/antlr4yak"
	"github.com/yaklang/yaklang/common/yak/yaklib"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"net/url"
	"reflect"
	"strings"
	"time"
)

type sender interface {
	Send(*ypb.ExecResult) error
	Context() context.Context
}

func (s *Server) execScriptWithExecParam(scriptName string, input string, stream sender, params []*ypb.KVPair) error {
	scriptInstance, err := yakit.GetYakScriptByName(s.GetProfileDatabase(), scriptName)
	if err != nil {
		return err
	}
	var (
		debugType = scriptInstance.Type
		isTemp    = scriptInstance.Ignored && strings.HasPrefix(scriptInstance.ScriptName, "tmp-")
	)
	runtimeId := uuid.New().String()
	stream.Send(&ypb.ExecResult{IsMessage: false, RuntimeID: runtimeId}) // 触发前端切换结果页面
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
	sendLog := func(res interface{}) {
		raw, _ := yaklib.YakitMessageGenerator(res)
		execResult := &ypb.ExecResult{
			IsMessage: true,
			Message:   raw,
		}
		execResult.RuntimeID = runtimeId
		stream.Send(execResult)
	}
	switch strings.ToLower(debugType) {
	case "codec":
		tabName := "Codec结果"
		subEngine, err := engine.ExecuteExWithContext(stream.Context(), scriptInstance.Content, map[string]any{
			"CTX":         stream.Context(),
			"PLUGIN_NAME": scriptName,
		})
		if err != nil {
			return utils.Errorf("execute file %s code failed: %s", scriptName, err.Error())
		}
		result, err := subEngine.CallYakFunction(context.Background(), "handle", []interface{}{input})
		if err != nil {
			return utils.Errorf("import %v' s handle failed: %s", scriptName, err)
		}

		feedbackClient.SetYakLog(yaklib.CreateYakLogger()) // 重置log避免获取不到行号的问题
		err = feedbackClient.Output(&yaklib.YakitFeature{
			Feature: "text",
			Params: map[string]interface{}{
				"tab_name": tabName,
				"at_head":  true,
			},
		})

		if err != nil {
			return err
		}

		resData, err := json.Marshal(&yaklib.YakitTextTabData{
			TableName: tabName,
			Data:      utils.InterfaceToString(result),
		})

		sendLog(&yaklib.YakitLog{
			Level:     "feature-text-data",
			Data:      string(resData),
			Timestamp: time.Now().Unix(),
		})
		return nil
	case "yak":
		tempArgs := makeArgs(params)
		engine.RegisterEngineHooks(func(engine *antlr4yak.Engine) error {
			hook := func(f interface{}) interface{} {
				funcValue := reflect.ValueOf(f)
				funcType := funcValue.Type()
				hookFunc := reflect.MakeFunc(funcType, func(args []reflect.Value) (results []reflect.Value) {
					TempParams := []cli.SetCliExtraParam{cli.SetTempArgs(tempArgs)}
					index := len(args) - 1 //获取 option 参数的 index
					interfaceValue := args[index].Interface()
					args = args[:index]
					cliExtraParams, ok := interfaceValue.([]cli.SetCliExtraParam)
					if ok {
						TempParams = append(TempParams, cliExtraParams...)
					}
					for _, p := range TempParams {
						args = append(args, reflect.ValueOf(p))
					}
					res := funcValue.Call(args)
					return res
				})
				return hookFunc.Interface()
			}

			hookFuncList := []string{
				"String",
				"Bool",
				"Have",
				"Int",
				"Integer",
				"Float",
				"Double",
				"YakitPlugin",
				"Urls",
				"Url",
				"Ports",
				"Port",
				"Hosts",
				"Host",
				"Network",
				"Net",
				"File",
				"FileOrContent",
				"LineDict",
			}
			for _, name := range hookFuncList {
				engine.GetVM().RegisterMapMemberCallHandler("cli", name, hook)
			}
			return nil
		})
		_, err := engine.ExecuteExWithContext(stream.Context(), scriptInstance.Content, map[string]any{
			"CTX":         stream.Context(),
			"PLUGIN_NAME": scriptName,
		})
		if err != nil {
			log.Warnf("execute debug script failed: %v", err)
			return err
		}
		return nil
	default:
		return utils.Error("unsupported plugin type: " + debugType)
	}
}

func (s *Server) execScriptWithRequest(scriptName string, targetInput string, stream sender, params ...*ypb.HTTPRequestBuilderParams) error {
	if scriptName == "" {
		return utils.Error("code N scriptName is empty")
	}

	runtimeId := uuid.New().String()
	stream.Send(&ypb.ExecResult{IsMessage: false, RuntimeID: runtimeId}) // 触发前端切换结果页面
	var baseBuilderParams *ypb.HTTPRequestBuilderParams
	if len(params) > 0 {
		baseBuilderParams = params[0]
	}

	if baseBuilderParams == nil {
		baseBuilderParams = &ypb.HTTPRequestBuilderParams{
			Method: "GET",
			Path:   []string{"/"},
		}
	}

	if targetInput == "" && !baseBuilderParams.IsRawHTTPRequest {
		return utils.Error("target is empty")
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

	var targets []*url.URL // build request targets
	for _, target := range utils.PrettifyListFromStringSplitEx(targetInput, "\n", "|", ",") {
		target = strings.TrimSpace(target)
		if target == "" {
			continue
		}
		if utils.IsValidHost(target) { // 处理没有单独一个host情况 不含port
			targets = append(targets, &url.URL{Host: target, Path: "/"})
		}
		urlIns := utils.ParseStringToUrl(target)
		if urlIns.Host == "" {
			continue
		}

		host, port, _ := utils.ParseStringToHostPort(urlIns.Host) // 处理包含 port 的情况
		if !utils.IsValidHost(host) {                             // host不合规情况 比如 a:80
			continue
		}

		if port > 0 && urlIns.Scheme == "" { // fix https
			if port == 443 {
				urlIns.Scheme = "https"
			}
		}
		if urlIns.Path == "" {
			urlIns.Path = "/"
		}

		targets = append(targets, urlIns)

	}

	if len(targets) != 0 { // 调试目标分支

		//var results = builderResponse.GetResults()
		var baseTemplates = []byte("GET {{Path}} HTTP/1.1\r\nHost: {{Hostname}}\r\n\r\n")

		for _, target := range targets {
			builderParams := mergeBuildParams(baseBuilderParams, target)
			builderResponse, err := s.HTTPRequestBuilder(stream.Context(), builderParams)
			if err != nil {
				log.Errorf("failed to build http request: %v", err)
			}
			var results = builderResponse.GetResults()
			if len(results) <= 0 {
				packet := bytes.ReplaceAll(baseTemplates, []byte(`{{Hostname}}`), []byte(target.Host))
				packet = bytes.ReplaceAll(packet, []byte(`{{Path}}`), []byte(target.Path))
				feed(lowhttp.AppendAllHTTPPacketQueryParam(packet, target.Query()), target.Scheme == "https")
			} else {
				for _, result := range results {
					packet := bytes.ReplaceAll(result.HTTPRequest, []byte(`{{Hostname}}`), []byte(target.Host))
					feed(packet, result.IsHttps)
				}
			}
		}

	} else if baseBuilderParams.GetIsRawHTTPRequest() { // 原始请求分支
		feed(baseBuilderParams.RawHTTPRequest, baseBuilderParams.IsHttps)
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
	input string,
	debugType string,
	debugCode string,
	stream sender,
	execParams []*ypb.KVPair,
	params ...*ypb.HTTPRequestBuilderParams) error {
	tempName, err := yakit.CreateTemporaryYakScript(debugType, debugCode)
	if err != nil {
		return err
	}

	switch debugType {
	case "yak", "codec":
		return s.execScriptWithExecParam(tempName, input, stream, execParams)
	case "mitm", "nuclei", "port-scan":
		return s.execScriptWithRequest(tempName, input, stream, params...)
	}
	return utils.Error("unsupported plugin type: " + debugType)
}

func makeArgs(execParams []*ypb.KVPair) []string {
	var args = []string{"yak"}
	canFilter := true
	for _, p := range execParams {
		switch p.Key {
		case "__yakit_plugin_names__": // 直接查询插件名
			tempName, err := utils.SaveTempFile(p.Value, "yakit-plugin-selector-*.txt")
			if err != nil {
				log.Errorf("save temp file failed: %v", err)
				return nil
			}
			args = append(args, "--yakit-plugin-file", tempName)
			canFilter = false
		case "__yakit_plugin_filter__": // 筛选情况
			if !canFilter {
				continue
			}
			var pluginFilter *ypb.QueryYakScriptRequest
			var pluginName []string
			err := json.Unmarshal([]byte(p.Value), pluginFilter)
			if err != nil {
				log.Errorf("unmarshal plugin filter failed: %v", err)
				continue
			}
			yakit.FilterYakScript(consts.GetGormProfileDatabase(), pluginFilter).Pluck("script_name", pluginName)
			tempName, err := utils.SaveTempFile(strings.Join(pluginName, "|"), "yakit-plugin-selector-*.txt")
			if err != nil {
				log.Errorf("save temp file failed: %v", err)
				continue
			}
			args = append(args, "--yakit-plugin-file", tempName)
		default:
			args = append(args, "--"+p.Key, p.Value)
		}

	}
	return args
}

func mergeBuildParams(params *ypb.HTTPRequestBuilderParams, t *url.URL) *ypb.HTTPRequestBuilderParams { // 根据单个目标和总体配置生成针对单个目标的build参数
	pathFlag := true
	for _, p := range params.Path {
		if normalizeString(p) == normalizeString(t.Path) {
			pathFlag = false
			break
		}
	}
	if pathFlag {
		params.Path = append(params.Path, t.Path)
	}

	for key, values := range t.Query() { // 插入所有的 get 参数
		for _, value := range values {
			params.GetParams = append(params.GetParams, &ypb.KVPair{
				Key: key, Value: value,
			})
		}
	}

	if t.Scheme != "" { // 目标标识优先级更高
		params.IsHttps = t.Scheme == "https"
	}

	return params
}

func normalizeString(s string) string {
	if s == "" || s == "/" {
		return ""
	}
	return s
}
