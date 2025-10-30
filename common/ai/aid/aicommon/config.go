package aicommon

import (
	"bytes"
	"context"
	"fmt"
	"github.com/google/uuid"
	"github.com/yaklang/yaklang/common/ai"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/ai/aid/aitool/buildinaitools"
	"github.com/yaklang/yaklang/common/ai/aid/aitool/buildinaitools/fstools"
	"github.com/yaklang/yaklang/common/ai/aid/aitool/buildinaitools/searchtools"
	"github.com/yaklang/yaklang/common/ai/aispec"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/chanx"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"math/rand/v2"
	"os"
	"sync"
	"sync/atomic"
	"time"
)

type ConfigOption func(*Config) error

type Config struct {
	// Embedded structs
	*Emitter
	*BaseCheckpointableStorage
	*KeyValueConfig

	/*
		Basic config
	*/
	// Identity
	Id string

	// async wait
	wg sync.WaitGroup

	// config lock
	m *sync.Mutex

	// Context controller
	Ctx    context.Context
	cancel context.CancelFunc

	// counter
	IdSequence  int64
	IdGenerator func() int64

	// session id
	PersistentSessionId string

	/*
		Event
	*/
	// Input Event Loop
	StartInputEventOnce sync.Once
	EventInputChan      *chanx.UnlimitedChan[*ypb.AIInputEvent]

	InputEventManager *AIInputEventProcessor

	// hotPatch loop
	HotPatchBroadcaster *chanx.Broadcaster[ConfigOption]
	HotPatchOptionChan  *chanx.UnlimitedChan[ConfigOption]
	StartHotPatchOnce   sync.Once

	// output Event Handle
	EventHandler           func(e *schema.AiOutputEvent)
	DisableOutputEventType []string
	SaveEvent              bool

	// asyncGuardian process special output event
	Guardian *AsyncGuardian

	/*
		AI Call
	*/
	// call back
	OriginalAICallback        AICallbackType // 原始 ai 回调, 用于 异步任务，不占用id
	QualityPriorityAICallback AICallbackType // 质量优先 ai 回调
	SpeedPriorityAICallback   AICallbackType // 速度优先 ai 回调

	//aiServiceName
	AiServerName string

	// ai call config
	AiCallTokenLimit       int64
	AiAutoRetry            int64
	AiTransactionAutoRetry int64
	PromptHook             func(string) string

	// ai consumption index
	InputConsumption  *int64
	OutputConsumption *int64
	consumptionUUID   string

	/*
		Prompt Manager
	*/
	TopToolsCount           int // Number of top tools to display in prompt
	AiForgeManager          AIForgeFactory
	PendingContextProviders []ContextProviderEntry

	/*
		AI Tool
	*/
	// tool manager
	AiToolManager *buildinaitools.AiToolManager

	// tool config
	DisableToolUse      bool
	AiToolManagerOption []buildinaitools.ToolManagerOption

	// Interactive(review/require_user/sync) features
	// endpoint manager
	Epm *EndpointManager

	// require_user and review config
	AllowRequireForUserInteract bool
	AgreePolicy                 AgreePolicyType
	AiAgreeRiskControl          RiskControl
	AgreeInterval               time.Duration
	AgreeAIScoreLow             float64 // 0 ~ low ~ mid ~ 1 ; default 0.4
	AgreeAIScoreMiddle          float64 // default 0.7
	AgreeManualCallback         func(context.Context, *Config) (aitool.InvokeParams, error)

	// sync config
	SyncMutex *sync.RWMutex
	SyncMap   map[string]func() any

	/*
		AI Memory :     General Memory = Timeline + Triage + Other Context
	*/
	// timeline
	Timeline                  *Timeline
	TimelineContentSizeLimit  int
	TimelineTotalContentLimit int

	// triage
	MemoryTriage         MemoryTriage
	MemoryPoolSize       int64
	EnableSelfReflection bool

	// other context
	PersistentMemory []string

	/*
		PE Mode special config
	*/
	// Plan manager
	AllowPlanUserInteract    bool
	PlanUserInteractMaxCount int64

	// result processer
	GenerateReport  bool
	MaxTaskContinue int64

	// other
	ResultHandler          func(*Config)
	ExtendedActionCallback map[string]func(config *Config, action *Action)

	/*
		Re-Act Mode special config
	*/
	// Call PE
	EnablePlanAndExec bool // Enable plan and execution action
	HijackPERequest   func(ctx context.Context, planPayload string) error

	// default Task for call tool directly
	DefaultTask AITask

	// iteration limit
	MaxIterationCount int64

	// task config
	EnhanceKnowledgeManager            *EnhanceKnowledgeManager
	DisableEnhanceDirectlyAnswer       bool
	PerTaskUserInteractiveLimitedTimes int64

	/*
		Meta Info
	*/
	Keywords  []string // AI task keywords, maybe tools name, maybe task keywords ,help ai's decision
	ForgeName string   // if current config use in forge , this is the forge name
	Workdir   string
	Language  string // Response language preference

	/*
		debug config
	*/
	DebugPrompt bool
	DebugEvent  bool
}

// NewConfig creates a new Config with options
func NewConfig(ctx context.Context, opts ...ConfigOption) *Config {
	config := newConfig(ctx)

	// Apply options
	for _, opt := range opts {
		opt(config)
	}

	// Initialize tool manager if not set
	if config.AiToolManager == nil {
		config.AiToolManager = buildinaitools.NewToolManager(config.AiToolManagerOption...)
	}

	// Restore persistent session if configured
	config.restorePersistentSession()

	return config
}

