package yakit

import (
	"strings"

	"github.com/yaklang/gorm"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/bizhelper"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func normalizeKnowledgeBaseFilterIDs(filter *ypb.KnowledgeBaseFilter) []int64 {
	if filter == nil {
		return nil
	}

	result := make([]int64, 0, len(filter.GetIDs()))
	seen := make(map[int64]struct{}, len(filter.GetIDs()))
	for _, id := range filter.GetIDs() {
		if id <= 0 {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		result = append(result, id)
	}
	return result
}

func normalizeKnowledgeBaseFilterStrings(values []string) []string {
	result := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}

// FilterKnowledgeBaseByQuery applies the query filter contract. Unlike the
// legacy GetKnowledgeBase request, independent filter groups are combined.
func FilterKnowledgeBaseByQuery(db *gorm.DB, filter *ypb.KnowledgeBaseFilter) *gorm.DB {
	db = db.Model(&schema.KnowledgeBaseInfo{})
	if filter == nil {
		return db
	}

	if ids := normalizeKnowledgeBaseFilterIDs(filter); len(ids) > 0 {
		db = bizhelper.ExactQueryInt64ArrayOr(db, "id", ids)
	}
	if names := normalizeKnowledgeBaseFilterStrings(filter.GetNames()); len(names) > 0 {
		db = bizhelper.ExactQueryStringArrayOr(db, "knowledge_base_name", names)
	}
	if keyword := strings.TrimSpace(filter.GetKeyword()); keyword != "" {
		db = bizhelper.FuzzSearchEx(db, []string{
			"knowledge_base_name",
			"knowledge_base_description",
			"knowledge_base_type",
			"tags",
		}, keyword, false)
	}
	if types := normalizeKnowledgeBaseFilterStrings(filter.GetTypes()); len(types) > 0 {
		db = bizhelper.ExactQueryStringArrayOr(db, "knowledge_base_type", types)
	}
	if tags := normalizeKnowledgeBaseFilterStrings(filter.GetTags()); len(tags) > 0 {
		db = bizhelper.FuzzQueryStringArrayOrLike(db, "tags", tags)
	}
	if serialVersionIDs := normalizeKnowledgeBaseFilterStrings(filter.GetSerialVersionIDs()); len(serialVersionIDs) > 0 {
		db = bizhelper.ExactQueryStringArrayOr(db, "serial_version_uid", serialVersionIDs)
	}
	return db
}

// QueryKnowledgeBaseByFilter queries knowledge-base metadata with stable filter
// and pagination semantics for the public QueryKnowledgeBase RPC.
func QueryKnowledgeBaseByFilter(
	db *gorm.DB,
	filter *ypb.KnowledgeBaseFilter,
	paging *ypb.Paging,
) (*bizhelper.Paginator, []*schema.KnowledgeBaseInfo, error) {
	if db == nil {
		return nil, nil, utils.Error("database is nil")
	}

	db = FilterKnowledgeBaseByQuery(db, filter)
	db = bizhelper.OrderByPaging(db, paging)
	items := make([]*schema.KnowledgeBaseInfo, 0)
	paginator, result := bizhelper.YakitPagingQuery(db, paging, &items)
	if result.Error != nil {
		return nil, nil, utils.Errorf("query knowledge bases failed: %s", result.Error)
	}
	return paginator, items, nil
}
