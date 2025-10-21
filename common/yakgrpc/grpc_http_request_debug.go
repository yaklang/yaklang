package yakgrpc

import (
	"context"
	"encoding/json"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/schema"

	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/mutate"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/cli"
	"github.com/yaklang/yaklang/common/yak"
	"github.com/yaklang/yaklang/common/yak/antlr4yak"
	"github.com/yaklang/yaklang/common/yak/yaklib"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

type sender interface {
	Send(*ypb.ExecResult) error
	Context() context.Context
}

func (s *Server) tickerRiskCountFeedback(ctx context.Context, tickerTime time.Duration, runtimeId string, yakClient *yaklib.YakitClient) {
	tickerRiskCountFeedback(ctx, tickerTime, runtimeId, yakClient, s.GetProjectDatabase())
}

func tickerRiskCountFeedback(ctx context.Context, tickerTime time.Duration, runtimeId string, yakClient *yaklib.YakitClient, projectDB *gorm.DB) {
	ticker := time.NewTicker(tickerTime)
	go func() {
		defer ticker.Stop()
		lastCount := 0
		feedbackRiskCount := func() {
			currentCount, err := yakit.CountRiskByRuntimeId(projectDB, runtimeId)
			if err != nil {
				log.Errorf("count risk failed: %v", err)
			} else if lastCount != currentCount {
				yakClient.Output(&yaklib.YakitStatusCard{
					Id: "漏洞/风险/指纹", Data: strconv.Itoa(currentCount), Tags: nil,
				})
			}
			lastCount = currentCount
		}
		for {
			select {
			case <-ctx.Done():
				feedbackRiskCount()
				return
			case <-ticker.C:
				feedbackRiskCount()
			}
		}
	}()
}

func (s *Server) forceRiskCountFeedback(runtimeId string, yakClient *yaklib.YakitClient) (int, error) {
	return forceRiskCountFeedback(runtimeId, yakClient, s.GetProjectDatabase())
}
func forceRiskCountFeedback(runtimeId string, yakClient *yaklib.YakitClient, projectDB *gorm.DB) (int, error) {
	riskCount, err := yakit.CountRiskByRuntimeId(projectDB, runtimeId)
	if err != nil {
		log.Errorf("count risk failed: %v", err)
	} else {
		yakClient.Output(&yaklib.YakitStatusCard{ // card
			Id: "漏洞/风险/指纹", Data: strconv.Itoa(riskCount), Tags: nil,
		})
	}
	return riskCount, err
}

func execScriptWithExecParam(script *schema.YakScript, input string, stream sender, params []*ypb.KVPair, runtimeId string, projectDB *gorm.DB) error {
	var (
		scriptName = script.ScriptName
		scriptType = script.Type
	)
	streamCtx, cancel := context.WithCancel(stream.Context())
	stream.Send(&ypb.ExecResult{IsMessage: false, RuntimeID: runtimeId}) // 触发前端切换结果页面
	defer func() {
		if err := recover(); err != nil {
			log.Warn(err)
			utils.PrintCurrentGoroutineRuntimeStack()
		}
	}()

	feedbackClient := yaklib.NewVirtualYakitClientWithRuntimeID(func(result *ypb.ExecResult) error {
		result.RuntimeID = runtimeId
		return stream.Send(result)
	}, runtimeId) // set risk count
	tickerRiskCountFeedback(streamCtx, 2*time.Second, runtimeId, feedbackClient, projectDB)
	defer forceRiskCountFeedback(runtimeId, feedbackClient, projectDB)

	engine := yak.NewYakitVirtualClientScriptEngine(feedbackClient)
	log.Infof("engine.ExecuteExWithContext(stream.Context(), debugScript ... \n")
	engine.RegisterEngineHooks(func(engine *antlr4yak.Engine) error {
		engine.SetVars(map[string]any{
			"RUNTIME_ID": runtimeId,
		})
		app := cli.DefaultCliApp
		// 额外处理 cli，新建 cli app
		if strings.ToLower(scriptType) == "yak" {
			tempArgs := makeArgs(streamCtx, params)
			app = yak.GetHookCliApp(tempArgs)
		}
		yak.BindYakitPluginContextToEngine(engine, yak.CreateYakitPluginContext(
			runtimeId,
		).WithPluginName(
			scriptName,
		).WithContext(
			streamCtx,
		).WithCliApp(
			app,
		).WithContextCancel(
			cancel,
		).WithPluginUUID(
			script.Uuid,
		).WithYakitClient(feedbackClient))

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
	switch strings.ToLower(scriptType) {
	case "codec":
		tabName := "Codec结果"
		subEngine, err := engine.ExecuteExWithContext(streamCtx, script.Content, map[string]any{
			"CTX":          streamCtx,
			"PLUGIN_NAME":  scriptName,
			"YAK_FILENAME": scriptName,
		})
		if err != nil {
			return utils.Errorf("execute file %s code failed: %s", scriptName, err.Error())
		}
		result, err := subEngine.SafeCallYakFunction(streamCtx, "handle", []interface{}{input})
		if err != nil {
			return utils.Errorf("call %v' s handle function failed: %s", scriptName, err)
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
		_, err := engine.ExecuteExWithContext(streamCtx, script.Content, map[string]any{
			"RUNTIME_ID":   runtimeId,
			"CTX":          streamCtx,
			"PLUGIN_NAME":  scriptName,
			"YAK_FILENAME": scriptName,
		})
		if err != nil {
			log.Warnf("execute debug script failed: %v", err)
			return err
		}
		return nil
	default:
		return utils.Error("unsupported plugin type: " + scriptType)
	}
}

func (s *Server) execScriptWithRequest(scriptInstance *schema.YakScript, targetInput string, stream sender, execParams []*ypb.KVPair, runtimeId string, params ...*ypb.HTTPRequestBuilderParams) error {
	var (
		scriptName = scriptInstance.ScriptName
		scriptCode = scriptInstance.Content
		scriptType = scriptInstance.Type
		isTemp     = scriptInstance.Ignored && (strings.HasPrefix(scriptInstance.ScriptName, "[TMP]") || strings.HasPrefix(scriptInstance.ScriptName, "]"))
		projectDB  = s.GetProjectDatabase()
	)
	streamCtx, cancel := context.WithCancel(stream.Context())
	if scriptName == "" {
		return utils.Error("script name is empty")
	}

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

	switch strings.ToLower(scriptType) {
	case "mitm", "port-scan", "nuclei":
	default:
		return utils.Error("unsupported plugin type: " + scriptType)
	}

	var reqs []any

	// smoking
	isSmoking := false
	isStrict := false
	shouldRenderFuzzTag := false
	if len(execParams) > 0 {
		for _, p := range execParams {
			if p.Key == "State" && p.Value == "Smoking" {
				isSmoking = true
			}
			if p.Key == "Mode" && p.Value == "Strict" {
				isStrict = true
			}
			if p.Key == "FuzzTag" && p.Value == "true" {
				shouldRenderFuzzTag = true
			}
		}
	}

	BuildRes, err := BuildHttpRequestPacket(s.GetProjectDatabase(), baseBuilderParams, targetInput)
	if err != nil {
		return utils.Wrapf(err, "build http request failed")
	}

	for packet := range BuildRes {
		requestBytes := packet.Request

		// 如果需要渲染 fuzztag，在这里处理
		if shouldRenderFuzzTag {
			// 使用 mutate 包渲染 fuzztag
			rendered, err := mutate.FuzzTagExec(string(requestBytes))
			if err == nil && len(rendered) > 0 {
				// 使用第一个渲染结果
				requestBytes = []byte(rendered[0])
			}
		}

		reqs = append(reqs, map[string]any{
			"RawHTTPRequest": requestBytes,
			"IsHttps":        packet.IsHttps,
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

	feedbackClient := yaklib.NewVirtualYakitClientWithRuntimeID(func(result *ypb.ExecResult) error {
		result.RuntimeID = runtimeId
		return stream.Send(result)
	}, runtimeId) // set risk count
	tickerRiskCountFeedback(streamCtx, 2*time.Second, runtimeId, feedbackClient, projectDB)
	defer forceRiskCountFeedback(runtimeId, feedbackClient, projectDB)

	engine := yak.NewYakitVirtualClientScriptEngine(feedbackClient)
	log.Infof("engine.ExecuteExWithContext(stream.Context(), debugScript ... \n")
	engine.RegisterEngineHooks(func(engine *antlr4yak.Engine) error {
		engine.SetVars(map[string]any{
			"RUNTIME_ID": runtimeId,
		})
		yak.BindYakitPluginContextToEngine(
			engine,
			yak.CreateYakitPluginContext(runtimeId).
				WithPluginName(scriptName).
				WithContext(streamCtx).
				WithContextCancel(cancel).
				WithYakitClient(feedbackClient),
		)
		return nil
	})
	subEngine, err := engine.ExecuteExWithContext(streamCtx, debugScriptCode, map[string]any{
		"REQUESTS":     reqs,
		"CTX":          streamCtx,
		"PLUGIN":       scriptInstance,
		"PLUGIN_CODE":  scriptCode,
		"PLUGIN_NAME":  scriptName,
		"YAK_FILENAME": scriptName,
		"PLUGIN_TYPE":  strings.ToLower(scriptType),
		"IS_SMOKING":   isSmoking,
		"IS_STRICT":    isStrict,
		"RUNTIME_ID":   runtimeId,
		"CLI_PARAMS":   KVPairToParamItem(execParams),
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
	runtimeId string,
	params ...*ypb.HTTPRequestBuilderParams,
) error {
	script, err := yakit.NewTemporaryYakScript(debugType, debugCode)
	if err != nil {
		return err
	}
	return s.execScriptEx(input, script, stream, execParams, runtimeId, params...)
}

func (s *Server) execScript(
	input string, // only "codec" / url: "mitm" "nuclei" "port-scan"
	scriptType string,
	name string,
	stream sender,
	execParams []*ypb.KVPair, // 脚本执行的参数, only "yak"
	runtimeId string,
	params ...*ypb.HTTPRequestBuilderParams, // 用于构建请求, only used in "mitm", "nuclei", "port-scan"
) error {
	script, err := yakit.GetYakScriptByName(consts.GetGormProfileDatabase(), name)
	if err != nil {
		return err
	}
	return s.execScriptEx(input, script, stream, execParams, runtimeId, params...)
}

func (s *Server) execScriptEx(
	input string, // only "codec" / url: "mitm" "nuclei" "port-scan"
	script *schema.YakScript,
	stream sender,
	execParams []*ypb.KVPair, // 脚本执行的参数, only "yak"
	runtimeId string,
	params ...*ypb.HTTPRequestBuilderParams, // 用于构建请求, only used in "mitm", "nuclei", "port-scan"
) error {
	scriptType := script.Type
	switch scriptType {
	case "yak", "codec":
		return execScriptWithExecParam(script, input, stream, execParams, runtimeId, s.GetProjectDatabase())
	case "mitm", "nuclei", "port-scan":
		return s.execScriptWithRequest(script, input, stream, execParams, runtimeId, params...)
	}
	return utils.Error("unsupported plugin type: " + scriptType)
}

func makeArgs(ctx context.Context, execParams []*ypb.KVPair) []string {
	args := []string{"yak"}
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
			go func() {
				select {
				case <-ctx.Done():
					os.Remove(tempName)
				}
			}()

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
	var res *ypb.HTTPRequestBuilderParams

	buffer, err := json.Marshal(params)
	if err != nil {
		log.Errorf("json marshal err")
		return nil
	}
	err = json.Unmarshal(buffer, &res)
	if err != nil {
		log.Errorf("json unmarshal err")
		return nil
	}

	if res == nil {
		res = &ypb.HTTPRequestBuilderParams{
			IsRawHTTPRequest:    false,
			IsHttps:             false,
			Method:              "GET",
			Path:                []string{"/"},
			GetParams:           []*ypb.KVPair{},
			Headers:             []*ypb.KVPair{},
			Cookie:              []*ypb.KVPair{},
			Body:                []byte{},
			PostParams:          []*ypb.KVPair{},
			MultipartParams:     []*ypb.KVPair{},
			MultipartFileParams: []*ypb.KVPair{},
		}
	}

	pathFlag := true
	for _, p := range res.Path {
		if normalizeString(p) == normalizeString(t.Path) {
			pathFlag = false
			break
		}
	}
	if pathFlag {
		res.Path = append(res.Path, t.Path)
	}

	for key, values := range t.Query() { // 插入所有的 get 参数
		for _, value := range values {
			res.GetParams = append(res.GetParams, &ypb.KVPair{
				Key: key, Value: value,
			})
		}
	}

	if t.Scheme != "" { // 目标标识优先级更高
		res.IsHttps = t.Scheme == "https"
	}

	return res
}

func normalizeString(s string) string {
	if s == "" || s == "/" {
		return ""
	}
	return s
}