// newConfig creates a new Config with default values
func newConfig(ctx context.Context) *Config {
	if ctx == nil {
		ctx = context.Background()
	}

	ctx, cancel := context.WithCancel(ctx)
	id := uuid.New().String()

	// Initialize ID generator
	var idGenerator = new(int64)
	offset := rand.Int64N(3000)

	config := &Config{
		HotPatchBroadcaster: chanx.NewBroadcastChannel[ConfigOption](ctx, 10),
		KeyValueConfig:      NewKeyValueConfig(),
		DefaultTask:         &AIStatefulTaskBase{name: "ai-task"},
		Ctx:                 ctx,
		cancel:              cancel,
		StartInputEventOnce: sync.Once{},
		EventInputChan:      chanx.NewUnlimitedChan[*ypb.AIInputEvent](ctx, 10),
		InputEventManager:   NewAIInputEventProcessor(),
		Id:                  id,
		IdSequence:          atomic.AddInt64(idGenerator, offset), // Start with offset
		IdGenerator: func() int64 {
			return atomic.AddInt64(idGenerator, 1)
		},
		AgreePolicy:                        AgreePolicyManual,
		AgreeAIScoreLow:                    0.4,
		AgreeAIScoreMiddle:                 0.7,
		AiAgreeRiskControl:                 DefaultAIAssistantRiskControl,
		MaxIterationCount:                  100,
		Language:                           "zh", // Default to Chinese
		TopToolsCount:                      100,
		InputConsumption:                   new(int64),
		OutputConsumption:                  new(int64),
		AiAutoRetry:                        5,
		AiTransactionAutoRetry:             5,
		TimelineContentSizeLimit:           50 * 1024, // Default limit for 50k
		Guardian:                           NewAsyncGuardian(ctx, id),
		PerTaskUserInteractiveLimitedTimes: 3, // Default to 3 times
		EnablePlanAndExec:                  true,
		AllowRequireForUserInteract:        true,
		Workdir:                            "",
		MemoryPoolSize:                     10 * 1024,
		MaxTaskContinue:                    3,
	}

	// Initialize emitter
	config.Emitter = NewEmitter(id, func(e *schema.AiOutputEvent) error {
		if config.Guardian != nil {
			config.Guardian.Feed(e)
		}
		config.emitBaseHandler(e)
		return nil
	})

	// Initialize checkpoint storage
	config.BaseCheckpointableStorage = NewCheckpointableStorageWithDB(id, consts.GetGormProjectDatabase())

	// Initialize endpoint manager
	config.Epm = NewEndpointManagerContext(ctx)
	config.Epm.SetConfig(config)
	config.Timeline = NewTimeline(nil, nil)
	config.Timeline.BindConfig(config, config)

	if config.QualityPriorityAICallback == nil && config.SpeedPriorityAICallback == nil && config.OriginalAICallback == nil {
		if config.AiServerName != "" {
			err := config.LoadAIServiceByName(config.AiServerName)
			if err != nil {
				log.Errorf("load ai service failed: %v", err)
			}
		}
	}

	return config
}

/*
	config option
*/

// WithID sets the runtime id for the config.
func WithID(id string) ConfigOption {
	return func(c *Config) error {
		if c.m == nil {
			c.m = &sync.Mutex{}
		}
		c.m.Lock()
		defer c.m.Unlock()
		c.Id = id
		return nil
	}
}

// WithContext sets the context (and optional cancel) for the config.
func WithContext(ctx context.Context) ConfigOption {
	return func(c *Config) error {
		if ctx == nil {
			return utils.Error("context cannot be nil")
		}
		if c.m == nil {
			c.m = &sync.Mutex{}
		}
		c.m.Lock()
		ctx, cancel := context.WithCancel(ctx)
		defer c.m.Unlock()
		c.Ctx = ctx
		c.cancel = cancel
		return nil
	}
}

// WithIdGenerator sets a config option helpers that set common fields on Config (ID, context, callbacks, limits, handlers, memory/plan flags, metadata, debug flags, etc.).
func WithIdGenerator(gen func() int64) ConfigOption {
	return func(c *Config) error {
		if gen == nil {
			return utils.Error("id generator cannot be nil")
		}
		if c.m == nil {
			c.m = &sync.Mutex{}
		}
		c.m.Lock()
		c.IdGenerator = gen
		c.m.Unlock()
		return nil
	}
}

// WithPersistentSessionId sets persistentSessionId.
func WithPersistentSessionId(sid string) ConfigOption {
	return func(c *Config) error {
		if c.m == nil {
			c.m = &sync.Mutex{}
		}
		c.m.Lock()
		c.PersistentSessionId = sid
		c.m.Unlock()
		return nil
	}
}

// Callback setters
func WithAICallback(cb AICallbackType) ConfigOption {
	return func(c *Config) error {
		if c.m == nil {
			c.m = &sync.Mutex{}
		}
		cb = c.wrapper(cb)
		c.m.Lock()
		c.OriginalAICallback = cb
		c.QualityPriorityAICallback = cb
		c.SpeedPriorityAICallback = cb
		c.m.Unlock()
		return nil
	}
}

func WithToolManager(tm *buildinaitools.AiToolManager) ConfigOption {
	return func(c *Config) error {
		if c.m == nil {
			c.m = &sync.Mutex{}
		}
		c.m.Lock()
		c.AiToolManager = tm
		c.m.Unlock()
		return nil
	}
}

func WithOriginalAICallback(cb AICallbackType) ConfigOption {
	return func(c *Config) error {
		if c.m == nil {
			c.m = &sync.Mutex{}
		}
		cb = c.wrapper(cb)
		c.m.Lock()
		c.OriginalAICallback = cb
		c.m.Unlock()
		return nil
	}
}

func WithQualityPriorityAICallback(cb AICallbackType) ConfigOption {
	return func(c *Config) error {
		if c.m == nil {
			c.m = &sync.Mutex{}
		}
		cb = c.wrapper(cb)
		c.m.Lock()
		c.QualityPriorityAICallback = cb
		c.m.Unlock()
		return nil
	}
}

func WithSpeedPriorityAICallback(cb AICallbackType) ConfigOption {
	return func(c *Config) error {
		if c.m == nil {
			c.m = &sync.Mutex{}
		}
		cb = c.wrapper(cb)
		c.m.Lock()
		c.SpeedPriorityAICallback = cb
		c.m.Unlock()
		return nil
	}
}

