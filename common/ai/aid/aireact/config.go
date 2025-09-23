package aireact

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/yaklang/yaklang/common/aiforge"
	"github.com/yaklang/yaklang/common/utils/chanx"

	"github.com/google/uuid"
	"github.com/yaklang/yaklang/common/ai"
	"github.com/yaklang/yaklang/common/ai/aid"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/ai/aid/aitool/buildinaitools"
	"github.com/yaklang/yaklang/common/ai/aid/aitool/buildinaitools/searchtools"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

type AICallbackType = aicommon.AICallbackType

// contextProviderEntry holds pending context provider registration info
type contextProviderEntry struct {
	name     string
	provider aicommon.ContextProvider
	traced   bool // Whether to register as traced provider
}

// ToolReviewInfo contains information needed for tool use review
type ToolReviewInfo struct {
	Tool            *aitool.Tool
	Params          aitool.InvokeParams
	ID              string
	ResponseChannel chan *ToolReviewResponse
}

// ToolReviewResponse contains the user's review decision
type ToolReviewResponse struct {
	Suggestion        string              // continue, wrong_tool, wrong_params, direct_answer
	ExtraPrompt       string              // Additional user prompt
	SuggestionTool    string              // Suggested tool name for wrong_tool
	SuggestionKeyword string              // Suggested keyword for tool search
	ModifiedParams    aitool.InvokeParams // Modified parameters for wrong_params
	OverrideResult    *aitool.ToolResult  // Override result
	DirectlyAnswer    bool                // Skip tool and answer directly
	Cancel            bool                // Cancel the operation
}

// ReactTask implements aicommon.AITask interface for ReAct
type ReactTask struct {
	index string
	name  string
}

func (t *ReactTask) GetIndex() string {
	return t.index
}

func (t *ReactTask) GetName() string {
	return t.name
}

type ReActConfig struct {
	*aicommon.Emitter
	*aicommon.BaseCheckpointableStorage

	disableEnhanceDirectlyAnswer bool
	directlyAnswerEnhanceHandle  func(ctx context.Context, query string) (<-chan aicommon.EnhanceKnowledge, error)

	promptManager           *PromptManager         // Prompt manager for ReAct
	pendingContextProviders []contextProviderEntry // Pending context providers to register

	cfgMutex sync.Mutex

	// Task interface
	task aicommon.AITask // prepared for toolcall

	ctx    context.Context
	cancel context.CancelFunc

	// Event loop management
	startInputEventOnce sync.Once
	eventInputChan      *chanx.UnlimitedChan[*ypb.AIInputEvent]

	// ID management
	id          string
	idSequence  int64
	idGenerator func() int64

	// AI callback for handling LLM calls
	aiCallback AICallbackType

	toolKeywords []string // Keywords for tool suggestions

	// Tool management
	disableToolUse      bool
	aiToolManager       *buildinaitools.AiToolManager
	aiToolManagerOption []buildinaitools.ToolManagerOption
	aiBlueprintManager  *aiforge.ForgeFactory

	// Event handling
	eventHandler func(e *schema.AiOutputEvent)
	debugEvent   bool
	debugPrompt  bool

	// Tool review and interaction
	enableUserInteract bool // Enable user interaction for tool review
	enablePlanAndExec  bool // Enable plan and execution mode

	// Interactive features
	epm *aicommon.EndpointManager

	// Auto approve tool usage in non-interactive mode
	reviewPolicy aicommon.AgreePolicyType

	// ReAct specific settings
	maxIterations int

	// Memory and state
	memory        *aid.Memory // Replace conversationHistory with Memory/Timeline
	language      string      // Response language preference
	topToolsCount int         // Number of top tools to display in prompt

	// Consumption tracking
	inputConsumption  *int64
	outputConsumption *int64

	// field config
	aiTransactionAutoRetry   int64
	timelineLimit            int64 // Limit for timeline records
	timelineContentSizeLimit int64 // Limit for timeline content size

	// async Guardian
	guardian *aicommon.AsyncGuardian

	userInteractiveLimitedTimes int64

	workdir string

	// Plan and execute hijack function for advanced usage/testing
	hijackPlanRequest func(ctx context.Context, planPayload string) error
}

