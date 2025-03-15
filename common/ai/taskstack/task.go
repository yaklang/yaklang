package taskstack

import (
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strings"
	"text/template"

	"github.com/yaklang/yaklang/common/jsonextractor"
	"github.com/yaklang/yaklang/common/utils"
)

//go:embed jsonschema/task.json
var taskJsonSchema string

//go:embed prompts/execute-task.txt
var executeTaskPromptTemplate string

//go:embed prompts/describe-tool.txt
var describeToolPromptTemplate string

// 用于识别工具操作请求的正则表达式
var toolActionRegex = regexp.MustCompile(`(?s)\` + "`" + `jsonschema.*?\{.*?"@action"\s*:\s*"([^"]+)".*?"tool"\s*:\s*"([^"]+)".*?\}\s*\` + "`" + ``)

// TaskAICallback 定义Task执行过程中AI调用回调函数类型
type TaskAICallback func(prompt string) (io.Reader, error)

// TaskProgress 记录任务执行的进度信息
type TaskProgress struct {
	TotalTasks     int    `json:"total_tasks"`     // 总任务数
	CompletedTasks int    `json:"completed_tasks"` // 已完成任务数
	CurrentTask    string `json:"current_task"`    // 当前执行的任务
	CurrentGoal    string `json:"current_goal"`    // 当前任务的目标
}

type Task struct {
	Name       string
	Goal       string
	Subtasks   []Task
	AICallback TaskAICallback // AI回调函数
}

// MarshalJSON 实现自定义的JSON序列化，跳过AICallback字段
func (t Task) MarshalJSON() ([]byte, error) {
	type TaskAlias Task // 创建一个别名类型以避免递归调用

	// 创建一个不包含AICallback的结构体
	return json.Marshal(struct {
		Name     string `json:"name"`
		Goal     string `json:"goal"`
		Subtasks []Task `json:"subtasks,omitempty"`
	}{
		Name:     t.Name,
		Goal:     t.Goal,
		Subtasks: t.Subtasks,
	})
}

// UnmarshalJSON 实现自定义的JSON反序列化，跳过AICallback字段
func (t *Task) UnmarshalJSON(data []byte) error {
	// 创建一个临时结构体，不包含AICallback
	aux := struct {
		Name     string `json:"name"`
		Goal     string `json:"goal"`
		Subtasks []Task `json:"subtasks,omitempty"`
	}{}

	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}

	t.Name = aux.Name
	t.Goal = aux.Goal
	t.Subtasks = aux.Subtasks
	return nil
}

// SetAICallback 设置Task的AI回调函数
func (t *Task) SetAICallback(callback TaskAICallback) {
	t.AICallback = callback

	// 递归设置子任务的回调函数
	for i := range t.Subtasks {
		t.Subtasks[i].SetAICallback(callback)
	}
}

// Invoke 执行当前任务，如果有子任务则依次执行子任务
func (t *Task) Invoke(tools []*Tool, metadata map[string]interface{}) (string, error) {
	if t.AICallback == nil {
		return "", errors.New("no AI callback function set")
	}

	// 如果有子任务，先执行所有子任务
	if len(t.Subtasks) > 0 {
		results := make([]string, 0, len(t.Subtasks))

		// 计算总任务数（当前任务+所有子任务）
		totalTasks := 1 + len(t.Subtasks)

		for i, subtask := range t.Subtasks {
			// 更新进度信息
			progress := TaskProgress{
				TotalTasks:     totalTasks,
				CompletedTasks: i,
				CurrentTask:    subtask.Name,
				CurrentGoal:    subtask.Goal,
			}

			// 添加进度信息到metadata
			metadataCopy := make(map[string]interface{})
			for k, v := range metadata {
				metadataCopy[k] = v
			}
			metadataCopy["progress"] = progress

			// 执行子任务
			result, err := subtask.Invoke(tools, metadataCopy)
			if err != nil {
				return "", fmt.Errorf("error executing subtask '%s': %w", subtask.Name, err)
			}

			results = append(results, result)
		}

		// 所有子任务完成后，执行当前任务
		progress := TaskProgress{
			TotalTasks:     totalTasks,
			CompletedTasks: len(t.Subtasks),
			CurrentTask:    t.Name,
			CurrentGoal:    t.Goal,
		}

		// 添加进度信息到metadata
		metadataCopy := make(map[string]interface{})
		for k, v := range metadata {
			metadataCopy[k] = v
		}
		metadataCopy["progress"] = progress
		metadataCopy["subtask_results"] = results

		return t.executeTask(tools, metadataCopy)
	}

	// 没有子任务，直接执行当前任务
	progress := TaskProgress{
		TotalTasks:     1,
		CompletedTasks: 0,
		CurrentTask:    t.Name,
		CurrentGoal:    t.Goal,
	}

	// 添加进度信息到metadata
	metadataCopy := make(map[string]interface{})
	for k, v := range metadata {
		metadataCopy[k] = v
	}
	metadataCopy["progress"] = progress

	return t.executeTask(tools, metadataCopy)
}

// executeTask 实际执行任务并返回结果
func (t *Task) executeTask(tools []*Tool, metadata map[string]interface{}) (string, error) {
	// 生成初始执行任务的prompt
	prompt, err := t.generateTaskPrompt(tools, metadata)
	if err != nil {
		return "", fmt.Errorf("error generating task prompt: %w", err)
	}

	// 开始交互式执行
	var finalResponse string
	currentPrompt := prompt
	conversationHistory := []string{prompt}

	for {
		// 调用AI回调函数
		responseReader, err := t.AICallback(currentPrompt)
		if err != nil {
			return "", fmt.Errorf("error calling AI: %w", err)
		}

		// 读取AI的响应
		responseBytes, err := io.ReadAll(responseReader)
		if err != nil {
			return "", fmt.Errorf("error reading AI response: %w", err)
		}

		response := string(responseBytes)
		conversationHistory = append(conversationHistory, response)

		// 检查是否有工具操作请求
		matches := toolActionRegex.FindStringSubmatch(response)
		if len(matches) > 2 {
			action := matches[1]
			toolName := matches[2]

			// 处理不同的操作
			switch action {
			case "describe-tool":
				// 生成工具描述响应
				toolDescription, err := t.handleDescribeTool(tools, toolName)
				if err != nil {
					toolDescription = fmt.Sprintf("错误：%s", err.Error())
				}
				conversationHistory = append(conversationHistory, toolDescription)
				currentPrompt = strings.Join(conversationHistory, "\n\n")
				continue
			default:
				// 未知操作，继续进行
				finalResponse = response
				break
			}
		} else {
			// 没有工具操作请求，将当前响应作为最终响应
			finalResponse = response
			break
		}
	}

	return finalResponse, nil
}

// handleDescribeTool 处理描述工具的请求
func (t *Task) handleDescribeTool(tools []*Tool, toolName string) (string, error) {
	// 查找请求的工具
	var targetTool *Tool
	for _, tool := range tools {
		if tool.Name == toolName {
			targetTool = tool
			break
		}
	}

	if targetTool == nil {
		return "", fmt.Errorf("找不到名为 '%s' 的工具", toolName)
	}

	// 生成工具的JSONSchema描述
	toolJSONSchema := targetTool.ToJSONSchemaString()

	// 创建模板数据
	templateData := map[string]interface{}{
		"Tool":           targetTool,
		"ToolJSONSchema": toolJSONSchema,
	}

	// 解析工具描述模板
	tmpl, err := template.New("describe-tool").Parse(describeToolPromptTemplate)
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

// generateTaskPrompt 生成执行任务的prompt
func (t *Task) generateTaskPrompt(tools []*Tool, metadata map[string]interface{}) (string, error) {
	// 创建模板数据
	templateData := map[string]interface{}{
		"Task":     t,
		"Tools":    tools,
		"Metadata": metadata,
	}

	// 解析prompt模板
	tmpl, err := template.New("execute-task").Parse(executeTaskPromptTemplate)
	if err != nil {
		return "", fmt.Errorf("error parsing task prompt template: %w", err)
	}

	// 渲染模板
	var promptBuilder strings.Builder
	err = tmpl.Execute(&promptBuilder, templateData)
	if err != nil {
		return "", fmt.Errorf("error executing task prompt template: %w", err)
	}

	return promptBuilder.String(), nil
}

type Runtime struct {
	Freeze bool
	Task   Task
	Stack  *utils.Stack[Task]
}

func ExtractTaskFromRawResponse(rawResponse string) (*Task, error) {
	for _, item := range jsonextractor.ExtractObjectIndexes(rawResponse) {
		start, end := item[0], item[1]
		taskJSON := rawResponse[start:end]

		// 尝试解析为完整的 task 结构
		var taskObj struct {
			Tasks []struct {
				SubtaskName string `json:"subtask_name"`
				SubtaskGoal string `json:"subtask_goal"`
			} `json:"tasks"`
		}

		err := json.Unmarshal([]byte(taskJSON), &taskObj)
		if err == nil && len(taskObj.Tasks) > 0 {
			// 找到了合法的任务结构
			mainTask := &Task{
				Name:     taskObj.Tasks[0].SubtaskName,
				Goal:     taskObj.Tasks[0].SubtaskGoal,
				Subtasks: make([]Task, 0),
			}

			// 如果有多个任务，将后续任务作为子任务
			if len(taskObj.Tasks) > 1 {
				for _, subtask := range taskObj.Tasks[1:] {
					if subtask.SubtaskName != "" {
						mainTask.Subtasks = append(mainTask.Subtasks, Task{
							Name: subtask.SubtaskName,
							Goal: subtask.SubtaskGoal,
						})
					}
				}
			}

			// 检查主任务 Name 是否存在
			if mainTask.Name != "" {
				return mainTask, nil
			}
		}

		// 尝试直接解析为单个 Task 对象
		var simpleTask Task
		err = json.Unmarshal([]byte(taskJSON), &simpleTask)
		if err == nil && simpleTask.Name != "" {
			return &simpleTask, nil
		}

		// 尝试解析为一个简单的 map 并创建 Task
		var taskMap map[string]interface{}
		err = json.Unmarshal([]byte(taskJSON), &taskMap)
		if err == nil {
			if name, ok := taskMap["name"].(string); ok && name != "" {
				task := &Task{
					Name: name,
				}

				if goal, ok := taskMap["goal"].(string); ok {
					task.Goal = goal
				}

				if subtasks, ok := taskMap["subtasks"].([]interface{}); ok {
					for _, st := range subtasks {
						if subtaskMap, ok := st.(map[string]interface{}); ok {
							if stName, ok := subtaskMap["name"].(string); ok && stName != "" {
								subtask := Task{
									Name: stName,
								}

								if stGoal, ok := subtaskMap["goal"].(string); ok {
									subtask.Goal = stGoal
								}

								task.Subtasks = append(task.Subtasks, subtask)
							}
						}
					}
				}

				return task, nil
			}
		}
	}
	return nil, errors.New("no task found")
}
