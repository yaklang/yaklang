package yak

import (
	"context"
	"slices"

	"github.com/yaklang/yaklang/common/ai/aid"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"

	"github.com/yaklang/yaklang/common/ai/aid/aitool/buildinaitools/yakscripttools"
	"github.com/yaklang/yaklang/common/schema"

	"github.com/yaklang/yaklang/common/aiforge"
	"github.com/yaklang/yaklang/common/log"
)

type AIAgentOption func(*Agent) error

type Agent struct {
	ForgeName string

	ctx    context.Context
	cancel context.CancelFunc

	CoordinatorId string

	ExtendAICommonOptions []aicommon.ConfigOption
	AiForgeOptions        []aiforge.Option

	AgentEventHandler func(e *schema.AiOutputEvent)
}

func NewAgent(iopts ...any) *Agent {
	ag := &Agent{}
	for _, opt := range iopts {
		switch o := opt.(type) {
		case AIAgentOption:
			if err := o(ag); err != nil {
				log.Errorf("failed to apply agent option: %v", err)
				return nil
			}
		case aicommon.ConfigOption:
			ag.ExtendAICommonOptions = append(ag.ExtendAICommonOptions, o)
		case aiforge.Option:
			ag.AiForgeOptions = append(ag.AiForgeOptions, o)
		}
	}
	if ag.ctx == nil {
		ag.ctx, ag.cancel = context.WithCancel(context.Background())
	}

	aiCommonConfigId, ok := aicommon.GetLastIDFromConfigOptions(ag.ExtendAICommonOptions...)
	if ok {
		ag.CoordinatorId = aiCommonConfigId
	}

	return ag
}

// WithForgeName 设置 AI Agent 使用的 Forge 名称（导出名为 aiagent.forgeName）
// 参数:
//   - forgeName: Forge 名称
//
// 返回值:
//   - AI Agent 可选项
//
// Example:
// ```
// opt = aiagent.forgeName("my-forge")
// println(opt)
// ```
func WithForgeName(forgeName string) AIAgentOption {
	return func(ag *Agent) error {
		ag.ForgeName = forgeName
		return nil
	}
}

// WithContext 设置 AI Agent 的上下文，用于控制取消（导出名为 aiagent.context）
// 参数:
//   - ctx: 上下文对象
//
// 返回值:
//   - AI Agent 可选项
//
// Example:
// ```
// opt = aiagent.context(context.Background())
// println(opt)
// ```
func WithContext(ctx context.Context) AIAgentOption {
	return func(ag *Agent) error {
		ag.ctx = ctx
		return nil
	}
}

// WithExtendAICommonOptions 追加底层 aicommon 配置选项（导出名为 aiagent.extendAIDOptions）
// 参数:
//   - opts: 一个或多个 aicommon 配置选项
//
// 返回值:
//   - AI Agent 可选项
//
// Example:
// ```
// // opt 由 aicommon 提供（示意性示例）
// opt = aiagent.extendAIDOptions(aiagent.debug(true))
// println(opt)
// ```
func WithExtendAICommonOptions(opts ...aicommon.ConfigOption) AIAgentOption {
	return func(ag *Agent) error {
		ag.ExtendAICommonOptions = append(ag.ExtendAICommonOptions, opts...)
		return nil
	}
}

func (ag *Agent) SubOption() []AIAgentOption {
	opts := make([]AIAgentOption, 0)
	if ag.ctx != nil {
		opts = append(opts, WithContext(ag.ctx))
	}
	if ag.CoordinatorId != "" {
		opts = append(opts, WithCoordinatorId(ag.CoordinatorId))
	}
	if ag.ExtendAICommonOptions != nil {
		opts = append(opts, WithExtendAICommonOptions(ag.ExtendAICommonOptions...))
	}
	return opts
}

var (
	// Additional With options
	WithCoordinatorId = func(id string) AIAgentOption {
		return func(ag *Agent) error {
			ag.CoordinatorId = id
			return nil
		}
	}

	WithAiAgentEventHandler = func(handler func(e *schema.AiOutputEvent)) AIAgentOption {
		return func(ag *Agent) error {
			ag.AgentEventHandler = handler
			return nil
		}
	}
	WithDisallowRequireForUserPrompt = aicommon.WithDisallowRequireForUserPrompt
	WithAICallback                   = aicommon.WithAICallback
	WithPromptContextProvider        = aid.WithPromptContextProvider
	WithResultHandler                = aid.WithResultHandler

	// aitools
	AllYakScriptTools = yakscripttools.GetAllYakScriptAiTools
)

