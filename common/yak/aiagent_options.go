package yak

import (
	"context"
	"github.com/yaklang/yaklang/common/ai/aid/aitool/buildinaitools/yakscripttools"
	"github.com/yaklang/yaklang/common/schema"
	"slices"

	"github.com/yaklang/yaklang/common/ai/aid"
	"github.com/yaklang/yaklang/common/aiforge"
	"github.com/yaklang/yaklang/common/log"
)

type AIAgentOption func(*Agent) error

type Agent struct {
	ForgeName string

	ctx    context.Context
	cancel context.CancelFunc

	CoordinatorId string

	ExtendAIDOptions []aid.Option
	AiForgeOptions   []aiforge.Option

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
		case aid.Option:
			ag.ExtendAIDOptions = append(ag.ExtendAIDOptions, o)
		case aiforge.Option:
			ag.AiForgeOptions = append(ag.AiForgeOptions, o)
		}
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

func WithExtendAIDOptions(opts ...aid.Option) AIAgentOption {
	return func(ag *Agent) error {
		ag.ExtendAIDOptions = append(ag.ExtendAIDOptions, opts...)
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
	if ag.ExtendAIDOptions != nil {
		opts = append(opts, WithExtendAIDOptions(ag.ExtendAIDOptions...))
	}
	return opts
}

var (
	// aid options
	WithAgreeYOLO = aid.WithAgreeYOLO

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
	WithOffsetSeq                    = aid.WithSequence
	WithTool                         = aid.WithTool
	WithExtendedActionCallback       = aid.WithExtendedActionCallback
	WithDisallowRequireForUserPrompt = aid.WithDisallowRequireForUserPrompt
	WithManualAssistantCallback      = aid.WithManualAssistantCallback
	WithAgreePolicy                  = aid.WithAgreePolicy
	WithAIAgree                      = aid.WithAIAgree
	WithAgreeManual                  = aid.WithAgreeManual
	WithAgreeAuto                    = aid.WithAgreeAuto
	WithAllowRequireForUserInteract  = aid.WithAllowRequireForUserInteract
	WithTools                        = aid.WithTools
	WithAICallback                   = aid.WithAICallback
	WithToolManager                  = aid.WithToolManager
	WithMemory                       = aid.WithMemory
	WithTaskAICallback               = aid.WithTaskAICallback
	WithCoordinatorAICallback        = aid.WithCoordinatorAICallback
	WithPlanAICallback               = aid.WithPlanAICallback
	WithSystemFileOperator           = aid.WithSystemFileOperator
	WithJarOperator                  = aid.WithJarOperator
	WithOmniSearchTool               = aid.WithOmniSearchTool
	WithAiToolsSearchTool            = aid.WithAiToolsSearchTool
	WithAiForgeSearchTool            = aid.WithAiForgeSearchTool
	WithDebugPrompt                  = aid.WithDebugPrompt
	WithEventHandler                 = aid.WithEventHandler
	WithEventInputChan               = aid.WithEventInputChan
	WithDebug                        = aid.WithDebug
	WithGenerateReport               = aid.WithGenerateReport
	WithResultHandler                = aid.WithResultHandler
	WithAppendPersistentMemory       = aid.WithAppendPersistentMemory

	// Deprecated: use WithTimelineContentLimit instead
	WithTimeLineLimit        = aid.WithTimeLineLimit
	WithTimelineContentLimit = aid.WithTimelineContentLimit
	WithPlanMocker           = aid.WithPlanMocker
	WithForgeParams          = aid.WithForgeParams
	WithDisableToolUse       = aid.WithDisableToolUse
	WithAIAutoRetry          = aid.WithAIAutoRetry
	WithAITransactionRetry   = aid.WithAITransactionRetry
	WithDisableOutputType    = aid.WithDisableOutputEvent

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
	aiforgeOpts = append(aiforgeOpts, aiforge.WithAIDOptions(ag.AIDOptions()...))
	return aiforge.NewForgeBlueprint(name, aiforgeOpts...)
}
func NewExecutorFromForge(forge *aiforge.ForgeBlueprint, i any, opts ...any) (*aid.Coordinator, error) {
	ag := NewAgent(opts...)
	ag.ForgeName = forge.Name
	params := aiforge.Any2ExecParams(i)
	return forge.CreateCoordinator(context.Background(), params, ag.AIDOptions()...)
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
	ins, err := bp.CreateCoordinator(context.Background(), params, ag.AIDOptions()...)
	if err != nil {
		return nil, err
	}
	return ins, nil
}
