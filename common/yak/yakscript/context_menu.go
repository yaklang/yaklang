package yakscript

import (
	"context"
	"strings"

	"github.com/yaklang/gorm"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/cli"
	"github.com/yaklang/yaklang/common/yak"
	"github.com/yaklang/yaklang/common/yak/antlr4yak"
	"github.com/yaklang/yaklang/common/yak/contextmenu"
	"github.com/yaklang/yaklang/common/yak/yaklib"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

// ExecContextMenuScript loads one context-menu plugin for one invocation and
// calls exactly the hook selected by the caller. The engine is not retained
// after the call returns.
func ExecContextMenuScript(
	script *schema.YakScript,
	hookName string,
	actionCtx *contextmenu.ActionContext,
	hookArgs []any,
	stream StreamSender,
	params []*ypb.KVPair,
	runtimeID string,
	projectDB *gorm.DB,
) (callResult any, callErr error) {
	if script == nil {
		return nil, utils.Error("context-menu script is nil")
	}
	if strings.ToLower(strings.TrimSpace(script.Type)) != contextmenu.PluginType {
		return nil, utils.Errorf("unsupported context-menu plugin type: %s", script.Type)
	}
	if stream == nil {
		return nil, utils.Error("context-menu stream is nil")
	}
	if !contextmenu.IsKnownHook(hookName) {
		return nil, utils.Errorf("unknown context-menu hook: %s", hookName)
	}

	streamCtx, cancel := context.WithCancel(stream.Context())
	defer cancel()
	defer func() {
		if recovered := recover(); recovered != nil {
			log.Warnf("context-menu execution panic: %v", recovered)
			utils.PrintCurrentGoroutineRuntimeStack()
			callResult = nil
			callErr = utils.Errorf("context-menu execution panic: %v", recovered)
		}
	}()

	feedbackClient := yaklib.NewVirtualYakitClientWithRuntimeID(func(execResult *ypb.ExecResult) error {
		execResult.RuntimeID = runtimeID
		return stream.Send(execResult)
	}, runtimeID)
	if projectDB != nil {
		defer ForceRiskCountFeedback(runtimeID, feedbackClient, projectDB)
	}

	engine := yak.NewYakitVirtualClientScriptEngine(feedbackClient)
	engine.RegisterEngineHooks(func(engine *antlr4yak.Engine) error {
		engine.SetVars(map[string]any{"RUNTIME_ID": runtimeID})
		app := cli.DefaultCliApp
		app = yak.GetHookCliApp(makeArgs(streamCtx, params))
		yak.BindYakitPluginContextToEngine(engine, yak.CreateYakitPluginContext(runtimeID).
			WithPluginName(script.ScriptName).
			WithContext(streamCtx).
			WithCliApp(app).
			WithContextCancel(cancel).
			WithPluginUUID(script.Uuid).
			WithYakitClient(feedbackClient))
		return nil
	})

	subEngine, err := engine.ExecuteExWithContext(streamCtx, script.Content, map[string]any{
		"CTX":          actionCtx,
		"RUNTIME_ID":   runtimeID,
		"PLUGIN_NAME":  script.ScriptName,
		"YAK_FILENAME": script.ScriptName,
	})
	if err != nil {
		return nil, utils.Errorf("load context-menu plugin %s failed: %s", script.ScriptName, err)
	}

	callArgs := make([]any, 0, len(hookArgs)+1)
	callArgs = append(callArgs, actionCtx)
	callArgs = append(callArgs, hookArgs...)
	callResult, err = subEngine.SafeCallYakFunction(streamCtx, hookName, callArgs)
	if err != nil {
		return nil, utils.Errorf("call %s.%s failed: %s", script.ScriptName, hookName, err)
	}
	return callResult, nil
}
