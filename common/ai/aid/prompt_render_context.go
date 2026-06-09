package aid

func clonePromptContextForTask(base *PromptContextProvider, task *AiTask) *PromptContextProvider {
	if base == nil {
		return &PromptContextProvider{
			CurrentTask: task,
			RootTask:    getTaskRoot(task),
		}
	}

	cloned := *base
	cloned.CurrentTask = task
	if cloned.RootTask == nil {
		cloned.RootTask = getTaskRoot(task)
	}
	return &cloned
}

func getTaskRoot(task *AiTask) *AiTask {
	if task == nil {
		return nil
	}
	if task.rootTask != nil {
		return task.rootTask
	}
	if task.Coordinator != nil && task.Coordinator.rootTask != nil {
		return task.Coordinator.rootTask
	}
	root := task
	for root.ParentTask != nil {
		root = root.ParentTask
	}
	return root
}


func (t *AiTask) taskPromptContext() *PromptContextProvider {
	if t == nil {
		return nil
	}
	if t.ContextProvider != nil {
		return t.ContextProvider.RenderContextForTask(t)
	}
	if t.Coordinator != nil && t.Coordinator.ContextProvider != nil {
		return t.Coordinator.ContextProvider.RenderContextForTask(t)
	}
	return NewPromptRenderContext(clonePromptContextForTask(nil, t), t)
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
