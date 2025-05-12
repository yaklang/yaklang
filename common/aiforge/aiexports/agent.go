package aiexports

import (
	"context"
	"github.com/yaklang/yaklang/common/ai"
	"github.com/yaklang/yaklang/common/ai/aid"
	"github.com/yaklang/yaklang/common/aiforge"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

type Option func(*Agent) error

type Agent struct {
	ForgeName string

	ctx    context.Context
	cancel context.CancelFunc

	PlanAICallback    aid.AICallbackType
	TaskAICallback    aid.AICallbackType
	GeneralAICallback aid.AICallbackType
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

func (a *Agent) IsAICallbackAvailable() bool {
	if a.PlanAICallback != nil || a.TaskAICallback != nil || a.GeneralAICallback != nil {
		return true
	}
	return false
}

func ExecuteForge(name string, i any, opts ...Option) error {
	ag := &Agent{
		ForgeName: name,
	}
	for _, opt := range opts {
		if err := opt(ag); err != nil {
			return err
		}
	}
	if ag.ctx == nil {
		ag.ctx, ag.cancel = context.WithCancel(context.Background())
	} else {
		ag.ctx, ag.cancel = context.WithCancel(ag.ctx)
	}

	var params []*ypb.ExecParamItem
	if utils.IsMap(i) {
		for k, v := range utils.InterfaceToGeneralMap(i) {
			params = append(params, &ypb.ExecParamItem{
				Key:   k,
				Value: utils.InterfaceToString(v),
			})
		}
	} else {
		params = append(params, &ypb.ExecParamItem{
			Key:   "query",
			Value: utils.InterfaceToString(i),
		})
	}

	var aidopts []aid.Option
	if ag.PlanAICallback != nil {
		aidopts = append(aidopts, aid.WithPlanAICallback(ag.PlanAICallback))
	}
	if ag.TaskAICallback != nil {
		aidopts = append(aidopts, aid.WithTaskAICallback(ag.TaskAICallback))
	}
	if ag.GeneralAICallback != nil {
		aidopts = append(aidopts, aid.WithAICallback(ag.GeneralAICallback))
	}

	// no ai set, use ai default
	if !ag.IsAICallbackAvailable() {
		ag.GeneralAICallback = aid.AIChatToAICallbackType(ai.Chat)
		aidopts = append(aidopts, aid.WithAICallback(ag.GeneralAICallback))
	}

	if ag.ForgeName != "" {
		_, err := aiforge.ExecuteForge(ag.ForgeName, ag.ctx, params, aidopts...)
		if err != nil {
			return err
		}
	} else {
		// TODO: handle params carefully
		ins, err := aid.NewCoordinator(string(utils.Jsonify(params)), aidopts...)
		if err != nil {
			return err
		}
		return ins.Run()
	}
	return nil
}

// exports to yaklang
var Exports = map[string]any{
	"ExecuteForge":   ExecuteForge,
	"planAICallback": WithPlanAICallback,
	"taskAICallback": WithTaskAICallback,
	"aiCallback":     WithAICallback,
	"ctx":            WithContext,
}
