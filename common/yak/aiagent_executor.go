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
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
)

var HOOK_AI_FORGE = "forgeHandle"

type Option func(*Agent) error

type Agent struct {
	ForgeName string

	ctx    context.Context
	cancel context.CancelFunc

	RuntimeID string

	PlanAICallback    aid.AICallbackType
	TaskAICallback    aid.AICallbackType
	GeneralAICallback aid.AICallbackType

	ExtendAIDOptions []aid.Option
}

func WithForgeName(forgeName string) Option {
	return func(ag *Agent) error {
		ag.ForgeName = forgeName
		return nil
	}
}

func WithContext(ctx context.Context) Option {
	return func(ag *Agent) error {
		ag.ctx = ctx
		return nil
	}
}

func WithExtendAIDOptions(opts ...aid.Option) Option {
	return func(ag *Agent) error {
		ag.ExtendAIDOptions = append(ag.ExtendAIDOptions, opts...)
		return nil
	}
}

func WithRuntimeID(runtimeID string) Option {
	return func(ag *Agent) error {
		ag.RuntimeID = runtimeID
		return nil
	}
}

func WithPlanAICallback(callback aid.AICallbackType) Option {
	return func(ag *Agent) error {
		ag.PlanAICallback = callback
		return nil
	}
}

func WithTaskAICallback(callback aid.AICallbackType) Option {
	return func(ag *Agent) error {
		ag.TaskAICallback = callback
		return nil
	}
}

func WithAICallback(callback aid.AICallbackType) Option {
	return func(ag *Agent) error {
		ag.GeneralAICallback = callback
		return nil
	}
}

func (ag *Agent) IsAICallbackAvailable() bool {
	if ag.PlanAICallback != nil || ag.TaskAICallback != nil || ag.GeneralAICallback != nil {
		return true
	}
	return false
}

func (ag *Agent) SubOption() []Option {
	opts := make([]Option, 0)
	if ag.GeneralAICallback != nil {
		opts = append(opts, WithAICallback(ag.GeneralAICallback))
	}
	if ag.PlanAICallback != nil {
		opts = append(opts, WithPlanAICallback(ag.PlanAICallback))
	}
	if ag.TaskAICallback != nil {
		opts = append(opts, WithTaskAICallback(ag.TaskAICallback))
	}
	if ag.RuntimeID != "" {
		opts = append(opts, WithRuntimeID(ag.RuntimeID))
	}
	if ag.ctx != nil {
		opts = append(opts, WithContext(ag.ctx))
	}
	if ag.ExtendAIDOptions != nil {
		opts = append(opts, WithExtendAIDOptions(ag.ExtendAIDOptions...))
	}
	return opts
}

func ExecuteForge(forgeName string, i any, opts ...Option) (any, error) {
	ag := &Agent{
		ForgeName: forgeName,
	}
	for _, opt := range opts {
		if err := opt(ag); err != nil {
			return nil, err
		}
	}

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

	params := aiforge.Any2ExecParams(i)
	engine := NewYakitVirtualClientScriptEngine(nil)
	engine.RegisterEngineHooks(func(engine *antlr4yak.Engine) error {
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

	subEngine, err := engine.ExecuteExWithContext(ag.ctx, forgeIns.ForgeContent, nil)
	if err != nil {
		return nil, err
	}
	result, err := subEngine.SafeCallYakFunction(ag.ctx, HOOK_AI_FORGE, []any{params})
	return result, err
}

func (ag *Agent) AIDOptions() []aid.Option {
	var aidopts []aid.Option
	if ag.RuntimeID != "" {
		aidopts = append(aidopts, aid.WithRuntimeID(ag.RuntimeID))
	}
	if ag.PlanAICallback != nil {
		aidopts = append(aidopts, aid.WithPlanAICallback(ag.PlanAICallback))
	}
	if ag.TaskAICallback != nil {
		aidopts = append(aidopts, aid.WithTaskAICallback(ag.TaskAICallback))
	}
	if ag.GeneralAICallback != nil {
		aidopts = append(aidopts, aid.WithAICallback(ag.GeneralAICallback))
	}
	return append(aidopts, ag.ExtendAIDOptions...)
}
