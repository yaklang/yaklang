package yak

import (
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aiexec"
	"github.com/yaklang/yaklang/common/utils"
)

//func AIForgeExecWithConfig(forgeName string, forgeParams any, config *aicommon.Config) (any, error) {
//	forgeIns, err := yakit.GetAIForgeByName(consts.GetGormProfileDatabase(), forgeName)
//	if err != nil {
//		return nil, utils.Errorf("failed to get forge instance: %v", err)
//	}
//	if forgeIns.ForgeType == schema.FORGE_TYPE_Config {
//
//	}
//
//	ctx, cancel := context.WithCancel(config.Ctx)
//
//	params := aiforge.Any2ExecParams(forgeParams)
//	engine := NewYakitVirtualClientScriptEngine(yaklib.NewVirtualYakitClient(func(i *ypb.ExecResult) error {
//		if i != nil && config.EventHandler != nil {
//			event := &schema.AiOutputEvent{
//				CoordinatorId: config.Id,
//				Type:          schema.EVENT_TYPE_YAKIT_EXEC_RESULT,
//				NodeId:        "yakit",
//				Content:       utils.Jsonify(i),
//				Timestamp:     time.Now().Unix(),
//				IsJson:        true,
//			}
//			config.EventHandler(event)
//		}
//		return nil
//	}))
//	engine.RegisterEngineHooks(func(engine *antlr4yak.Engine) error {
//		app := GetHookCliApp(makeArgs(ctx, params))
//		BindYakitPluginContextToEngine(engine, CreateYakitPluginContext(
//			config.Id,
//		).WithPluginName(
//			forgeName,
//		).WithContext(
//			ctx,
//		).WithCliApp(
//			app,
//		).WithContextCancel(
//			cancel,
//		))
//		buildConfigHookForgeHandle(engine, config)
//		return nil
//	})
//	forgeCode := `query = cli.String("query", cli.setHelp("用户输入"),cli.setRequired(true), cli.setVerboseName("原始用户输入"))`
//	if forgeIns.ForgeContent != "" {
//		forgeCode = forgeIns.ForgeContent
//	}
//	subEngine, err := engine.ExecuteExWithContext(ctx, forgeCode, nil)
//	if err != nil {
//		return nil, err
//	}
//	if v, ok := subEngine.GetVar(HOOK_AI_FORGE); ok {
//		if yakFunc, ok := v.(*yakvm.Function); ok {
//			if yakFunc.IsVariableParameter() {
//				log.Infof("call yak function %s with params %v with variable parameter", HOOK_AI_FORGE, utils.ShrinkString(params, 200))
//				return subEngine.SafeCallYakFunction(ctx, HOOK_AI_FORGE, append([]any{params}))
//			} else {
//				log.Infof("call yak function %s with params %v", HOOK_AI_FORGE, utils.ShrinkString(params, 200))
//				return subEngine.SafeCallYakFunction(ctx, HOOK_AI_FORGE, []any{params})
//			}
//		} else {
//			return nil, utils.Errorf("forge handle is not a function")
//		}
//	} else {
//		return nil, utils.Errorf("forge handle is nil")
//	}
//}
//
//func ForgeExecWithConfig(forgeIns *schema.AIForge, config *aicommon.Config) (any, error) {
//	initPrompt := forgeIns.InitPrompt
//
//	persistentPrompt := forgeIns.PersistentPrompt
//
//	planPrompt := forgeIns.PlanPrompt
//
//	resultPrompt := forgeIns.ResultPrompt
//
//	cfg := aiforge.NewYakForgeBlueprintConfigFromSchemaForge(forgeIns).
//		WithInitPrompt(initPrompt).
//		WithPersistentPrompt(persistentPrompt).
//		WithPlanPrompt(planPrompt).
//		WithResultPrompt(resultPrompt)
//	blueprint, err := cfg.Build()
//	if err != nil {
//		return nil, utils.Errorf("failed to build forge handle: %v", err)
//	}
//	ins, err := blueprint.CreateCoordinator(ctx, items, aiCommonOpts...)
//	if err != nil {
//		return nil, err
//	}
//	if err := ins.Run(); err != nil {
//		return nil, err
//	}
//	return cfg.ForgeResult, nil
//}
//
//func buildConfigHookForgeHandle(nIns *antlr4yak.Engine, config *aicommon.Config) {
//	nIns.GetVM().RegisterMapMemberCallHandler("aiagent", "ExecuteForge", func(i interface{}) interface{} {
//		_, ok := i.(func(forgeName string, i any, opts ...any) (any, error))
//		if ok {
//			return func(forgeName string, i any, opts ...any) (any, error) {
//				return AIForgeExecWithConfig(forgeName, i, config), nil
//			}
//		}
//		return i
//	})
//
//	nIns.GetVM().RegisterMapMemberCallHandler("liteforge", "Execute", func(i interface{}) interface{} {
//		_, ok := i.(func(query string, opts ...any) (*aiforge.ForgeResult, error))
//		if ok {
//			return func(query string, opts ...any) (*aiforge.ForgeResult, error) {
//				return aiforge.ExecuteLiteForgeWithConfig(query, config, utils.FilterInterface[aiforge.LiteForgeExecOption](opts)...)
//			}
//		}
//		return i
//	})
//}

func AIForgeExec(forgeName string, forgeParams any, opts ...aicommon.ConfigOption) (any, error) {
	return ExecuteForge(forgeName, forgeParams, utils.InterfaceToSliceInterface(opts)...)
}

func init() {
	aiexec.RegisterForgeRunner(AIForgeExec)
}
