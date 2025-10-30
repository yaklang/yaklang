package yak

import (
	"context"
	"github.com/yaklang/yaklang/common/ai/aid"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"slices"

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
	return ag
}

func WithForgeName(forgeName string) AIAgentOption {
	return func(ag *Agent) error {
		ag.ForgeName = forgeName
		return nil
	}
}

func WithContext(ctx context.Context) AIAgentOption {
	return func(ag *Agent) error {
		ag.ctx = ctx
		return nil
	}
}

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
	// aicommon options
	WithAgreeYOLO = aicommon.WithAgreeYOLO

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
	WithOffsetSeq                    = aicommon.WithSequence
	WithTool                         = aicommon.WithTool
	WithExtendedActionCallback       = aicommon.WithExtendedActionCallback
	WithDisallowRequireForUserPrompt = aicommon.WithDisallowRequireForUserPrompt
	WithManualAssistantCallback      = aicommon.WithManualAssistantCallback
	WithAgreePolicy                  = aicommon.WithAgreePolicy
	WithAIAgree                      = aicommon.WithAIAgree
	WithAgreeManual                  = aicommon.WithAgreeManual
	WithAgreeAuto                    = aicommon.WithAgreeAuto
	WithAllowRequireForUserInteract  = aicommon.WithAllowRequireForUserInteract
	WithTools                        = aicommon.WithTools
	WithAICallback                   = aicommon.WithAICallback
	WithToolManager                  = aicommon.WithToolManager
	WithTimeline                     = aicommon.WithTimeline
	WithMemoryTriage                 = aicommon.WithMemoryTriage
	WithMemory                       = aid.WithMemoryProvider
	WithTaskAICallback               = aicommon.WithSpeedPriorityAICallback
	WithCoordinatorAICallback        = aicommon.WithQualityPriorityAICallback
	WithPlanAICallback               = aicommon.WithQualityPriorityAICallback
	WithSystemFileOperator           = aicommon.WithSystemFileOperator
	WithJarOperator                  = aicommon.WithJarOperator
	WithOmniSearchTool               = aicommon.WithOmniSearchTool
	WithAiToolsSearchTool            = aid.WithAiToolsSearchTool
	WithAiForgeSearchTool            = aid.WithAiForgeSearchTool
	WithDebugPrompt                  = aicommon.WithDebugPrompt
	WithEventHandler                 = aicommon.WithEventHandler
	WithEventInputChan               = aicommon.WithEventInputChanx
	WithDebug                        = aicommon.WithDebug
	WithGenerateReport               = aicommon.WithGenerateReport
	WithResultHandler                = aid.WithResultHandler
	WithAppendPersistentMemory       = aicommon.WithAppendPersistentMemory

	// Deprecated: use WithTimelineContentLimit instead
	WithTimeLineLimit        = aicommon.WithTimeLineLimit
	WithTimelineContentLimit = aicommon.WithTimelineContentLimit
	WithPlanMocker           = aid.WithPlanMocker
	WithForgeParams          = aicommon.WithForgeParams
	WithDisableToolUse       = aicommon.WithDisableToolUse
	WithAIAutoRetry          = aicommon.WithAIAutoRetry
	WithAITransactionRetry   = aicommon.WithAITransactionRetry
	WithDisableOutputType    = aicommon.WithDisableOutputEvent

	// aiforge options
	WithForgePlanMocker      = aiforge.WithPlanMocker
	WithInitializePrompt     = aiforge.WithInitializePrompt
	WithResultPrompt         = aiforge.WithResultPrompt
	WithResultHandlerForge   = aiforge.WithResultHandler
	WithPersistentPrompt     = aiforge.WithPersistentPrompt
	WithToolKeywords         = aiforge.WithToolKeywords
	WithForgeTools           = aiforge.WithTools
	WithOriginYaklangCliCode = aiforge.WithOriginYaklangCliCode

	// lite aiforge options
	WithLiteForgePrompt          = aiforge.WithLiteForge_Prompt
	WithLiteForgeOutputSchema    = aiforge.WithLiteForge_OutputSchema
	WithLiteForgeRequireParams   = aiforge.WithLiteForge_RequireParams
	WithLiteForgeOutputMemoryOP  = aiforge.WithLiteForge_OutputMemoryOP
	WithLiteForgeOutputSchemaRaw = aiforge.WithLiteForge_OutputSchemaRaw

	// aitools
	AllYakScriptTools = yakscripttools.GetAllYakScriptAiTools
)

func NewLiteForge(name string, opts ...any) (*aiforge.LiteForge, error) {
	return aiforge.NewLiteForge(name, BuildLiteForgeCreateOption(opts...)...)
}

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
func NewExecutorFromJson(json string, i any, opts ...any) (*aid.Coordinator, error) {
	bp, err := aiforge.NewYakForgeBlueprintConfigFromJson(json)
	if err != nil {
		return nil, err
	}
	params := aiforge.Any2ExecParams(i)
	return NewExecutorFromForge(bp, params, opts...)
}
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
