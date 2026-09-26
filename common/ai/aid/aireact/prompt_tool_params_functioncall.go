package aireact

import (
	_ "embed"
	"encoding/json"
	"fmt"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
)

//go:embed prompts/tool-params/functioncall_high_static.txt
var functionCallToolParamsHighStatic string

//go:embed prompts/tool-params/functioncall_semi_dynamic_2.txt
var functionCallToolParamsSemiDynamic2 string

//go:embed prompts/tool-params/functioncall_dynamic.txt
var functionCallToolParamsDynamic string

// GenerateFunctionCallToolParamsPromptForTask builds R2's native prompt. It
// reuses the main loop's frozen/open timeline rendering, but keeps the selected
// business tool schema after every cache boundary. The only projected native
// function is the stable submit_tool_params declaration in semi-dynamic-2.
func (pm *PromptManager) GenerateFunctionCallToolParamsPromptForTask(
	task aicommon.AIStatefulTask, tool *aitool.Tool,
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
	materials.AllowToolCall = false
	materials.ToolInventory = false
	materials.ForgeInventory = false
	materials.TopTools = nil
	materials.ForcedSkills = ""
	materials.FrozenPartitions = nil
	materials.SessionEvidenceFrozen = ""
	materials.SessionEvidenceOpen = ""
	materials.PromotedSemiDynamic1 = ""
	materials.PromotedTimelineOpen = ""
	materials.SkillsContext = ""
	materials.TaskInstruction = ""
	materials.OutputExample = ""
	materials.Schema = ""
	materials.FunctionCallSchemas = ""
	materials.ExecutionPolicy = ""

	inputSchema, err := json.MarshalIndent(tool.InputSchema, "", "  ")
	if err != nil {
		return "", fmt.Errorf("marshal selected tool schema: %w", err)
	}
	builder := aicommon.NewDefaultPromptPrefixBuilder()
	builder.HighStaticTemplateName = "r2-functioncall-high-static"
	builder.HighStaticTemplate = functionCallToolParamsHighStatic
	builder.SemiDynamic2TemplateName = "r2-functioncall-semi-dynamic-2"
	builder.SemiDynamic2Template = functionCallToolParamsSemiDynamic2
	return builder.AssemblePromptWithDynamicSection(materials,
		"r2-functioncall-dynamic", functionCallToolParamsDynamic,
		map[string]any{
			"UserQuery": query, "ToolName": tool.Name,
			"ToolDescription": tool.Description, "ToolUsage": tool.Usage,
			"ToolInputSchema": string(inputSchema),
			"CurrentTime":     base.CurrentTime, "WorkingDir": base.WorkingDir,
		}, currentNonce)
}
