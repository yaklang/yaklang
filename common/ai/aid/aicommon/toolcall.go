package aicommon

import (
	"fmt"
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
	onCallToolStart func()
	onCallToolEnd   func()
}

type ToolCallerOption func(tc *ToolCaller)

func WithToolCaller_Task(task AITask) ToolCallerOption {
	return func(tc *ToolCaller) {
		tc.task = task
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

func WithToolCaller_OnStart(i func()) ToolCallerOption {
	return func(tc *ToolCaller) {
		tc.onCallToolStart = i
	}
}

func WithToolCaller_OnEnd(i func()) ToolCallerOption {
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

func NewToolCaller(runtimeId string) *ToolCaller {
	return &ToolCaller{
		runtimeId:  runtimeId,
		callToolId: ksuid.New().String(),
		done:       &sync.Once{},
		m:          &sync.Mutex{},
	}
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
	if t.onCallToolStart != nil {
		t.onCallToolStart()
	}

	handleDone := func() {
		t.done.Do(func() {
			emitter.EmitToolCallStatus(t.callToolId, "done")
			emitter.EmitToolCallDone(callToolId)
			t.m.Lock()
			defer t.m.Unlock()

			if t.onCallToolEnd != nil {
				t.onCallToolEnd()
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
				t.onCallToolEnd()
			}
		})
	}

	handleError := func(err any) {
		t.done.Do(func() {
			emitter.EmitToolCallError(callToolId, err)
			t.m.Lock()
			defer t.m.Unlock()

			if t.onCallToolEnd != nil {
				t.onCallToolEnd()
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

	invokeParams := new(aitool.InvokeParams)
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

	emitter.EmitInfo("start to invoke tool:%v 's callback function", tool.Name)

	// DANGER: NoNeedUserReview
	if tool.NoNeedUserReview {
		emitter.EmitInfo("tool[%v] (internal helper tool) no need user review, skip review", tool.Name)
	} else {
		emitter.EmitInfo("start to require review for tool use: %v", tool.Name)

	}

	// _ = paramsPrompt
	// _ = invokeParams

	return nil, false, nil
}