// AI retry / limits
func WithAITransactionAutoRetry(n int64) ConfigOption {
	return func(c *Config) error {
		if n < 0 {
			return utils.Error("ai transaction auto retry must be >= 0")
		}
		if c.m == nil {
			c.m = &sync.Mutex{}
		}
		c.m.Lock()
		c.AiTransactionAutoRetry = n
		c.m.Unlock()
		return nil
	}
}

func WithAiCallTokenLimit(limit int64) ConfigOption {
	return func(c *Config) error {
		if limit < 0 {
			return utils.Error("ai call token limit must be >= 0")
		}
		if c.m == nil {
			c.m = &sync.Mutex{}
		}
		c.m.Lock()
		c.AiCallTokenLimit = limit
		c.m.Unlock()
		return nil
	}
}

func WithPromptHook(hook func(string) string) ConfigOption {
	return func(c *Config) error {
		if c.m == nil {
			c.m = &sync.Mutex{}
		}
		c.m.Lock()
		c.PromptHook = hook
		c.m.Unlock()
		return nil
	}
}

// Consumption pointers
func WithAIConsumptionPointers(input *int64, output *int64) ConfigOption {
	return func(c *Config) error {
		if c.m == nil {
			c.m = &sync.Mutex{}
		}
		c.m.Lock()
		c.InputConsumption = input
		c.OutputConsumption = output
		c.m.Unlock()
		return nil
	}
}

func WithHotPatchOptionChan(ch *chanx.UnlimitedChan[ConfigOption]) ConfigOption {
	return func(c *Config) error {
		if c.m == nil {
			c.m = &sync.Mutex{}
		}
		c.m.Lock()
		c.HotPatchOptionChan = ch
		c.m.Unlock()
		return nil
	}
}

// Event / output
func WithEventHandler(handler func(e *schema.AiOutputEvent)) ConfigOption {
	return func(c *Config) error {
		if c.m == nil {
			c.m = &sync.Mutex{}
		}
		c.m.Lock()
		c.EventHandler = handler
		c.m.Unlock()
		return nil
	}
}

func WithDisableOutputEventType(types ...string) ConfigOption {
	return func(c *Config) error {
		if c.m == nil {
			c.m = &sync.Mutex{}
		}
		c.m.Lock()
		c.DisableOutputEventType = append([]string{}, types...)
		c.m.Unlock()
		return nil
	}
}

func WithSaveEvent(v bool) ConfigOption {
	return func(c *Config) error {
		if c.m == nil {
			c.m = &sync.Mutex{}
		}
		c.m.Lock()
		c.SaveEvent = v
		c.m.Unlock()
		return nil
	}
}

func WithGuardianEventTrigger(eventTrigger schema.EventType, callback GuardianEventTrigger) ConfigOption {
	return func(config *Config) error {
		config.m.Lock()
		defer config.m.Unlock()

		if config.Guardian == nil {
			return utils.Error("BUG: guardian cannot be empty (ASYNC Guardian)")
		}
		return config.Guardian.RegisterEventTrigger(eventTrigger, callback)
	}
}

func WithGuardianMirrorStreamMirror(streamName string, callback GuardianMirrorStreamTrigger) ConfigOption {
	return func(config *Config) error {
		config.m.Lock()
		defer config.m.Unlock()

		if config.Guardian == nil {
			return utils.Error("BUG: guardian cannot be empty (ASYNC Guardian)")
		}
		return config.Guardian.RegisterMirrorStreamTrigger(streamName, callback)
	}
}

// Prompt/tool/blueprint related
func WithTopToolsCount(n int) ConfigOption {
	return func(c *Config) error {
		if n < 0 {
			return utils.Error("top tools count must be >= 0")
		}
		if c.m == nil {
			c.m = &sync.Mutex{}
		}
		c.m.Lock()
		c.TopToolsCount = n
		c.m.Unlock()
		return nil
	}
}

func WithAIBlueprintManager(factory AIForgeFactory) ConfigOption {
	return func(c *Config) error {
		if c.m == nil {
			c.m = &sync.Mutex{}
		}
		c.m.Lock()
		c.AiForgeManager = factory
		c.m.Unlock()
		return nil
	}
}

func WithDynamicContextProvider(name string, provider ContextProvider) ConfigOption {
	return func(c *Config) error {
		if name != "" {
			c.PendingContextProviders = append(c.PendingContextProviders, ContextProviderEntry{
				Traced:   false,
				Name:     name,
				Provider: provider,
			})
		}
		return nil
	}
}

// WithTracedDynamicContextProvider registers a dynamic context provider with tracing capabilities
// It tracks changes between calls and provides diff information
func WithTracedDynamicContextProvider(name string, provider ContextProvider) ConfigOption {
	return func(c *Config) error {
		if name != "" {
			c.PendingContextProviders = append(c.PendingContextProviders, ContextProviderEntry{
				Traced:   true,
				Name:     name,
				Provider: provider,
			})
		}
		return nil
	}
}

// WithTracedFileContext monitors a file and provides its content as context with change tracking
func WithTracedFileContext(name string, filePath string) ConfigOption {
	return func(c *Config) error {
		if name != "" && filePath != "" {
			provider := func(config AICallerConfigIf, emitter *Emitter, key string) (string, error) {
				contentBytes, err := os.ReadFile(filePath)
				if err != nil {
					return "", fmt.Errorf("failed to read file %s: %w", filePath, err)
				}
				content := string(contentBytes)
				return fmt.Sprintf("File: %s\nContent:\n%s", filePath, content), nil
			}

			c.PendingContextProviders = append(c.PendingContextProviders, ContextProviderEntry{
				Traced:   true,
				Name:     name,
				Provider: provider,
			})
		}
		return nil
	}
}

func WithAiToolManagerOptions(opts ...buildinaitools.ToolManagerOption) ConfigOption {
	return func(c *Config) error {
		if c.m == nil {
			c.m = &sync.Mutex{}
		}
		c.m.Lock()
		c.AiToolManagerOption = append([]buildinaitools.ToolManagerOption{}, opts...)
		c.m.Unlock()
		return nil
	}
}

