package aireact

import (
	"fmt"
	"io"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

// handleRequireTool handles tool requirement action using aicommon.ToolCaller
func (r *ReAct) handleRequireTool(toolName string) error {
	// Find the required tool
	tool, err := r.config.aiToolManager.GetToolByName(toolName)
	if err != nil {
		return utils.Errorf("tool '%s' not found: %v", toolName, err)
	}

	log.Infof("preparing tool: %s - %s", tool.Name, tool.Description)

	// Create ToolCaller with parameter generation prompt builder
	toolCaller, err := aicommon.NewToolCaller(
		aicommon.WithToolCaller_AICallerConfig(r.config),
		aicommon.WithToolCaller_AICaller(r.config),
		aicommon.WithToolCaller_Task(r.config.task),
		aicommon.WithToolCaller_RuntimeId(r.config.id),
		aicommon.WithToolCaller_Emitter(r.config.Emitter),
		aicommon.WithToolCaller_GenerateToolParamsBuilder(func(tool *aitool.Tool, toolName string) (string, error) {
			return r.generateToolParamsPrompt(tool, toolName)
		}),
	)
	if err != nil {
		return utils.Errorf("failed to create tool caller: %v", err)
	}

	// Call the tool using ToolCaller
	result, directlyAnswer, err := toolCaller.CallTool(tool)
	if err != nil {
		return utils.Errorf("tool call failed: %v", err)
	}

	// Handle the result
	if directlyAnswer {
		log.Infof("Tool caller suggests to answer directly without using tools")
		r.EmitInfo("AI suggests answering directly without using additional tools")
		return nil
	}

	if result != nil {
		// Store the result in memory
		if r.config.memory != nil {
			r.config.memory.PushToolCallResults(result)
		}

		// Log the result
		if result.Success {
			log.Infof("Tool '%s' executed successfully", toolName)
			if r.config.debugEvent {
				log.Infof("Tool result: %v", result.Data)
			}
		} else {
			log.Errorf("Tool '%s' execution failed: %s", toolName, result.Error)
		}

		// Emit the result
		r.EmitInfo("Tool execution completed: %s", result.Name)
	}

	return nil
}

// generateToolParamsPrompt generates the prompt for tool parameter generation
func (r *ReAct) generateToolParamsPrompt(tool *aitool.Tool, toolName string) (string, error) {
	if tool == nil {
		return "", fmt.Errorf("找不到名为 '%s' 的工具", toolName)
	}

	if r.config.memory == nil {
		return "", utils.Error("memory is not initialized")
	}

	// 生成工具的JSONSchema描述
	toolJSONSchema := tool.ToJSONSchemaString()

	// 获取原始用户查询
	originalQuery := ""
	if r.config.memory != nil {
		originalQuery = r.config.memory.Query
	}

	// Build the parameter generation prompt with schema information
	prompt := fmt.Sprintf(`# 用户原始请求
用户的明确要求: %s

## 重要说明
请严格按照用户的原始请求生成工具参数。用户要求什么就执行什么，不要过度解释或偏离用户的直接意图。

## 工具详情
工具名称: %s
工具描述: %s

## 参数生成规则
1. **直接执行用户请求** - 如果用户要求执行"ls current"，就生成参数执行"ls"命令（不带路径表示当前目录）
2. **避免过度复杂化** - 不要因为历史失败而偏离用户的直接请求
3. **参数准确性** - 确保参数结构、数据类型与Schema完全一致

## 工具参数Schema
`+"```schema\n%s\n```"+`

## 任务上下文
%s

## 历史记录（仅供参考，不要被失败记录误导）
%s

# 输出要求
严格生成标准JSON对象，格式：`+"`{\"@action\": \"call-tool\", \"tool\": \"%s\", \"params\": {...}}`"+`

重要：直接按照用户的原始请求"%s"生成对应的工具参数，不要偏离用户意图。
`, originalQuery, tool.Name, tool.Description, toolJSONSchema, r.config.memory.CurrentTaskInfo(), r.config.memory.Timeline(), tool.Name, originalQuery)

	if r.config.debugPrompt {
		log.Infof("Tool params prompt: %s", prompt)
	}

	return prompt, nil
}

// extractResponseContent extracts content from AI response (legacy method)
func (r *ReAct) extractResponseContent(resp *aicommon.AIResponse) string {
	if resp == nil {
		return ""
	}

	reader := resp.GetUnboundStreamReader(false)

	content, err := io.ReadAll(reader)
	if err != nil {
		log.Errorf("failed to read response content: %v", err)
		return ""
	}

	return string(content)
}
