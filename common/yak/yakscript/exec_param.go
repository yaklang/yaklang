package yakscript

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"time"

	"github.com/jinzhu/gorm"

	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/cli"
	"github.com/yaklang/yaklang/common/yak"
	"github.com/yaklang/yaklang/common/yak/antlr4yak"
	"github.com/yaklang/yaklang/common/yak/yaklib"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

// ExecScriptWithExecParam runs yak/codec plugin content with CLI params (used by gRPC debug and [ExecScriptWithParam]).
func ExecScriptWithExecParam(script *schema.YakScript, input string, stream StreamSender, params []*ypb.KVPair, runtimeId string, projectDB *gorm.DB) error {
	var (
		scriptName = script.ScriptName
		scriptType = script.Type
	)
	streamCtx, cancel := context.WithCancel(stream.Context())
	stream.Send(&ypb.ExecResult{IsMessage: false, RuntimeID: runtimeId}) // 触发前端切换结果页面
	defer printStackOnRecover()

	feedbackClient := yaklib.NewVirtualYakitClientWithRuntimeID(func(result *ypb.ExecResult) error {
		result.RuntimeID = runtimeId
		return stream.Send(result)
	}, runtimeId) // set risk count
	TickerRiskCountFeedback(streamCtx, 2*time.Second, runtimeId, feedbackClient, projectDB)
	defer ForceRiskCountFeedback(runtimeId, feedbackClient, projectDB)

	engine := yak.NewYakitVirtualClientScriptEngine(feedbackClient)
	log.Infof("engine.ExecuteExWithContext(stream.Context(), debugScript ... \n")
	engine.RegisterEngineHooks(func(engine *antlr4yak.Engine) error {
		engine.SetVars(map[string]any{
			"RUNTIME_ID": runtimeId,
		})
		app := cli.DefaultCliApp
		// 额外处理 cli，新建 cli app
		tempArgs := makeArgs(streamCtx, params)
		app = yak.GetHookCliApp(tempArgs)
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
			pluginFilter := new(ypb.QueryYakScriptRequest)
			var pluginName []string
			err := json.Unmarshal([]byte(p.Value), pluginFilter)
			if err != nil {
				log.Errorf("unmarshal plugin filter failed: %v", err)
				continue
			}
			yakit.FilterYakScript(consts.GetGormProfileDatabase(), pluginFilter).Pluck("script_name", &pluginName)
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