func WithDisableToolUse(disable bool) ConfigOption {
	return func(c *Config) error {
		if c.m == nil {
			c.m = &sync.Mutex{}
		}
		c.m.Lock()
		c.DisableToolUse = disable
		c.m.Unlock()
		return nil
	}
}

func WithJarOperator() ConfigOption {
	return func(c *Config) error {
		tools, err := fstools.CreateJarOperator()
		if err != nil {
			return utils.Errorf("create jar operator tools: %v", err)
		}
		return WithTools(tools...)(c)
	}
}

func WithOmniSearchTool() ConfigOption {
	return func(c *Config) error {
		tools, err := searchtools.CreateOmniSearchTools()
		if err != nil {
			return utils.Errorf("create omnisearch tools: %v", err)
		}
		return WithTools(tools...)(c)
	}
}

func WithQwenNoThink() ConfigOption {
	return WithPromptHook(func(origin string) string {
		return origin + "/nothink"
	})
}

// Interactive / review / require_user
func WithAllowRequireForUserInteract(v bool) ConfigOption {
	return func(c *Config) error {
		if c.m == nil {
			c.m = &sync.Mutex{}
		}
		c.m.Lock()
		c.AllowRequireForUserInteract = v
		c.m.Unlock()
		return nil
	}
}

func WithAgreePolicy(p AgreePolicyType) ConfigOption {
	return func(c *Config) error {
		if c.m == nil {
			c.m = &sync.Mutex{}
		}
		c.m.Lock()
		c.AgreePolicy = p
		c.m.Unlock()
		return nil
	}
}

func WithAIAgree() ConfigOption {
	return func(c *Config) error {
		c.m.Lock()
		defer c.m.Unlock()
		c.AgreePolicy = AgreePolicyAI
		return nil
	}
}

func WithAgreeManual() ConfigOption {
	return func(c *Config) error {
		c.m.Lock()
		defer c.m.Unlock()
		c.AgreePolicy = AgreePolicyManual
		return nil
	}
}

func WithAgreeAuto() ConfigOption {
	return func(c *Config) error {
		c.m.Lock()
		defer c.m.Unlock()
		c.AgreePolicy = AgreePolicyAuto
		return nil
	}
}

func WithAiAgreeRiskControl(rc RiskControl) ConfigOption {
	return func(c *Config) error {
		if c.m == nil {
			c.m = &sync.Mutex{}
		}
		c.m.Lock()
		c.AiAgreeRiskControl = rc
		c.m.Unlock()
		return nil
	}
}

func WithAgreeInterval(d time.Duration) ConfigOption {
	return func(c *Config) error {
		if d < 0 {
			return utils.Error("agree interval cannot be negative")
		}
		if c.m == nil {
			c.m = &sync.Mutex{}
		}
		c.m.Lock()
		c.AgreeInterval = d
		c.m.Unlock()
		return nil
	}
}

func WithAgreeAIScoreLowMid(low, mid float64) ConfigOption {
	return func(c *Config) error {
		if low < 0 || low > 1 || mid < 0 || mid > 1 {
			return utils.Error("agree scores must be in [0,1]")
		}
		if c.m == nil {
			c.m = &sync.Mutex{}
		}
		c.m.Lock()
		c.AgreeAIScoreLow = low
		c.AgreeAIScoreMiddle = mid
		c.m.Unlock()
		return nil
	}
}

func WithAgreeManualCallback(cb func(context.Context, *Config) (aitool.InvokeParams, error)) ConfigOption {
	return func(c *Config) error {
		if c.m == nil {
			c.m = &sync.Mutex{}
		}
		c.m.Lock()
		c.AgreeManualCallback = cb
		c.m.Unlock()
		return nil
	}
}

// Memory / timeline
func WithMemoryLimits(contentSizeLimit, totalContentLimit int) ConfigOption {
	return func(c *Config) error {
		if contentSizeLimit < 0 || totalContentLimit < 0 {
			return utils.Error("memory limits cannot be negative")
		}
		if c.m == nil {
			c.m = &sync.Mutex{}
		}
		c.m.Lock()
		c.TimelineContentSizeLimit = contentSizeLimit
		c.TimelineTotalContentLimit = totalContentLimit
		c.m.Unlock()
		return nil
	}
}

func WithPersistentMemory(keys ...string) ConfigOption {
	return func(c *Config) error {
		if c.m == nil {
			c.m = &sync.Mutex{}
		}
		c.m.Lock()
		c.PersistentMemory = append([]string{}, keys...)
		c.m.Unlock()
		return nil
	}
}

func WithAllowPlanUserInteract(v bool) ConfigOption {
	return func(c *Config) error {
		if c.m == nil {
			c.m = &sync.Mutex{}
		}
		c.m.Lock()
		c.AllowPlanUserInteract = v
		c.m.Unlock()
		return nil
	}
}

func WithDisableToolsName(toolsName ...string) ConfigOption {
	return func(c *Config) error {
		c.m.Lock()
		defer c.m.Unlock()
		if c.AiToolManagerOption == nil {
			c.AiToolManagerOption = make([]buildinaitools.ToolManagerOption, 0)
		}
		c.AiToolManagerOption = append(c.AiToolManagerOption, buildinaitools.WithDisableTools(toolsName))
		return nil
	}
}

func WithAiToolsManagerOption(opt ...buildinaitools.ToolManagerOption) ConfigOption {
	return func(c *Config) error {
		c.m.Lock()
		defer c.m.Unlock()
		if c.AiToolManagerOption == nil {
			c.AiToolManagerOption = make([]buildinaitools.ToolManagerOption, 0)
		}
		c.AiToolManagerOption = append(c.AiToolManagerOption, opt...)
		return nil
	}
}

func WithEnableToolsName(toolsName ...string) ConfigOption {
	return func(c *Config) error {
		c.m.Lock()
		defer c.m.Unlock()
		if c.AiToolManagerOption == nil {
			c.AiToolManagerOption = make([]buildinaitools.ToolManagerOption, 0)
		}
		c.AiToolManagerOption = append(c.AiToolManagerOption, buildinaitools.WithEnabledTools(toolsName))
		return nil
	}
}

