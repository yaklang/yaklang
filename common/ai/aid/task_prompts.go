package aid

import (
	"fmt"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/omap"
	"io"
	"slices"
	"strings"
	"text/template"

	"github.com/yaklang/yaklang/common/ai/aid/aitool"

	_ "embed"
)

// generate task prompt just can direct answer.
func (t *aiTask) generateDirectAnswerPrompt() (string, error) {
	templateData := map[string]interface{}{
		"Memory": t.config.memory,
	}

	// 解析prompt模板
	tmpl, err := template.New("execute-aiTask").Parse(__prompt_DirectAnswer)
	if err != nil {
		return "", fmt.Errorf("error parsing aiTask prompt template: %w", err)
	}

	// 渲染模板
	var promptBuilder strings.Builder
	err = tmpl.Execute(&promptBuilder, templateData)
	if err != nil {
		return "", fmt.Errorf("error executing aiTask prompt template: %w", err)
	}

	return promptBuilder.String(), nil
}

// generateTaskPrompt 生成执行任务的prompt
func (t *aiTask) generateTaskPrompt() (string, error) {
	// 创建模板数据
	alltools, err := t.config.aiToolManager.GetEnableTools()
	if err != nil {
		return "", fmt.Errorf("error getting all tools: %w", err)
	}
	templateData := map[string]interface{}{
		"Tools":  alltools,
		"Memory": t.config.memory,
	}

	// 解析prompt模板
	tmpl, err := template.New("execute-aiTask").Parse(__prompt_ExecuteTaskPromptTemplate)
	if err != nil {
		return "", fmt.Errorf("error parsing aiTask prompt template: %w", err)
	}

	// 渲染模板
	var promptBuilder strings.Builder
	err = tmpl.Execute(&promptBuilder, templateData)
	if err != nil {
		return "", fmt.Errorf("error executing aiTask prompt template: %w", err)
	}

	return promptBuilder.String(), nil
}

// generateRequireToolResponsePrompt 生成描述工具参数的 Prompt
func (t *aiTask) generateRequireToolResponsePrompt(targetTool *aitool.Tool, toolName string) (string, error) {
	if targetTool == nil {
		return "", fmt.Errorf("找不到名为 '%s' 的工具", toolName)
	}

	// 生成工具的JSONSchema描述
	toolJSONSchema := targetTool.ToJSONSchemaString()
	// 创建模板数据
	templateData := map[string]interface{}{
		"Memory":         t.config.memory,
		"Tool":           targetTool,
		"ToolJSONSchema": toolJSONSchema,
	}

	// 解析工具描述模板
	tmpl, err := template.New("call-tool").Parse(__prompt_ToolParamSchemaPromptTemplate)
	if err != nil {
		return "", fmt.Errorf("error parsing tool description template: %w", err)
	}

	// 渲染模板
	var promptBuilder strings.Builder
	err = tmpl.Execute(&promptBuilder, templateData)
	if err != nil {
		return "", fmt.Errorf("error executing tool description template: %w", err)
	}

	return promptBuilder.String(), nil
}

// generateToolCallResponsePrompt 生成描述工具调用结果的 Prompt
func (t *aiTask) generateToolCallResponsePrompt(result *aitool.ToolResult, targetTool *aitool.Tool) (string, error) {
	templatedata := map[string]any{
		"Memory": t.config.memory,
		"Tool":   targetTool,
		"Result": result,
	}
	temp, err := template.New("tool-result").Parse(__prompt_ToolResultToDecisionPromptTemplate)
	if err != nil {
		return "", fmt.Errorf("error parsing tool result template: %w", err)
	}
	var promptBuilder strings.Builder
	err = temp.Execute(&promptBuilder, templatedata)
	if err != nil {
		return "", fmt.Errorf("error executing tool result template: %w", err)
	}
	return promptBuilder.String(), nil
}

func (t *aiTask) generateToolCallResultsPrompt() (string, error) {
	templatedata := map[string]interface{}{
		"ToolCallResults": t.toolCallResultIds.Values(),
	}
	temp, err := template.New("tool-result-history").Parse(__prompt_ToolResultHistoryPromptTemplate)
	if err != nil {
		return "", fmt.Errorf("error parsing tool result history template: %w", err)
	}
	var promptBuilder strings.Builder
	err = temp.Execute(&promptBuilder, templatedata)
	if err != nil {
		return "", fmt.Errorf("error executing tool result history template: %w", err)
	}
	return promptBuilder.String(), nil
}

func (t *aiTask) generateDynamicPlanPrompt(userInput string) (string, error) {
	// 创建模板数据
	templateData := map[string]interface{}{
		"Memory":    t.config.memory,
		"UserInput": userInput,
	}

	// 解析prompt模板
	tmpl, err := template.New("dynamic-plan").Parse(__prompt_DynamicPlan)
	if err != nil {
		return "", fmt.Errorf("error parsing dynamic plan prompt template: %w", err)
	}

	// 渲染模板
	var promptBuilder strings.Builder
	err = tmpl.Execute(&promptBuilder, templateData)
	if err != nil {
		return "", fmt.Errorf("error executing dynamic plan prompt template: %w", err)
	}

	return promptBuilder.String(), nil
}

