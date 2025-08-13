package aireact

import (
	"context"
	"fmt"
	"io"

	"github.com/yaklang/yaklang/common/ai/aid"
	"github.com/yaklang/yaklang/common/ai/aid/aitool/buildinaitools"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils/chanx"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

/*
TDD:

react, err = NewReAct(WithTool(...), WithContext(...)) //
if err != nil {
	return
}
react.Start()

react.Feed(ypb.AIInputEvent{}) // ...
react.FeedUserQuery()
react.FeedUserImage()

*/

type ReActInvoker interface {
	Invoke(input chan *ypb.AITriageInputEvent) (chan *ypb.AIOutputEvent, error)
	UnlimitedInvoke(input *chanx.UnlimitedChan[*ypb.AITriageInputEvent]) (chan *ypb.AIOutputEvent, error)
}

type ReAct struct {
	config *ReActConfig
	*ReActEmitter
}

func NewReAct(opts ...Option) (*ReAct, error) {
	cfg := newReActConfig(context.Background())
	for _, opt := range opts {
		opt(cfg)
	}

	// Initialize tool manager if not provided
	if cfg.aiToolManager == nil {
		cfg.aiToolManager = buildinaitools.NewToolManager(cfg.aiToolManagerOption...)
	}

	// Initialize emitter
	emitter := newReActEmitter("", "", "")

	react := &ReAct{
		config:       cfg,
		ReActEmitter: emitter,
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

func (r *ReAct) Invoke(input chan *ypb.AITriageInputEvent) (chan *ypb.AIOutputEvent, error) {
	ulc := chanx.NewUnlimitedChan[*ypb.AITriageInputEvent](r.config.ctx, 100)
	go func() {
		defer ulc.Close()
		for i := range input {
			ulc.SafeFeed(i)
		}
	}()

	return r.UnlimitedInvoke(ulc)
}

func (r *ReAct) UnlimitedInvoke(input *chanx.UnlimitedChan[*ypb.AITriageInputEvent]) (chan *ypb.AIOutputEvent, error) {
	outputChan := make(chan *ypb.AIOutputEvent, 100)

	if r.config.debugEvent {
		log.Info("ReAct UnlimitedInvoke starting goroutine")
	}

	go func() {
		defer close(outputChan)
		defer func() {
			if r.config.cancel != nil {
				r.config.cancel()
			}
			if r.config.debugEvent {
				log.Info("ReAct UnlimitedInvoke goroutine finished")
			}
		}()

		if r.config.debugEvent {
			log.Info("ReAct UnlimitedInvoke goroutine running, waiting for events")
		}

		for {
			select {
			case <-r.config.ctx.Done():
				if r.config.debugEvent {
					log.Info("ReAct context cancelled, stopping processing")
				}
				return
			case event, ok := <-input.OutputChannel():
				if !ok {
					if r.config.debugEvent {
						log.Info("Input channel closed, stopping ReAct processing")
					}
					return
				}

				if r.config.debugEvent {
					log.Infof("ReAct received input event: IsFreeInput=%v, FreeInput=%s", event.IsFreeInput, event.FreeInput)
				}

				if err := r.processInputEvent(event, outputChan); err != nil {
					log.Errorf("process input event failed: %v", err)
					r.emitError(outputChan, fmt.Sprintf("Process input event failed: %v", err))
				}
			}
		}
	}()

	return outputChan, nil
}

// processInputEvent processes a single input event and triggers ReAct loop
func (r *ReAct) processInputEvent(event *ypb.AITriageInputEvent, outputChan chan *ypb.AIOutputEvent) error {
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
		r.config.conversationHistory = make([]string, 0)
		r.config.mu.Unlock()
		if r.config.debugEvent {
			log.Infof("Reset ReAct session for new input")
		}
	}

	// Execute the main ReAct loop using the new schema-based approach
	if r.config.debugEvent {
		log.Infof("Executing main loop with user input: %s", userInput)
	}
	return r.executeMainLoop(userInput, outputChan)
}

// Legacy method - replaced by executeMainLoop in invoke.go
// Kept for backward compatibility, but redirects to new implementation
func (r *ReAct) startReActLoop(userQuery string, outputChan chan *ypb.AIOutputEvent) error {
	return r.executeMainLoop(userQuery, outputChan)
}

// Legacy methods - replaced by new schema-based implementation in invoke.go
// These are kept for potential compatibility but may be removed in future versions

// extractResponseContent extracts content from AI response
func (r *ReAct) extractResponseContent(resp *aid.AIResponse) string {
	if resp == nil {
		log.Error("AI response is nil")
		return ""
	}

	// Use the same method as other parts of the system
	// Create a temporary aid.Config for the response reader
	tempConfig := aid.NewConfig(r.config.ctx)
	reader := resp.GetOutputStreamReader("react-response", false, tempConfig)
	if reader == nil {
		log.Error("Failed to get output stream reader")
		return ""
	}

	content, err := io.ReadAll(reader)
	if err != nil {
		log.Errorf("Failed to read AI response: %v", err)
		return ""
	}

	contentStr := string(content)
	if r.config.debugEvent {
		log.Infof("AI response content: %s", contentStr)
	}

	if len(contentStr) == 0 {
		log.Warn("AI response content is empty - this should trigger error learning")
		// Don't return a hardcoded response - let the error propagate so error learning can work
		return ""
	}

	return contentStr
}

// Legacy utility methods - may be removed in future versions

// Legacy event emission methods - redirected to emit.go implementation
// These are kept for backward compatibility
