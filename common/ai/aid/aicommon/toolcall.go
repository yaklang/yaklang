package aicommon

import (
	"fmt"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"sync"

	"github.com/segmentio/ksuid"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
)

type ToolCaller struct {
	runtimeId  string
	task       AITask
	config     AICallerConfigIf
	emitter    *Emitter // specific, backup for config.GetEmitter()
	ai         AICaller
	done       *sync.Once
	callToolId string

	generateToolParamsBuilder func(tool *aitool.Tool, toolName string) (string, error)

	m               *sync.Mutex
	onCallToolStart func(callToolId string)
	onCallToolEnd   func(callToolId string)

	reviewWrongToolHandler func(tool *aitool.Tool, newToolName, keyword string) (*aitool.Tool, error)
}

type ToolCallerOption func(tc *ToolCaller)

func WithToolCaller_ReviewWrongTool(
	handler func(tool *aitool.Tool, newToolName, keyword string) (*aitool.Tool, error),
) ToolCallerOption {
	return func(tc *ToolCaller) {
		tc.reviewWrongToolHandler = handler
	}
}

func WithToolCaller_Task(task AITask) ToolCallerOption {
	return func(tc *ToolCaller) {
		tc.task = task
	}
}

func WithToolCaller_RuntimeId(runtimeId string) ToolCallerOption {
	return func(tc *ToolCaller) {
		tc.runtimeId = runtimeId
	}
}

func WithToolCaller_AICaller(ai AICaller) ToolCallerOption {
	return func(tc *ToolCaller) {
		tc.ai = ai
	}
}

func WithToolCaller_Emitter(e *Emitter) ToolCallerOption {
	return func(tc *ToolCaller) {
		tc.emitter = e
	}
}

func WithToolCaller_AICallerConfig(config AICallerConfigIf) ToolCallerOption {
	return func(tc *ToolCaller) {
		tc.config = config
	}
}

func WithToolCaller_OnStart(i func(callToolId string)) ToolCallerOption {
	return func(tc *ToolCaller) {
		tc.onCallToolStart = i
	}
}

func WithToolCaller_OnEnd(i func(callToolId string)) ToolCallerOption {
	return func(tc *ToolCaller) {
		tc.onCallToolEnd = i
	}
}

func WithToolCaller_GenerateToolParamsBuilder(
	builder func(tool *aitool.Tool, toolName string) (string, error),
) ToolCallerOption {
	return func(tc *ToolCaller) {
		tc.generateToolParamsBuilder = builder
	}
}

func NewToolCaller(opts ...ToolCallerOption) (*ToolCaller, error) {
	caller := &ToolCaller{
		callToolId: ksuid.New().String(),
		done:       &sync.Once{},
		m:          &sync.Mutex{},
	}
	for _, opt := range opts {
		opt(caller)
	}
	if caller.runtimeId == "" {
		caller.runtimeId = caller.config.GetRuntimeId()
	}

	if caller.config == nil || utils.IsNil(caller.config) {
		return nil, fmt.Errorf("config is nil in ToolCaller")
	}

	if caller.ai == nil || utils.IsNil(caller.ai) {
		return nil, fmt.Errorf("ai caller is nil in ToolCaller")
	}

	return caller, nil
}

func (t *ToolCaller) GetParamGeneratingPrompt(tool *aitool.Tool, toolName string) (string, error) {
	if t.generateToolParamsBuilder == nil {
		return "", fmt.Errorf("generateToolParamsBuilder is nil")
	}

	return t.generateToolParamsBuilder(tool, toolName)
}

