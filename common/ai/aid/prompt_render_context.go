package aid

import (
	"strings"

	"github.com/yaklang/yaklang/common/utils"
)

type PromptRenderContext struct {
	*PromptContextProvider
	CurrentTask *AiTask
}

func NewPromptRenderContext(base *PromptContextProvider, task *AiTask) *PromptRenderContext {
	return &PromptRenderContext{
		PromptContextProvider: base,
		CurrentTask:           task,
	}
}

func (m *PromptContextProvider) RenderContextForTask(task *AiTask) *PromptRenderContext {
	return NewPromptRenderContext(m, task)
}

func (m *PromptContextProvider) RenderCurrentTaskInfo(task *AiTask) string {
	return NewPromptRenderContext(m, task).CurrentTaskInfo()
}

func (p *PromptRenderContext) Progress() string {
	if p == nil {
		return "empty *PromptRenderContext maybe a BUG"
	}

	root := promptRenderRootTask(p.PromptContextProvider, p.CurrentTask)
	if root == nil {
		return ""
	}
	return root.Progress()
}

func (p *PromptRenderContext) CurrentTaskTimeline() string {
	if p == nil || p.PromptContextProvider == nil || p.timeline == nil {
		return ""
	}
	if p.CurrentTask == nil {
		return p.TimelineDump()
	}
	stl := p.timeline.CreateSubTimeline(p.CurrentTask.ToolCallResultsID()...)
	if stl == nil {
		return "no-toolcall, so not timeline"
	}
	timelineDump := stl.Dump()
	if timelineDump == "" {
		timelineDump = p.TimelineDump()
	}
	return timelineDump
}

func (p *PromptRenderContext) TaskMaxContinue() int64 {
	if p == nil || p.CurrentTask == nil || p.CurrentTask.Coordinator == nil {
		return 0
	}
	return p.CurrentTask.Coordinator.MaxTaskContinue
}

func (p *PromptRenderContext) SharedEvidenceContext() string {
	if p == nil || p.CurrentTask == nil {
		return ""
	}

	evidence := strings.TrimSpace(getTaskPlanEvidence(p.CurrentTask))
	if evidence == "" {
		return ""
	}

	const maxEvidenceRunes = 1600
	runes := []rune(evidence)
	if len(runes) <= maxEvidenceRunes {
		return evidence
	}
	return string(runes[:maxEvidenceRunes]) + "\n\n..."
}

func (p *PromptRenderContext) CurrentTaskInfo() string {
	if p == nil || p.CurrentTask == nil {
		return "BUG:... currentTaskInfo cannot be generated in `CurrentTaskInfo`, no current task"
	}
	results, err := utils.RenderTemplate(__prompt_currentTaskInfo, map[string]any{
		"ContextProvider": p,
	})
	if err != nil {
		return "BUG:... currentTaskInfo cannot be generated in `CurrentTaskInfo` err: " + err.Error()
	}
	return results
}

func promptRenderRootTask(base *PromptContextProvider, task *AiTask) *AiTask {
	if base != nil && base.RootTask != nil {
		return base.RootTask
	}
	if task == nil {
		return nil
	}
	if task.rootTask != nil {
		return task.rootTask
	}
	cur := task
	for cur.ParentTask != nil {
		cur = cur.ParentTask
	}
	return cur
}

func (t *AiTask) taskPromptContext() *PromptRenderContext {
	if t == nil {
		return nil
	}
	if t.ContextProvider != nil {
		return t.ContextProvider.RenderContextForTask(t)
	}
	if t.Coordinator != nil && t.Coordinator.ContextProvider != nil {
		return t.Coordinator.ContextProvider.RenderContextForTask(t)
	}
	return NewPromptRenderContext(nil, t)
}

func (t *AiTask) quickBuildTaskPrompt(tmp string, data map[string]any) (string, error) {
	if data == nil {
		data = make(map[string]any)
	}
	if _, ok := data["ContextProvider"]; !ok {
		data["ContextProvider"] = t.taskPromptContext()
	}
	return t.quickBuildPrompt(tmp, data)
}
