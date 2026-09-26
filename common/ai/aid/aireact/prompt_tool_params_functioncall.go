package aireact

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
)

//go:embed prompts/tool-params/functioncall_high_static.txt
var functionCallToolParamsHighStatic string

//go:embed prompts/tool-params/functioncall_semi_dynamic_2.txt
var functionCallToolParamsSemiDynamic2 string

//go:embed prompts/tool-params/functioncall_semi_dynamic_1.txt
var functionCallToolParamsSemiDynamic1 string

//go:embed prompts/tool-params/functioncall_selected_tool.txt
var functionCallToolParamsSelectedTool string

//go:embed prompts/tool-params/functioncall_timeline_open.txt
var functionCallToolParamsTimelineOpen string

//go:embed prompts/tool-params/functioncall_dynamic.txt
var functionCallToolParamsDynamic string

// GenerateFunctionCallToolParamsPromptForTask builds R2's native prompt. It
// reuses the main loop's frozen/open timeline rendering. The full task context
// is in semi-dynamic-1, the selected business tool is in semi-dynamic-2, and
// the open timeline plus current environment form the volatile tail.
func (pm *PromptManager) GenerateFunctionCallToolParamsPromptForTask(
	task aicommon.AIStatefulTask, tool *aitool.Tool, intent aicommon.ToolParamsCallIntent,
) (string, error) {
	if tool == nil {
		return "", fmt.Errorf("selected tool is nil")
	}
	query := ""
	if task != nil {
		query = task.GetUserInput()
	}
	currentNonce := nonce()
	loop := promptLoopForTask(task)
	base, materials, err := pm.preparePromptPrefixMaterialsForLoop(nil,
		&reactloops.LoopPromptAssemblyInput{Nonce: currentNonce}, loop)
	if err != nil {
		return "", err
	}
	// R2 needs the same timeline buckets as the main decision loop, including
	// the latest model turn, while its protocol and available function are its own.
	base.PromptFrozenOpenMaterials = aicommon.BuildPromptFrozenOpenMaterialsWithLatestModelReplay(pm.react.config, currentNonce)
	aicommon.ApplyPromptFrozenOpenMaterials(materials, base.PromptFrozenOpenMaterials)
	materials.FunctionCallMode = true
	materials.AllowToolCall = false
	materials.ToolInventory = false
	materials.ForgeInventory = false
	materials.TopTools = nil
	materials.ForcedSkills = ""
	materials.SkillsContext = ""
	materials.OutputExample = ""
	materials.Schema = ""
	materials.ExecutionPolicy = ""
	materials.OriginalUserInput = escapeR2PromptControlTags(query)

	inputSchema, err := json.MarshalIndent(tool.InputSchema, "", "  ")
	if err != nil {
		return "", fmt.Errorf("marshal selected tool schema: %w", err)
	}
	toolDescription := tool.Description
	if toolDescription == "" {
		toolDescription = tool.Name
	}
	selectedTool, err := aicommon.RenderPromptTemplate("r2-selected-tool", functionCallToolParamsSelectedTool,
		map[string]any{
			"ToolName": tool.Name, "ToolDescription": toolDescription,
			"ToolUsage": tool.Usage, "ToolInputSchema": string(inputSchema),
		})
	if err != nil {
		return "", err
	}
	tools, err := buildFunctionCallParamTools()
	if err != nil {
		return "", err
	}
	toolTags, err := renderFunctionCallParamSchemaTags(tools)
	if err != nil {
		return "", err
	}
	materials.FunctionCallSchemas = toolTags
	materials.TaskInstruction = escapeR2PromptControlTags(selectedTool)
	callIntent := ""
	if intent.Reason != "" || intent.DestinationIdentifier != "" || intent.CallExpectations != "" {
		encoded, err := json.Marshal(map[string]string{
			"reason": intent.Reason, "identifier": intent.DestinationIdentifier,
			"call_expectations": intent.CallExpectations,
		})
		if err != nil {
			return "", fmt.Errorf("marshal tool invocation intent: %w", err)
		}
		callIntent = escapeR2PromptControlTags(string(encoded))
	}
	builder := newFunctionCallToolParamsPrefixBuilder()
	return builder.AssemblePromptWithDynamicSection(materials,
		"r2-functioncall-dynamic", functionCallToolParamsDynamic,
		map[string]any{
			"RecentUserInput": escapeR2PromptControlTags(materials.UserHistory), "CallIntent": callIntent,
			"CurrentTime": base.CurrentTime, "OSArch": base.OSArch,
			"WorkingDir": base.WorkingDir, "WorkingDirGlance": base.WorkingDirGlance,
		}, currentNonce)
}

func newFunctionCallToolParamsPrefixBuilder() *aicommon.PromptPrefixBuilder {
	builder := aicommon.NewDefaultPromptPrefixBuilder()
	builder.HighStaticTemplateName = "r2-functioncall-high-static"
	builder.HighStaticTemplate = functionCallToolParamsHighStatic
	builder.SemiDynamicTemplateName = "r2-functioncall-semi-dynamic-1"
	builder.SemiDynamicTemplate = functionCallToolParamsSemiDynamic1
	builder.SemiDynamic2TemplateName = "r2-functioncall-semi-dynamic-2"
	builder.SemiDynamic2Template = functionCallToolParamsSemiDynamic2
	builder.TimelineOpenTemplateName = "r2-functioncall-timeline-open"
	builder.TimelineOpenTemplate = functionCallToolParamsTimelineOpen
	return builder
}

// Selected tool metadata and user input are data, never a source of projection
// tags. Escape only control-tag openers; other AITAGs (such as task context)
// remain readable by the model.
func escapeR2PromptControlTags(content string) string {
	replacer := strings.NewReplacer(
		"<|SCHEMA|>", "&lt;|SCHEMA|>",
		"<|FUNCTION_CALL_TOOL_PARAM_SCHEMA_", "&lt;|FUNCTION_CALL_TOOL_PARAM_SCHEMA_",
		"<|FUNCTION_CALL_ACTION_SCHEMA_", "&lt;|FUNCTION_CALL_ACTION_SCHEMA_",
		"<|PROMPT_SECTION_", "&lt;|PROMPT_SECTION_",
		"<|AI_CACHE_SYSTEM_", "&lt;|AI_CACHE_SYSTEM_",
		"<|AI_CACHE_FROZEN_", "&lt;|AI_CACHE_FROZEN_",
		"<|AI_CACHE_SEMI2_", "&lt;|AI_CACHE_SEMI2_",
		"<|AI_CACHE_SEMI_", "&lt;|AI_CACHE_SEMI_",
	)
	return replacer.Replace(content)
}
