package yak

import (
	"context"
	"time"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/yak/yaklib"

	"github.com/google/uuid"
	"github.com/yaklang/yaklang/common/aiforge"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/antlr4yak"
	"github.com/yaklang/yaklang/common/yak/antlr4yak/yakvm"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

var HOOK_AI_FORGE = "forgeHandle"
var DEFAULT_INIT_PROMPT_NAME = "__INIT_PROMPT__"
var DEFAULT_PERSISTENT_PROMPT_NAME = "__PERSISTENT_PROMPT__"
var DEFAULT_PLAN_PROMPT_NAME = "__PLAN_PROMPT__"
var DEFAULT_RESULT_PROMPT_NAME = "__RESULT_PROMPT__"
var DEFAULT_FORGE_HANDLE_NAME = "__DEFAULT_FORGE_HANDLE__"

func ExecuteForge(forgeName string, i any, iopts ...any) (any, error) {
	ag := NewAgent(iopts...)
	ag.ForgeName = forgeName
	if ag.CoordinatorId == "" {
		ag.CoordinatorId = uuid.NewString()
		iopts = append(iopts, WithCoordinatorId(ag.CoordinatorId))
	}

	if ag.ctx == nil {
		ag.ctx, ag.cancel = context.WithCancel(context.Background())
	} else {
		ag.ctx, ag.cancel = context.WithCancel(ag.ctx)
	}

	forgeIns, err := yakit.GetAIForgeByName(consts.GetGormProfileDatabase(), forgeName)
	if err != nil {
		return nil, utils.Errorf("failed to get forge instance: %v", err)
	}

	if forgeIns.ForgeType != schema.FORGE_TYPE_YAK {
		// todo: support json config forge
	}
	var defaultForgeHandle func(items []*ypb.ExecParamItem, opts ...any) (any, error)

	params := aiforge.Any2ExecParams(i)
	engine := NewYakitVirtualClientScriptEngine(yaklib.NewVirtualYakitClient(func(i *ypb.ExecResult) error {
		if i != nil && ag.AgentEventHandler != nil {
			event := &schema.AiOutputEvent{
				CoordinatorId: ag.CoordinatorId,
				Type:          schema.EVENT_TYPE_YAKIT_EXEC_RESULT,
				NodeId:        "yakit",
				Content:       utils.Jsonify(i),
				Timestamp:     time.Now().Unix(),
				IsJson:        true,
			}
			ag.AgentEventHandler(event)
		}
		return nil
	}))
	engine.RegisterEngineHooks(func(engine *antlr4yak.Engine) error {
		defaultForgeHandle = buildDefaultForgeHandle(ag.ctx, forgeIns, engine)
		engine.GetVM().SetVars(map[string]any{
			DEFAULT_INIT_PROMPT_NAME:       forgeIns.InitPrompt,
			DEFAULT_PERSISTENT_PROMPT_NAME: forgeIns.PersistentPrompt,
			DEFAULT_PLAN_PROMPT_NAME:       forgeIns.PlanPrompt,
			DEFAULT_RESULT_PROMPT_NAME:     forgeIns.ResultPrompt,
			DEFAULT_FORGE_HANDLE_NAME:      defaultForgeHandle,
			HOOK_AI_FORGE:                  defaultForgeHandle,
		})
		app := GetHookCliApp(makeArgs(ag.ctx, params))
		BindYakitPluginContextToEngine(engine, CreateYakitPluginContext(
			ag.CoordinatorId,
		).WithPluginName(
			forgeName,
		).WithContext(
			ag.ctx,
		).WithCliApp(
			app,
		).WithContextCancel(
			ag.cancel,
		))
		BindAIConfigToEngine(engine, iopts...)
		return nil
	})
	forgeCode := `query = cli.String("query", cli.setHelp("用户输入"),cli.setRequired(true), cli.setVerboseName("原始用户输入"))`
	if forgeIns.ForgeContent != "" {
		forgeCode = forgeIns.ForgeContent
	}
	subEngine, err := engine.ExecuteExWithContext(ag.ctx, forgeCode, nil)
	if err != nil {
		return nil, err
	}
	if v, ok := subEngine.GetVar(HOOK_AI_FORGE); ok {
		if yakFunc, ok := v.(*yakvm.Function); ok {
			if yakFunc.IsVariableParameter() {
				log.Infof("call yak function %s with params %v with variable parameter", HOOK_AI_FORGE, utils.ShrinkString(params, 200))
				return subEngine.SafeCallYakFunction(ag.ctx, HOOK_AI_FORGE, append([]any{params}, iopts...))
			} else {
				log.Infof("call yak function %s with params %v", HOOK_AI_FORGE, utils.ShrinkString(params, 200))
				return subEngine.SafeCallYakFunction(ag.ctx, HOOK_AI_FORGE, []any{params})
			}
		} else {
			log.Infof("call yak function (defaultForgeHandle) %s with params %v", HOOK_AI_FORGE, utils.ShrinkString(params, 200))
			return defaultForgeHandle(params, iopts...)
		}
	} else {
		return nil, utils.Errorf("forge handle is nil")
	}
}

func (ag *Agent) AICommonOptions() []aicommon.ConfigOption {
	opts := make([]aicommon.ConfigOption, 0)
	if ag.CoordinatorId != "" {
		opts = append(opts, aicommon.WithID(ag.CoordinatorId))
	}
	opts = append(ag.ExtendAICommonOptions, opts...)
	return opts
}

func buildDefaultForgeHandle(ctx context.Context, forgeIns *schema.AIForge, engine *antlr4yak.Engine) func(items []*ypb.ExecParamItem, opts ...any) (any, error) {
	getStringVar := func(name string) (string, bool) {
		initPrompt, ok := engine.GetVM().GetVar(name)
		if !ok {
			return "", false
		}
		initPromptStr, ok := initPrompt.(string)
		if !ok {
			return "", false
		}
		return initPromptStr, true
	}
	return func(items []*ypb.ExecParamItem, anyOpts ...any) (any, error) {
		ag := NewAgent(anyOpts...)
		aiCommonOpts := ag.AICommonOptions()

		initPrompt, ok := getStringVar(DEFAULT_INIT_PROMPT_NAME)
		if !ok {
			return nil, utils.Errorf("init prompt is nil")
		}
		persistentPrompt, ok := getStringVar(DEFAULT_PERSISTENT_PROMPT_NAME)
		if !ok {
			return nil, utils.Errorf("persistent prompt is nil")
		}
		planPrompt, ok := getStringVar(DEFAULT_PLAN_PROMPT_NAME)
		if !ok {
			return nil, utils.Errorf("plan prompt is nil")
		}
		resultPrompt, ok := getStringVar(DEFAULT_RESULT_PROMPT_NAME)
		if !ok {
			return nil, utils.Errorf("result prompt is nil")
		}
		cfg := aiforge.NewYakForgeBlueprintConfigFromSchemaForge(forgeIns).
			WithInitPrompt(initPrompt).
			WithPersistentPrompt(persistentPrompt).
			WithPlanPrompt(planPrompt).
			WithResultPrompt(resultPrompt)
		blueprint, err := cfg.Build()
		if err != nil {
			return nil, utils.Errorf("failed to build forge handle: %v", err)
		}
		ins, err := blueprint.CreateCoordinator(ctx, items, aiCommonOpts...)
		if err != nil {
			return nil, err
		}
		if err := ins.Run(); err != nil {
			return nil, err
		}
		return cfg.ForgeResult, nil
	}
}