func WithReActWorkdir(dir string) Option {
	return func(opt *ReActConfig) {
		existedDir := utils.IsDir(dir)
		if !existedDir {
			log.Warnf("%s is not a directory", dir)
			return
		}
		if filepath.IsAbs(dir) {
			opt.workdir = dir
			return
		}

		dir, err := filepath.Abs(dir)
		if err != nil {
			log.Warnf("%s is not a directory", dir)
			return
		}
		if dir != "" {
			opt.workdir = dir
		}
	}
}

func (cfg *ReActConfig) GetUserInteractiveLimitedTimes() int64 {
	if cfg.userInteractiveLimitedTimes <= 0 {
		return 3 // Default to 100 if not set
	}
	return cfg.userInteractiveLimitedTimes
}

func (cfg *ReActConfig) GetTimelineRecordLimit() int64 {
	return cfg.timelineLimit
}

func (cfg *ReActConfig) GetTimelineContentSizeLimit() int64 {
	return cfg.timelineContentSizeLimit
}

type Option func(*ReActConfig)

func WithKnowledgeEnhanceHandle(f func(ctx context.Context, query string) (<-chan aicommon.EnhanceKnowledge, error)) Option {
	return func(cfg *ReActConfig) {
		cfg.directlyAnswerEnhanceHandle = f
	}
}

// WithContext sets the context for ReAct
func WithContext(ctx context.Context) Option {
	return func(cfg *ReActConfig) {
		if ctx != nil {
			cfg.ctx = ctx
		}
	}
}

// WithAICallback sets the AI callback for LLM interactions
func WithAICallback(callback aicommon.AICallbackType) Option {
	return func(cfg *ReActConfig) {
		cfg.aiCallback = callback
	}
}

// WithEventHandler sets the event handler for output events
func WithEventHandler(handler func(e *schema.AiOutputEvent)) Option {
	return func(cfg *ReActConfig) {
		cfg.eventHandler = handler
	}
}

// WithEventInputChan sets the event input channel for ReAct
func WithEventInputChan(ch chan *ypb.AIInputEvent) Option {
	return func(cfg *ReActConfig) {
		if ch != nil {
			go func() {
				for event := range ch {
					if cfg.debugEvent {
						log.Debugf("Received event: %s", event)
					}
					cfg.eventInputChan.SafeFeed(event)
				}
			}()
		}
	}
}

// WithToolManager sets a custom tool manager
func WithToolManager(manager *buildinaitools.AiToolManager) Option {
	return func(cfg *ReActConfig) {
		cfg.aiToolManager = manager
	}
}

// WithDebug enables debug mode
func WithDebug(enabled ...bool) Option {
	return func(cfg *ReActConfig) {
		debugEnabled := true
		if len(enabled) > 0 {
			debugEnabled = enabled[0]
		}
		cfg.debugEvent = debugEnabled
		cfg.debugPrompt = debugEnabled
		// Also control global debug mode for system logs
		if debugEnabled {
			log.SetLevel(log.DebugLevel)
		} else {
			log.SetLevel(log.InfoLevel)
		}
	}
}

// WithMaxIterations sets the maximum number of ReAct iterations
func WithMaxIterations(max int) Option {
	return func(cfg *ReActConfig) {
		if max > 0 {
			cfg.maxIterations = max
		}
	}
}

func WithTimelineLimit(limit int64) Option {
	return func(cfg *ReActConfig) {
		if limit > 0 {
			cfg.timelineLimit = limit
		}
	}
}

func WithTimelineContentSizeLimit(limit int64) Option {
	return func(cfg *ReActConfig) {
		if limit > 0 {
			cfg.timelineContentSizeLimit = limit
		}
	}
}

