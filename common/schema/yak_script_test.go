package schema

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestYakScriptToUpdateMap_NilReceiver(t *testing.T) {
	var script *YakScript
	require.Nil(t, script.ToUpdateMap())
}

func TestYakScriptToUpdateMap_OnlyMutableFieldsIncluded(t *testing.T) {
	updateMap := (&YakScript{
		ScriptName:           "test-script",
		Type:                 "yak",
		Content:              "print('ok')",
		Params:               "\"[]\"",
		Help:                 "help",
		Level:                "high",
		Tags:                 "tag-a,tag-b",
		IsHistory:            true,
		Ignored:              true,
		IsGeneralModule:      true,
		GeneralModuleVerbose: "verbose",
		GeneralModuleKey:     "module-key",
		FromGit:              "https://example.com/repo.git",
		EnablePluginSelector: true,
		PluginSelectorTypes:  "mitm,port-scan",
		IsCorePlugin:         true,
		RiskType:             "sqli",
		RiskDetail:           "{}",
		RiskAnnotation:       "annotation",
		PluginEnvKey:         "\"[]\"",
		EnableForAI:          true,
		AIDesc:               "ai-desc",
		AIKeywords:           "k1,k2",
		AIUsage:              "usage",
		Author:               "should-not-be-updated",
		OnlineId:             12345,
		Uuid:                 "should-be-preserved",
		SkipUpdate:           true,
	}).ToUpdateMap()

	mutableFields := []string{
		"script_name", "type", "content", "params", "help", "level", "tags", "is_history",
		"ignored", "is_general_module", "general_module_verbose", "general_module_key", "from_git",
		"enable_plugin_selector", "plugin_selector_types", "is_core_plugin", "risk_type",
		"risk_detail", "risk_annotation", "plugin_env_key", "enable_for_ai", "ai_desc",
		"ai_keywords", "ai_usage",
	}
	require.Len(t, updateMap, len(mutableFields))
	for _, field := range mutableFields {
		require.Contains(t, updateMap, field)
	}

	protectedFields := []string{
		"author", "from_local", "local_path", "force_interactive", "from_store",
		"is_batch_script", "is_external", "online_id", "online_script_name",
		"online_contributors", "online_is_private", "user_id", "uuid", "head_img",
		"online_base_url", "base_online_id", "online_official", "online_group",
		"collaborator_info", "skip_update",
	}
	for _, field := range protectedFields {
		require.NotContains(t, updateMap, field)
	}
}
