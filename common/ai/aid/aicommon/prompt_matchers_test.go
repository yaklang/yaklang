package aicommon

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParameterPromptMatchersIgnoreInventoryAndHistory(t *testing.T) {
	const inventory = "# Prioritized Tools\n* first_tool\n* second_tool\n# AI Blueprint Inventory\n* test_blueprint\n"
	const history = "# Tool Context\n需要为 `first_tool` 生成参数，以下是当前工具上下文：\nold call\n"
	toolPrompt := inventory + history + "# Tool Context\n需要为 `second_tool` 生成参数，以下是当前工具上下文：\n# Parameter Generation Task\n"
	require.True(t, IsToolParamGenerationPrompt(toolPrompt, "second_tool"))
	require.True(t, IsToolParamGenPromptForTool(toolPrompt, "second_tool"))
	require.False(t, IsToolParamGenerationPrompt(toolPrompt, "first_tool"))
	require.False(t, IsToolParamGenPromptForTool(toolPrompt, "first_tool"))
	require.False(t, IsToolParamGenPromptForBlueprint(toolPrompt, "test_blueprint"))

	blueprintPrompt := inventory + history + "# Blueprint Context\nYou need to generate parameters for the AI Blueprint 'test_blueprint'.\n# Parameter Generation Task\n"
	require.True(t, IsToolParamGenerationPrompt(blueprintPrompt, "test_blueprint"))
	require.True(t, IsToolParamGenPromptForBlueprint(blueprintPrompt, "test_blueprint"))
	require.False(t, IsToolParamGenerationPrompt(blueprintPrompt, "first_tool"))
	require.False(t, IsToolParamGenPromptForTool(blueprintPrompt, "first_tool"))
	require.False(t, IsToolParamGenPromptForTool(blueprintPrompt, ""))

	legacy := "Generate appropriate parameters based on the context above and the schema. first_tool"
	require.True(t, IsToolParamGenerationPrompt(legacy, "first_tool"))
	require.False(t, IsToolParamGenerationPrompt(legacy, "second_tool"))
}
