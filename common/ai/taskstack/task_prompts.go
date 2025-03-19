package taskstack

import (
	"fmt"
	"strings"
	"text/template"

	_ "embed"
)

// generateTaskPrompt 生成执行任务的prompt
func (t *Task) generateTaskPrompt(tools []*Tool, systemContext *TaskSystemContext, metadata map[string]interface{}) (string, error) {
	// 创建模板数据
	templateData := map[string]interface{}{
		"Task":            t,
		"Tools":           tools,
		"Metadata":        metadata,
		"Context":         systemContext,
		"ToolCallHistory": t.ToolCallResults,
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

// generateRequireToolResponsePrompt 生成描述工具参数的 Prompt
func (t *Task) generateRequireToolResponsePrompt(runtime *TaskSystemContext, targetTool *Tool, toolName string) (string, error) {
	if targetTool == nil {
		return "", fmt.Errorf("找不到名为 '%s' 的工具", toolName)
	}

	// 生成工具的JSONSchema描述
	toolJSONSchema := targetTool.ToJSONSchemaString()
	// 创建模板数据
	templateData := map[string]interface{}{
		"Runtime":        runtime,
		"Task":           t,
		"Tool":           targetTool,
		"ToolJSONSchema": toolJSONSchema,
	}

	// 解析工具描述模板
	tmpl, err := template.New("call-tool").Parse(toolParamSchemaPromptTemplate)
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
func (t *Task) generateToolCallResponsePrompt(result *ToolResult, runtime *TaskSystemContext, targetTool *Tool) (string, error) {
	templatedata := map[string]any{
		"Runtime": runtime,
		"Task":    t,
		"Tool":    targetTool,
		"Result":  result,
	}
	temp, err := template.New("tool-result").Parse(toolResultToDecisionPromptTemplate)
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

func (t *Task) generateToolCallResultsPrompt() (string, error) {
	templatedata := map[string]interface{}{
		"ToolCallResults": t.ToolCallResults,
	}
	temp, err := template.New("tool-result-history").Parse(toolResultHistoryPromptTemplate)
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
