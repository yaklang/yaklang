package aicommon

import (
	"context"
	"fmt"
	"math/rand/v2"
	"reflect"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/yaklang/yaklang/common/utils/omap"

	"github.com/google/uuid"
	"github.com/yaklang/yaklang/common/ai"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon/aiskillloader"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/ai/aid/aitool/buildinaitools"
	"github.com/yaklang/yaklang/common/ai/aid/aitool/buildinaitools/fstools"
	"github.com/yaklang/yaklang/common/ai/aispec"
	"github.com/yaklang/yaklang/common/ai/ytoken"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/chanx"
	fi "github.com/yaklang/yaklang/common/utils/filesys/filesys_interface"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

const DefaultPeriodicVerificationInterval = 5

type ConfigOption func(*Config) error

var configOptionIDRegistry sync.Map

const ConfigKeyToolCallIntervalReviewExtraPrompt = "tool_call_interval_review_extra_prompt"

type configIDOptionMeta struct {
	id  string
	key uintptr
}

func (o *configIDOptionMeta) apply(c *Config) error {
	if c.m == nil {
		c.m = &sync.Mutex{}
	}
	c.m.Lock()
	defer c.m.Unlock()
	c.Id = o.id
	// Also update Emitter's id to keep them in sync
	if c.Emitter != nil {
		c.Emitter.SetId(o.id)
	}
	return nil
}

// GetLastIDFromConfigOptions returns the final id configured by WithID in opts.
// It scans from the end and stops on the first matching WithID option.
func GetLastIDFromConfigOptions(opts ...ConfigOption) (string, bool) {
	for i := len(opts) - 1; i >= 0; i-- {
		opt := opts[i]
		if opt == nil {
			continue
		}
		if id, ok := configOptionIDRegistry.Load(reflect.ValueOf(opt).Pointer()); ok {
			return id.(string), true
		}
	}
	return "", false
}

type ConfigInitStatus struct {
	PersistentSessionRestored *utils.AtomicBool
	ConsumptionState          *ConfigConsumptionState
	m                         *sync.Mutex
}

func NewConfigInitStatus() *ConfigInitStatus {
	return &ConfigInitStatus{
		PersistentSessionRestored: utils.NewAtomicBool(),
		ConsumptionState:          NewConfigConsumptionState(),
		m:                         &sync.Mutex{},
	}
}

func (cis *ConfigInitStatus) String() string {
	return fmt.Sprintf(
		"PersistentSessionRestored: %v, ConsumptionTracked: %v",
		cis.PersistentSessionRestored.IsSet(),
		cis.GetOrCreateConsumptionState() != nil,
	)
}

func (cis *ConfigInitStatus) SetPersistentSessionRestored(value bool) {
	cis.PersistentSessionRestored.SetTo(value)
}

func (cis *ConfigInitStatus) IsPersistentSessionRestored() bool {
	return cis.PersistentSessionRestored.IsSet()
}

func (cis *ConfigInitStatus) GetOrCreateConsumptionState() *ConfigConsumptionState {
	if cis == nil {
		return nil
	}
	if cis.m == nil {
		cis.m = &sync.Mutex{}
	}
	cis.m.Lock()
	defer cis.m.Unlock()
	if cis.ConsumptionState == nil {
		cis.ConsumptionState = NewConfigConsumptionState()
	}
	return cis.ConsumptionState
}

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
	Seq           int64
	SeqIdProvider *utils.AtomicInt64IDProvider

	// session id
	PersistentSessionId string
	SessionTitle        string
	SessionPromptState  *SessionPromptState

	// memory triage id
	MemoryTriageId string

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
	AiModelName  string

	// ai call config
	AiCallTokenLimit       int64
	AiAutoRetry            int64
	AiTransactionAutoRetry int64
	PromptHook             func(string) string

	/*
		Prompt Manager
	*/
	TopToolsCount          int  // Number of top tools to display in prompt
	ShowForgeListInPrompt  bool // Whether to show forge list in base prompt (default false, forges discoverable via search_capabilities)
	AiForgeManager         AIForgeFactory
	ContextProviderManager *ContextProviderManager
	UserPresetPrompt       string // max 4000 chars, retained for request-scoped compatibility; global guidance comes from AIGlobalConfig.AIPresetPrompt

	/*
		AI Tool
	*/
	// tool manager
	AiToolManager *buildinaitools.AiToolManager

	// tool config
	DisableToolUse      bool
	AiToolManagerOption []buildinaitools.ToolManagerOption
	EnableAISearch      bool
	DisableWebSearch    bool // disable enhanced web search tool, default false (enabled)
	DisallowMCPServers  bool // 禁用 MCP Servers，默认为 false（即默认启用）

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

	// Dynamic planning: when enabled, YOLO/Auto modes will call AI review callbacks
	// for plan and task review decisions instead of blindly auto-continuing.
	DisableDynamicPlanning bool
	AiPlanReviewControl    PlanningReviewControl
	AiTaskReviewControl    PlanningReviewControl

	// sync config
	SyncMutex *sync.RWMutex
	SyncMap   map[string]func() any

	/*
		AI Memory :     General Memory = Timeline + Triage + Other Context
	*/
	// timeline
	Timeline                  *Timeline
	TimelineDiffer            *TimelineDiffer
	TimelineContentSizeLimit  int // in tokens
	TimelineTotalContentLimit int // in tokens

	// triage
	MemoryTriage MemoryTriage

	TimelineArchiveStore TimelineArchiveStore
	MemoryPoolSize       int64
	MemoryPool           *omap.OrderedMap[string, *MemoryEntity]
	EnableSelfReflection bool

	// other context
	PersistentMemory []string

	/*
		PE Mode special config
	*/
	// Plan manager
	AllowPlanUserInteract    bool
	PlanUserInteractMaxCount int64

	// PlanPrompt: Additional context that will be injected into the Plan phase only.
	// This content appears once during plan initialization and does not affect subsequent task execution.
	PlanPrompt string

	// result processer
	GenerateReport               bool
	MaxTaskContinue              int64
	PeriodicVerificationInterval int64

	// other
	ExtendedActionCallback map[string]func(config *Config, action *Action)
	EnableTaskAnalyze      bool

	/*
		Re-Act Mode special config
	*/
	// Call PE
	EnablePlanAndExec bool // Enable plan and execution action
	HijackPERequest   func(ctx context.Context, planPayload string) error

	// default Task for call tool directly
	DefaultTask AIStatefulTask

	// Interval review config for long-running tool execution
	// By default, AI will periodically review tool execution progress
	// Set DisableIntervalReview to true to disable this feature
	DisableIntervalReview             bool          // Disable interval review during tool execution (default: false, meaning enabled)
	IntervalReviewDuration            time.Duration // Duration between reviews (default 20s)
	ToolCallIntervalReviewExtraPrompt string        // Extra prompt injected into tool-call interval review
	ToolComposeConcurrency            int           // Max concurrent tool calls in tool_compose DAG (default 2)

	// iteration limit
	MaxIterationCount int64

	// task config
	EnhanceKnowledgeManager            *EnhanceKnowledgeManager
	DisableEnhanceDirectlyAnswer       bool
	DisableIntentRecognition           bool // 禁用意图识别（用于测试环境，避免子循环消耗 mock 响应）
	DisablePerception                  bool // 禁用感知层（用于测试环境，避免异步 AI 调用干扰 mock 回调）
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

	// other Options
	OtherOption []any

	// focus config
	Focus string

	// output schema
	// worked for liteforge
	LiteForgeActionName   string
	LiteForgeOutputSchema string

	// init status
	InitStatus *ConfigInitStatus
	// origin options
	originOptions []ConfigOption

	ExtendedForge []*schema.AIForge

	/*
		Skill Loader Configuration
	*/
	// skillLoader is the loader for AI skills
	// When set, it enables the loading_skills and change_skill_view_offset actions in ReActLoop
	// Multiple sources can be added via WithSkillsLocalDir, WithSkillsZipFile, WithSkillsFS
	skillLoader *aiskillloader.AutoSkillLoader

	// disableAutoSkills controls whether to automatically load skills from the default directory
	// (~/.yakit-projects/ai-skills). Default is false (auto-load enabled).
	// Use WithDisableAutoSkills(true) to disable this behavior (e.g., in tests).
	disableAutoSkills bool

	// restoredSkillNames holds skill names restored from a previous persistent session.
	// These are loaded back into SkillsContextManager when a new ReActLoop starts.
	restoredSkillNames []string

	/*
		Lazy WorkDir for semantic artifact directory naming
	*/
	// DatabaseRecordID is the gorm primary key ID from AIAgentRuntime
	DisableCreateDBRuntime bool // some liteforge or async lite agent , not save runtime to database, keep persistSession data clean
	DatabaseRecordID       uint
	// workDir is the lazily-created working directory path (set once, never changes)
	workDir         string
	workDirOnce     sync.Once
	workDirMu       sync.RWMutex
	artifactsPinned bool
}