// WithSystemFileOperator adds system file operation tools
func WithSystemFileOperator() Option {
	return func(cfg *ReActConfig) {
		// This will be populated when we import the necessary tools
		log.Info("System file operator tools will be added")
	}
}

// WithLanguage sets the response language preference
func WithLanguage(lang string) Option {
	return func(cfg *ReActConfig) {
		cfg.language = lang
	}
}

func WithReviewPolicy(policy aicommon.AgreePolicyType) Option {
	return func(cfg *ReActConfig) {
		switch policy {
		case aicommon.AgreePolicyYOLO, aicommon.AgreePolicyAuto, aicommon.AgreePolicyAI, aicommon.AgreePolicyAIAuto, aicommon.AgreePolicyManual:
			cfg.reviewPolicy = policy
		default:
			log.Warnf("Invalid review policy: %s, defaulting to manual", policy)
			cfg.reviewPolicy = aicommon.AgreePolicyManual
		}
	}
}

func WithDisableToolUse(i ...bool) Option {
	return func(cfg *ReActConfig) {
		disable := true
		if len(i) > 0 {
			disable = i[0]
		}
		cfg.disableToolUse = disable
	}
}

// WithTopToolsCount sets the number of top tools to display in prompt
func WithTopToolsCount(count int) Option {
	return func(cfg *ReActConfig) {
		if count > 0 {
			cfg.topToolsCount = count
		}
	}
}

func WithAiToolsSearchTool() Option {
	return func(cfg *ReActConfig) {
		if cfg.aiToolManagerOption == nil {
			cfg.aiToolManagerOption = make([]buildinaitools.ToolManagerOption, 0)
		}
		// Enable tool search functionality
		cfg.aiToolManagerOption = append(cfg.aiToolManagerOption,
			buildinaitools.WithSearchEnabled(true),
		)
	}
}

func WithEnableToolsName(toolsName ...string) Option {
	return func(cfg *ReActConfig) {
		cfg.aiToolManagerOption = append(cfg.aiToolManagerOption, buildinaitools.WithEnabledTools(toolsName))
	}
}

func WithToolKeywords(keywords ...string) Option {
	return func(cfg *ReActConfig) {
		cfg.toolKeywords = append(cfg.toolKeywords, keywords...)
	}
}

func WithDisableToolsName(toolsName ...string) Option {
	return func(config *ReActConfig) {
		config.aiToolManagerOption = append(config.aiToolManagerOption, buildinaitools.WithDisableTools(toolsName))
	}
}

func WithTools(tool ...*aitool.Tool) Option {
	return func(cfg *ReActConfig) {
		if cfg.aiToolManagerOption == nil {
			cfg.aiToolManagerOption = make([]buildinaitools.ToolManagerOption, 0)
		}
		cfg.aiToolManagerOption = append(cfg.aiToolManagerOption,
			buildinaitools.WithExtendTools(tool, true))
	}
}

// WithBuiltinTools adds all builtin AI tools including search capabilities
func WithBuiltinTools() Option {
	return func(cfg *ReActConfig) {
		if cfg.aiToolManagerOption == nil {
			cfg.aiToolManagerOption = make([]buildinaitools.ToolManagerOption, 0)
		}

		// Get all builtin tools
		allTools := buildinaitools.GetAllTools()

		// Create a simple AI chat function for the searcher
		aiChatFunc := func(prompt string) (io.Reader, error) {
			response, err := ai.Chat(prompt)
			if err != nil {
				return nil, err
			}
			return strings.NewReader(response), nil
		}

		// Create keyword searcher
		keywordSearcher := searchtools.NewKeyWordSearcher[*aitool.Tool](aiChatFunc)

		// Enable tool search functionality
		cfg.aiToolManagerOption = append(cfg.aiToolManagerOption,
			buildinaitools.WithExtendTools(allTools, true),
			buildinaitools.WithSearchEnabled(true),
			buildinaitools.WithSearcher(keywordSearcher),
		)

		log.Infof("Added %d builtin AI tools with search capability", len(allTools))
	}
}

