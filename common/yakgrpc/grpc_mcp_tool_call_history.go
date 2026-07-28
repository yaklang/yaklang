package yakgrpc

import (
	"context"

	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

// QueryMCPToolCallHistory lists calls made by external clients to Yaklang MCP tools.
func (s *Server) QueryMCPToolCallHistory(
	ctx context.Context,
	req *ypb.QueryMCPToolCallHistoryRequest,
) (*ypb.QueryMCPToolCallHistoryResponse, error) {
	paginator, histories, err := yakit.QueryMCPToolCallHistories(s.GetProfileDatabase(), req)
	if err != nil {
		return nil, err
	}

	items := make([]*ypb.MCPToolCallHistorySummary, 0, len(histories))
	for _, history := range histories {
		items = append(items, &ypb.MCPToolCallHistorySummary{
			ID:             int64(history.ID),
			ToolName:       history.ToolName,
			Success:        history.Success,
			ErrorMessage:   history.ErrorMessage,
			DurationMillis: history.DurationMillis,
			ClientID:       history.ClientID,
			SessionID:      history.SessionID,
			ClientName:     history.ClientName,
			ClientVersion:  history.ClientVersion,
			CreatedAt:      history.CreatedAt.Unix(),
		})
	}
	return &ypb.QueryMCPToolCallHistoryResponse{
		Histories: items,
		Pagination: &ypb.Paging{
			Page:  int64(paginator.Page),
			Limit: int64(paginator.Limit),
		},
		Total: int64(paginator.TotalRecord),
	}, nil
}

// GetMCPToolCallHistoryDetail returns the complete payload of one MCP tool call.
func (s *Server) GetMCPToolCallHistoryDetail(
	ctx context.Context,
	req *ypb.GetMCPToolCallHistoryDetailRequest,
) (*ypb.MCPToolCallHistory, error) {
	history, err := yakit.GetMCPToolCallHistory(s.GetProfileDatabase(), req.GetID())
	if err != nil {
		return nil, err
	}
	return &ypb.MCPToolCallHistory{
		ID:             int64(history.ID),
		ToolName:       history.ToolName,
		Arguments:      history.Arguments,
		Result:         history.Result,
		Success:        history.Success,
		ErrorMessage:   history.ErrorMessage,
		DurationMillis: history.DurationMillis,
		ClientID:       history.ClientID,
		SessionID:      history.SessionID,
		ClientName:     history.ClientName,
		ClientVersion:  history.ClientVersion,
		CreatedAt:      history.CreatedAt.Unix(),
	}, nil
}

// DeleteMCPToolCallHistory deletes selected MCP histories or clears all of them.
func (s *Server) DeleteMCPToolCallHistory(
	ctx context.Context,
	req *ypb.DeleteMCPToolCallHistoryRequest,
) (*ypb.Empty, error) {
	if err := yakit.DeleteMCPToolCallHistories(s.GetProfileDatabase(), req); err != nil {
		return nil, err
	}
	return &ypb.Empty{}, nil
}
