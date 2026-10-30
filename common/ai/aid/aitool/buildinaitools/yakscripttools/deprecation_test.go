package yakscripttools

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/schema"
)

func TestDeprecatedAIToolsSyncAndConversion(t *testing.T) {
	scripts := map[string]string{"query_pcapdb": `println("new")`}
	for _, name := range schema.DeprecatedAIToolNames() {
		// An old gzip archive must not reintroduce a retired tool.
		scripts[name] = `println("old")`
	}
	db := newIncrementalAIToolDB(t, scripts)
	for _, name := range schema.DeprecatedAIToolNames() {
		require.NoError(t, db.Create(&schema.AIYakTool{Name: name, IsBuiltin: true}).Error)
	}
	tools, err := loadAllYakScriptFromEmbedFS()
	require.NoError(t, err)
	require.Len(t, tools, 1)
	require.Equal(t, "query_pcapdb", tools[0].Name)
	require.NoError(t, syncBuiltinAITools(db, "deprecated:1"))
	var count int
	require.NoError(t, db.Unscoped().Model(&schema.AIYakTool{}).Where("name IN (?)", schema.DeprecatedAIToolNames()).Count(&count).Error)
	require.Zero(t, count, "upgrade must physically remove old builtin rows")

	previous := toolCovertHandle
	t.Cleanup(func() { toolCovertHandle = previous })
	var converted []*schema.AIYakTool
	toolCovertHandle = func(tools []*schema.AIYakTool) []*aitool.Tool {
		converted = tools
		return nil
	}
	input := []*schema.AIYakTool{{Name: "query_pcapdb"}, nil}
	for _, name := range schema.DeprecatedAIToolNames() {
		input = append(input, &schema.AIYakTool{Name: name})
	}
	ConvertTools(input)
	require.Equal(t, input[:1], converted)
}