// Implement AICallerConfigIf interface
func (cfg *ReActConfig) AcquireId() int64 {
	return cfg.idGenerator()
}

func (cfg *ReActConfig) GetRuntimeId() string {
	return cfg.id
}

func (cfg *ReActConfig) IsCtxDone() bool {
	select {
	case <-cfg.ctx.Done():
		return true
	default:
		return false
	}
}

func (cfg *ReActConfig) GetContext() context.Context {
	return cfg.ctx
}

func (cfg *ReActConfig) CallAIResponseConsumptionCallback(current int) {
	atomic.AddInt64(cfg.outputConsumption, int64(current))
}

func (cfg *ReActConfig) GetAITransactionAutoRetryCount() int64 {
	return cfg.aiTransactionAutoRetry
}

func (cfg *ReActConfig) RetryPromptBuilder(originalPrompt string, err error) string {
	if err == nil {
		return originalPrompt
	}
	return originalPrompt + "\n\n[Retry due to error: " + err.Error() + "]"
}

func (cfg *ReActConfig) GetEmitter() *aicommon.Emitter {
	return cfg.Emitter
}

func (cfg *ReActConfig) NewAIResponse() *aicommon.AIResponse {
	return aicommon.NewAIResponse(cfg)
}

func (cfg *ReActConfig) CallAIResponseOutputFinishedCallback(s string) {
	// Process any extended actions in the response
	log.Debugf("AI response finished: %s", s)
}

// Implement Interactivable interface
func (cfg *ReActConfig) Feed(endpointId string, params aitool.InvokeParams) {
	if cfg.epm != nil {
		cfg.epm.Feed(endpointId, params)
	}
}

func (cfg *ReActConfig) GetEndpointManager() *aicommon.EndpointManager {
	if cfg.epm == nil {
		cfg.epm = aicommon.NewEndpointManager()
	}
	return cfg.epm
}

func (cfg *ReActConfig) CallAfterReview(seq int64, reviewQuestion string, userInput aitool.InvokeParams) {
	if cfg.memory != nil {
		cfg.memory.PushUserInteraction(aicommon.UserInteractionStage_Review, seq, reviewQuestion, string(utils.Jsonify(userInput)))
	}
}

func (cfg *ReActConfig) CallAfterInteractiveEventReleased(eventID string, invoke aitool.InvokeParams) {
	// Store interactive user input
	if cfg.memory != nil {
		cfg.memory.StoreInteractiveUserInput(eventID, invoke)
	}
}

// Implement AICaller interface
func (cfg *ReActConfig) CallAI(request *aicommon.AIRequest) (*aicommon.AIResponse, error) {
	if cfg.aiCallback == nil {
		return nil, utils.Error("AI callback is not configured")
	}

	// Call the configured AI callback
	return cfg.aiCallback(cfg, request)
}

// Legacy methods for backward compatibility
func (cfg *ReActConfig) callAI(prompt string, opts ...aicommon.AIRequestOption) (*aicommon.AIResponse, error) {
	req := aicommon.NewAIRequest(prompt, opts...)
	return cfg.CallAI(req)
}

// CreateToolCaller creates a ToolCaller for this ReAct config
func (cfg *ReActConfig) CreateToolCaller() (*aicommon.ToolCaller, error) {
	return aicommon.NewToolCaller(
		aicommon.WithToolCaller_AICallerConfig(cfg),
		aicommon.WithToolCaller_AICaller(cfg),
		aicommon.WithToolCaller_Task(cfg.task),
		aicommon.WithToolCaller_RuntimeId(cfg.id),
		aicommon.WithToolCaller_Emitter(cfg.Emitter),
	)
}

