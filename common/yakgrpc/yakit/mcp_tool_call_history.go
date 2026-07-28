package yakit

import (
	"strings"

	"github.com/yaklang/gorm"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/bizhelper"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

const (
	defaultMCPToolCallHistoryPageSize = 30
	maxMCPToolCallHistoryPageSize     = 100
)

func filterMCPToolCallHistories(query *gorm.DB, keyword, status string) *gorm.DB {
	if keyword = strings.TrimSpace(keyword); keyword != "" {
		like := "%" + keyword + "%"
		query = query.Where(
			"tool_name LIKE ? OR client_name LIKE ? OR client_id LIKE ? OR session_id LIKE ?",
			like,
			like,
			like,
			like,
		)
	}
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "success":
		query = query.Where("success = ?", true)
	case "failed":
		query = query.Where("success = ?", false)
	}
	return query
}

// QueryMCPToolCallHistories returns external MCP calls in reverse chronological order.
func QueryMCPToolCallHistories(
	db *gorm.DB,
	req *ypb.QueryMCPToolCallHistoryRequest,
) (*bizhelper.Paginator, []*schema.MCPToolCallHistory, error) {
	if req == nil {
		req = &ypb.QueryMCPToolCallHistoryRequest{}
	}
	paging := req.GetPagination()
	if paging == nil {
		paging = &ypb.Paging{Page: 1, Limit: defaultMCPToolCallHistoryPageSize}
	}
	if paging.Page <= 0 {
		paging.Page = 1
	}
	if paging.Limit <= 0 {
		paging.Limit = defaultMCPToolCallHistoryPageSize
	}
	if paging.Limit > maxMCPToolCallHistoryPageSize {
		paging.Limit = maxMCPToolCallHistoryPageSize
	}

	// Arguments and Result may contain large tool payloads. Keep them out of the
	// list query and cap the error preview; callers load the complete record
	// through the detail endpoint.
	query := db.Model(&schema.MCPToolCallHistory{}).Select(
		"id, created_at, updated_at, deleted_at, tool_name, success, " +
			"substr(error_message, 1, 500) AS error_message, duration_millis, " +
			"client_id, session_id, client_name, client_version",
	)
	query = filterMCPToolCallHistories(query, req.GetKeyword(), req.GetStatus())
	query = query.Order("created_at desc").Order("id desc")

	var histories []*schema.MCPToolCallHistory
	paginator, result := bizhelper.Paging(query, int(paging.Page), int(paging.Limit), &histories)
	return paginator, histories, result.Error
}

// GetMCPToolCallHistory returns one complete MCP tool call record.
func GetMCPToolCallHistory(db *gorm.DB, id int64) (*schema.MCPToolCallHistory, error) {
	if id <= 0 {
		return nil, utils.Error("mcp tool call history id must be positive")
	}
	var history schema.MCPToolCallHistory
	if result := db.Where("id = ?", id).First(&history); result.Error != nil {
		return nil, utils.Wrap(result.Error, "get mcp tool call history failed")
	}
	return &history, nil
}

// DeleteMCPToolCallHistories permanently deletes selected, filtered, or all records.
func DeleteMCPToolCallHistories(db *gorm.DB, req *ypb.DeleteMCPToolCallHistoryRequest) error {
	if req == nil {
		return utils.Error("delete mcp tool call history request is required")
	}

	hasIDs := len(req.GetIDs()) > 0
	modeCount := 0
	for _, enabled := range []bool{hasIDs, req.GetDeleteAll(), req.GetDeleteFiltered()} {
		if enabled {
			modeCount++
		}
	}
	if modeCount != 1 {
		return utils.Error("exactly one delete mode is required")
	}

	if req.GetDeleteAll() {
		if strings.TrimSpace(req.GetKeyword()) != "" || strings.TrimSpace(req.GetStatus()) != "" {
			return utils.Error("delete all cannot be combined with history filters")
		}
		if result := db.Unscoped().Where("id > 0").Delete(&schema.MCPToolCallHistory{}); result.Error != nil {
			return utils.Wrap(result.Error, "delete all mcp tool call histories failed")
		}
		return nil
	}

	if req.GetDeleteFiltered() {
		keyword := strings.TrimSpace(req.GetKeyword())
		status := strings.ToLower(strings.TrimSpace(req.GetStatus()))
		if status != "" && status != "success" && status != "failed" {
			return utils.Error("mcp tool call history status must be success or failed")
		}
		if keyword == "" && status == "" {
			return utils.Error("at least one mcp tool call history filter is required")
		}
		query := filterMCPToolCallHistories(
			db.Unscoped().Where("id > 0").Where("deleted_at IS NULL"),
			keyword,
			status,
		)
		if result := query.Delete(&schema.MCPToolCallHistory{}); result.Error != nil {
			return utils.Wrap(result.Error, "delete filtered mcp tool call histories failed")
		}
		return nil
	}

	if strings.TrimSpace(req.GetKeyword()) != "" || strings.TrimSpace(req.GetStatus()) != "" {
		return utils.Error("history ids cannot be combined with history filters")
	}
	validIDs := make([]int64, 0, len(req.GetIDs()))
	for _, id := range req.GetIDs() {
		if id <= 0 {
			return utils.Error("mcp tool call history ids must be positive")
		}
		validIDs = append(validIDs, id)
	}
	if result := db.Unscoped().Where("id IN (?)", validIDs).Delete(&schema.MCPToolCallHistory{}); result.Error != nil {
		return utils.Wrap(result.Error, "delete mcp tool call histories failed")
	}
	return nil
}
