package aireact

import (
	"context"
	"io"
	"strings"
	"sync"
	"sync/atomic"

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
)

type AICallbackType = aicommon.AICallbackType

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

	ctx    context.Context
	cancel context.CancelFunc

	// ID management
	id          string
	idSequence  int64
	idGenerator func() int64

	// Task interface
	task aicommon.AITask

	// AI callback for handling LLM calls
	aiCallback AICallbackType

	// Tool management
	aiToolManager       *buildinaitools.AiToolManager
	aiToolManagerOption []buildinaitools.ToolManagerOption

	// Event handling
	eventHandler func(e *schema.AiOutputEvent)
	debugEvent   bool
	debugPrompt  bool

	// Tool review and interaction
	enableToolReview bool                             // Enable tool use review
	reviewHandler    func(reviewInfo *ToolReviewInfo) // Custom review handler

	// Interactive features
	epm *aicommon.EndpointManager

	// Auto approve tool usage in non-interactive mode
	autoApproveTools bool

	// ReAct specific settings
	maxIterations     int
	maxThoughts       int
	maxActions        int
	temperatureThink  float32
	temperatureAction float32

	// Memory and state
	memory            *aid.Memory // Replace conversationHistory with Memory/Timeline
	cumulativeSummary string      // Cumulative summary for conversation memory
	currentIteration  int
	finished          bool
	language          string // Response language preference
	topToolsCount     int    // Number of top tools to display in prompt

	// Consumption tracking
	inputConsumption  *int64
	outputConsumption *int64

	// Retry settings
	aiTransactionAutoRetry int64

	// Synchronization
	mu sync.RWMutex

	// Output channel
	outputChan chan *schema.AiOutputEvent
}

type Option func(*ReActConfig)

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

// WithTools adds tools to the tool manager
func WithTools(tools ...*aitool.Tool) Option {
	return func(cfg *ReActConfig) {
		if cfg.aiToolManagerOption == nil {
			cfg.aiToolManagerOption = make([]buildinaitools.ToolManagerOption, 0)
		}
		cfg.aiToolManagerOption = append(cfg.aiToolManagerOption,
			buildinaitools.WithExtendTools(tools, true))
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

// WithMaxThoughts sets the maximum number of thoughts per iteration
func WithMaxThoughts(max int) Option {
	return func(cfg *ReActConfig) {
		if max > 0 {
			cfg.maxThoughts = max
		}
	}
}

// WithMaxActions sets the maximum number of actions per iteration
func WithMaxActions(max int) Option {
	return func(cfg *ReActConfig) {
		if max > 0 {
			cfg.maxActions = max
		}
	}
}

// WithTemperature sets the temperature for thinking and action phases
func WithTemperature(think, action float32) Option {
	return func(cfg *ReActConfig) {
		if think >= 0 && think <= 2.0 {
			cfg.temperatureThink = think
		}
		if action >= 0 && action <= 2.0 {
			cfg.temperatureAction = action
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

// WithTopToolsCount sets the number of top tools to display in prompt
func WithTopToolsCount(count int) Option {
	return func(cfg *ReActConfig) {
		if count > 0 {
			cfg.topToolsCount = count
		}
	}
}

// WithToolReview enables tool use review functionality
func WithToolReview(enabled bool) Option {
	return func(cfg *ReActConfig) {
		cfg.enableToolReview = enabled
	}
}

// WithReviewHandler sets a custom review handler for tool use review
func WithReviewHandler(handler func(reviewInfo *ToolReviewInfo)) Option {
	return func(cfg *ReActConfig) {
		cfg.reviewHandler = handler
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

func (cfg *ReActConfig) DoWaitAgree(ctx context.Context, endpoint *aicommon.Endpoint) {
	// In auto-approve mode, automatically approve the request
	if cfg.autoApproveTools {
		log.Infof("Auto-approving tool usage (non-interactive mode)")
		// Set default continue response
		endpoint.SetParams(aitool.InvokeParams{"suggestion": "continue"})
		endpoint.Release()
		return
	}

	// If tool review is enabled and we have a custom handler (non-interactive), auto-approve with logging
	if cfg.enableToolReview && cfg.reviewHandler != nil {
		log.Infof("Using custom review handler - auto-approving tool usage")

		// Extract tool information from endpoint if available
		materials := endpoint.GetReviewMaterials()
		if materials != nil {
			if toolName, ok := materials["tool"].(string); ok {
				log.Infof("Auto-approving tool: %s", toolName)
			}
			if toolDesc, ok := materials["tool_description"].(string); ok {
				log.Infof("Tool description: %s", toolDesc)
			}
		}

		// Auto-approve in CLI mode
		endpoint.SetParams(aitool.InvokeParams{"suggestion": "continue"})
		endpoint.Release()
		return
	}

	// Default behavior: wait for user interaction
	endpoint.Wait()
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
		ctx:        ctx,
		cancel:     cancel,
		id:         id,
		idSequence: atomic.AddInt64(idGenerator, 1000), // Start with offset
		idGenerator: func() int64 {
			return atomic.AddInt64(idGenerator, 1)
		},
		task:                   task,
		maxIterations:          10,
		maxThoughts:            3,
		maxActions:             5,
		temperatureThink:       0.7,
		temperatureAction:      0.3,
		memory:                 aid.GetDefaultMemory(), // Initialize with default memory
		currentIteration:       0,
		finished:               false,
		language:               "zh", // Default to Chinese
		topToolsCount:          20,   // Default to show top 20 tools
		outputChan:             make(chan *schema.AiOutputEvent, 100),
		aiToolManagerOption:    make([]buildinaitools.ToolManagerOption, 0),
		inputConsumption:       new(int64),
		outputConsumption:      new(int64),
		aiTransactionAutoRetry: 5,
	}

	// Initialize emitter
	config.Emitter = aicommon.NewEmitter(id, func(e *schema.AiOutputEvent) error {
		if config.eventHandler != nil {
			config.eventHandler(e)
		}
		return nil
	})

	// Initialize checkpoint storage
	config.BaseCheckpointableStorage = aicommon.NewCheckpointableStorageWithDB(id, consts.GetGormProjectDatabase())

	// Initialize endpoint manager
	config.epm = aicommon.NewEndpointManagerContext(ctx)
	config.epm.SetConfig(config)

	return config
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

// WithAutoApproveTools enables automatic tool approval (for non-interactive mode)
func WithAutoApproveTools() Option {
	return func(cfg *ReActConfig) {
		cfg.autoApproveTools = true
	}
}
