package schema

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAIYakToolToUpdateMap_NilReceiver(t *testing.T) {
	var tool *AIYakTool
	require.Nil(t, tool.ToUpdateMap())
}

func TestAIYakToolToUpdateMap_OnlyMutableFieldsIncluded(t *testing.T) {
	updateMap := (&AIYakTool{
		Name:              "tool-name",
		VerboseName:       "verbose-name",
		Description:       "desc",
		Keywords:          "keyword-a,keyword-b",
		Usage:             "usage",
		Content:           "content",
		Params:            `{"type":"object"}`,
		Path:              "/tmp/tool.yak",
		Author:            "should-not-be-updated",
		IsBuiltin:         true,
		IsFavorite:        true,
		EnableAIOutputLog: 2,
	}).ToUpdateMap()

	mutableFields := []string{
		"name", "verbose_name", "verbose_name_zh", "description", "keywords", "usage", "content",
		"params", "path", "is_builtin", "hash", "is_favorite", "enable_ai_output_log",
	}
	require.Len(t, updateMap, len(mutableFields))
	for _, field := range mutableFields {
		require.Contains(t, updateMap, field)
	}

	require.NotContains(t, updateMap, "author")
}

func TestAIYakToolVerboseNameToI18nAndToGRPC(t *testing.T) {
	require.Nil(t, (*AIYakTool)(nil).VerboseNameToI18n())
	require.Nil(t, (&AIYakTool{}).VerboseNameToI18n())
	require.Nil(t, NewI18n("", ""))

	tool := &AIYakTool{
		Name:          "grep",
		VerboseName:   "Text Grep Tool",
		VerboseNameZh: "文本查找工具",
	}
	i18n := tool.VerboseNameToI18n()
	require.NotNil(t, i18n)
	require.Equal(t, "文本查找工具", i18n.Zh)
	require.Equal(t, "Text Grep Tool", i18n.En)
	require.Equal(t, map[string]string{"Zh": "文本查找工具", "En": "Text Grep Tool"}, i18n.ToAIOutputMap())

	grpcTool := tool.ToGRPC()
	require.Equal(t, "Text Grep Tool", grpcTool.VerboseName)
	require.NotNil(t, grpcTool.VerboseNameI18N)
	require.Equal(t, "文本查找工具", grpcTool.VerboseNameI18N.Zh)
	require.Equal(t, "Text Grep Tool", grpcTool.VerboseNameI18N.En)

	onlyZh := (&AIYakTool{VerboseNameZh: "仅中文"}).VerboseNameToI18n()
	require.Equal(t, "仅中文", onlyZh.Zh)
	require.Equal(t, "仅中文", onlyZh.En)
}
