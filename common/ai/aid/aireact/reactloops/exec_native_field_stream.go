package reactloops

import (
	"io"
	"strings"
	"sync"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/jsonextractor"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/omap"
)

// nativeFieldStreamAttempt tracks one provider response. Top-level fields are
// emitted while arguments arrive; nested-only legacy fields are emitted after
// validation. A new attempt is created for every retry/provider response.
type nativeFieldStreamAttempt struct {
	loop    *ReActLoop
	allowed map[string]bool
	mu      sync.Mutex
	emitted map[string]map[string]bool
	err     error
}

func (r *ReActLoop) nativeStreamFields(action *LoopAction) *omap.OrderedMap[string, *LoopStreamField] {
	fields := omap.NewEmptyOrderedMap[string, *LoopStreamField]()
	if r.streamFields != nil {
		fields = r.streamFields.Copy()
	}
	for _, field := range action.StreamFields {
		if field != nil && field.FieldName != "" {
			fields.Set(field.FieldName, field)
		}
	}
	return fields
}

func newNativeFieldStreamAttempt(loop *ReActLoop, advertised []string) *nativeFieldStreamAttempt {
	var allowed map[string]bool
	if advertised != nil {
		allowed = make(map[string]bool, len(advertised))
		for _, name := range advertised {
			allowed[name] = true
		}
	}
	return &nativeFieldStreamAttempt{loop: loop, allowed: allowed, emitted: make(map[string]map[string]bool)}
}

func (a *nativeFieldStreamAttempt) mark(id, field string) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.emitted[id] == nil {
		a.emitted[id] = make(map[string]bool)
	}
	if a.emitted[id][field] {
		return false
	}
	a.emitted[id][field] = true
	return true
}

func (a *nativeFieldStreamAttempt) unmark(id, field string) {
	a.mu.Lock()
	delete(a.emitted[id], field)
	a.mu.Unlock()
}

func (a *nativeFieldStreamAttempt) wasEmitted(id, field string) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.emitted[id][field]
}

func (a *nativeFieldStreamAttempt) setError(err error) {
	if err == nil {
		return
	}
	a.mu.Lock()
	if a.err == nil {
		a.err = err
	}
	a.mu.Unlock()
}

func (a *nativeFieldStreamAttempt) error() error {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.err
}

func (a *nativeFieldStreamAttempt) stream(id, name string, argumentReader io.Reader) {
	r := a.loop
	emitter := r.GetEmitter()
	if emitter == nil || !validActionToolName(name) || (a.allowed != nil && !a.allowed[name]) {
		_, _ = io.Copy(io.Discard, argumentReader)
		return
	}
	handler, err := r.GetActionHandler(name)
	if err != nil || handler == nil {
		_, _ = io.Copy(io.Discard, argumentReader)
		return
	}
	fields := r.nativeStreamFields(handler)
	if fields.Len() == 0 {
		_, _ = io.Copy(io.Discard, argumentReader)
		return
	}
	taskIndex := ""
	if task := r.GetCurrentTask(); task != nil {
		taskIndex = task.GetIndex()
	}
	var options []jsonextractor.CallbackOption
	for _, field := range fields.Values() {
		if field == nil || field.FieldName == "" {
			continue
		}
		field := field
		options = append(options, jsonextractor.WithRegisterFieldStreamHandler(field.FieldName,
			func(_ string, raw io.Reader, parents []string) {
				// A later top-level key is canonical. Defer nested-only legacy
				// fields until the complete, verified call is available.
				if len(parents) != 0 {
					_, _ = io.Copy(io.Discard, raw)
					return
				}
				if !a.mark(id, field.FieldName) {
					a.setError(utils.Errorf("duplicate top-level native argument %q in tool call %q", field.FieldName, id))
					_, _ = io.Copy(io.Discard, raw)
					return
				}
				readable, err := emitNativeActionField(emitter, taskIndex, field, raw)
				if !readable {
					a.unmark(id, field.FieldName)
				}
				a.setError(err)
			}))
	}
	if len(options) == 0 {
		_, _ = io.Copy(io.Discard, argumentReader)
		return
	}
	// Extractor's field readers run concurrently with this parse and the
	// emitter consumes each decoded string as its JSON bytes arrive.
	if err := jsonextractor.ExtractStructuredJSONFromStream(argumentReader, options...); err != nil {
		a.setError(utils.Wrapf(err, "extract native arguments for %q", id))
	}
	_, _ = io.Copy(io.Discard, argumentReader)
}

