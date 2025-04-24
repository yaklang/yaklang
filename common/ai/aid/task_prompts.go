package aid

import (
	"fmt"
	"strings"
	"text/template"

	"github.com/yaklang/yaklang/common/ai/aid/aitool"

	_ "embed"
)

// generateTaskPrompt 生成执行任务的prompt
func (t *aiTask) generateTaskPrompt() (string, error) {
	// 创建模板数据
	alltools, err := t.config.aiToolManager.GetAllTools()
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