func (t *aiTask) GenerateDeepThinkPlanPrompt(suggestion string) (string, error) {
	return t.config.quickBuildPrompt(__prompt_DeepthinkTaskListPrompt, map[string]any{
		"Memory":    t.config.memory,
		"UserInput": suggestion,
	})
}

func (t *aiTask) DeepThink(suggestion string) error {
	prompt, err := t.GenerateDeepThinkPlanPrompt(suggestion)
	if err != nil {
		return fmt.Errorf("生成深入思考划分子任务 prompt 失败: %v", err)
	}

	defer func() {
		// Ensure config is propagated to the new task and its subtasks
		var propagateConfig func(task *aiTask)
		propagateConfig = func(task *aiTask) {
			if task == nil {
				return
			}
			task.config = t.config
			if task.toolCallResultIds == nil {
				task.toolCallResultIds = omap.NewOrderedMap(make(map[int64]*aitool.ToolResult))
			}
			for _, sub := range task.Subtasks {
				sub.ParentTask = task // Ensure parent is set
				propagateConfig(sub)
			}
		}
		propagateConfig(t)
		t.GenerateIndex()
	}()

	err = t.config.callAiTransaction(
		prompt, t.callAI,
		func(rsp *AIResponse) error {
			action, err := ExtractActionFromStream(rsp.GetOutputStreamReader("plan", false, t.config), "plan", "require-user-interact")
			if err != nil {
				return utils.Error("parse @action field from AI response failed: " + err.Error())
			}
			switch action.ActionType() {
			case "plan":
				for _, subtask := range action.GetInvokeParamsArray("tasks") {
					if subtask.GetAnyToString("subtask_name") == "" {
						continue
					}
					t.Subtasks = append(t.Subtasks, &aiTask{
						config: t.config,
						Name:   subtask.GetAnyToString("subtask_name"),
						Goal:   subtask.GetAnyToString("subtask_goal"),
					})
				}
				if t.Name == "" {
					return fmt.Errorf("AI response does not contain any tasks, please check your AI model or prompt")
				}
				return nil
			}
			return utils.Error("no any ai callback is set, cannot found ai config")
		},
	)
	if err != nil {
		t.config.EmitError(err.Error())
		return err
	}

	return nil
}

func (t *aiTask) AdjustPlan(suggestion string) error {
	planPrompt, err := t.generateDynamicPlanPrompt(suggestion)
	if err != nil {
		t.config.EmitError("error generating dynamic plan prompt: %v", err)
		return utils.Errorf("error generating dynamic plan prompt: %v", err)
	}
	defer func() {
		// Ensure config is propagated to the new task and its subtasks
		var propagateConfig func(task *aiTask)
		propagateConfig = func(task *aiTask) {
			if task == nil {
				return
			}
			task.config = t.config
			if task.toolCallResultIds == nil {
				task.toolCallResultIds = omap.NewOrderedMap(make(map[int64]*aitool.ToolResult))
			}
			for _, sub := range task.Subtasks {
				sub.ParentTask = task // Ensure parent is set
				propagateConfig(sub)
			}
		}
		propagateConfig(t)
		t.GenerateIndex()
	}()

	err = t.config.callAiTransaction(
		planPrompt,
		t.callAI,
		func(response *AIResponse) error {
			// 读取 AI 的响应
			responseReader := response.GetOutputStreamReader("dynamic-plan", false, t.config)
			taskResponse, err := io.ReadAll(responseReader)
			if err != nil {
				t.config.EmitError("error reading AI response: %v", err)
				return utils.Errorf("error reading AI response: %v", err)
			}
			nextPlanTask, err := ExtractNextPlanTaskFromRawResponse(t.config, string(taskResponse))
			if err != nil {
				t.config.EmitError("error extracting task from raw response: %v", err)
				return utils.Errorf("error extracting task from raw response: %v", err)
			}

			if len(nextPlanTask) <= 0 {
				t.config.EmitError("any task not found in next plan")
				return utils.Errorf("any task not found in next plan task, re-do-plan")
			}

			// 解析 AI 的响应
			parentTask := t.ParentTask
			index := -1
			for i, subtask := range parentTask.Subtasks {
				if subtask.Name == t.Name {
					index = i
					break
				}
			}
			if index == -1 {
				t.config.EmitError("current task not found in parent task")
				return utils.Error("current task not found in parent task")
			}
			// 保留之前的任务, 删除后续任务
			parentTask.Subtasks = parentTask.Subtasks[:index+1]
			parentTask.Subtasks = slices.Grow(parentTask.Subtasks, len(parentTask.Subtasks)+len(nextPlanTask))

			// 添加新的任务
			for _, subTask := range nextPlanTask {
				subTask.config = t.config
				subTask.ParentTask = parentTask
				parentTask.Subtasks = append(parentTask.Subtasks, subTask)
				subTask.config.EmitInfo("new dynamic plan: %s", subTask.Name)
			}
			return nil
		},
	)
	if err != nil {
		t.config.EmitError("error calling AI transaction: %v", err)
		return utils.Errorf("error calling AI transaction: %v", err)
	}
	return nil
}