func (t *ToolCaller) CallTool(tool *aitool.Tool) (result *aitool.ToolResult, directlyAnswer bool, err error) {
	emitter := t.emitter
	if emitter == nil {
		emitter = t.config.GetEmitter()
		if emitter == nil {
			return nil, false, fmt.Errorf("no emitter found in ToolCaller")
		}
		t.emitter = emitter
	}

	emitter.EmitInfo("start to generate tool[%v] params in task: %v", tool.Name, t.task.GetName())
	callToolId := t.callToolId

	emitter.EmitToolCallStart(callToolId, tool)
	t.m.Lock()
	if t.onCallToolStart != nil {
		t.onCallToolStart(callToolId)
	}
	t.m.Unlock()

	handleDone := func() {
		t.done.Do(func() {
			emitter.EmitToolCallStatus(t.callToolId, "done")
			emitter.EmitToolCallDone(callToolId)
			t.m.Lock()
			defer t.m.Unlock()

			if t.onCallToolEnd != nil {
				t.onCallToolEnd(callToolId)
			}
		})
	}

	handleUserCancel := func(reason any) {
		t.done.Do(func() {
			emitter.EmitToolCallStatus(t.callToolId, fmt.Sprintf("cancelled by reason: %v", reason))
			emitter.EmitToolCallUserCancel(callToolId)
			t.m.Lock()
			defer t.m.Unlock()

			if t.onCallToolEnd != nil {
				t.onCallToolEnd(callToolId)
			}
		})
	}

	handleError := func(err any) {
		t.done.Do(func() {
			emitter.EmitToolCallError(callToolId, err)
			t.m.Lock()
			defer t.m.Unlock()

			if t.onCallToolEnd != nil {
				t.onCallToolEnd(callToolId)
			}
		})
	}

	var (
		_ = handleDone
		_ = handleUserCancel
		_ = handleError
	)

	defer handleDone()

	paramsPrompt, err := t.GetParamGeneratingPrompt(tool, tool.Name)
	if err != nil {
		emitter.EmitError("error generate tool[%v] params in task: %v", tool.Name, t.task.GetName())
		handleError(fmt.Sprintf("error generate tool[%v] params in task: %v", tool.Name, t.task.GetName()))
		return nil, false, err
	}

	invokeParams := aitool.InvokeParams{}
	invokeParams.Set("runtime_id", t.runtimeId)

	err = CallAITransaction(t.config, paramsPrompt, func(request *AIRequest) (*AIResponse, error) {
		request.SetTaskIndex(t.task.GetIndex())
		return t.ai.CallAI(request)
	}, func(rsp *AIResponse) error {
		stream := rsp.GetOutputStreamReader("call-tools", true, emitter)
		callToolAction, err := ExtractActionFromStream(stream, "call-tool")
		if err != nil {
			emitter.EmitError("error extract tool params: %v", err)
			return utils.Errorf("error extracting action params: %v", err)
		}
		for k, v := range callToolAction.GetInvokeParams("params") {
			invokeParams.Set(k, v)
		}
		return nil
	})
	if err != nil {
		emitter.EmitError("error calling AI for tool[%v] params: %v", tool.Name, err)
		handleError(fmt.Sprintf("error calling AI for tool[%v] params: %v", tool.Name, err))
		return nil, false, err
	}

	emitter.EmitInfo("start to invoke callback function for tool:%v", tool.Name)

	epm := t.config.GetEndpointManager()
	config := t.config
	// DANGER: NoNeedUserReview
	if tool.NoNeedUserReview {
		emitter.EmitInfo("tool[%v] (internal helper tool) no need user review, skip review", tool.Name)
	} else {
		emitter.EmitInfo("start to require review for tool use: %v", tool.Name)
		ep := epm.CreateEndpointWithEventType(schema.EVENT_TYPE_TOOL_USE_REVIEW_REQUIRE)
		ep.SetDefaultSuggestionContinue()
		reqs := map[string]any{
			"id":               ep.GetId(),
			"selectors":        ToolUseReviewSuggestions,
			"tool":             tool.Name,
			"tool_description": tool.Description,
			"params":           invokeParams,
		}
		ep.SetReviewMaterials(reqs)
		err := t.config.SubmitCheckpointRequest(ep.GetCheckpoint(), reqs)
		if err != nil {
			log.Errorf("submit request review to db for tool use failed: %v", err)
		}
		emitter.EmitInteractiveJSON(ep.GetId(), schema.EVENT_TYPE_TOOL_USE_REVIEW_REQUIRE, "review-require", reqs)

		// wait for agree
		config.DoWaitAgree(nil, ep)
		params := ep.GetParams()
		config.ReleaseInteractiveEvent(ep.GetId(), params)
		if params == nil {
			emitter.EmitError("tool use [%v] review params is nil, user may cancel the review", tool.Name)
			handleError(fmt.Sprintf("tool use [%v] review params is nil, user may cancel the review", tool.Name))
			return nil, false, fmt.Errorf("tool use [%v] review params is nil", tool.Name)
		}
		var overrideResult *aitool.ToolResult
		var next HandleToolUseNext
		tool, invokeParams, overrideResult, next, err = t.review(
			tool, invokeParams, params, handleUserCancel,
		)
		if err != nil {
			emitter.EmitError("error handling tool use review: %v", err)
			handleError(fmt.Sprintf("error handling tool use review: %v", err))
			return nil, false, err
		}

		switch next {
		case HandleToolUseNext_Override:
			return overrideResult, false, nil
		case HandleToolUseNext_DirectlyAnswer:
			return nil, true, nil
		case HandleToolUseNext_Default:
		default:
			return nil, false, utils.Errorf("unknown handle tool use next action: %v", next)
		}
	}

	stdoutReader, stdoutWriter := utils.NewPipe()
	defer stdoutWriter.Close()
	stderrReader, stderrWriter := utils.NewPipe()
	defer stderrWriter.Close()

	emitter.EmitToolCallStd(tool.Name, stdoutReader, stderrReader, t.task.GetIndex())
	emitter.EmitInfo("start to invoke tool: %v", tool.Name)

	toolResult, err := t.invoke(tool, invokeParams, handleUserCancel, handleError, stdoutWriter, stderrWriter)
	if err != nil {
		if toolResult == nil {
			toolResult = &aitool.ToolResult{
				Param:       invokeParams,
				Name:        tool.Name,
				Description: tool.Description,
				ToolCallID:  callToolId,
			}
		}
		toolResult.Error = fmt.Sprintf("error invoking tool[%v]: %v", tool.Name, err)
		toolResult.Success = false
	}
	emitter.EmitInfo("start to generate and feedback tool[%v] result in task: %#v", tool.Name, t.task.GetName())
	return toolResult, false, nil
}
