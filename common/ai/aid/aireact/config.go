package aireact

import (
	"context"
	"io"
	"strings"
	"sync"

	"github.com/yaklang/yaklang/common/ai"
	"github.com/yaklang/yaklang/common/ai/aid"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/ai/aid/aitool/buildinaitools"
	"github.com/yaklang/yaklang/common/ai/aid/aitool/buildinaitools/searchtools"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

type AICallbackType = aid.AICallbackType

type ReActConfig struct {
	ctx    context.Context
	cancel context.CancelFunc

	// AI callback for handling LLM calls
	aiCallback AICallbackType

	// Tool management
	aiToolManager       *buildinaitools.AiToolManager
	aiToolManagerOption []buildinaitools.ToolManagerOption

	// Event handling
	eventHandler func(e *ypb.AIOutputEvent)
	debugEvent   bool
	debugPrompt  bool

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

	// Synchronization
	mu sync.RWMutex

	// Output channel
	outputChan chan *ypb.AIOutputEvent
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
func WithAICallback(callback aid.AICallbackType) Option {
	return func(cfg *ReActConfig) {
		cfg.aiCallback = callback
	}
}

// WithEventHandler sets the event handler for output events
func WithEventHandler(handler func(e *ypb.AIOutputEvent)) Option {
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

// newReActConfig creates a new ReActConfig with default values
func newReActConfig(ctx context.Context) *ReActConfig {
	if ctx == nil {
		ctx = context.Background()
	}

	ctx, cancel := context.WithCancel(ctx)

	return &ReActConfig{
		ctx:                 ctx,
		cancel:              cancel,
		maxIterations:       10,
		maxThoughts:         3,
		maxActions:          5,
		temperatureThink:    0.7,
		temperatureAction:   0.3,
		memory:              aid.GetDefaultMemory(), // Initialize with default memory
		currentIteration:    0,
		finished:            false,
		language:            "zh", // Default to Chinese
		topToolsCount:       20,   // Default to show top 20 tools
		outputChan:          make(chan *ypb.AIOutputEvent, 100),
		aiToolManagerOption: make([]buildinaitools.ToolManagerOption, 0),
	}
}
