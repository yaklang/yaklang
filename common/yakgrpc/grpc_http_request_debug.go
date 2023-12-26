package yakgrpc

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/google/uuid"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/go-funk"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/cli"
	"github.com/yaklang/yaklang/common/yak"
	"github.com/yaklang/yaklang/common/yak/antlr4yak"
	"github.com/yaklang/yaklang/common/yak/yaklib"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
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

type urlTarget struct {
	host             string
	path             string
	isHttps          bool
	needSendOriginal bool
}

func (s *Server) execScriptWithRequest(scriptName string, targetInput string, stream sender, params ...*ypb.HTTPRequestBuilderParams) error {
	if targetInput == "" {
		return utils.Error("input is empty")
	}

	if scriptName == "" {
		return utils.Error("code N scriptName is empty")
	}

	runtimeId := uuid.New().String()
	stream.Send(&ypb.ExecResult{IsMessage: false, RuntimeID: runtimeId}) // 触发前端切换结果页面
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

	var targets []*urlTarget // build request targets
	for _, target := range utils.PrettifyListFromStringSplitEx(targetInput, "\n", "|", ",") {
		target = strings.TrimSpace(target)
		if target == "" {
			continue
		}
		if utils.IsValidHost(target) { // 处理没有单独一个host情况 不含port
			targets = append(targets, &urlTarget{
				host:             target,
				needSendOriginal: false,
			})
		}
		urlIns := utils.ParseStringToUrl(target)
		if urlIns.Host == "" {
			continue
		}

		ishttps := urlIns.Scheme == "https"
		host, port, _ := utils.ParseStringToHostPort(urlIns.Host) // 处理包含 port 的情况

		//todo 是否需要支持hosts?
		if !utils.IsValidHost(host) { // host不合规情况 比如 a:80
			continue
		}

		if port > 0 && urlIns.Scheme == "" { // fix https
			if port == 443 {
				ishttps = true
			}
		}

		if urlIns.RawQuery != "" {
			urlIns.Path += "?" + urlIns.RawQuery
		}

		targets = append(targets, &urlTarget{
			host:    urlIns.Host,
			isHttps: ishttps,
			path:    urlIns.Path,
			//当请求和模板的path或者https配置不同时需要补充发包
			needSendOriginal: ishttps != builderParams.IsHttps || (urlIns.Path != "" && !utils.StringArrayContains(builderParams.Path, urlIns.Path)),
		})

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
	var baseTemplates = []byte("GET {{Path}} HTTP/1.1\r\nHost: {{Hostname}}\r\n\r\n")
	if len(results) <= 0 { // 请求模板构造失败时直接用http get请求目标
		for _, target := range targets {
			packet := bytes.ReplaceAll(baseTemplates, []byte(`{{Hostname}}`), []byte(target.host))
			packet = bytes.ReplaceAll(packet, []byte(`{{Path}}`), []byte(target.path))
			feed(packet, target.isHttps)
		}
	} else {
		funk.ForEach(results, func(i *ypb.HTTPRequestBuilderResult) {
			for _, target := range targets {
				packet := bytes.ReplaceAll(i.HTTPRequest, []byte(`{{Hostname}}`), []byte(target.host))
				feed(packet, i.IsHttps)
			}
		})
	}

	for _, target := range targets { // 发送补充请求
		if !target.needSendOriginal {
			continue
		}
		packet := bytes.ReplaceAll(baseTemplates, []byte(`{{Hostname}}`), []byte(target.host))
		packet = bytes.ReplaceAll(packet, []byte(`{{Path}}`), []byte(target.path))
		feed(packet, target.isHttps)
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
