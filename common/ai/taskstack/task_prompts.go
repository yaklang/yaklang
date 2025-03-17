package taskstack

import (
	"bytes"
	"fmt"
	"strings"
	"text/template"

	_ "embed"

	"github.com/yaklang/yaklang/common/log"
)

//go:embed prompts/tool-prompt.txt
var toolPrompt string

func (t *Task) ToolPrompt() string {
	if len(t.tools) <= 0 {
		return ""
	}
	var buf bytes.Buffer
	temp, err := template.New("tool-prompt").Parse(toolPrompt)
	if err != nil {
		log.Errorf("error parsing tool prompt template: %v", err)
		return ""
	}
	err = temp.Execute(&buf, map[string]any{
		"Tools": t.tools,
	})
	if err != nil {
		log.Errorf("error for rendering tool prompt: %v", err)
		return ""
	}
	return buf.String()
}

func (t *Task) generateToolCallResponsePrompt(result *ToolResult, runtime *TaskSystemContext, targetTool *Tool) (string, error) {
	templatedata := map[string]any{
		"Runtime": runtime,
		"Task":    t,
		"Tool":    targetTool,
		"Result":  result,
	}
	temp, err := template.New("tool-result").Parse(toolResultPromptTemplate)
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

// handleDescribeTool 处理描述工具的请求
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
func (t *Task) generateTaskPrompt(tools []*Tool, systemContext *TaskSystemContext, metadata map[string]interface{}) (string, error) {
	// 创建模板数据
	templateData := map[string]interface{}{
		"Task":     t,
		"Tools":    tools,
		"Metadata": metadata,
		"Runtime":  systemContext,
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