// newReActConfig creates a new ReActConfig with default values
func newReActConfig(ctx context.Context) *ReActConfig {
	if ctx == nil {
		ctx = context.Background()
	}

	ctx, cancel := context.WithCancel(ctx)
	id := uuid.New().String()

	// Initialize ID generator
	var idGenerator = new(int64)

	// Create task
	task := &ReactTask{
		index: id,
		name:  "react-task",
	}

	config := &ReActConfig{
		task:                task,
		ctx:                 ctx,
		cancel:              cancel,
		startInputEventOnce: sync.Once{},
		eventInputChan:      chanx.NewUnlimitedChan[*ypb.AIInputEvent](ctx, 2),
		id:                  id,
		idSequence:          atomic.AddInt64(idGenerator, 1000), // Start with offset
		idGenerator: func() int64 {
			return atomic.AddInt64(idGenerator, 1)
		},
		reviewPolicy:                aicommon.AgreePolicyManual,
		maxIterations:               100,
		memory:                      aid.GetDefaultMemory(), // Initialize with default memory
		language:                    "zh",                   // Default to Chinese
		topToolsCount:               100,                    //
		inputConsumption:            new(int64),
		outputConsumption:           new(int64),
		aiTransactionAutoRetry:      5,
		timelineLimit:               100,       // Default limit for timeline records
		timelineContentSizeLimit:    50 * 1024, // Default limit for 50k
		guardian:                    aicommon.NewAsyncGuardian(ctx, id),
		userInteractiveLimitedTimes: 3, // Default to 3 times
		enablePlanAndExec:           true,
		enableUserInteract:          true,
		workdir:                     consts.GetDefaultYakitBaseDir(),

		// aiforge manager
		aiBlueprintManager: aiforge.NewForgeFactory(),
	}

	// Initialize emitter
	config.Emitter = aicommon.NewEmitter(id, func(e *schema.AiOutputEvent) error {
		config.guardian.Feed(e)
		config.emitBaseHandler(e)
		return nil
	})

	// Initialize checkpoint storage
	config.BaseCheckpointableStorage = aicommon.NewCheckpointableStorageWithDB(id, consts.GetGormProjectDatabase())

	// Initialize endpoint manager
	config.epm = aicommon.NewEndpointManagerContext(ctx)
	config.epm.SetConfig(config)
	config.memory.GetTimelineInstance().BindConfig(config, config)

	return config
}

func (c *ReActConfig) emitBaseHandler(e *schema.AiOutputEvent) {
	select {
	case <-c.ctx.Done():
		return
	default:
	}
	c.cfgMutex.Lock()
	defer c.cfgMutex.Unlock()

	if e.ShouldSave() {
		err := yakit.CreateAIEvent(consts.GetGormProjectDatabase(), e)
		if err != nil {
			log.Errorf("create AI event failed: %v", err)
		}
	}

	if c.guardian != nil {
		c.guardian.Feed(e)
	}

	if c.eventHandler == nil {
		if e.IsStream {
			if c.debugEvent {
				fmt.Print(string(e.StreamDelta))
			}
			return
		}

		if e.Type == schema.EVENT_TYPE_CONSUMPTION {
			if c.debugEvent {
				log.Info(e.String())
			}
			return
		}
		if c.debugEvent {
			log.Info(e.String())
		} else {
			//log.Info(utils.ShrinkString(e.String(), 200))
		}
		return
	}
	c.eventHandler(e)
}

// NewReActConfig creates a new ReActConfig with options
func NewReActConfig(ctx context.Context, opts ...Option) *ReActConfig {
	config := newReActConfig(ctx)

	// Apply options
	for _, opt := range opts {
		opt(config)
	}

	// Initialize tool manager if not set
	if config.aiToolManager == nil {
		config.aiToolManager = buildinaitools.NewToolManager(config.aiToolManagerOption...)
	}

	return config
}

func WithGuardianEventTrigger(i schema.EventType, callback aicommon.GuardianEventTrigger) Option {
	return func(cfg *ReActConfig) {
		cfg.guardian.RegisterEventTrigger(i, func(event *schema.AiOutputEvent, emitter aicommon.GuardianEmitter, aicaller aicommon.AICaller) {
			callback(event, emitter, aicaller)
		})
	}
}

