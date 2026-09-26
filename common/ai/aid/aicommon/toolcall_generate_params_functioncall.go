package aicommon

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"

	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/ai/aispec"
	"github.com/yaklang/yaklang/common/schema"
)

const submitToolParamsFunctionName = "submit_tool_params"

// ToolParamGenerationAbandonedError means the model deliberately returned
// ordinary content instead of calling submit_tool_params. It is terminal for
// this parameter transaction, while the owning loop may choose another action.
type ToolParamGenerationAbandonedError struct {
	Reason  string
	Content string
}

func (e *ToolParamGenerationAbandonedError) Error() string {
	if e == nil {
		return "tool parameter generation abandoned"
	}
	message := strings.TrimSpace(strings.Join([]string{e.Reason, e.Content}, "\n"))
	if message == "" {
		message = "model did not provide a reason"
	}
	return "tool parameter generation abandoned: " + message
}

// nativeParamSubmission accepts incremental tool-call metadata from both Chat
// Completions and Responses. Only one call to the stable submission function
// is valid; its arguments are accumulated independently of ordinary content.
type nativeParamSubmission struct {
	mu        sync.Mutex
	seen      bool
	index     int
	id        string
	name      string
	arguments strings.Builder
	err       error
}

func (s *nativeParamSubmission) observe(calls []*aispec.ToolCall) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, call := range calls {
		if call == nil || s.err != nil {
			continue
		}
		if s.seen && (call.Index != s.index || (call.ID != "" && s.id != "" && call.ID != s.id)) {
			s.err = fmt.Errorf("parameter generation returned more than one native tool call")
			continue
		}
		s.seen = true
		s.index = call.Index
		if call.ID != "" {
			s.id = call.ID
		}
		if part := call.Function.Name; part != "" {
			if part == submitToolParamsFunctionName && strings.HasPrefix(submitToolParamsFunctionName, s.name) {
				s.name = part
			} else {
				s.name += part
			}
			if !strings.HasPrefix(submitToolParamsFunctionName, s.name) {
				s.err = fmt.Errorf("unexpected parameter submission function %q", s.name)
			}
		}
		s.arguments.WriteString(call.Function.Arguments)
	}
}

func (s *nativeParamSubmission) snapshot() (string, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.err != nil {
		return "", s.seen, s.err
	}
	if !s.seen {
		return "", false, nil
	}
	if s.name != submitToolParamsFunctionName {
		return "", true, fmt.Errorf("incomplete parameter submission function %q", s.name)
	}
	return s.arguments.String(), true, nil
}

func (t *ToolCaller) functionCallGenerateParams(tool *aitool.Tool, handleError func(i any)) (*GenerateParamsResult, error) {
	t.emitter.EmitToolCallStatus(t.callToolId, schema.TOOL_CALL_STATUS_PROCESSING_PARAMS)
	defer t.emitter.EmitToolCallStatus(t.callToolId, schema.TOOL_CALL_STATUS_RUNNING)
	if t.generateFunctionCallParamsPrompt == nil {
		err := fmt.Errorf("function-call parameter prompt builder is nil")
		handleError(err)
		return nil, err
	}
	prompt, err := t.generateFunctionCallParamsPrompt(tool, tool.Name)
	if err != nil {
		handleError(err)
		return nil, err
	}
	if t.reason != "" || t.destinationIdentifier != "" || t.callExpectations != "" {
		intent, _ := json.Marshal(map[string]string{
			"reason": t.reason, "identifier": t.destinationIdentifier,
			"call_expectations": t.callExpectations,
		})
		prompt += "\n\nCurrent invocation intent: " + string(intent)
	}

	release := func() {}
	if t.paramGenerationGate != nil {
		release, err = t.paramGenerationGate(t.ctx)
		release = idempotentToolCallerRelease(release)
		if err != nil {
			handleError(err)
			return nil, err
		}
		if err = toolCallerContextErr(t.ctx); err != nil {
			release()
			handleError(err)
			return nil, err
		}
	}
	defer release()
	if err = toolCallerContextErr(t.ctx); err != nil {
		handleError(err)
		return nil, err
	}
	seq := t.paramTransactionSeq
	t.paramTransactionSeq = 0

	var result *GenerateParamsResult
	var abandoned *ToolParamGenerationAbandonedError
	var submission *nativeParamSubmission
	err = CallAITransaction(t.config, prompt, func(request *AIRequest) (*AIResponse, error) {
		// The native tool is projected from the stable prompt tag at send time.
		// The callback only observes model output; it never injects a per-tool schema.
		submission = &nativeParamSubmission{}
		WithAIRequest_ExtraSpecOpts(
			aispec.WithToolCallCallback(submission.observe),
			aispec.WithToolChoice("auto"),
		)(request)
		request.SetTaskIndex(t.task.GetIndex())
		return t.ai.CallAI(request)
	}, func(rsp *AIResponse) error {
		boundEmitter := rsp.BindEmitter(t.emitter)
		rsp.SetOnReasonChunk(func(b []byte) {
			if len(b) > 0 && !t.reasonFinalized {
				boundEmitter.EmitToolCallReason(t.callToolId, string(b))
			}
		})
		start := time.Now()
		content, readErr := io.ReadAll(rsp.GetOutputStreamReader("call-tools", true, t.emitter))
		if readErr != nil {
			return readErr
		}
		if submission == nil {
			return fmt.Errorf("native parameter submission collector is missing")
		}
		arguments, seen, snapshotErr := submission.snapshot()
		if snapshotErr != nil {
			return snapshotErr
		}
		if !seen {
			abandoned = &ToolParamGenerationAbandonedError{
				Reason: rsp.GetPlainReason(), Content: string(content),
			}
			return nil
		}
		abandoned = nil
		var envelope struct {
			Params           json.RawMessage `json:"params"`
			Identifier       string          `json:"identifier"`
			CallExpectations string          `json:"call_expectations"`
		}
		if err := json.Unmarshal([]byte(arguments), &envelope); err != nil {
			return fmt.Errorf("invalid submit_tool_params arguments: %w", err)
		}
		var params aitool.InvokeParams
		if len(envelope.Params) == 0 || !json.Valid(envelope.Params) ||
			json.Unmarshal(envelope.Params, &params) != nil || params == nil {
			return fmt.Errorf("submit_tool_params requires an object in arguments.params")
		}
		normalizeToolParamBooleans(tool, params)
		if valid, errors := tool.ValidateParams(params); !valid {
			return fmt.Errorf("generated parameters for %q failed schema validation: %s", tool.Name, strings.Join(errors, "; "))
		}
		result = &GenerateParamsResult{
			Params: params, Identifier: sanitizeIdentifier(envelope.Identifier),
			CallExpectations: envelope.CallExpectations,
			Duration:         time.Since(start), RawAIResponse: arguments,
		}
		_, _ = boundEmitter.EmitDefaultSystemStreamEvent("generating-tool-call-params", strings.NewReader(arguments), t.task.GetIndex())
		return nil
	}, WithAIRequest_CallerLabel("toolcall-params"), WithAIRequest_Context(t.ctx), WithAIRequest_SeqId(seq))
	release()
	if err != nil {
		handleError(fmt.Sprintf("error calling AI for tool[%v] params: %v", tool.Name, err))
		return nil, err
	}
	if abandoned != nil {
		handleError(abandoned)
		return nil, abandoned
	}
	return result, nil
}
