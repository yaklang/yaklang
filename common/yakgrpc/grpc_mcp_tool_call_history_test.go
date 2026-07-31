package yakgrpc

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/gorm"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func TestGRPCMCPToolCallHistoryQueryDetailAndDelete(t *testing.T) {
	db, err := gorm.Open("sqlite3", ":memory:")
	require.NoError(t, err)
	defer func() {
		require.NoError(t, db.Close())
	}()
	require.NoError(t, db.AutoMigrate(&schema.MCPToolCallHistory{}).Error)
	defer func() {
		require.NoError(t, db.Unscoped().Where("id > 0").Delete(&schema.MCPToolCallHistory{}).Error)
	}()

	records := []*schema.MCPToolCallHistory{
		{
			ToolName:      "port_scan",
			Arguments:     `{"targets":["127.0.0.1"]}`,
			Result:        `{"content":[{"type":"text","text":"open"}]}`,
			Success:       true,
			ClientName:    "Mock Agent",
			ClientVersion: "1.2.3",
			ClientID:      "client-1",
			SessionID:     "session-1",
		},
		{
			ToolName:      "syntaxflow_scan",
			Arguments:     `{"target":"demo"}`,
			Result:        `null`,
			Success:       false,
			ErrorMessage:  strings.Repeat("错", 600),
			ClientName:    "Other Agent",
			ClientVersion: "2.0.0",
			ClientID:      "client-2",
			SessionID:     "session-2",
		},
	}
	for _, record := range records {
		require.NoError(t, db.Create(record).Error)
	}

	server := &Server{profileDatabase: db}
	ctx := context.Background()
	queryResponse, err := server.QueryMCPToolCallHistory(ctx, &ypb.QueryMCPToolCallHistoryRequest{
		Keyword: "Mock Agent",
		Status:  "success",
		Pagination: &ypb.Paging{
			Page:  1,
			Limit: 500,
		},
	})
	require.NoError(t, err)
	require.Equal(t, int64(1), queryResponse.GetTotal())
	require.Equal(t, int64(100), queryResponse.GetPagination().GetLimit())
	require.Len(t, queryResponse.GetHistories(), 1)
	summary := queryResponse.GetHistories()[0]
	require.Equal(t, "port_scan", summary.GetToolName())
	require.Equal(t, "Mock Agent", summary.GetClientName())
	require.Equal(t, "1.2.3", summary.GetClientVersion())
	require.True(t, summary.GetSuccess())

	detail, err := server.GetMCPToolCallHistoryDetail(ctx, &ypb.GetMCPToolCallHistoryDetailRequest{
		ID: summary.GetID(),
	})
	require.NoError(t, err)
	require.Equal(t, `{"targets":["127.0.0.1"]}`, detail.GetArguments())
	require.Contains(t, detail.GetResult(), "open")
	require.Equal(t, "port_scan", detail.GetToolName())

	failedResponse, err := server.QueryMCPToolCallHistory(ctx, &ypb.QueryMCPToolCallHistoryRequest{
		Status:     "failed",
		Pagination: &ypb.Paging{Page: 1, Limit: 30},
	})
	require.NoError(t, err)
	require.Len(t, failedResponse.GetHistories(), 1)
	require.Len(t, []rune(failedResponse.GetHistories()[0].GetErrorMessage()), 500)
	failedDetail, err := server.GetMCPToolCallHistoryDetail(ctx, &ypb.GetMCPToolCallHistoryDetailRequest{
		ID: failedResponse.GetHistories()[0].GetID(),
	})
	require.NoError(t, err)
	require.Len(t, []rune(failedDetail.GetErrorMessage()), 600)

	_, err = server.GetMCPToolCallHistoryDetail(ctx, &ypb.GetMCPToolCallHistoryDetailRequest{ID: 0})
	require.Error(t, err)

	_, err = server.DeleteMCPToolCallHistory(ctx, &ypb.DeleteMCPToolCallHistoryRequest{
		IDs: []int64{summary.GetID()},
	})
	require.NoError(t, err)

	queryResponse, err = server.QueryMCPToolCallHistory(ctx, &ypb.QueryMCPToolCallHistoryRequest{
		Pagination: &ypb.Paging{Page: 1, Limit: 30},
	})
	require.NoError(t, err)
	require.Equal(t, int64(1), queryResponse.GetTotal())
	require.Equal(t, "syntaxflow_scan", queryResponse.GetHistories()[0].GetToolName())

	filteredRecords := []*schema.MCPToolCallHistory{
		{
			ToolName:   "kept_success",
			Success:    true,
			ClientName: "Other Agent",
		},
		{
			ToolName:   "kept_failed",
			Success:    false,
			ClientName: "Third Agent",
		},
	}
	for _, record := range filteredRecords {
		require.NoError(t, db.Create(record).Error)
	}
	softDeletedRecord := &schema.MCPToolCallHistory{
		ToolName:   "soft_deleted",
		Success:    false,
		ClientName: "Other Agent",
	}
	require.NoError(t, db.Create(softDeletedRecord).Error)
	require.NoError(t, db.Delete(softDeletedRecord).Error)

	_, err = server.DeleteMCPToolCallHistory(ctx, &ypb.DeleteMCPToolCallHistoryRequest{
		DeleteFiltered: true,
		Keyword:        "Other Agent",
		Status:         "failed",
	})
	require.NoError(t, err)
	queryResponse, err = server.QueryMCPToolCallHistory(ctx, &ypb.QueryMCPToolCallHistoryRequest{
		Pagination: &ypb.Paging{Page: 1, Limit: 30},
	})
	require.NoError(t, err)
	require.Equal(t, int64(2), queryResponse.GetTotal())
	remainingTools := make(map[string]struct{}, len(queryResponse.GetHistories()))
	for _, history := range queryResponse.GetHistories() {
		remainingTools[history.GetToolName()] = struct{}{}
	}
	require.Contains(t, remainingTools, "kept_success")
	require.Contains(t, remainingTools, "kept_failed")
	var retainedSoftDeleted schema.MCPToolCallHistory
	require.NoError(t, db.Unscoped().Where("id = ?", softDeletedRecord.ID).First(&retainedSoftDeleted).Error)

	_, err = server.DeleteMCPToolCallHistory(ctx, &ypb.DeleteMCPToolCallHistoryRequest{
		DeleteFiltered: true,
	})
	require.Error(t, err)
	_, err = server.DeleteMCPToolCallHistory(ctx, &ypb.DeleteMCPToolCallHistoryRequest{
		DeleteFiltered: true,
		Status:         "unknown",
	})
	require.Error(t, err)
	_, err = server.DeleteMCPToolCallHistory(ctx, &ypb.DeleteMCPToolCallHistoryRequest{
		IDs:            []int64{int64(filteredRecords[0].ID)},
		DeleteFiltered: true,
		Status:         "success",
	})
	require.Error(t, err)

	_, err = server.DeleteMCPToolCallHistory(ctx, &ypb.DeleteMCPToolCallHistoryRequest{DeleteAll: true})
	require.NoError(t, err)
	queryResponse, err = server.QueryMCPToolCallHistory(ctx, &ypb.QueryMCPToolCallHistoryRequest{
		Pagination: &ypb.Paging{Page: 1, Limit: 30},
	})
	require.NoError(t, err)
	require.Zero(t, queryResponse.GetTotal())
	var physicalCount int
	require.NoError(t, db.Unscoped().Model(&schema.MCPToolCallHistory{}).Count(&physicalCount).Error)
	require.Zero(t, physicalCount)

	_, err = server.DeleteMCPToolCallHistory(ctx, &ypb.DeleteMCPToolCallHistoryRequest{})
	require.Error(t, err)
	_, err = server.DeleteMCPToolCallHistory(ctx, &ypb.DeleteMCPToolCallHistoryRequest{
		IDs:       []int64{1},
		DeleteAll: true,
	})
	require.Error(t, err)
}