// Emit fields that could only be resolved from the complete accepted action.
// A top-level field already streamed above must never be replayed.
func (r *ReActLoop) emitNativeActionStreamFields(resp *aicommon.AIResponse, calls []LoopCall, attempt *nativeFieldStreamAttempt) error {
	emitter := resp.BindEmitter(r.GetEmitter())
	if emitter == nil {
		return nil
	}
	taskIndex := resp.GetTaskIndex()
	if taskIndex == "" && r.GetCurrentTask() != nil {
		task := r.GetCurrentTask()
		taskIndex = task.GetIndex()
	}
	for _, call := range calls {
		if call.LoopAction == nil {
			continue
		}
		fields := r.nativeStreamFields(call.LoopAction)
		var options []jsonextractor.CallbackOption
		var fieldErr error
		var fieldErrMu sync.Mutex
		for _, field := range fields.Values() {
			if field == nil || field.FieldName == "" || attempt.wasEmitted(call.ToolCallID, field.FieldName) {
				continue
			}
			field := field
			_, hasTopLevelField := call.Action.LookupParam(field.FieldName)
			options = append(options, jsonextractor.WithRegisterFieldStreamHandler(field.FieldName,
				func(_ string, raw io.Reader, parents []string) {
					// Prefer the canonical top-level argument when a nested object
					// contains a field with the same name. Some actions only declare
					// their display field inside a nested payload, so retain those.
					if hasTopLevelField && len(parents) != 0 {
						_, _ = io.Copy(io.Discard, raw)
						return
					}
					if _, err := emitNativeActionField(emitter, taskIndex, field, raw); err != nil {
						fieldErrMu.Lock()
						if fieldErr == nil {
							fieldErr = err
						}
						fieldErrMu.Unlock()
					}
				}))
		}
		if len(options) == 0 {
			continue
		}
		if err := jsonextractor.ExtractStructuredJSON(call.ArgumentsJSON, options...); err != nil {
			return utils.Wrapf(err, "extract fields from native tool call %q", call.ToolCallID)
		}
		if fieldErr != nil {
			return utils.Wrapf(fieldErr, "emit fields from native tool call %q", call.ToolCallID)
		}
	}
	return nil
}

func emitNativeActionField(emitter *aicommon.Emitter, taskIndex string, field *LoopStreamField, raw io.Reader) (bool, error) {
	reader := utils.JSONStringReader(raw)
	if field.StreamHandler != nil {
		pipeReader, pipeWriter := io.Pipe()
		go func() {
			defer pipeWriter.Close()
			field.StreamHandler(reader, pipeWriter)
		}()
		reader = pipeReader
	} else if field.Prefix != "" {
		reader = io.MultiReader(strings.NewReader(field.Prefix+": "), reader)
	}
	prepared, readable, err := waitReadableStream(reader)
	if err != nil || !readable {
		return false, err
	}
	nodeID := field.AINodeId
	if nodeID == "" {
		nodeID = "re-act-loop-thought"
	}
	done := make(chan struct{})
	_, err = emitter.EmitStreamEventWithVizSource(nodeID, prepared, taskIndex, field.ContentType,
		field.FieldName, field.IsSystem, func() { close(done) })
	if err != nil {
		_, _ = io.Copy(io.Discard, prepared)
		return true, err
	}
	<-done
	return true, nil
}