// NewLiteForge 创建一个 LiteForge 实例（导出名为 aiagent.CreateLiteForge）
// LiteForge 是轻量的一次性 AI 任务执行单元
// 参数:
//   - name: LiteForge 名称
//   - opts: 可选项，如 aiagent.liteForgePrompt、aiagent.liteForgeOutputSchema 等
//
// 返回值:
//   - LiteForge 实例
//   - 错误信息
//
// Example:
// ```
// // 需要配置可用的 AI 服务（示意性示例）
// lf = aiagent.CreateLiteForge("demo", aiagent.liteForgePrompt("extract the title"))~
// dump(lf)
// ```
func NewLiteForge(name string, opts ...any) (*aiforge.LiteForge, error) {
	return aiforge.NewLiteForge(name, BuildLiteForgeCreateOption(opts...)...)
}

// NewForgeBlueprint 创建一个 Forge 蓝图（导出名为 aiagent.CreateForge）
// Forge 蓝图描述了一个可复用的 AI 工作流，可基于它创建执行器
// 参数:
//   - name: Forge 名称
//   - opts: 可选项，如 aiagent.initPrompt、aiagent.persistentPrompt、aiagent.resultPrompt 等
//
// 返回值:
//   - Forge 蓝图对象
//
// Example:
// ```
// forge = aiagent.CreateForge("demo",
//
//	aiagent.initPrompt("you are a security expert"),
//
// )
// dump(forge)
// ```
func NewForgeBlueprint(name string, opts ...any) *aiforge.ForgeBlueprint {
	ag := NewAgent(opts...)
	ag.ForgeName = name
	aiforgeOpts := slices.Clone(ag.AiForgeOptions)
	aiforgeOpts = append(aiforgeOpts, aiforge.WithAIOptions(ag.AICommonOptions()...))
	return aiforge.NewForgeBlueprint(name, aiforgeOpts...)
}
func NewExecutorFromForge(forge *aiforge.ForgeBlueprint, i any, opts ...any) (*aid.Coordinator, error) {
	ag := NewAgent(opts...)
	ag.ForgeName = forge.Name
	params := aiforge.Any2ExecParams(i)
	return forge.CreateCoordinator(context.Background(), params, ag.AICommonOptions()...)
}

// NewExecutorFromJson 通过 JSON 描述的 Forge 蓝图创建执行器（导出名为 aiagent.NewExecutorFromJson）
// 参数:
//   - json: Forge 蓝图的 JSON 描述
//   - i: 执行参数（map 或键值对）
//   - opts: 可选项，如 aiagent.aiCallback、aiagent.context 等
//
// 返回值:
//   - 协调器对象（可调用 Run 执行）
//   - 错误信息
//
// Example:
// ```
// // 需要配置可用的 AI 服务（示意性示例）
// coordinator = aiagent.NewExecutorFromJson(forgeJson, {"query": "hello"})~
// dump(coordinator)
// ```
func NewExecutorFromJson(json string, i any, opts ...any) (*aid.Coordinator, error) {
	bp, err := aiforge.NewYakForgeBlueprintConfigFromJson(json)
	if err != nil {
		return nil, err
	}
	params := aiforge.Any2ExecParams(i)
	return NewExecutorFromForge(bp, params, opts...)
}

// NewForgeExecutor 基于已注册的 Forge 名称创建执行器（导出名为 aiagent.NewExecutor）
// 参数:
//   - name: Forge 名称
//   - i: 执行参数（map 或键值对）
//   - opts: 可选项，如 aiagent.aiCallback、aiagent.context 等
//
// 返回值:
//   - 协调器对象（可调用 Run 执行）
//   - 错误信息
//
// Example:
// ```
// // 需要配置可用的 AI 服务（示意性示例）
// coordinator = aiagent.NewExecutor("my-forge", {"query": "hello"})~
// dump(coordinator)
// ```
func NewForgeExecutor(name string, i any, opts ...any) (*aid.Coordinator, error) {
	params := aiforge.Any2ExecParams(i)
	ag := NewAgent(opts...)
	bp := NewForgeBlueprint(name, opts...)
	ins, err := bp.CreateCoordinator(context.Background(), params, ag.AICommonOptions()...)
	if err != nil {
		return nil, err
	}
	return ins, nil
}
