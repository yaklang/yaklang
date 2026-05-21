package aid

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
