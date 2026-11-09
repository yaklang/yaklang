package yakit

import (
	"strings"

	"github.com/yaklang/gorm"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/bizhelper"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func normalizeAIYakToolFilterIDs(filter *ypb.AIToolFilter) []int64 {
	if filter == nil {
		return nil
	}

	result := make([]int64, 0, len(filter.GetIDs())+1)
	seen := make(map[int64]struct{}, len(filter.GetIDs())+1)
	appendID := func(id int64) {
		if id <= 0 {
			return
		}
		if _, ok := seen[id]; ok {
			return
		}
		seen[id] = struct{}{}
		result = append(result, id)
	}

	appendID(filter.GetID())
	for _, id := range filter.GetIDs() {
		appendID(id)
	}
	return result
}

func normalizeAIYakToolFilterNames(filter *ypb.AIToolFilter) []string {
	if filter == nil {
		return nil
	}

	result := make([]string, 0, len(filter.GetToolNames())+1)
	seen := make(map[string]struct{}, len(filter.GetToolNames())+1)
	appendName := func(name string) {
		name = strings.TrimSpace(name)
		if name == "" {
			return
		}
		if _, ok := seen[name]; ok {
			return
		}
		seen[name] = struct{}{}
		result = append(result, name)
	}

	appendName(filter.GetToolName())
	for _, name := range filter.GetToolNames() {
		appendName(name)
	}
	return result
}

// QueryAIYakTools queries active AI tools using the shared filter and paging contract.
func QueryAIYakTools(
	db *gorm.DB,
	filter *ypb.AIToolFilter,
	paging *ypb.Paging,
) (*bizhelper.Paginator, []*schema.AIYakTool, error) {
	if db == nil {
		return nil, nil, utils.Error("database is nil")
	}

	db = FilterAIYakTool(db, filter)
	db = bizhelper.OrderByPaging(db, paging)
	tools := make([]*schema.AIYakTool, 0)
	paginator, result := bizhelper.YakitPagingQuery(db, paging, &tools)
	if result.Error != nil {
		return nil, nil, utils.Errorf("query AI tools failed: %s", result.Error)
	}
	return paginator, tools, nil
}
