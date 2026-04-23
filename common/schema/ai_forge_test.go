package schema

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAIForgeToUpdateMap_NilReceiver(t *testing.T) {
	var forge *AIForge
	require.Nil(t, forge.ToUpdateMap())
}

func TestAIForgeToUpdateMap_OnlyMutableFieldsIncluded(t *testing.T) {
	updateMap := (&AIForge{
		ForgeVerboseName:   "verbose",
		ForgeName:          "forge-name",
		ForgeContent:       "content",
		ForgeType:          FORGE_TYPE_YAK,
		ParamsUIConfig:     `{"type":"object"}`,
		Params:             "params",
		UserPersistentData: "data",
		Description:        "desc",
		Tools:              "tool-a,tool-b",
		ToolKeywords:       "kw-a,kw-b",
		Actions:            "action",
		Tags:               "tag-a,tag-b",
		Author:             "should-not-be-updated",
		IsBuiltin:          true,
		InitPrompt:         "init",
		PersistentPrompt:   "persistent",
		PlanPrompt:         "plan",
		ResultPrompt:       "result",
		SkillPath:          "/tmp/skill",
		FSBytes:            []byte("zip-bytes"),
		IsTemporary:        true,
	}).ToUpdateMap()

	mutableFields := []string{
		"forge_verbose_name", "forge_name", "forge_content", "forge_type", "params_ui_config",
		"params", "user_persistent_data", "description", "tools", "tool_keywords", "actions",
		"tags", "is_builtin", "init_prompt", "persistent_prompt", "plan_prompt", "result_prompt", "skill_path",
		"fs_bytes", "is_temporary",
	}
	require.Len(t, updateMap, len(mutableFields))
	for _, field := range mutableFields {
		require.Contains(t, updateMap, field)
	}

	require.NotContains(t, updateMap, "author")
}
