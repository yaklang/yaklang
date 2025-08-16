package aireact

import (
	"context"

	"github.com/yaklang/yaklang/common/ai/aid"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

type ReAct struct {
	config        *ReActConfig
	promptManager *PromptManager
	*aicommon.Emitter
}

func NewReAct(opts ...Option) (*ReAct, error) {
	cfg := NewReActConfig(context.Background(), opts...)

	react := &ReAct{
		config:  cfg,
		Emitter: cfg.Emitter, // Use the emitter from config
	}

	// Initialize prompt manager
	react.promptManager = NewPromptManager(react)

	// Initialize memory with AI capability
	if cfg.memory != nil && cfg.aiCallback != nil {
		// Note: Timeline AI caller will be set automatically when tools are used
		// No need to manually set it here

		// Store tools function
		cfg.memory.StoreTools(func() []*aitool.Tool {
			if cfg.aiToolManager == nil {
				return []*aitool.Tool{}
			}
			tools, err := cfg.aiToolManager.GetEnableTools()
			if err != nil {
				return []*aitool.Tool{}
			}
			return tools
		})
	}

	return react, nil
}

// UpdateDebugMode dynamically updates the debug mode settings
func (r *ReAct) UpdateDebugMode(debug bool) {
	r.config.mu.Lock()
	defer r.config.mu.Unlock()
	r.config.debugEvent = debug
	r.config.debugPrompt = debug
}

// ProcessQuery processes a query string directly and emits events via the configured event handler
func (r *ReAct) ProcessQuery(query string) error {
	if r.config.debugEvent {
		log.Infof("ReAct processing query: %s", query)
	}

	// Create a mock input event for compatibility
	event := &ypb.AITriageInputEvent{
		IsFreeInput: true,
		FreeInput:   query,
	}

	return r.processInputEvent(event)
}

// ProcessInputEvent processes a single input event directly
func (r *ReAct) ProcessInputEvent(event *ypb.AITriageInputEvent) error {
	if r.config.debugEvent {
		log.Infof("ReAct received input event: IsFreeInput=%v, FreeInput=%s", event.IsFreeInput, event.FreeInput)
	}

	return r.processInputEvent(event)
}

// processInputEvent processes a single input event and triggers ReAct loop
func (r *ReAct) processInputEvent(event *ypb.AITriageInputEvent) error {
	if r.config.debugEvent {
		log.Infof("Processing input event: IsFreeInput=%v, FreeInput=%s", event.IsFreeInput, event.FreeInput)
	}

	// Handle different types of input events
	var userInput string
	var shouldResetSession bool

	if event.IsFreeInput {
		userInput = event.FreeInput
		shouldResetSession = true // Reset session for new free input
		if r.config.debugEvent {
			log.Infof("Using free input: %s", userInput)
		}
	} else if event.IsStart && event.Params != nil {
		// Handle structured input from AIStartParams
		userInput = "Start new conversation"
		shouldResetSession = true
		if r.config.debugEvent {
			log.Info("Using start conversation input")
		}
	} else {
		// Handle other event types
		userInput = "No user input available"
		log.Warn("No valid input found in event")
	}

	// Reset session state if needed
	if shouldResetSession {
		r.config.mu.Lock()
		r.config.finished = false
		r.config.currentIteration = 0
		// Reset memory for new session
		r.config.memory = aid.GetDefaultMemory()
		// Re-initialize memory with tools and AI capability
		if r.config.memory != nil && r.config.aiCallback != nil {
			// Reset memory state for new session
			// No need to create a coordinator - just reset the memory
			r.config.memory.StoreTools(func() []*aitool.Tool {
				if r.config.aiToolManager == nil {
					return []*aitool.Tool{}
				}
				tools, err := r.config.aiToolManager.GetEnableTools()
				if err != nil {
					return []*aitool.Tool{}
				}
				return tools
			})
		}
		r.config.mu.Unlock()
		if r.config.debugEvent {
			log.Infof("Reset ReAct session for new input")
		}
	}

	// Execute the main ReAct loop using the new schema-based approach
	if r.config.debugEvent {
		log.Infof("Executing main loop with user input: %s", userInput)
	}
	return r.executeMainLoop(userInput)
}
