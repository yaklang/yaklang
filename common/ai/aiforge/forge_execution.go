package aiforge

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/coordinator"
	"github.com/yaklang/yaklang/common/ai/aid/liteforge"
	"github.com/yaklang/yaklang/common/jsonextractor"
	"github.com/yaklang/yaklang/common/utils"
)

// ForgeExecution preserves the Forge-facing API while Session owns scheduling,
// approval, worker attempts and completion. Results belong to this invocation.
type ForgeExecution struct {
	*coordinator.Session
	blueprint      *ForgeBlueprint
	query          string
	resultParams   *ForgePromptParams
	mu             sync.Mutex
	runOnce        sync.Once
	callbackOnce   sync.Once
	result         ForgeResult
	err            error
	resultCallback func(*ForgeExecution)
}

type executorResultCallback struct{ callback func(*ForgeExecution) }

func WithExecutorResultHandler(handler func(*ForgeExecution)) aicommon.ConfigOption {
	return aicommon.WithAppendOtherOption(executorResultCallback{handler})
}

func (f *ForgeBlueprint) newExecution(ctx context.Context, query string, opts []aicommon.ConfigOption) (*ForgeExecution, error) {
	e := &ForgeExecution{blueprint: f, query: query, result: ForgeResult{Forge: f}}

	opts = append(append([]aicommon.ConfigOption{}, opts...), coordinator.WithResultDelivery(e.deliver))
	s, err := coordinator.NewForgeSession(ctx, query, opts...)
	if err != nil {
		return nil, err
	}
	e.Session = s
	for _, opt := range s.OtherOption {
		if callback, ok := opt.(executorResultCallback); ok {
			e.resultCallback = callback.callback
		}
	}
	return e, nil
}

func (e *ForgeExecution) GetConfig() *aicommon.Config   { return e.Config }
func (e *ForgeExecution) GetAIConfig() *aicommon.Config { return e.Config }
func (e *ForgeExecution) GetContextProvider() *coordinator.ContextSnapshot {
	return e.ContextSnapshot(e.query, e.Snapshot())
}

func (e *ForgeExecution) Result() *ForgeResult {
	e.mu.Lock()
	defer e.mu.Unlock()
	r := e.result
	// Action is copied as data, never shared with another run or the caller.
	if r.Action != nil {
		raw, _ := json.Marshal(r.Action.GetParams())
		r.Action, _ = aicommon.ExtractAction(string(raw), r.Action.Name())
	}
	return &r
}

func (e *ForgeExecution) Run() error {
	e.runOnce.Do(func() {
		e.err = e.Session.Run()
		if e.err == nil && e.Snapshot().Report.DeliveryPath != "" && e.Result().Formated == nil {
			raw, err := os.ReadFile(e.Snapshot().Report.DeliveryPath)
			if err != nil {
				e.err = fmt.Errorf("restore Forge business result: %w", err)
			} else {
				result := ForgeResult{Forge: e.blueprint, Formated: string(raw)}
				if len(e.blueprint.ResultActionNames) > 0 {
					result.Action, e.err = aicommon.ExtractAction(string(raw), e.blueprint.ResultActionNames[0], e.blueprint.ResultActionNames[1:]...)
				}
				e.mu.Lock()
				e.result = result
				e.mu.Unlock()
			}
		}
		if e.err != nil || e.Snapshot().Finished {
			e.finish(e.err)
		}
	})
	return e.err
}

func (e *ForgeExecution) finish(err error) {
	e.callbackOnce.Do(func() {
		if e.blueprint.ResultHandler != nil {
			e.blueprint.ResultHandler(utils.InterfaceToString(e.Result().Formated), err)
		}
		if e.resultCallback != nil {
			e.resultCallback(e)
		}
	})
}

func (e *ForgeExecution) deliver(ctx context.Context, _ *coordinator.Session, state coordinator.Snapshot) (coordinator.Delivery, error) {
	view := e.ContextSnapshot(e.query, state)
	summary := ""
	if view.RootTask != nil {
		summary = view.RootTask.TaskSummary
	}
	result := &ForgeResult{Forge: e.blueprint, Formated: summary}
	contentType := "text/plain"
	var err error
	if e.blueprint.ResultPrompt != "" {
		prompt, renderErr := e.blueprint.renderResultPromptWithInvocation(view, e.resultParams)
		if renderErr != nil {
			return coordinator.Delivery{}, fmt.Errorf("render Forge result prompt: %w", renderErr)
		}
		generate := e.blueprint.ResultGenerator
		// A custom generator owns its context composition (e.g. Legion's input
		// and evidence appendix); the default formatter receives the snapshot once.
		if generate == nil {
			prompt += "\n\n<execution_material>\n" + view.ResultMaterial() + "\n</execution_material>"
		}
		if generate != nil {
			result.Formated, err = generate(e, prompt)
		} else if len(e.blueprint.ResultActionNames) == 0 {
			result.Formated, err = e.blueprint.GenerateResult(e, prompt)
		} else {
			name := e.blueprint.ResultActionNames[0]
			opts := aicommon.ConvertConfigToOptionsWithoutHotPatch(e.Config)
			opts = append(opts, aicommon.WithAICallbacks(e.GetRawAICallbacks()), aicommon.WithEnableFunctionCallMode(e.EnableFunctionCallMode))
			output, callErr := liteforge.Execute(ctx, liteforge.Request{Name: e.blueprint.Name + "-result", ActionName: name, ActionAliases: e.blueprint.ResultActionNames[1:],
				Schema: forgeResultSchema(e.blueprint.ResultPrompt, name), Prompt: prompt, DisableTimeline: true, Emitter: e.GetEmitter()}, opts...)
			err = callErr
			if output != nil {
				result.Action = output.Action
				raw, _ := json.Marshal(output.Action.GetParams())
				result.Formated = string(raw)
			}
		}
		if err == nil && result.Action == nil && len(e.blueprint.ResultActionNames) > 0 {
			result.Action, err = aicommon.ExtractAction(utils.InterfaceToString(result.Formated), e.blueprint.ResultActionNames[0], e.blueprint.ResultActionNames[1:]...)
		}
		if result.Action != nil {
			contentType = "application/json"
		}
	}
	e.mu.Lock()
	e.result = *result
	e.mu.Unlock()
	return coordinator.Delivery{Content: utils.InterfaceToString(result.Formated), ContentType: contentType, Summary: summary}, err
}

// Explicit schemas in old result templates remain authoritative. Templates
// that only name an action retain their open business object, including arrays
// and unknown nested values; inventing a string-only schema would lose data.
func forgeResultSchema(prompt, name string) string {
	for _, raw := range jsonextractor.ExtractStandardJSON(prompt) {
		var candidate map[string]any
		if json.Unmarshal([]byte(raw), &candidate) == nil && candidate["type"] == "object" && candidate["properties"] != nil {
			return raw
		}
	}
	data, _ := json.Marshal(map[string]any{"type": "object", "properties": map[string]any{"@action": map[string]any{"type": "string", "const": name}}, "additionalProperties": true})
	return strings.TrimSpace(string(data))
}
