package yakit

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/gorm"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func TestQueryMCPToolCallHistories(t *testing.T) {
	db, err := gorm.Open("sqlite3", ":memory:")
	require.NoError(t, err)
	defer func() {
		require.NoError(t, db.Close())
	}()
	require.NoError(t, db.AutoMigrate(&schema.MCPToolCallHistory{}).Error)
	defer func() {
		require.NoError(t, db.Unscoped().Where("id > 0").Delete(&schema.MCPToolCallHistory{}).Error)
	}()

	histories := []*schema.MCPToolCallHistory{
		{ToolName: "port_scan", ClientID: "codex", Success: true},
		{ToolName: "syntaxflow_scan", ClientID: "claude", Success: false},
		{ToolName: "http_fuzzer", SessionID: "codex-session", Success: true},
	}
	for _, history := range histories {
		require.NoError(t, db.Create(history).Error)
	}

	_, items, err := QueryMCPToolCallHistories(db, &ypb.QueryMCPToolCallHistoryRequest{
		Keyword: "codex",
		Status:  "success",
		Pagination: &ypb.Paging{
			Page:  1,
			Limit: 10,
		},
	})
	require.NoError(t, err)
	require.Len(t, items, 2)

	paginator, items, err := QueryMCPToolCallHistories(db, &ypb.QueryMCPToolCallHistoryRequest{
		Status: "failed",
		Pagination: &ypb.Paging{
			Page:  1,
			Limit: 1,
		},
	})
	require.NoError(t, err)
	require.Equal(t, 1, paginator.TotalRecord)
	require.Len(t, items, 1)
	require.Equal(t, "syntaxflow_scan", items[0].ToolName)
}
