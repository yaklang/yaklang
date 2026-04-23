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
		"name", "verbose_name", "description", "keywords", "usage", "content",
		"params", "path", "is_builtin", "hash", "is_favorite", "enable_ai_output_log",
	}
	require.Len(t, updateMap, len(mutableFields))
	for _, field := range mutableFields {
		require.Contains(t, updateMap, field)
	}

	require.NotContains(t, updateMap, "author")
}