// NewConfig creates a new Config with options
func NewConfig(ctx context.Context, opts ...ConfigOption) *Config {
	config := newConfig(ctx)

	// Apply options
	for _, opt := range opts {
		opt(config)
	}
	config.originOptions = opts

	// Initialize checkpoint storage
	config.BaseCheckpointableStorage = NewCheckpointableStorageWithDB(config.id, consts.GetGormProjectDatabase())

	// Initialize endpoint manager
	config.Epm = NewEndpointManagerContext(ctx)
	config.Epm.SetConfig(config)
	if !config.AICallbackAvailable() {
		if err := WithTieredAICallback()(config); err != nil || !config.AICallbackAvailable() {
			log.Errorf("Failed to set AI callback: %v", err)
		}
	}
	// Only create new Timeline if not already set via options (e.g., WithTimeline)
	// This ensures that when a parent coordinator passes its Timeline to a child invoker,
	// the child uses the same Timeline instance for proper timeline diff tracking
	if config.Timeline == nil {
		config.Timeline = NewTimeline(config, nil)
	}
	if config.TimelineDiffer == nil {
		config.TimelineDiffer = NewTimelineDiffer(config.Timeline)
	}
	config.Timeline.SoftBindConfig(config, config)

	// init default task
	config.DefaultTask = NewStatefulTaskBase(
		"default-task",
		"",
		config.Ctx,
		config.Emitter,
		true,
	)

	// Initialize tool manager if not set
	if config.AiToolManager == nil {
		config.AiToolManager = buildinaitools.NewToolManager(config.AiToolManagerOption...)
	}

	// Restore persistent session if configured
	if !config.InitStatus.IsPersistentSessionRestored() {
		config.restorePersistentSession()
	}

	// Auto-load skills from all well-known directories unless explicitly disabled.
	// Scanned dirs: ~/yakit-projects/ai-skills, ~/.cursor/skills, $CWD/.cursor/skills
	// RefreshFromDirs is protected by a 60-second cooldown inside AutoSkillLoader.
	if !config.disableAutoSkills && config.skillLoader == nil {
		loader := config.ensureSkillLoader()
		if loader != nil {
			loader.RefreshFromDirs(consts.GetAllAISkillsDirs())
		}
	}
	config.loadSkillMDForgesIntoSkillLoader()

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
	seq := rand.Int64N(300) + 200 // avoid zero seq number
	var provider = utils.NewAtomicInt64IDProvider(seq)
	initStatus := NewConfigInitStatus()

	config := &Config{
		HotPatchBroadcaster:                chanx.NewBroadcastChannel[ConfigOption](ctx, 10),
		KeyValueConfig:                     NewKeyValueConfig(),
		Ctx:                                ctx,
		cancel:                             cancel,
		StartInputEventOnce:                sync.Once{},
		EventInputChan:                     chanx.NewUnlimitedChan[*ypb.AIInputEvent](ctx, 10),
		HotPatchOptionChan:                 chanx.NewUnlimitedChan[ConfigOption](ctx, 10),
		InputEventManager:                  NewAIInputEventProcessor(),
		Id:                                 id,
		Seq:                                seq,
		SeqIdProvider:                      provider,
		AgreePolicy:                        AgreePolicyManual,
		AgreeAIScoreLow:                    0.4,
		AgreeAIScoreMiddle:                 0.7,
		AiAgreeRiskControl:                 DefaultAIAssistantRiskControl,
		AiPlanReviewControl:                DefaultAIPlanReviewControl,
		AiTaskReviewControl:                DefaultAITaskReviewControl,
		MaxIterationCount:                  100,
		Language:                           "zh", // Default to Chinese
		TopToolsCount:                      15,
		ContextProviderManager:             NewContextProviderManager(),
		AiAutoRetry:                        5,
		AiTransactionAutoRetry:             5,
		TimelineContentSizeLimit:           50 * 1024, // Default limit for 50k tokens
		Guardian:                           NewAsyncGuardian(ctx, id),
		PerTaskUserInteractiveLimitedTimes: 3, // Default to 3 times
		EnablePlanAndExec:                  true,
		AllowRequireForUserInteract:        true,
		ToolComposeConcurrency:             2,
		Workdir:                            "",
		MemoryPoolSize:                     10 * 1024, // 10k tokens
		MemoryPool:                         omap.NewOrderedMap(make(map[string]*MemoryEntity)),
		MaxTaskContinue:                    3,
		PeriodicVerificationInterval:       DefaultPeriodicVerificationInterval,
		GenerateReport:                     true,
		DisallowMCPServers:                 false, // 默认启用 MCP Servers
		MemoryTriageId:                     "default",
		m:                                  new(sync.Mutex),
		InitStatus:                         initStatus,
		AiCallTokenLimit:                   40 * 1024, // Default to 40 k
		SessionPromptState:                 NewSessionPromptState(),
	}
	config.AiToolManagerOption = append(config.AiToolManagerOption,
		buildinaitools.WithNoToolsCache(),
		buildinaitools.WithEnableAllTools(),
	)

	// Register the session artifacts context provider.
	// This provider scans the session's working directory on every prompt build
	// and injects a file listing (paths, sizes, modification times) into DynamicContext,
	// so that all subsequent AI turns can see artifacts produced by async plan/forge tasks.
	config.ContextProviderManager.Register("session_artifacts", ArtifactsContextProvider)

	// Initialize emitter
	config.Emitter = NewEmitter(id, func(e *schema.AiOutputEvent) (*schema.AiOutputEvent, error) {
		if config.Guardian != nil {
			config.Guardian.Feed(e)
		}
		config.emitBaseHandler(e)
		return e, nil
	})

	if config.SpeedPriorityAICallback != nil {
		config.Emitter.SetStreamNodeIdI18nProvider(
			config.buildStreamNodeIdI18nProvider(),
		)
	}

	return config
}

/*
	config option
*/

