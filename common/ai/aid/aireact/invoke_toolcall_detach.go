package aireact

import (
	"fmt"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/asyncexecmanager"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
)

func (r *ReAct) finishDetachedToolCall(task aicommon.AIStatefulTask, result *aitool.ToolResult, directly bool, callErr error) {
	loop, _ := task.GetReActLoop().(*reactloops.ReActLoop)
	if directly {
		if loop != nil {
			loop.RequestAsyncDirectAnswer()
		}
		return
	}
	if callErr != nil {
		return
	}
	_, _, _ = r.finalizeToolCallResult(task, result, false)
	if loop != nil && result != nil && result.Success {
		reactloops.MarkEditBeforeExecutionCompleted(loop, result.Name)
		if tool, err := r.config.GetAiToolManager().GetToolByName(result.Name); err == nil {
			loop.RecordRecentlyUsedTool(tool)
		}
	}
}

func (r *ReAct) newDetachableToolCall(task aicommon.AIStatefulTask, caller *aicommon.ToolCaller, name string) (*aicommon.ToolCallExecution, error) {
	loop, _ := task.GetReActLoop().(*reactloops.ReActLoop)
	if loop == nil {
		return nil, aicommon.ErrToolDetachUnavailable
	}
	execution, err := aicommon.NewToolCallExecution(caller, func(resource asyncexecmanager.Resource) (asyncexecmanager.Summary, error) {
		manager, err := loop.GetAsyncExecutionManager()
		if err != nil {
			return asyncexecmanager.Summary{}, err
		}
		return manager.Adopt(asyncexecmanager.Request{Resource: resource, Metadata: asyncexecmanager.Metadata{
			Name: name, Attributes: map[string]string{"task_id": task.GetId(), "loop": loop.GetLoopName(), "call_tool_id": caller.CallToolID(), "result_id": fmt.Sprint(caller.ResultID())},
		}})
	}, func(result *aitool.ToolResult, directly bool, err error, detached bool) {
		if detached {
			r.finishDetachedToolCall(task, result, directly, err)
		}
	})
	if err != nil {
		return nil, err
	}
	go func() {
		select {
		case <-loop.AsyncDirectAnswerSignal():
			caller.Cancel()
		case <-execution.Done():
		}
	}()
	return execution, nil
}

func (r *ReAct) runDetachableToolCall(task aicommon.AIStatefulTask, caller *aicommon.ToolCaller, name string, run func() (*aitool.ToolResult, bool, error)) (*aitool.ToolResult, bool, error) {
	if task == nil || task.GetReActLoop() == nil {
		return run()
	}
	execution, err := r.newDetachableToolCall(task, caller, name)
	if err != nil {
		return nil, false, err
	}
	execution.Start(run)
	return execution.Wait()
}