func WithPlanUserInteractMaxCount(i int64) ConfigOption {
	return func(c *Config) error {
		c.m.Lock()
		defer c.m.Unlock()

		if i <= 0 {
			i = 3
		}
		c.PlanUserInteractMaxCount = i
		return nil
	}
}

func WithSystemFileOperator() ConfigOption {
	return func(config *Config) error {
		tools, err := fstools.CreateSystemFSTools()
		if err != nil {
			return utils.Errorf("create system fs tools: %v", err)
		}
		return WithTools(tools...)(config)
	}
}

func WithTools(tool ...*aitool.Tool) ConfigOption {
	return func(c *Config) error {
		if c.m == nil {
			c.m = &sync.Mutex{}
		}
		c.m.Lock()
		defer c.m.Unlock()
		if c.AiToolManagerOption == nil {
			c.AiToolManagerOption = make([]buildinaitools.ToolManagerOption, 0)
		}
		c.AiToolManagerOption = append(c.AiToolManagerOption,
			buildinaitools.WithExtendTools(tool, true))
		return nil
	}
}

func WithMemoryTriage(mt MemoryTriage) ConfigOption {
	return func(c *Config) error {
		if c.m == nil {
			c.m = &sync.Mutex{}
		}
		c.m.Lock()
		c.MemoryTriage = mt
		c.m.Unlock()
		return nil
	}
}

func WithMemoryPoolSize(sz int64) ConfigOption {
	return func(c *Config) error {
		if sz < 0 {
			return utils.Error("memory pool size cannot be negative")
		}
		if c.m == nil {
			c.m = &sync.Mutex{}
		}
		c.m.Lock()
		c.MemoryPoolSize = sz
		c.m.Unlock()
		return nil
	}
}

func WithEnableSelfReflection(v bool) ConfigOption {
	return func(c *Config) error {
		if c.m == nil {
			c.m = &sync.Mutex{}
		}
		c.m.Lock()
		c.EnableSelfReflection = v
		c.m.Unlock()
		return nil
	}
}

func WithEnablePlanAndExec(enable bool) ConfigOption {
	return func(c *Config) error {
		if c.m == nil {
			c.m = &sync.Mutex{}
		}
		c.m.Lock()
		c.EnablePlanAndExec = enable
		c.m.Unlock()
		return nil
	}
}

func WithHijackPERequest(fn func(ctx context.Context, planPayload string) error) ConfigOption {
	return func(c *Config) error {
		if c.m == nil {
			c.m = &sync.Mutex{}
		}
		c.m.Lock()
		c.HijackPERequest = fn
		c.m.Unlock()
		return nil
	}
}

func WithDefaultTask(t AITask) ConfigOption {
	return func(c *Config) error {
		if c.m == nil {
			c.m = &sync.Mutex{}
		}
		c.m.Lock()
		c.DefaultTask = t
		c.m.Unlock()
		return nil
	}
}

// Misc / meta
func WithMaxIterationCount(n int64) ConfigOption {
	return func(c *Config) error {
		if n < 0 {
			return utils.Error("max iteration count must be >= 0")
		}
		if c.m == nil {
			c.m = &sync.Mutex{}
		}
		c.m.Lock()
		c.MaxIterationCount = n
		c.m.Unlock()
		return nil
	}
}

func WithEnhanceKnowledgeManager(m *EnhanceKnowledgeManager) ConfigOption {
	return func(c *Config) error {
		if c.m == nil {
			c.m = &sync.Mutex{}
		}
		c.m.Lock()
		c.EnhanceKnowledgeManager = m
		c.m.Unlock()
		return nil
	}
}

// WithAICallback sets the AI callback for LLM interactions
func WithAIServiceName(name string) ConfigOption {
	return func(cfg *Config) error {
		if cfg.m == nil {
			cfg.m = &sync.Mutex{}
		}
		cfg.m.Lock()
		defer cfg.m.Unlock()
		cfg.AiServerName = name
		return nil
	}
}

func WithDisableEnhanceDirectlyAnswer(disable bool) ConfigOption {
	return func(c *Config) error {
		if c.m == nil {
			c.m = &sync.Mutex{}
		}
		c.m.Lock()
		c.DisableEnhanceDirectlyAnswer = disable
		c.m.Unlock()
		return nil
	}
}

func WithPerTaskUserInteractiveLimitedTimes(n int64) ConfigOption {
	return func(c *Config) error {
		if n < 0 {
			n = 3
		}
		if c.m == nil {
			c.m = &sync.Mutex{}
		}
		c.m.Lock()
		c.PerTaskUserInteractiveLimitedTimes = n
		c.m.Unlock()
		return nil
	}
}

func WithKeywords(keys ...string) ConfigOption {
	return func(c *Config) error {
		if c.m == nil {
			c.m = &sync.Mutex{}
		}
		c.m.Lock()
		c.Keywords = append([]string{}, keys...)
		c.m.Unlock()
		return nil
	}
}

func WithForgeName(name string) ConfigOption {
	return func(c *Config) error {
		if c.m == nil {
			c.m = &sync.Mutex{}
		}
		c.m.Lock()
		c.ForgeName = name
		c.m.Unlock()
		return nil
	}
}

func WithWorkdir(dir string) ConfigOption {
	return func(c *Config) error {
		if c.m == nil {
			c.m = &sync.Mutex{}
		}
		c.m.Lock()
		c.Workdir = dir
		c.m.Unlock()
		return nil
	}
}

func WithLanguage(lang string) ConfigOption {
	return func(c *Config) error {
		if c.m == nil {
			c.m = &sync.Mutex{}
		}
		c.m.Lock()
		c.Language = lang
		c.m.Unlock()
		return nil
	}
}

// Debug flags
func WithDebugPrompt(v ...bool) ConfigOption {
	return func(c *Config) error {
		if c.m == nil {
			c.m = &sync.Mutex{}
		}
		if len(v) == 0 {
			v = []bool{true}
		}
		b := v[0]
		c.m.Lock()
		c.DebugPrompt = b
		c.m.Unlock()
		return nil
	}
}

