package aicommon

import (
	"fmt"
	"sync"

	"github.com/segmentio/ksuid"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
)

type ToolCaller struct {
	*Emitter

	runtimeId  string
	task       AITask
	done       *sync.Once
	callToolId string

	generateToolParamsBuilder func(tool *aitool.Tool, toolName string) (string, error)

	m               *sync.Mutex
	onCallToolStart func()
	onCallToolEnd   func()
}

func NewToolCaller(
	runtimeId string,
	task AITask,
	emitter *Emitter,
) *ToolCaller {
	return &ToolCaller{
		runtimeId:  runtimeId,
		task:       task,
		callToolId: ksuid.New().String(),
		Emitter:    emitter,
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
	t.EmitInfo("start to generate tool[%v] params in task: %v", tool.Name, t.task.GetName())

	callToolId := t.callToolId

	t.EmitToolCallStart(callToolId, tool)

	handleDone := func() {
		t.done.Do(func() {
			t.EmitToolCallStatus(t.callToolId, "done")
			t.EmitToolCallDone(callToolId)
			t.m.Lock()
			defer t.m.Unlock()

			if t.onCallToolEnd != nil {
				t.onCallToolEnd()
			}
		})
	}

	handleUserCancel := func(reason any) {
		t.done.Do(func() {
			t.EmitToolCallStatus(t.callToolId, fmt.Sprintf("cancelled by reason: %v", reason))
			t.EmitToolCallUserCancel(callToolId)
			t.m.Lock()
			defer t.m.Unlock()

			if t.onCallToolEnd != nil {
				t.onCallToolEnd()
			}
		})
	}

	handleError := func(err any) {
		t.done.Do(func() {
			t.EmitToolCallError(callToolId, err)
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

	// paramsPrompt, err := t.GetParamGeneratingPrompt(tool, tool.Name)
	// if err != nil {
	// 	t.EmitError("error generate tool[%v] params in task: %v", tool.Name, t.task.GetName())
	// 	handleError(fmt.Sprintf("error generate tool[%v] params in task: %v", tool.Name, t.task.GetName()))
	// 	return nil, false, err
	// }

	// invokeParams := new(aitool.InvokeParams)
	// invokeParams["runtime_id"] = t.runtimeId

	// _ = paramsPrompt
	// _ = invokeParams

	return nil, false, nil
}