func WithGuardianStreamTrigger(nodeId string, trigger aicommon.GuardianMirrorStreamTrigger) Option {
	return func(cfg *ReActConfig) {
		cfg.guardian.RegisterMirrorStreamTrigger(nodeId, func(unlimitedChan *chanx.UnlimitedChan[*schema.AiOutputEvent], emitter aicommon.GuardianEmitter) {
			trigger(unlimitedChan, emitter)
		})
	}
}

func WithUserInteractiveLimitedTimes(times int64) Option {
	return func(cfg *ReActConfig) {
		if times > 0 {
			cfg.userInteractiveLimitedTimes = times
		} else {
			cfg.userInteractiveLimitedTimes = 3 // Default to 3 if not set
		}
	}
}

func WithUserInteractive(enabled ...bool) Option {
	return func(cfg *ReActConfig) {
		cfg.enableUserInteract = true
		if len(enabled) > 0 {
			cfg.enableUserInteract = enabled[0]
		}
	}
}

func WithReActAllowPlanAndExec(enabled ...bool) Option {
	return func(cfg *ReActConfig) {
		// This is a placeholder for future implementation
		cfg.enablePlanAndExec = true
		if len(enabled) > 0 {
			cfg.enablePlanAndExec = enabled[0]
		}
	}
}

func WithReActHijackPlanRequest(f func(ctx context.Context, planPayload string) error) Option {
	return func(cfg *ReActConfig) {
		if f != nil {
			cfg.hijackPlanRequest = f
		}
	}
}

func WithDynamicContextProvider(name string, provider aicommon.ContextProvider) Option {
	return func(cfg *ReActConfig) {
		if name != "" && provider != nil {
			// If promptManager is already available, register directly
			if cfg.promptManager != nil {
				cfg.promptManager.cpm.Register(name, provider)
			} else {
				// Otherwise, add to pending list for later registration
				cfg.pendingContextProviders = append(cfg.pendingContextProviders, contextProviderEntry{
					name:     name,
					provider: provider,
				})
			}
		}
	}
}

// WithTracedDynamicContextProvider registers a dynamic context provider with tracing capabilities
// It tracks changes between calls and provides diff information
func WithTracedDynamicContextProvider(name string, provider aicommon.ContextProvider) Option {
	return func(cfg *ReActConfig) {
		if name != "" && provider != nil {
			// If promptManager is already available, register directly
			if cfg.promptManager != nil {
				cfg.promptManager.cpm.RegisterTracedContent(name, provider)
			} else {
				// Otherwise, add to pending list for later registration
				cfg.pendingContextProviders = append(cfg.pendingContextProviders, contextProviderEntry{
					name:     name,
					provider: provider,
					traced:   true, // Mark as traced
				})
			}
		}
	}
}

// WithTracedFileContext monitors a file and provides its content as context with change tracking
func WithTracedFileContext(name string, filePath string) Option {
	return func(cfg *ReActConfig) {
		if name != "" && filePath != "" {
			provider := func(config aicommon.AICallerConfigIf, emitter *aicommon.Emitter, key string) (string, error) {
				contentBytes, err := os.ReadFile(filePath)
				if err != nil {
					return "", fmt.Errorf("failed to read file %s: %w", filePath, err)
				}
				content := string(contentBytes)
				return fmt.Sprintf("File: %s\nContent:\n%s", filePath, content), nil
			}

			// If promptManager is already available, register directly
			if cfg.promptManager != nil {
				cfg.promptManager.cpm.RegisterTracedContent(name, provider)
			} else {
				// Otherwise, add to pending list for later registration
				cfg.pendingContextProviders = append(cfg.pendingContextProviders, contextProviderEntry{
					name:     name,
					provider: provider,
					traced:   true, // Mark as traced
				})
			}
		}
	}
}