// WithID sets the runtime id for the config.
func WithID(id string) ConfigOption {
	meta := &configIDOptionMeta{id: id}
	opt := ConfigOption(meta.apply)
	meta.key = reflect.ValueOf(opt).Pointer()
	configOptionIDRegistry.Store(meta.key, id)
	runtime.SetFinalizer(meta, func(m *configIDOptionMeta) {
		configOptionIDRegistry.Delete(m.key)
	})
	return opt
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
		defer c.m.Unlock()
		ctx, cancel := context.WithCancel(ctx)
		c.Ctx = ctx
		c.cancel = cancel
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

func WithDisableCreateDBRuntime(disable bool) ConfigOption {
	return func(c *Config) error {
		if c.m == nil {
			c.m = &sync.Mutex{}
		}
		c.m.Lock()
		c.DisableCreateDBRuntime = disable
		c.m.Unlock()
		return nil
	}
}

func WithSessionPromptState(state *SessionPromptState) ConfigOption {
	return func(c *Config) error {
		if state != nil {
			c.SessionPromptState = state
		}
		return nil
	}
}

func WithSessionTitle(title string) ConfigOption {
	return func(c *Config) error {
		if c.m == nil {
			c.m = &sync.Mutex{}
		}
		c.m.Lock()
		c.SessionTitle = title
		c.m.Unlock()
		return nil
	}
}

// WithAICallback
// WARNING 粗粒度的ai callback 设置，只可在测试或者功能固定单一的ai模块（如知识库蒸馏）使用，其他位置应该用 WithAutoTieredAICallback
func WithAICallback(cb AICallbackType) ConfigOption {
	return func(c *Config) error {
		if c.m == nil {
			c.m = &sync.Mutex{}
		}

		// if callback is nil, use default ai.Chat
		if cb == nil {
			cb = AIChatToAICallbackType(ai.Chat)
		}

		oCb := cb
		qualityCb := c.wrapper(cb, consts.TierIntelligent)
		speedCb := c.wrapper(cb, consts.TierLightweight)
		c.m.Lock()
		c.OriginalAICallback = oCb
		c.QualityPriorityAICallback = qualityCb
		c.SpeedPriorityAICallback = speedCb
		c.m.Unlock()
		return nil
	}
}

// WithFastAICallback 快速 ai callback 设置, 只做设置，不做任何其他的处理 包括 wrapper：主要使用场景：调用主线无关的liteforge时
func WithFastAICallback(cb AICallbackType) ConfigOption {
	return func(c *Config) error {
		if c.m == nil {
			c.m = &sync.Mutex{}
		}

		var qualityCb AICallbackType
		var speedCb AICallbackType

		c.m.Lock()
		defer c.m.Unlock()
		c.OriginalAICallback = cb
		c.QualityPriorityAICallback = qualityCb
		c.SpeedPriorityAICallback = speedCb
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

func WithQualityPriorityAICallback(cb AICallbackType) ConfigOption {
	return func(c *Config) error {
		if c.m == nil {
			c.m = &sync.Mutex{}
		}
		cb = c.wrapper(cb, consts.TierIntelligent)
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
		cb = c.wrapper(cb, consts.TierLightweight)
		c.m.Lock()
		c.SpeedPriorityAICallback = cb
		c.m.Unlock()
		return nil
	}
}

// WithTieredAICallback configures both quality and speed priority callbacks using tiered AI configuration
// This automatically uses intelligent model for quality priority and lightweight model for speed priority
func WithTieredAICallback() ConfigOption {
	return func(c *Config) error {
		if c.m == nil {
			c.m = &sync.Mutex{}
		}

		// Check if tiered AI config is enabled
		if !consts.IsTieredAIModelConfigEnabled() {
			log.Debugf("Tiered AI config not enabled, skipping tiered callback configuration")
			return nil
		}

		serviceName, modelName, err := GetIntelligentAIModelInfo()
		if err != nil {
			log.Warnf("Failed to get service and model name from tiered config: %v", err)
		} else {
			log.Debugf("Configuring tiered AI callbacks with service=%s model=%s", serviceName, modelName)
			c.AiModelName = modelName
			c.AiServerName = serviceName
		}

		// Configure quality priority callback (uses intelligent model)
		intelligentCB, err := GetIntelligentAIModelCallback()
		if err == nil {
			intelligentCB = c.wrapper(intelligentCB, consts.TierIntelligent)
			c.m.Lock()
			c.QualityPriorityAICallback = intelligentCB
			c.m.Unlock()
			log.Debugf("Configured quality priority callback from intelligent model")
		} else {
			log.Warnf("Failed to load intelligent model callback: %v", err)
		}

		lightweightCB, err := GetLightweightAIModelCallback()
		if err == nil {
			lightweightCB = c.wrapper(lightweightCB, consts.TierLightweight)
			c.m.Lock()
			c.SpeedPriorityAICallback = lightweightCB
			c.m.Unlock()
			log.Debugf("Configured speed priority callback from lightweight model")
		} else {
			log.Warnf("Failed to load lightweight model callback: %v", err)
		}

		return nil
	}
}

// WithAutoTieredAICallback automatically configures tiered AI callbacks if tiered config is enabled
// Otherwise, it falls back to the provided default callback
func WithAutoTieredAICallback(defaultCallback AICallbackType) ConfigOption {
	return func(c *Config) error {
		if c.m == nil {
			c.m = &sync.Mutex{}
		}

		// Check if tiered AI config is enabled
		if consts.IsTieredAIModelConfigEnabled() {
			// Try to configure tiered callbacks
			if err := WithTieredAICallback()(c); err == nil {
				// Also set the original callback if not already set
				if defaultCallback != nil { // force set original callback to default if tiered config is enabled, to ensure async tasks have a valid callback
					c.m.Lock()
					c.OriginalAICallback = defaultCallback
					c.m.Unlock()
				}
				return nil
			}
		}

		// Fall back to default callback for all priorities
		if defaultCallback != nil {
			originalCb := defaultCallback
			qualityCb := c.wrapper(defaultCallback, consts.TierIntelligent)
			speedCb := c.wrapper(defaultCallback, consts.TierLightweight)
			c.m.Lock()
			c.OriginalAICallback = originalCb
			c.QualityPriorityAICallback = qualityCb
			c.SpeedPriorityAICallback = speedCb
			c.m.Unlock()
		}
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

// UserPresetPromptMaxLength is the maximum size of user preset prompt in tokens.
const UserPresetPromptMaxLength = 4000

func WithUserPresetPrompt(prompt string) ConfigOption {
	return func(c *Config) error {
		if MeasureTokens(prompt) > UserPresetPromptMaxLength {
			prompt = ShrinkByTokens(prompt, UserPresetPromptMaxLength)
		}
		c.UserPresetPrompt = prompt
		return nil
	}
}

// ============ Skill Loader Configuration ============
// These options provide user-friendly ways to configure AI skills.
// Skills are loaded from SKILL.md files and made available to the ReActLoop
// via the loading_skills and change_skill_view_offset actions.
//
// Multiple sources can be accumulated: calling WithSkillsLocalDir, WithSkillsZipFile,
// or WithSkillsFS multiple times will add sources to the same loader, not replace it.

// ensureSkillLoader lazily initializes the skill loader on first use.
func (c *Config) ensureSkillLoader() *aiskillloader.AutoSkillLoader {
	if c.skillLoader == nil {
		loader, err := aiskillloader.NewAutoSkillLoader()
		if err != nil {
			log.Errorf("failed to create skill loader: %v", err)
			return nil
		}
		c.skillLoader = loader
	}
	return c.skillLoader
}

// WithSkillsLocalDir adds a local directory as a skill source.
// This is the most user-friendly option for loading skills from a directory.
// The directory should contain subdirectories, each with a SKILL.md file.
// Can be called multiple times to add multiple directories.
//
// Example:
//
//	aicommon.WithSkillsLocalDir("/path/to/skills")
//	aicommon.WithSkillsLocalDir("/another/path/to/skills")
func WithSkillsLocalDir(dirPath string) ConfigOption {
	return func(c *Config) error {
		loader := c.ensureSkillLoader()
		if loader == nil {
			return utils.Error("failed to ensure skill loader")
		}
		_, err := loader.AddLocalDir(dirPath)
		if err != nil {
			return utils.Wrapf(err, "failed to add skills from directory: %s", dirPath)
		}
		return nil
	}
}

// WithSkillsZipFile adds a zip file as a skill source.
// Useful for distributing skills as a single file.
// The zip file should contain subdirectories, each with a SKILL.md file.
// Can be called multiple times to add multiple zip files.
//
// Example:
//
//	aicommon.WithSkillsZipFile("/path/to/skills.zip")
func WithSkillsZipFile(zipPath string) ConfigOption {
	return func(c *Config) error {
		loader := c.ensureSkillLoader()
		if loader == nil {
			return utils.Error("failed to ensure skill loader")
		}
		_, err := loader.AddZipFile(zipPath)
		if err != nil {
			return utils.Wrapf(err, "failed to add skills from zip: %s", zipPath)
		}
		return nil
	}
}

// WithSkillsFS adds a filesystem as a skill source.
// Advanced option for custom filesystem implementations.
// The filesystem should contain subdirectories, each with a SKILL.md file.
// Can be called multiple times to add multiple filesystems.
//
// Example:
//
//	vfs := filesys.NewVirtualFs()
//	vfs.AddFile("my-skill/SKILL.md", skillContent)
//	aicommon.WithSkillsFS(vfs)
func WithSkillsFS(fsys fi.FileSystem) ConfigOption {
	return func(c *Config) error {
		loader := c.ensureSkillLoader()
		if loader == nil {
			return utils.Error("failed to ensure skill loader")
		}
		_, err := loader.AddSource(fsys)
		if err != nil {
			return utils.Wrapf(err, "failed to add skills from filesystem")
		}
		return nil
	}
}

// WithSkillLoader replaces the skill loader with a pre-configured one.
// Advanced option for users who want full control over skill loading.
// Note: this replaces any previously added sources.
//
// Example:
//
//	loader, _ := aiskillloader.NewAutoSkillLoader(
//	    aiskillloader.WithAutoLoad_LocalDir("/path/to/skills"),
//	)
//	aicommon.WithSkillLoader(loader)
func WithSkillLoader(loader *aiskillloader.AutoSkillLoader) ConfigOption {
	return func(c *Config) error {
		c.skillLoader = loader
		return nil
	}
}

// GetSkillLoader returns the configured SkillLoader.
// This is used internally by ReActLoop to access skills.
// Returns nil interface if no skill loader is configured (avoids typed-nil interface pitfall).
func (c *Config) GetSkillLoader() aiskillloader.SkillLoader {
	if c.skillLoader == nil {
		return nil
	}
	return c.skillLoader
}

// IsAutoSkillsDisabled reports whether automatic skill discovery and built-in
// skill loading are disabled for this config.
func (c *Config) IsAutoSkillsDisabled() bool {
	if c == nil {
		return true
	}
	return c.disableAutoSkills
}

// GetRestoredSkillNames returns skill names restored from a previous persistent session.
func (c *Config) GetRestoredSkillNames() []string {
	return c.restoredSkillNames
}

// SaveLoadedSkillNames persists the given skill names to the DB for the current persistent session.
func (c *Config) SaveLoadedSkillNames(skillNames []string) {
	if c.PersistentSessionId == "" {
		return
	}
	db := c.GetDB()
	if db == nil {
		return
	}
	joined := strings.Join(skillNames, ",")
	if err := yakit.UpdateAIAgentRuntimeLoadedSkillNames(db, c.PersistentSessionId, joined); err != nil {
		log.Warnf("failed to save loaded skill names for session [%s]: %v", c.PersistentSessionId, err)
	}
}

// SaveRecentToolCache persists the recent-tool cache to the DB for the current persistent session.
func (c *Config) SaveRecentToolCache() {
	if c.PersistentSessionId == "" {
		return
	}
	tm := c.GetAiToolManager()
	if tm == nil {
		return
	}
	db := c.GetDB()
	if db == nil {
		return
	}
	cacheJSON := tm.ExportRecentToolCache()
	if err := yakit.UpdateAIAgentRuntimeRecentToolsCache(db, c.PersistentSessionId, cacheJSON); err != nil {
		log.Warnf("failed to save recent tool cache for session [%s]: %v", c.PersistentSessionId, err)
	}
}

func (c *Config) AppendRelatedRuntimeID(runtimeID string) {
	if c == nil || c.PersistentSessionId == "" {
		return
	}
	db := c.GetDB()
	if db == nil {
		return
	}
	if err := yakit.AppendAISessionMetaRelatedRuntimeID(db, c.PersistentSessionId, runtimeID); err != nil {
		log.Warnf("failed to append related runtime id for session [%s]: %v", c.PersistentSessionId, err)
	}
}

// WithDisableAutoSkills controls automatic loading of skills from the default directory
// and built-in embedded skills.
// By default (false), NewConfig will automatically load skills from ~/.yakit-projects/ai-skills
// if the directory exists, and NewReAct will load built-in skills.
// Pass true to disable that behavior.
//
// This is typically used in test environments to isolate tests from user-installed skills.
//
// Example:
//
//	config := aicommon.NewConfig(ctx, aicommon.WithDisableAutoSkills(true))
func WithDisableAutoSkills(disable bool) ConfigOption {
	return func(c *Config) error {
		c.disableAutoSkills = disable
		return nil
	}
}

// LoadBuiltinSkillsFS adds a built-in skills filesystem to the skill loader.
// It respects disableAutoSkills: if auto-skills are disabled, this is a no-op.
// This is called by aireact.NewReAct to load embedded production skills
// that ship with the binary.
func (c *Config) LoadBuiltinSkillsFS(fsys fi.FileSystem) error {
	if c.disableAutoSkills {
		return nil
	}
	loader := c.ensureSkillLoader()
	if loader == nil {
		return utils.Error("failed to ensure skill loader")
	}
	_, err := loader.AddSource(fsys)
	return err
}

// LoadBuiltinSkillsFromDir loads skills from a local directory on disk.
// It respects disableAutoSkills: if auto-skills are disabled, this is a no-op.
// This is the preferred way to load skills: built-in skills are first extracted
// to the directory, then loaded from there, allowing users to view and modify them.
// Uses RescanLocalDir to always re-walk the directory, since built-in skills may
// have been freshly extracted after the initial RefreshFromDirs scan.
func (c *Config) LoadBuiltinSkillsFromDir(dirPath string) error {
	if c.disableAutoSkills {
		return nil
	}
	loader := c.ensureSkillLoader()
	if loader == nil {
		return utils.Error("failed to ensure skill loader")
	}
	_, err := loader.RescanLocalDir(dirPath)
	return err
}

// Consumption pointers
func WithAIConsumptionPointers(input *int64, output *int64, tierStats ...*omap.OrderedMap[consts.ModelTier, *ConsumptionStats]) ConfigOption {
	return func(c *Config) error {
		state := c.ensureConsumptionState()
		if state == nil {
			return nil
		}
		state.SetConsumptionPointers(input, output)
		if len(tierStats) > 0 && tierStats[0] != nil {
			state.SetTierConsumptionStats(tierStats[0])
		}
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

// WithShowForgeListInPrompt controls whether the AI Forge list is rendered in the base prompt.
// Default is false: forges are hidden from prompt and discoverable via search_capabilities tool.
// Set to true to restore the original behavior of showing forge list directly in the prompt.
func WithShowForgeListInPrompt(show bool) ConfigOption {
	return func(c *Config) error {
		if c.m == nil {
			c.m = &sync.Mutex{}
		}
		c.m.Lock()
		c.ShowForgeListInPrompt = show
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
			c.ContextProviderManager.Register(name, provider)
		}
		return nil
	}
}

// WithTracedDynamicContextProvider registers a dynamic context provider with tracing capabilities
// It tracks changes between calls and provides diff information
func WithTracedDynamicContextProvider(name string, provider ContextProvider) ConfigOption {
	return func(c *Config) error {
		if name != "" {
			c.ContextProviderManager.RegisterTracedContent(name, provider)
		}
		return nil
	}
}

// WithTracedFileContext monitors a file and provides its content as context with change tracking
func WithTracedFileContext(name string, filePath string) ConfigOption {
	return func(c *Config) error {
		if name != "" && filePath != "" {
			provider := FileContextProvider(filePath)
			c.ContextProviderManager.RegisterTracedContent(name, provider)
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
		defer c.m.Unlock()
		if c.AiToolManagerOption == nil {
			c.AiToolManagerOption = make([]buildinaitools.ToolManagerOption, 0)
		}
		c.AiToolManagerOption = append(c.AiToolManagerOption, opts...)
		return nil
	}
}

func WithAiToolManager(manager *buildinaitools.AiToolManager) ConfigOption {
	return func(c *Config) error {
		if c.m == nil {
			c.m = &sync.Mutex{}
		}
		c.m.Lock()
		c.AiToolManager = manager
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

func WithEnableToolManagerAISearch(enable bool) ConfigOption {
	return func(c *Config) error {
		if c.m == nil {
			c.m = &sync.Mutex{}
		}
		c.m.Lock()
		defer c.m.Unlock()
		c.EnableAISearch = enable
		return nil
	}
}

func WithDisallowMCPServers(disallow bool) ConfigOption {
	return func(c *Config) error {
		if c.m == nil {
			c.m = &sync.Mutex{}
		}
		c.m.Lock()
		defer c.m.Unlock()
		c.DisallowMCPServers = disallow
		return nil
	}
}

func WithDisableWebSearch(disable bool) ConfigOption {
	return func(c *Config) error {
		if c.m == nil {
			c.m = &sync.Mutex{}
		}
		c.m.Lock()
		defer c.m.Unlock()
		c.DisableWebSearch = disable
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
		return nil
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

func WithDisableDynamicPlanning(b ...bool) ConfigOption {
	return func(c *Config) error {
		if len(b) > 0 {
			c.DisableDynamicPlanning = b[0]
		} else {
			c.DisableDynamicPlanning = true
		}
		return nil
	}
}

func WithAiPlanReviewControl(rc PlanningReviewControl) ConfigOption {
	return func(c *Config) error {
		c.AiPlanReviewControl = rc
		return nil
	}
}

func WithAiTaskReviewControl(rc PlanningReviewControl) ConfigOption {
	return func(c *Config) error {
		c.AiTaskReviewControl = rc
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

func WithAgreeAIRiskCtrlScore(score float64) ConfigOption {
	return func(c *Config) error {
		if c.m == nil {
			c.m = &sync.Mutex{}
		}
		low := score
		if score > 0.2 {
			low = score - 0.2
		}
		c.m.Lock()
		c.AgreeAIScoreLow = low
		c.AgreeAIScoreMiddle = score
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

// WithPlanPrompt sets additional context that will be injected into the Plan phase only.
// This content appears once during plan initialization and does not affect subsequent task execution.
// It is useful for providing planning-specific instructions or constraints.
// The prompt is also stored in KeyValueConfig with key "plan_prompt" for loop_plan to access.
func WithPlanPrompt(prompt string) ConfigOption {
	return func(c *Config) error {
		if c.m == nil {
			c.m = &sync.Mutex{}
		}
		c.m.Lock()
		c.PlanPrompt = prompt
		c.m.Unlock()
		// Also set to KeyValueConfig so loop_plan can access via GetConfigString
		c.SetConfig("plan_prompt", prompt)
		return nil
	}
}

func WithDisableToolsName(toolsName ...string) ConfigOption {
	return func(c *Config) error {
		return WithAiToolManagerOptions(buildinaitools.WithDisableTools(toolsName))(c)
	}
}

func WithEnableToolsName(toolsName ...string) ConfigOption {
	return func(c *Config) error {
		return WithAiToolManagerOptions(buildinaitools.WithEnabledTools(toolsName))(c)
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
		return WithAiToolManagerOptions(buildinaitools.WithExtendTools(tool, true))(c)
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

func WithNoOpMemoryTriage() ConfigOption {
	return WithMemoryTriage(NewNoOpMemoryTriage())
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

// WithDisableToolCallerIntervalReview disables interval review during tool execution.
// By default, interval review is ENABLED for long-running tool calls.
// AI will periodically review tool execution progress and decide whether to continue.
// Use this option to disable this safety feature if needed.
func WithDisableToolCallerIntervalReview(disable bool) ConfigOption {
	return func(c *Config) error {
		if c.m == nil {
			c.m = &sync.Mutex{}
		}
		c.m.Lock()
		c.DisableIntervalReview = disable
		c.m.Unlock()
		return nil
	}
}

// WithToolCallerIntervalReviewDuration sets the duration between interval reviews during tool execution.
// Default is 20 seconds if not set.
func WithToolCallerIntervalReviewDuration(duration time.Duration) ConfigOption {
	return func(c *Config) error {
		if c.m == nil {
			c.m = &sync.Mutex{}
		}
		if duration <= 0 {
			duration = time.Second * 20
		}
		c.m.Lock()
		c.IntervalReviewDuration = duration
		c.m.Unlock()
		return nil
	}
}

// WithToolCallIntervalReviewExtraPrompt injects extra instructions into the interval review prompt
// that runs while long-running tools are executing.
func WithToolCallIntervalReviewExtraPrompt(prompt string) ConfigOption {
	return func(c *Config) error {
		if c.m == nil {
			c.m = &sync.Mutex{}
		}
		prompt = strings.TrimSpace(prompt)
		c.m.Lock()
		c.ToolCallIntervalReviewExtraPrompt = prompt
		c.m.Unlock()
		c.SetConfig(ConfigKeyToolCallIntervalReviewExtraPrompt, prompt)
		return nil
	}
}

func WithToolComposeConcurrency(n int) ConfigOption {
	return func(c *Config) error {
		if c.m == nil {
			c.m = &sync.Mutex{}
		}
		if n <= 0 {
			n = 2
		}
		c.m.Lock()
		c.ToolComposeConcurrency = n
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

func WithAIModelName(name string) ConfigOption {
	return func(cfg *Config) error {
		if cfg.m == nil {
			cfg.m = &sync.Mutex{}
		}
		cfg.m.Lock()
		defer cfg.m.Unlock()
		cfg.AiModelName = name
		return nil
	}
}

func WithAIChatInfo(serviceName string, modelName string) ConfigOption {
	return func(cfg *Config) error {
		if cfg.m == nil {
			cfg.m = &sync.Mutex{}
		}
		cfg.m.Lock()
		defer cfg.m.Unlock()
		cfg.AiServerName = serviceName
		cfg.AiModelName = modelName
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

// WithDisableIntentRecognition disables intent recognition in loop_default's buildInitTask.
// This is primarily used in test environments where the mock AI callback cannot handle
// the intent recognition sub-loop (loop_intent), which would consume mock responses
// intended for the main test flow.
func WithDisableIntentRecognition(disable bool) ConfigOption {
	return func(c *Config) error {
		if c.m == nil {
			c.m = &sync.Mutex{}
		}
		c.m.Lock()
		c.DisableIntentRecognition = disable
		c.m.Unlock()
		// Also set in KV config so buildInitTask can check via GetConfigBool
		c.SetConfig("DisableIntentRecognition", disable)
		return nil
	}
}

// WithDisablePerception disables the perception layer in all loops created from this config.
// When disabled, no perception AI evaluations are triggered and the perception ContextProvider
// is not registered. This is primarily used in test environments where async perception calls
// would interfere with mocked AI callbacks.
func WithDisablePerception(disable bool) ConfigOption {
	return func(c *Config) error {
		if c.m == nil {
			c.m = &sync.Mutex{}
		}
		c.m.Lock()
		c.DisablePerception = disable
		c.m.Unlock()
		c.SetConfig("DisablePerception", disable)
		return nil
	}
}

// WithDisableSessionTitleGeneration disables the automatic session title generation in ReAct
func WithDisableSessionTitleGeneration(disable bool) ConfigOption {
	return func(c *Config) error {
		c.SetConfig("disable_session_title_generation", disable)
		return nil
	}
}

func WithContextProvider(cpm *ContextProviderManager) ConfigOption {
	return func(c *Config) error {
		if c.m == nil {
			c.m = &sync.Mutex{}
		}
		c.m.Lock()
		c.ContextProviderManager = cpm
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

func WithAgreeYOLO(b ...bool) ConfigOption {
	if len(b) > 0 && !b[0] {
		return func(c *Config) error {
			return nil
		}
	}
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
		c.Seq = seq
		c.SeqIdProvider = utils.NewAtomicInt64IDProvider(seq)
		return nil
	}
}

func WithSeqIdProvider(provider *utils.AtomicInt64IDProvider) ConfigOption {
	return func(c *Config) error {
		if c.m == nil {
			c.m = &sync.Mutex{}
		}
		c.m.Lock()
		defer c.m.Unlock()
		c.SeqIdProvider = provider
		return nil
	}
}

func WithInitConfigStatus(status *ConfigInitStatus) ConfigOption {
	return func(c *Config) error {
		if c.m == nil {
			c.m = &sync.Mutex{}
		}
		if status == nil {
			status = NewConfigInitStatus()
		}
		c.m.Lock()
		c.InitStatus = status
		c.m.Unlock()
		return nil
	}
}

func WithAIKBPath(path string) ConfigOption {
	return func(c *Config) error {
		c.SetConfig("aikb_path", path)
		return nil
	}
}

func WithAIKBRagPath(path string) ConfigOption {
	return func(c *Config) error {
		c.SetConfig("aikb_rag_path", path)
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

func WithConsumption(input, output *int64, logUUID string, tierStats ...*omap.OrderedMap[consts.ModelTier, *ConsumptionStats]) ConfigOption {
	return func(c *Config) error {
		state := c.ensureConsumptionState()
		if state == nil {
			return nil
		}
		state.SetConsumptionPointers(input, output)
		if logUUID != "" {
			state.SetConsumptionUUID(logUUID)
		}
		if len(tierStats) > 0 && tierStats[0] != nil {
			state.SetTierConsumptionStats(tierStats[0])
		}
		return nil
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

func WithEnablePETaskAnalyze(v bool) ConfigOption {
	return func(c *Config) error {
		if c.m == nil {
			c.m = &sync.Mutex{}
		}
		c.m.Lock()
		c.EnableTaskAnalyze = v
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

func WithPeriodicVerificationInterval(n int64) ConfigOption {
	return func(c *Config) error {
		if n < 0 {
			return nil
		}
		if c.m == nil {
			c.m = &sync.Mutex{}
		}
		c.m.Lock()
		c.PeriodicVerificationInterval = n
		c.m.Unlock()
		return nil
	}
}

func WithAppendOtherOption(opts any) ConfigOption {
	return func(c *Config) error {
		if opts == nil {
			return nil
		}
		if c.m == nil {
			c.m = &sync.Mutex{}
		}
		c.m.Lock()
		c.OtherOption = append(c.OtherOption, opts)
		c.m.Unlock()
		return nil
	}
}

// WithAppendPersistentContext appends keys to PersistentMemory.
func WithAppendPersistentContext(keys ...string) ConfigOption {
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

// WithTimelineLimit sets the timeline content size limit (deprecated name, kept for compatibility).
func WithTimelineLimit(limit int) ConfigOption {
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
	return WithTimelineLimit(limit)
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

func WithFocus(focus string) ConfigOption {
	return func(c *Config) error {
		c.m.Lock()
		c.Focus = focus
		c.m.Unlock()
		return nil
	}
}

func WithLiteForgeOutputSchema(i string) ConfigOption {
	return func(c *Config) error {
		c.m.Lock()
		c.LiteForgeOutputSchema = i
		c.m.Unlock()
		return nil
	}
}

func WithLiteForgeOutputSchemaFromAIToolOptions(params ...aitool.ToolOption) ConfigOption {
	return func(c *Config) error {
		c.m.Lock()
		defer c.m.Unlock()

		t := aitool.NewWithoutCallback("output", params...)
		c.LiteForgeOutputSchema = t.ToJSONSchemaString()
		c.LiteForgeActionName = "call-tool"
		return nil
	}
}

func WithLiteForgeActionName(i string) ConfigOption {
	return func(c *Config) error {
		c.m.Lock()
		c.LiteForgeActionName = i
		c.m.Unlock()
		return nil
	}
}

func WithMemoryTriageId(id string) ConfigOption {
	return func(c *Config) error {
		c.m.Lock()
		c.MemoryTriageId = id
		c.m.Unlock()
		return nil
	}
}

func WithTimelineArchiveStore(store TimelineArchiveStore) ConfigOption {
	return func(c *Config) error {
		c.m.Lock()
		c.TimelineArchiveStore = store
		c.m.Unlock()
		return nil
	}
}

func (c *Config) GetTimelineArchiveStore() TimelineArchiveStore {
	if c == nil {
		return nil
	}
	return c.TimelineArchiveStore
}

func (c *Config) GetPersistentSessionID() string {
	if c == nil {
		return ""
	}
	return c.PersistentSessionId
}

func WithForges(forge ...*schema.AIForge) ConfigOption {
	return func(c *Config) error {
		c.m.Lock()
		c.ExtendedForge = append(c.ExtendedForge, forge...)
		c.m.Unlock()
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

func (c *Config) CallOriginalAI(request *AIRequest) (*AIResponse, error) {
	for _, cb := range []AICallbackType{
		c.OriginalAICallback,
	} {
		if cb == nil {
			continue
		}
		return cb(c, request)
	}
	return nil, utils.Error("no any ai callback is set, cannot found ai config")
}

func (c *Config) CallQualityPriorityAI(request *AIRequest) (*AIResponse, error) {
	for _, cb := range []AICallbackType{
		c.QualityPriorityAICallback,
		c.OriginalAICallback,
	} {
		if cb == nil {
			continue
		}
		return cb(c, request)
	}
	return nil, utils.Error("no quality priority ai callback is set")
}

func (c *Config) CallSpeedPriorityAI(request *AIRequest) (*AIResponse, error) {
	for _, cb := range []AICallbackType{
		c.SpeedPriorityAICallback,
		c.OriginalAICallback,
	} {
		if cb == nil {
			continue
		}
		return cb(c, request)
	}
	return nil, utils.Error("no quality priority ai callback is set")
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
	if c.SeqIdProvider == nil {
		// fallback: create a simple generator if missing
		var gen = rand.Int64N(300) + 200
		c.Seq = gen
		c.SeqIdProvider = utils.NewAtomicInt64IDProvider(gen)
	}
	return c.SeqIdProvider.NewID()
}

func (c *Config) GetSessionPromptState() *SessionPromptState {
	if c == nil {
		return nil
	}
	if c.SessionPromptState == nil {
		c.SessionPromptState = NewSessionPromptState()
	}
	return c.SessionPromptState
}

func (c *Config) GetSessionTitle() string {
	if c == nil {
		return ""
	}
	return c.SessionTitle
}

func (c *Config) SetSessionTitle(title string) {
	if c == nil {
		return
	}
	if c.m == nil {
		c.m = &sync.Mutex{}
	}
	c.m.Lock()
	c.SessionTitle = title
	c.m.Unlock()
}

func (c *Config) GetPrevSessionUserInput() string {
	return c.GetSessionPromptState().GetPrevSessionUserInput()
}

func (c *Config) GetUserInputHistory() []schema.AIAgentUserInputRecord {
	return c.GetSessionPromptState().GetUserInputHistory()
}

func (c *Config) SetUserInputHistory(history []schema.AIAgentUserInputRecord) {
	c.GetSessionPromptState().SetUserInputHistory(history)
}

func (c *Config) AppendUserInputHistory(userInput string, timestamp time.Time) (string, error) {
	return c.GetSessionPromptState().AppendUserInputHistory(userInput, timestamp)
}

func (c *Config) GetSessionEvidenceRendered() string {
	return c.GetSessionPromptState().GetSessionEvidenceRendered()
}

func (c *Config) ApplySessionEvidenceOps(ops []EvidenceOperation) {
	if len(ops) == 0 {
		return
	}
	quotedEvidence := c.GetSessionPromptState().ApplySessionEvidenceOps(ops)
	if c.PersistentSessionId != "" && c.GetDB() != nil {
		if err := yakit.UpdateAIAgentRuntimeEvidence(c.GetDB(), c.PersistentSessionId, quotedEvidence); err != nil {
			log.Warnf("persist session evidence failed: %v", err)
		}
	}
}

// FlushRestoredSessionEvidence persists the in-memory session evidence (restored from
// a previous runtime) to the current runtime's DB row. This must be called after
// the runtime DB row is created, because restorePersistentSession runs before row creation.
func (c *Config) FlushRestoredSessionEvidence() {
	if c.PersistentSessionId == "" || c.GetDB() == nil {
		return
	}
	raw := c.GetSessionPromptState().GetSessionEvidence()
	if raw == "" {
		return
	}
	quoted := c.GetSessionPromptState().quoteEvidence(raw)
	if err := yakit.UpdateAIAgentRuntimeEvidence(c.GetDB(), c.PersistentSessionId, quoted); err != nil {
		log.Warnf("flush restored session evidence failed: %v", err)
	}
}

func (c *Config) FormatUserInputHistory() string {
	history := c.GetUserInputHistory()
	if len(history) == 0 {
		return ""
	}
	entries := c.formatUserInputHistoryEntries()
	var builder strings.Builder
	builder.WriteString("# Session User Input History\n")
	for _, entry := range entries {
		builder.WriteString(entry)
	}
	return builder.String()
}

func (c *Config) FormatUserInputHistoryAITag(nonce string, maxTokens int) string {
	body := c.formatUserInputHistoryForPrompt(maxTokens)
	if body == "" || strings.TrimSpace(nonce) == "" {
		return body
	}
	return fmt.Sprintf("<|PREV_USER_INPUT_%s|>\n%s\n<|PREV_USER_INPUT_END_%s|>", nonce, body, nonce)
}

func (c *Config) formatUserInputHistoryEntries() []string {
	history := c.GetUserInputHistory()
	if len(history) == 0 {
		return nil
	}
	entries := make([]string, 0, len(history))
	for _, item := range history {
		timestamp := "unknown"
		if !item.Timestamp.IsZero() {
			timestamp = item.Timestamp.Format("2006-01-02 15:04:05")
		}
		entries = append(entries, fmt.Sprintf("- Round %d | Time: %s | User Input: %s\n", item.Round, timestamp, item.UserInput))
	}
	return entries
}

func (c *Config) formatUserInputHistoryForPrompt(maxTokens int) string {
	entries := c.formatUserInputHistoryEntries()
	if len(entries) == 0 {
		return ""
	}
	header := "# Session User Input History\n"
	if maxTokens <= 0 {
		return header + strings.Join(entries, "")
	}

	marker := "[TRUNCATED_HEAD]\n"
	remaining := maxTokens - MeasureTokens(header)
	if remaining <= 0 {
		return header
	}

	markerTokens := MeasureTokens(marker)
	selected := make([]string, 0, len(entries))
	truncated := false
	for i := len(entries) - 1; i >= 0; i-- {
		entry := entries[i]
		needed := MeasureTokens(entry)
		if truncated {
			needed += markerTokens
		}
		if needed <= remaining {
			selected = append([]string{entry}, selected...)
			remaining -= MeasureTokens(entry)
			continue
		}

		keep := remaining
		if !truncated {
			keep -= markerTokens
		}
		if keep > 0 {
			// Keep the tail of the entry (most recent content is more relevant)
			tokens := ytoken.Encode(entry)
			if len(tokens) > keep {
				tokens = tokens[len(tokens)-keep:]
			}
			entry = marker + ytoken.Decode(tokens)
			selected = append([]string{entry}, selected...)
		}
		truncated = true
		break
	}

	return header + strings.Join(selected, "")
}

func (c *Config) GetRuntimeId() string {
	return c.Id
}

// CreateOrUpdateRuntimeRecord persists a runtime record unless DB runtime creation is disabled.
func (c *Config) CreateOrUpdateRuntimeRecord(runtime *schema.AIAgentRuntime) error {
	if c == nil || runtime == nil {
		return nil
	}
	if c.DisableCreateDBRuntime || c.GetDB() == nil {
		return nil
	}
	c.AppendRelatedRuntimeID(c.GetRuntimeId()) // just append self ID to related runtimes for long chain runtime

	dbID, err := yakit.CreateOrUpdateAIAgentRuntime(c.GetDB(), runtime)
	if err != nil {
		return err
	}
	c.DatabaseRecordID = dbID
	runtime.ID = dbID
	return nil
}

// IsWorkDirReady checks if the working directory has been created
func (c *Config) IsWorkDirReady() bool {
	c.workDirMu.RLock()
	defer c.workDirMu.RUnlock()
	return c.workDir != ""
}

// SetWorkDir sets the working directory path (called by ensureWorkDirectory, only sets once)
func (c *Config) SetWorkDir(path string) {
	c.workDirMu.Lock()
	defer c.workDirMu.Unlock()
	if c.workDir == "" {
		c.workDir = path
	}
}

// GetOrCreateWorkDir lazily creates and returns the working directory.
// Resolution order:
//  1. workDir (lowercase, set by ensureWorkDirectory via SetWorkDir)
//  2. Workdir (capital W, propagated from parent via ConvertConfigToOptions/WithWorkdir)
//  3. Fallback: create {DatabaseRecordID}_session_{date}_{shortUuid(5)}
//
// Normal flow: ensureWorkDirectory(userInput) will preemptively set both workDir and Workdir.
// In P&E mode, child configs receive Workdir from parent via ConvertConfigToOptions.
func (c *Config) GetOrCreateWorkDir() string {
	c.workDirMu.RLock()
	if c.workDir != "" {
		dir := c.workDir
		c.workDirMu.RUnlock()
		return dir
	}
	c.workDirMu.RUnlock()

	// Check if Workdir (capital W) was set by parent config via ConvertConfigToOptions.
	// This ensures P&E sub-invokers and forge executions reuse the parent's work directory
	// instead of creating their own fallback directories.
	if c.Workdir != "" {
		c.SetWorkDir(c.Workdir)
		return c.Workdir
	}

	c.workDirOnce.Do(func() {
		shortUuid := c.Id
		if len(shortUuid) > 5 {
			shortUuid = shortUuid[:5]
		}
		folderName := fmt.Sprintf("%d_session_%s_%s",
			c.DatabaseRecordID,
			time.Now().Format("20060102"),
			shortUuid,
		)
		dirPath := consts.TempAIDir(folderName)
		c.workDirMu.Lock()
		if c.workDir == "" {
			c.workDir = dirPath
		}
		c.workDirMu.Unlock()
		log.Infof("created fallback work directory: %s", dirPath)
	})

	c.workDirMu.RLock()
	defer c.workDirMu.RUnlock()
	return c.workDir
}

func (c *Config) GetContextProviderManager() *ContextProviderManager {
	return c.ContextProviderManager
}

// IsArtifactsPinned checks if EmitPinDirectory has been called
func (c *Config) IsArtifactsPinned() bool {
	c.workDirMu.RLock()
	defer c.workDirMu.RUnlock()
	return c.artifactsPinned
}

// SetArtifactsPinned marks that EmitPinDirectory has been called
func (c *Config) SetArtifactsPinned() {
	c.workDirMu.Lock()
	defer c.workDirMu.Unlock()
	c.artifactsPinned = true
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
	state := c.ensureConsumptionState()
	if state == nil {
		return
	}
	_, output := state.GetConsumptionPointers()
	if output == nil {
		return
	}
	atomic.AddInt64(output, int64(i))
}

func (c *Config) GetAITransactionAutoRetryCount() int64 {
	return c.AiTransactionAutoRetry
}

func (c *Config) GetToolComposeConcurrency() int {
	if c.ToolComposeConcurrency <= 0 {
		return 2
	}
	return c.ToolComposeConcurrency
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

func (c *Config) UpdateAIModelInfo(provider, model string) {
	c.m.Lock()
	defer c.m.Unlock()
	c.AiServerName = provider
	c.AiModelName = model
}

// EventFormat fills in common fields for AI output events
func (c *Config) EventFormat(e *schema.AiOutputEvent) *schema.AiOutputEvent {

	if e.AIModelName == "" {
		e.AIModelName = c.AiModelName
		e.AIModelVerboseName = aispec.ModelVerboseName(c.AiModelName)
	}

	if e.AIService == "" {
		e.AIService = c.AiServerName
	}

	if c.PersistentSessionId != "" {
		e.SessionId = c.PersistentSessionId
	}
	return e
}

func (c *Config) emitBaseHandler(e *schema.AiOutputEvent) {
	select {
	case <-c.Ctx.Done():
		return
	default:
	}

	e = c.EventFormat(e)

	if e.ShouldSave() {
		err := yakit.CreateOrUpdateAIOutputEvent(consts.GetGormProjectDatabase(), e)
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
	timelineInstance.SoftBindConfig(c, c)
	if !timelineInstance.Valid() {
		log.Errorf("restored timeline instance is invalid for session [%s]", c.PersistentSessionId)
		return
	}

	// Reassign IDs to all restored timeline items to avoid ID conflicts
	// This uses the current idGenerator to ensure sequential IDs
	lastID := timelineInstance.ReassignIDs(c.SeqIdProvider.CurrentID)
	if lastID > 0 {
		log.Infof("reassigned timeline IDs, last assigned ID: %d", lastID)
	}

	c.Timeline = timelineInstance
	c.TimelineDiffer = NewTimelineDiffer(timelineInstance)

	// Restore WorkDir from previous runtime so that Session Artifacts persist across restarts
	if runtime.WorkDir != "" {
		if utils.GetFirstExistedPath(runtime.WorkDir) != "" {
			c.SetWorkDir(runtime.WorkDir)
			c.Workdir = runtime.WorkDir
			log.Infof("restored work directory from persistent session [%s]: %s", c.PersistentSessionId, runtime.WorkDir)
		} else {
			log.Warnf("previous work directory no longer exists for session [%s]: %s, will create new one", c.PersistentSessionId, runtime.WorkDir)
		}
	}

	// Restore loaded skill names from previous session
	if runtime.LoadedSkillNames != "" {
		names := strings.Split(runtime.LoadedSkillNames, ",")
		var trimmed []string
		for _, n := range names {
			n = strings.TrimSpace(n)
			if n != "" {
				trimmed = append(trimmed, n)
			}
		}
		if len(trimmed) > 0 {
			c.restoredSkillNames = trimmed
			log.Infof("restored %d loaded skill names from persistent session [%s]: %v", len(trimmed), c.PersistentSessionId, trimmed)
		}
	}

	if history := runtime.GetUserInputHistory(); len(history) > 0 {
		c.SetUserInputHistory(history)
		log.Infof("restored %d user input history entries from session [%s], latest: %.80s",
			len(history), c.PersistentSessionId, c.GetPrevSessionUserInput())
	}

	if evidence := runtime.GetEvidence(); evidence != "" {
		c.GetSessionPromptState().SetSessionEvidence(evidence)
		log.Infof("restored session evidence from session [%s], length: %d runes",
			c.PersistentSessionId, len([]rune(evidence)))
	}

	// Restore recent-tool cache from previous session
	if runtime.RecentToolsCache != "" {
		if tm := c.GetAiToolManager(); tm != nil {
			tm.ImportRecentToolCache(runtime.RecentToolsCache)
			log.Infof("restored recent tool cache from persistent session [%s]", c.PersistentSessionId)
		}
	}

	c.InitStatus.SetPersistentSessionRestored(true)
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

func (c *Config) SetContext(ctx context.Context) {
	if ctx == nil {
		return
	}

	if c.m == nil {
		c.m = &sync.Mutex{}
	}

	c.m.Lock()
	defer c.m.Unlock()
	subCtx, cancel := context.WithCancel(ctx)
	c.Ctx = subCtx
	c.cancel = cancel
}

func (c *Config) CallAITransaction(
	prompt string,
	callAi func(*AIRequest) (*AIResponse, error),
	postHandler func(rsp *AIResponse) error,
	requestOpts ...AIRequestOption,
) error {
	return CallAITransaction(c, prompt, callAi, postHandler, requestOpts...)
}

//	func (c *Config) RegisterMirrorOfAIInputEvent(id string, f func(*ypb.AIInputEvent)) {
//		r.mirrorMutex.Lock()
//		defer r.mirrorMutex.Unlock()
//		r.mirrorOfAIInputEvent[id] = f
//	}
//
//	func (c *Config) CallMirrorOfAIInputEvent(event *ypb.AIInputEvent) {
//		r.mirrorMutex.RLock()
//		defer r.mirrorMutex.RUnlock()
//		for _, f := range r.mirrorOfAIInputEvent {
//			f(event)
//		}
//	}
//
//	func (c *Config) UnregisterMirrorOfAIInputEvent(id string) {
//		r.mirrorMutex.Lock()
//		defer r.mirrorMutex.Unlock()
//		delete(r.mirrorOfAIInputEvent, id)
//	}
func ConvertConfigToOptions(i *Config) []ConfigOption {
	// Return nil for nil input
	if i == nil {
		return nil
	}

	opts := make([]ConfigOption, 0)

	opts = append(opts, WithAllowRequireForUserInteract(i.AllowRequireForUserInteract))

	// aiCallback
	if i.AiServerName != "" {
		opts = append(opts, WithAIChatInfo(i.AiServerName, i.AiModelName))
	}

	// Keywords
	if len(i.Keywords) > 0 {
		opts = append(opts, WithKeywords(i.Keywords...))
	}

	// Disable tool use flag
	opts = append(opts, WithDisableToolUse(i.DisableToolUse))

	// Tool manager options
	if i.AiToolManager != nil {
		opts = append(opts, WithAiToolManager(i.AiToolManager))
	}
	if len(i.AiToolManagerOption) > 0 {
		opts = append(opts, WithAiToolManagerOptions(i.AiToolManagerOption...))
	}

	// Agree policy mapping
	opts = append(opts, WithAgreePolicy(i.AgreePolicy))
	if i.AiAgreeRiskControl != nil {
		opts = append(opts, WithAiAgreeRiskControl(i.AiAgreeRiskControl))
	}

	// Other boolean/flag options
	opts = append(opts, WithAllowRequireForUserInteract(i.AllowRequireForUserInteract))
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
	opts = append(opts, WithDisableToolCallerIntervalReview(i.DisableIntervalReview))
	if i.IntervalReviewDuration > 0 {
		opts = append(opts, WithToolCallerIntervalReviewDuration(i.IntervalReviewDuration))
	}
	if i.ToolCallIntervalReviewExtraPrompt != "" {
		opts = append(opts, WithToolCallIntervalReviewExtraPrompt(i.ToolCallIntervalReviewExtraPrompt))
	}
	if i.ToolComposeConcurrency > 0 {
		opts = append(opts, WithToolComposeConcurrency(i.ToolComposeConcurrency))
	}
	if i.MaxTaskContinue > 0 {
		opts = append(opts, WithMaxTaskContinue(i.MaxTaskContinue))
	}

	opts = append(opts, WithPeriodicVerificationInterval(i.PeriodicVerificationInterval))

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
	if i.TimelineArchiveStore != nil {
		opts = append(opts, WithTimelineArchiveStore(i.TimelineArchiveStore))
	}

	// Misc
	if i.PromptHook != nil {
		opts = append(opts, WithPromptHook(i.PromptHook))
	}
	if i.Language != "" {
		opts = append(opts, WithLanguage(i.Language))
	}
	// Propagate work directory: check both Workdir (capital W, explicitly set) and
	// the lazy workDir (lowercase, set by ensureWorkDirectory) to ensure P&E sub-invokers,
	// forge executions, and other child configs share the same artifact directory.
	if i.Workdir != "" {
		opts = append(opts, WithWorkdir(i.Workdir))
	} else if i.IsWorkDirReady() {
		// The lazy workDir was set (e.g. by ensureWorkDirectory) but Workdir was not.
		// This shouldn't normally happen after our fix, but handle it defensively.
		dir := i.GetOrCreateWorkDir()
		if dir != "" {
			opts = append(opts, WithWorkdir(dir))
		}
	}

	// Propagate DatabaseRecordID so child configs can use it for fallback directory naming.
	// Without this, child configs would create fallback dirs with DatabaseRecordID=0.
	if i.DatabaseRecordID > 0 {
		dbRecordID := i.DatabaseRecordID
		opts = append(opts, func(c *Config) error {
			if c.DatabaseRecordID == 0 {
				c.DatabaseRecordID = dbRecordID
			}
			return nil
		})
	}

	if i.EventHandler != nil {
		opts = append(opts, WithEventHandler(i.EventHandler))
	}

	if i.HotPatchBroadcaster != nil {
		hotPatchChan := i.HotPatchBroadcaster.Subscribe()
		opts = append(opts, WithHotPatchOptionChan(hotPatchChan))
	}

	if i.EnhanceKnowledgeManager != nil {
		opts = append(opts, WithEnhanceKnowledgeManager(i.EnhanceKnowledgeManager))
	}

	if i.ContextProviderManager != nil {
		opts = append(opts, WithContextProvider(i.ContextProviderManager))
	}

	// Dynamic planning propagation
	opts = append(opts, WithDisableDynamicPlanning(i.DisableDynamicPlanning))
	if i.AiPlanReviewControl != nil {
		opts = append(opts, WithAiPlanReviewControl(i.AiPlanReviewControl))
	}
	if i.AiTaskReviewControl != nil {
		opts = append(opts, WithAiTaskReviewControl(i.AiTaskReviewControl))
	}

	// PlanPrompt - additional context for plan phase only
	if i.PlanPrompt != "" {
		opts = append(opts, WithPlanPrompt(i.PlanPrompt))
	}

	if i.UserPresetPrompt != "" {
		opts = append(opts, WithUserPresetPrompt(i.UserPresetPrompt))
	}

	if i.PersistentSessionId != "" {
		opts = append(opts, WithPersistentSessionId(i.PersistentSessionId))
	}
	if i.SessionTitle != "" {
		opts = append(opts, WithSessionTitle(i.SessionTitle))
	}
	if i.SessionPromptState != nil {
		opts = append(opts, WithSessionPromptState(i.SessionPromptState))
	}

	if i.Seq > 0 {
		opts = append(opts, WithSequence(i.Seq))
	}

	if i.SeqIdProvider != nil {
		opts = append(opts, WithSeqIdProvider(i.SeqIdProvider))
	}

	// Propagate intent recognition disable flag so sub-loops (PE task, plan)
	// do not accidentally run deep intent recognition in test environments.
	if i.DisableIntentRecognition {
		opts = append(opts, WithDisableIntentRecognition(true))
	}

	// Propagate perception disable flag so sub-loops inherit the setting.
	if i.DisablePerception {
		opts = append(opts, WithDisablePerception(true))
	}

	// once init config flag
	opts = append(opts, WithInitConfigStatus(i.InitStatus))

	opts = append(opts, WithContext(i.Ctx))

	return opts
}

func (c *Config) GetConsumptionConfig() (*int64, *int64, string) {
	state := c.ensureConsumptionState()
	if state == nil {
		return nil, nil, ""
	}
	input, output := state.GetConsumptionPointers()
	return input, output, state.GetConsumptionUUID()
}

func (c *Config) OriginOptions() []ConfigOption {
	return c.originOptions
}

func (c *Config) AICallbackAvailable() bool {
	return !(c.QualityPriorityAICallback == nil && c.SpeedPriorityAICallback == nil && c.OriginalAICallback == nil)
}

func (c *Config) InvokeLiteForge(prompt string, opts ...any) (*ForgeResult, error) {
	if c.SpeedPriorityAICallback != nil {
		opts = append(opts, WithFastAICallback(c.SpeedPriorityAICallback))
	} else {
		opts = append(opts, WithFastAICallback(c.QualityPriorityAICallback))
	}
	opts = append(opts, WithDisableCreateDBRuntime(true)) // Avoid creating runtime records for lite forge calls
	return InvokeLiteForge(prompt, opts...)
}

func (c *Config) buildStreamNodeIdI18nProvider() func(nodeId string) *schema.I18n {
	return func(nodeId string) *schema.I18n {
		prompt := fmt.Sprintf(`You are a UI localization assistant for an AI agent system.
Translate the following technical stream/node identifier into concise, user-friendly display names.
The identifier uses underscores or hyphens as word separators.

Identifier: %s

Requirements:
- Chinese (zh): A short, natural Chinese phrase (2-6 characters preferred)
- English (en): A short, capitalized English phrase`, nodeId)

		result, err := c.InvokeLiteForge(prompt,
			WithLiteForgeOutputSchemaFromAIToolOptions(
				aitool.WithStringParam("zh", aitool.WithParam_Description("Chinese user-friendly display name")),
				aitool.WithStringParam("en", aitool.WithParam_Description("English user-friendly display name")),
			))
		if err != nil {
			log.Infof("stream nodeId i18n provider skipped for %q: %v", nodeId, err)
			return nil
		}
		zh := result.GetString("zh")
		en := result.GetString("en")
		if zh == "" && en == "" {
			return nil
		}
		return &schema.I18n{Zh: zh, En: en}
	}
}
