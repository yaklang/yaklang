package test

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/gorm"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/ai/aid/aitool/buildinaitools/yakscripttools"
	"github.com/yaklang/yaklang/common/schema"
)

func TestSearchMemoryRuntimeBinding(t *testing.T) {
	db, err := gorm.Open("sqlite3", filepath.Join(t.TempDir(), "bound.db"))
	require.NoError(t, err)
	defer db.Close()
	require.NoError(t, db.AutoMigrate(&schema.AIMemoryEntity{}).Error)
	for _, ns := range []string{"host-bound", "default"} {
		require.NoError(t, db.Create(&schema.AIMemoryEntity{MemoryID: ns, SessionID: ns, Content: "report memory " + ns}).Error)
	}
	content, err := yakscripttools.GetEmbedFS().ReadFile("yakscriptforai/memory/search_memory.yak")
	require.NoError(t, err)
	source := yakscripttools.LoadYakScriptToAiTools("search_memory", string(content))
	require.NotNil(t, source)
	tools := yakscripttools.ConvertTools([]*schema.AIYakTool{source})
	require.Len(t, tools, 1)
	tool := tools[0]
	config := aitool.NewToolInvokeConfig()
	aitool.WithRuntimeConfig(&aitool.ToolRuntimeConfig{ProjectDatabase: db, MemoryNamespace: "host-bound"})(config)
	result, err := tool.ExecuteToolWithCapture(context.Background(), map[string]any{"query": "report", "search_mode": "bm25"}, config)
	require.NoError(t, err)
	raw, err := json.Marshal(result.Result)
	require.NoError(t, err)
	require.Contains(t, string(raw), `"namespace":"host-bound"`)
	require.Contains(t, string(raw), `"memory_id":"host-bound"`)
	require.NotContains(t, string(raw), "report memory default")
	require.NotContains(t, string(raw), "workspace")
}
