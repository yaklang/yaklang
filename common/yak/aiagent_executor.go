package yak

import (
	"context"

	"github.com/google/uuid"
	"github.com/yaklang/yaklang/common/ai/aid"
	"github.com/yaklang/yaklang/common/aiforge"
	"github.com/yaklang/yaklang/common/consts"
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
	if ag.RuntimeID == "" {
		ag.RuntimeID = uuid.NewString()
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
	engine := NewYakitVirtualClientScriptEngine(nil)
	engine.RegisterEngineHooks(func(engine *antlr4yak.Engine) error {
		defaultForgeHandle = buildDefaultForgeHandle(forgeName, engine)
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
			ag.RuntimeID,
		).WithPluginName(
			forgeName,
		).WithContext(
			ag.ctx,
		).WithCliApp(
			app,
		).WithContextCancel(
			ag.cancel,
		))
		BindAIConfigToEngine(engine, ag)
		return nil
	})
	forgeCode := `query = cli.String("query", cli.setHelp("用户输入"),cli.setRequired(true))`
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
				return subEngine.SafeCallYakFunction(ag.ctx, HOOK_AI_FORGE, append([]any{params}, iopts...))
			} else {
				return subEngine.SafeCallYakFunction(ag.ctx, HOOK_AI_FORGE, []any{params})
			}
		} else {
			return defaultForgeHandle(params, iopts...)
		}
	} else {
		return nil, utils.Errorf("forge handle is nil")
	}
}

func (ag *Agent) AIDOptions() []aid.Option {
	opts := make([]aid.Option, 0)
	if ag.RuntimeID != "" {
		opts = append(opts, aid.WithRuntimeID(ag.RuntimeID))
	}
	opts = append(opts, ag.ExtendAIDOptions...)
	return opts
}

func buildDefaultForgeHandle(forgeName string, engine *antlr4yak.Engine) func(items []*ypb.ExecParamItem, opts ...any) (any, error) {
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
		var opts []Option
		for _, opt := range anyOpts {
			if o, ok := opt.(Option); ok {
				opts = append(opts, o)
			}
		}
		var aidOpts []aid.Option
		ag := &Agent{}
		for _, opt := range opts {
			if err := opt(ag); err != nil {
				return nil, err
			}
		}
		aidOpts = append(aidOpts, ag.AIDOptions()...)
		aidOpts = append(aidOpts, aid.WithDebugPrompt(true))
		aidOpts = append(aidOpts, aid.WithDebug(true))
		aidOpts = append(aidOpts, aid.WithAgreeYOLO(true))
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
		cfg := aiforge.NewYakForgeBlueprintConfig(forgeName, initPrompt, persistentPrompt).
			WithPlanPrompt(planPrompt).
			WithResultPrompt(resultPrompt)
		blueprint, err := cfg.Build()
		if err != nil {
			return nil, utils.Errorf("failed to build forge handle: %v", err)
		}
		ins, err := blueprint.CreateCoordinator(context.Background(), items, aidOpts...)
		if err != nil {
			return nil, err
		}
		if err := ins.Run(); err != nil {
			return nil, err
		}
		return cfg.ForgeResult, nil
	}
}
