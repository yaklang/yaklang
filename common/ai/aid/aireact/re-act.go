package aireact

import (
	"context"
	"fmt"
	"io"

	"github.com/yaklang/yaklang/common/ai/aid"
	"github.com/yaklang/yaklang/common/ai/aid/aitool/buildinaitools"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
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

	react := &ReAct{config: cfg}
	return react, nil
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

	go func() {
		defer close(outputChan)
		defer func() {
			if r.config.cancel != nil {
				r.config.cancel()
			}
		}()

		for {
			select {
			case <-r.config.ctx.Done():
				log.Info("ReAct context cancelled, stopping processing")
				return
			case event, ok := <-input.OutputChannel():
				if !ok {
					log.Info("Input channel closed, stopping ReAct processing")
					return
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
	r.config.mu.Lock()
	defer r.config.mu.Unlock()

	if r.config.finished {
		return utils.Error("ReAct session has finished")
	}

	// Handle different types of input events
	var userInput string
	if event.IsFreeInput {
		userInput = event.FreeInput
		r.config.currentIteration = 0
		r.config.finished = false
	} else if event.IsStart && event.Params != nil {
		// Handle structured input from AIStartParams
		userInput = "Start new conversation"
	} else {
		// Handle other event types
		userInput = "No user input available"
	}

	// Execute the main ReAct loop using the new schema-based approach
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
		return ""
	}

	// Try to read from the response stream
	reader := resp.GetUnboundStreamReader(false)
	content, err := io.ReadAll(reader)
	if err == nil && len(content) > 0 {
		return string(content)
	}

	// Fallback for demo purposes
	return "AI response content placeholder"
}

// Legacy utility methods - may be removed in future versions

// Legacy event emission methods - redirected to emit.go implementation
// These are kept for backward compatibility