func WithDebugEvent(v bool) ConfigOption {
	return func(c *Config) error {
		if c.m == nil {
			c.m = &sync.Mutex{}
		}
		c.m.Lock()
		c.DebugEvent = v
		c.m.Unlock()
		return nil
	}
}

func WithAgreeYOLO() ConfigOption {
	return WithAgreePolicy(AgreePolicyYOLO)
}

// Add new config option helpers to match aid options used elsewhere.

// WithSequence sets the starting sequence/id and installs a simple id generator that increments it.
func WithSequence(seq int64) ConfigOption {
	return func(c *Config) error {
		if c.m == nil {
			c.m = &sync.Mutex{}
		}
		c.m.Lock()
		defer c.m.Unlock()
		atomic.StoreInt64(&c.IdSequence, seq)
		c.IdGenerator = func() int64 {
			return atomic.AddInt64(&c.IdSequence, 1)
		}
		return nil
	}
}

func WithAIKBPath(path string) ConfigOption {
	return func(c *Config) error {
		c.SetConfig("aikb_path", path)
		return nil
	}
}

func WithAIKBResultMaxSize(maxSize int64) ConfigOption {
	return func(opt *Config) error {
		if maxSize <= 0 {
			maxSize = 20 * 1024 // Default 20KB
		}
		// Hard limit: cannot exceed 20KB
		if maxSize > 20*1024 {
			log.Warnf("aikb result max size %d exceeds hard limit 20KB, setting to 20KB", maxSize)
			maxSize = 20 * 1024
		}
		opt.SetConfig("aikb_result_max_size", int64(maxSize))
		return nil
	}
}

// WithTool is a convenience wrapper to add a single tool (delegates to WithTools).
func WithTool(tool *aitool.Tool) ConfigOption {
	return func(c *Config) error {
		return WithTools(tool)(c)
	}
}

// WithExtendedActionCallback sets the ExtendedActionCallback map.
func WithExtendedActionCallback(name string, callback func(config *Config, action *Action)) ConfigOption {
	return func(c *Config) error {
		if c.m == nil {
			c.m = &sync.Mutex{}
		}
		c.m.Lock()
		if c.ExtendedActionCallback == nil {
			c.ExtendedActionCallback = make(map[string]func(config *Config, action *Action))
		}
		c.ExtendedActionCallback[name] = callback
		c.m.Unlock()
		return nil
	}
}

// WithDisallowRequireForUserPrompt disables require-for-user-interact.
func WithDisallowRequireForUserPrompt() ConfigOption {
	return WithAllowRequireForUserInteract(false)
}

// WithManualAssistantCallback is an alias to the agree/manual callback setter.
func WithManualAssistantCallback(cb func(context.Context, *Config) (aitool.InvokeParams, error)) ConfigOption {
	return WithAgreeManualCallback(cb)
}

// WithEventInputChan sets a custom event input channel.
func WithEventInputChanx(ch *chanx.UnlimitedChan[*ypb.AIInputEvent]) ConfigOption {
	return func(c *Config) error {
		if ch == nil {
			return utils.Error("event input chan cannot be nil")
		}
		if c.m == nil {
			c.m = &sync.Mutex{}
		}
		c.m.Lock()
		c.EventInputChan = ch
		c.m.Unlock()
		return nil
	}
}

// WithDebug toggles both prompt and event debug flags.
func WithDebug(v bool) ConfigOption {
	return func(c *Config) error {
		if c.m == nil {
			c.m = &sync.Mutex{}
		}
		c.m.Lock()
		c.DebugPrompt = v
		c.DebugEvent = v
		c.m.Unlock()
		return nil
	}
}

// WithGenerateReport toggles GenerateReport.
func WithGenerateReport(v bool) ConfigOption {
	return func(c *Config) error {
		if c.m == nil {
			c.m = &sync.Mutex{}
		}
		c.m.Lock()
		c.GenerateReport = v
		c.m.Unlock()
		return nil
	}
}

func WithMaxTaskContinue(n int64) ConfigOption {
	return func(c *Config) error {
		if n < 0 {
			return nil
		}
		if c.m == nil {
			c.m = &sync.Mutex{}
		}
		c.m.Lock()
		c.MaxTaskContinue = n
		c.m.Unlock()
		return nil
	}
}

// WithResultHandler sets the result handler callback.
func WithResultHandler(fn func(*Config)) ConfigOption {
	return func(c *Config) error {
		if fn == nil {
			return nil
		}
		if c.m == nil {
			c.m = &sync.Mutex{}
		}
		c.m.Lock()
		c.ResultHandler = fn
		c.m.Unlock()
		return nil
	}
}

// WithAppendPersistentMemory appends keys to PersistentMemory.
func WithAppendPersistentMemory(keys ...string) ConfigOption {
	return func(c *Config) error {
		if len(keys) == 0 {
			return nil
		}
		if c.m == nil {
			c.m = &sync.Mutex{}
		}
		c.m.Lock()
		c.PersistentMemory = append(c.PersistentMemory, keys...)
		c.m.Unlock()
		return nil
	}
}

// WithAIAutoRetry sets AiAutoRetry count.
func WithAIAutoRetry(n int64) ConfigOption {
	return func(c *Config) error {
		if n < 0 {
			return utils.Error("ai auto retry must be >= 0")
		}
		if c.m == nil {
			c.m = &sync.Mutex{}
		}
		c.m.Lock()
		c.AiAutoRetry = n
		c.m.Unlock()
		return nil
	}
}

// WithAITransactionRetry alias to existing WithAITransactionAutoRetry for naming compatibility.
func WithAITransactionRetry(n int64) ConfigOption {
	return WithAITransactionAutoRetry(n)
}

// WithDisableOutputEvent is a name-compatible wrapper for disabling output event types.
func WithDisableOutputEvent(types ...string) ConfigOption {
	return WithDisableOutputEventType(types...)
}

