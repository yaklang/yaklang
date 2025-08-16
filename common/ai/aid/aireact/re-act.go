package aireact

import (
	"context"
	"fmt"

	"github.com/tidwall/gjson"
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
		// Set the AI instance for memory timeline
		cfg.memory.SetTimelineAI(cfg)

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

	// Start the event loop in background
	react.startEventLoop(cfg.ctx)

	return react, nil
}

// UpdateDebugMode dynamically updates the debug mode settings
func (r *ReAct) UpdateDebugMode(debug bool) {
	r.config.mu.Lock()
	defer r.config.mu.Unlock()
	r.config.debugEvent = debug
	r.config.debugPrompt = debug
}

// SendInputEvent sends an input event to the event loop (non-blocking)
// This is the only public API for external clients to send input to ReAct
func (r *ReAct) SendInputEvent(event *ypb.AIInputEvent) error {
	if r.config.eventInputChan == nil {
		return fmt.Errorf("event input channel is not initialized")
	}

	select {
	case r.config.eventInputChan <- event:
		if r.config.debugEvent {
			log.Infof("ReAct event sent to channel: IsFreeInput=%v, IsInteractive=%v", event.IsFreeInput, event.IsInteractiveMessage)
		}
		return nil
	default:
		return fmt.Errorf("event input channel is full, event dropped")
	}
}

// processInputEvent processes a single input event and triggers ReAct loop
func (r *ReAct) processInputEvent(event *ypb.AIInputEvent) error {
	if r.config.debugEvent {
		log.Infof("Processing input event: IsFreeInput=%v, IsInteractive=%v", event.IsFreeInput, event.IsInteractiveMessage)
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
	} else if event.IsInteractiveMessage {
		// Handle interactive messages (tool review responses)
		if r.config.debugEvent {
			log.Infof("Processing interactive message: ID=%s", event.InteractiveId)
		}

		// Parse the interactive JSON input to get the suggestion
		suggestion := gjson.Get(event.InteractiveJSONInput, "suggestion").String()
		if suggestion == "" {
			suggestion = "continue" // Default fallback
		}

		// Feed the response to the endpoint manager
		if r.config.epm != nil {
			params := aitool.InvokeParams{
				"suggestion": suggestion,
			}
			r.config.epm.Feed(event.InteractiveId, params)
			if r.config.debugEvent {
				log.Infof("Fed interactive response to endpoint: ID=%s, suggestion=%s", event.InteractiveId, suggestion)
			}
		}

		return nil
	} else {
		log.Warnf("No valid input found in event")
		return nil
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
			// Set the AI instance for memory timeline
			r.config.memory.SetTimelineAI(r.config)

			// Store tools function
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

// startEventLoop starts the background event processing loop
func (r *ReAct) startEventLoop(ctx context.Context) {
	r.config.startInputEventOnce.Do(func() {
		go func() {
			if r.config.debugEvent {
				log.Infof("ReAct event loop started for instance: %s", r.config.id)
			}

			for {
				if r.config.eventInputChan == nil {
					if r.config.debugEvent {
						log.Warnf("ReAct event input channel is nil, will retry...")
					}
					<-ctx.Done()
					return
				}

				select {
				case event, ok := <-r.config.eventInputChan:
					if !ok {
						log.Errorf("ReAct event input channel closed for instance: %s", r.config.id)
						return
					}
					if event == nil {
						continue
					}

					if r.config.debugEvent {
						log.Infof("ReAct event loop processing event: IsFreeInput=%v, IsInteractive=%v",
							event.IsFreeInput, event.IsInteractiveMessage)
					}

					// Process the event in the background (non-blocking)
					go func(event *ypb.AIInputEvent) {
						if err := r.processInputEvent(event); err != nil {
							log.Errorf("ReAct event processing failed: %v", err)
						}
					}(event)

				case <-ctx.Done():
					if r.config.debugEvent {
						log.Infof("ReAct event loop stopped for instance: %s", r.config.id)
					}
					return
				}
			}
		}()
	})
}
