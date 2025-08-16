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

	// Build the parameter generation prompt with schema information
	prompt := fmt.Sprintf(`## 任务状态与进度
%s

## 工具详情
工具名称: %s
工具描述: %s

## 工具参数Schema

作为JSON工具调用引擎，请依据以下原则生成符合Schema的参数：

# 核心原则
1. **参数完整性**
   - 确保参数结构、数据类型、字段名称与Schema定义完全一致
   - 对格式敏感字段（如URL/日期）进行有效性验证

2. **生成策略**
   - 动态分析历史参数特征，建立差异化生成模式
   - 对枚举类参数采用分布式选择策略
   - 数值参数应体现合理波动范围

3. **质量保障**
   - 执行参数生成前后双重校验机制
   - 发现Schema冲突时自动中止并记录异常
   - 建立参数相似度预警机制

# 输出要求
• 严格生成标准JSON对象
• 禁止包含Schema未定义的字段
• 嵌套对象保持合理深度层级
• 仅输出JSON对象即可，不需要输出解释/执行流程/注意事项等

# History
%s

`+"```schema\n%s\n```"+`
请根据Schema描述构造有效JSON对象来调用此工具，系统会执行工具内容。

一般来说，你应该生成数据类似于：`+"`{\"@action\": \"call-tool\", \"tool\": ..., \"params\": ... }`"+`。

注意观察历史记录中已有的参数，不要重复使用相似参数执行工具，已经执行过的结果不要重复执行
`, r.config.memory.CurrentTaskInfo(), tool.Name, tool.Description, r.config.memory.Timeline(), toolJSONSchema)

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