// WithTimeLineLimit sets the timeline content size limit (deprecated name, kept for compatibility).
func WithTimeLineLimit(limit int) ConfigOption {
	return func(c *Config) error {
		if limit < 0 {
			return utils.Error("timeline limit cannot be negative")
		}
		if c.m == nil {
			c.m = &sync.Mutex{}
		}
		c.m.Lock()
		c.TimelineContentSizeLimit = limit
		c.m.Unlock()
		return nil
	}
}

func WithTimeline(t *Timeline) ConfigOption {
	return func(c *Config) error {
		c.m.Lock()
		defer c.m.Unlock()
		c.Timeline = t
		return nil
	}
}

// WithTimelineContentLimit sets timeline content size limit (keeps naming parity).
func WithTimelineContentLimit(limit int) ConfigOption {
	return WithTimeLineLimit(limit)
}

func WithForgeParams(i any) ConfigOption {
	return func(c *Config) error {
		var buf bytes.Buffer
		nonce := utils.RandStringBytes(8)
		buf.WriteString("<user_input_" + nonce + ">\n")
		buf.WriteString(aispec.ShrinkAndSafeToFile(i))
		buf.WriteString("\n</user_input_" + nonce + ">\n")
		return WithPersistentMemory(buf.String())(c)
	}
}

func WithEventInputChan(ch chan *ypb.AIInputEvent) ConfigOption {
	return func(c *Config) error {
		if ch != nil {
			go func() {
				for event := range ch {
					if c.DebugEvent {
						log.Debugf("Received event: %s", event)
					}
					c.EventInputChan.SafeFeed(event)
				}
			}()
		}
		return nil
	}
}

/*
	implement methods
*/

func (c *Config) CallAI(request *AIRequest) (*AIResponse, error) {
	for _, cb := range []AICallbackType{
		c.QualityPriorityAICallback,
		c.SpeedPriorityAICallback,
		c.OriginalAICallback,
	} {
		if cb == nil {
			continue
		}
		return cb(c, request)
	}
	return nil, utils.Error("no any ai callback is set, cannot found ai config")
}

func (c *Config) Feed(endpointId string, params aitool.InvokeParams) {
	if c.Epm != nil {
		c.Epm.Feed(endpointId, params)
	}
}

func (c *Config) GetEndpointManager() *EndpointManager {
	if c.Epm == nil {
		// lazily create endpoint manager and bind to this config
		c.Epm = NewEndpointManagerContext(c.Ctx)
		c.Epm.SetConfig(c)
	}
	return c.Epm
}

func (c *Config) CallAfterInteractiveEventReleased(s string, params aitool.InvokeParams) {
	//c.memory.StoreInteractiveUserInput(eventID, invoke)
}

func (c *Config) CallAfterReview(seq int64, reviewQuestion string, userInput aitool.InvokeParams) {
	if c.Timeline != nil {
		c.Timeline.PushUserInteraction(UserInteractionStage_Review, seq, reviewQuestion, string(utils.Jsonify(userInput)))
	}
}

func (c *Config) AcquireId() int64 {
	if c.IdGenerator == nil {
		// fallback: create a simple generator if missing
		var gen = rand.Int64N(3000)
		c.IdGenerator = func() int64 {
			return atomic.AddInt64(&gen, 1)
		}
	}
	return c.IdGenerator()
}

func (c *Config) GetRuntimeId() string {
	return c.Id
}

func (c *Config) IsCtxDone() bool {
	select {
	case <-c.Ctx.Done():
		return true
	default:
		return false
	}
}

func (c *Config) GetContext() context.Context {
	return c.Ctx
}

func (c *Config) CallAIResponseConsumptionCallback(i int) {
	if c.OutputConsumption == nil {
		return
	}
	atomic.AddInt64(c.OutputConsumption, int64(i))
}

func (c *Config) GetAITransactionAutoRetryCount() int64 {
	return c.AiTransactionAutoRetry
}

func (c *Config) GetTimelineContentSizeLimit() int64 {
	return int64(c.TimelineContentSizeLimit)
}

func (c *Config) GetUserInteractiveLimitedTimes() int64 {
	if c.PerTaskUserInteractiveLimitedTimes <= 0 {
		return 3
	}
	return c.PerTaskUserInteractiveLimitedTimes
}

func (c *Config) GetMaxIterationCount() int64 {
	return c.MaxIterationCount
}

func (c *Config) GetAllowUserInteraction() bool {
	return c.AllowRequireForUserInteract
}

func (c *Config) RetryPromptBuilder(s string, err error) string {
	if err == nil {
		return s
	}
	return s + "\n\n[Retry due to error: " + err.Error() + "]"
}

func (c *Config) GetEmitter() *Emitter {
	return c.Emitter
}

func (c *Config) NewAIResponse() *AIResponse {
	return NewAIResponse(c)
}

func (c *Config) CallAIResponseOutputFinishedCallback(s string) {
	// Minimal hook: no-op. Implementers can override by setting callbacks or using emitter.
	_ = s
}

func (c *Config) emitBaseHandler(e *schema.AiOutputEvent) {
	select {
	case <-c.Ctx.Done():
		return
	default:
	}
	if c.m != nil {
		c.m.Lock()
		defer c.m.Unlock()
	}

	if e.ShouldSave() {
		err := yakit.CreateAIEvent(consts.GetGormProjectDatabase(), e)
		if err != nil {
			log.Errorf("create AI event failed: %v", err)
		}
	}

	if c.Guardian != nil {
		c.Guardian.Feed(e)
	}

	if c.EventHandler == nil {
		if e.IsStream {
			if c.DebugEvent {
				fmt.Print(string(e.StreamDelta))
			}
			return
		}

		if e.Type == schema.EVENT_TYPE_CONSUMPTION {
			if c.DebugEvent {
				log.Info(e.String())
			}
			return
		}
		if c.DebugEvent {
			log.Info(e.String())
		} else {
			//log.Info(utils.ShrinkString(e.String(), 200))
		}
		return
	}
	c.EventHandler(e)
}

// restorePersistentSession attempts to restore the timeline instance from a persistent session
func (c *Config) restorePersistentSession() {
	if c.PersistentSessionId == "" {
		return
	}

	runtime, err := yakit.GetLatestAIAgentRuntimeByPersistentSession(c.GetDB(), c.PersistentSessionId)
	if err != nil {
		log.Warnf("failed to fetch AI runtime for session [%s]: %v", c.PersistentSessionId, err)
		return
	}

	if runtime == nil {
		log.Debugf("no runtime found for persistent session [%s]", c.PersistentSessionId)
		return
	}

	timelineInstance, err := UnmarshalTimeline(runtime.GetTimeline())
	if err != nil {
		log.Errorf("failed to unmarshal timeline for session [%s]: %v", c.PersistentSessionId, err)
		return
	}

	// Bind config first so timeline can access it
	timelineInstance.BindConfig(c, c)
	if !timelineInstance.Valid() {
		log.Errorf("restored timeline instance is invalid for session [%s]", c.PersistentSessionId)
		return
	}

	// Reassign IDs to all restored timeline items to avoid ID conflicts
	// This uses the current idGenerator to ensure sequential IDs
	lastID := timelineInstance.ReassignIDs(c.IdGenerator)
	if lastID > 0 {
		log.Infof("reassigned timeline IDs, last assigned ID: %d", lastID)
		// Update idSequence to continue from the last assigned ID
		atomic.StoreInt64(&c.IdSequence, lastID)
	}

	c.Timeline = timelineInstance
	log.Infof("successfully restored timeline instance from persistent session [%s] with %d items",
		c.PersistentSessionId, timelineInstance.GetIdToTimelineItem().Len())
}

func (c *Config) Add(i int) {
	c.wg.Add(1)
}

func (c *Config) Done() {
	c.wg.Done()
}

func (c *Config) Wait() {
	c.wg.Wait()
}

func (c *Config) SetAICallback(callback AICallbackType) {
	if c.m == nil {
		c.m = &sync.Mutex{}
	}
	wCb := c.wrapper(callback)
	c.m.Lock()
	defer c.m.Unlock()
	c.OriginalAICallback = callback
	c.QualityPriorityAICallback = wCb
	c.SpeedPriorityAICallback = wCb
}

func (c *Config) CallAiTransaction(
	prompt string,
	callAi func(*AIRequest) (*AIResponse, error),
	postHandler func(rsp *AIResponse) error,
	requestOpts ...AIRequestOption,
) error {
	return CallAITransaction(c, prompt, callAi, postHandler, requestOpts...)
}

func ConvertConfigToOptions(i *Config) []ConfigOption {
	// Return nil for nil input
	if i == nil {
		return nil
	}

	opts := make([]ConfigOption, 0)

	// aiCallback
	if i.AiServerName != "" {
		opts = append(opts, WithAIServiceName(i.AiServerName))
	}

	// Keywords
	if len(i.Keywords) > 0 {
		opts = append(opts, WithKeywords(i.Keywords...))
	}

	// Disable tool use flag
	opts = append(opts, WithDisableToolUse(i.DisableToolUse))

	// Tool manager options
	if len(i.AiToolManagerOption) > 0 {
		opts = append(opts, WithAiToolManagerOptions(i.AiToolManagerOption...))
	}

	// Agree policy mapping
	switch i.AgreePolicy {
	case AgreePolicyYOLO:
		opts = append(opts, WithAgreeYOLO())
	case AgreePolicyAI:
		opts = append(opts, WithAIAgree())
	case AgreePolicyAuto:
		opts = append(opts, WithAgreeAuto())
	case AgreePolicyManual:
		fallthrough
	default:
		opts = append(opts, WithAgreeManual())
	}

	// Other boolean/flag options
	opts = append(opts, WithAllowPlanUserInteract(i.AllowPlanUserInteract))
	opts = append(opts, WithEnablePlanAndExec(i.EnablePlanAndExec))
	if i.GenerateReport {
		opts = append(opts, WithGenerateReport(true))
	}

	// Retry / limits
	if i.AiTransactionAutoRetry > 0 {
		opts = append(opts, WithAITransactionRetry(i.AiTransactionAutoRetry))
	}
	if i.AiAutoRetry > 0 {
		opts = append(opts, WithAIAutoRetry(i.AiAutoRetry))
	}
	if i.AiCallTokenLimit > 0 {
		opts = append(opts, WithAiCallTokenLimit(i.AiCallTokenLimit))
	}
	if i.MaxIterationCount > 0 {
		opts = append(opts, WithMaxIterationCount(i.MaxIterationCount))
	}
	if i.MaxTaskContinue > 0 {
		opts = append(opts, WithMaxTaskContinue(i.MaxTaskContinue))
	}
	if i.PerTaskUserInteractiveLimitedTimes > 0 {
		opts = append(opts, WithPerTaskUserInteractiveLimitedTimes(i.PerTaskUserInteractiveLimitedTimes))
	}

	// Timeline / memory limits
	if i.TimelineContentSizeLimit > 0 {
		opts = append(opts, WithTimelineContentLimit(i.TimelineContentSizeLimit))
	}

	if i.Timeline != nil {
		opts = append(opts, WithTimeline(i.Timeline))
	}
	if i.MemoryPoolSize > 0 {
		opts = append(opts, WithMemoryPoolSize(i.MemoryPoolSize))
	}
	if i.MemoryTriage != nil {
		opts = append(opts, WithMemoryTriage(i.MemoryTriage))
	}

	// Misc
	if i.PromptHook != nil {
		opts = append(opts, WithPromptHook(i.PromptHook))
	}
	if i.Language != "" {
		opts = append(opts, WithLanguage(i.Language))
	}
	if i.Workdir != "" {
		opts = append(opts, WithWorkdir(i.Workdir))
	}

	return opts
}

func (c *Config) LoadAIServiceByName(name string) error {
	chat, err := ai.LoadChater(name)
	if err != nil {
		return err
	}
	// update react config
	c.SetAICallback(AIChatToAICallbackType(chat))
	c.AiServerName = name

	// submit hotpatch options
	c.HotPatchBroadcaster.Submit(WithAIServiceName(c.AiServerName))
	c.HotPatchBroadcaster.Submit(WithAICallback(c.OriginalAICallback))
	return nil
}
