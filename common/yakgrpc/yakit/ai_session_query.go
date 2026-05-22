package yakit

import (
	"strings"
	"time"

	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/bizhelper"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func normalizeAISessionFilterStrings(vals []string) []string {
	seen := make(map[string]struct{}, len(vals))
	out := make([]string, 0, len(vals))
	for _, v := range vals {
		v = strings.TrimSpace(v)
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}
	return out
}

// applyAISessionSourceFilter matches source values. Empty string in the filter
// also matches legacy rows where source was never set (NULL in DB).
func applyAISessionSourceFilter(db *gorm.DB, sources []string) *gorm.DB {
	sources = normalizeAISessionFilterStrings(sources)
	if len(sources) == 0 {
		return db
	}

	includeEmpty := false
	nonEmpty := make([]string, 0, len(sources))
	for _, s := range sources {
		if s == "" {
			includeEmpty = true
			continue
		}
		nonEmpty = append(nonEmpty, s)
	}

	switch {
	case includeEmpty && len(nonEmpty) == 0:
		return db.Where("source IS NULL OR source = ?", "")
	case includeEmpty:
		return db.Where("(source IS NULL OR source = ?) OR source IN (?)", "", nonEmpty)
	default:
		return bizhelper.ExactQueryStringArrayOr(db, "source", sources)
	}
}

func FilterAISessionMeta(db *gorm.DB, filter *ypb.AISessionFilter) *gorm.DB {
	db = db.Model(&schema.AISession{})
	if filter == nil {
		return db
	}

	db = bizhelper.ExactQueryStringArrayOr(db, "session_id", filter.GetSessionID())
	if filter.GetKeyword() != "" {
		db = bizhelper.FuzzSearchWithStringArrayOrEx(db, []string{"session_id", "title"}, []string{filter.GetKeyword()}, false)
	}
	if len(filter.GetSource()) > 0 {
		db = applyAISessionSourceFilter(db, filter.GetSource())
	}
	return db
}

func QueryAISessionMetaPaging(db *gorm.DB, filter *ypb.AISessionFilter, paging *ypb.Paging) (*bizhelper.Paginator, []*schema.AISession, error) {
	if db == nil {
		return nil, nil, utils.Errorf("database is nil")
	}
	if paging == nil {
		paging = &ypb.Paging{
			Page:    1,
			Limit:   30,
			OrderBy: "updated_at",
			Order:   "desc",
		}
	}

	db = FilterAISessionMeta(db, filter)
	db = bizhelper.OrderByPaging(db, paging)

	records := make([]*schema.AISession, 0)
	pag, db := bizhelper.YakitPagingQuery(db, paging, &records)
	if db.Error != nil {
		return nil, nil, utils.Errorf("paging failed: %s", db.Error)
	}
	return pag, records, nil
}

func QueryAllAISessionMetaOrderByUpdated(db *gorm.DB) ([]*schema.AISession, error) {
	if db == nil {
		return nil, utils.Errorf("database is nil")
	}

	records := make([]*schema.AISession, 0)
	if err := db.Model(&schema.AISession{}).Order("updated_at desc").Find(&records).Error; err != nil {
		return nil, err
	}
	return records, nil
}

func QueryAISessionIDsForDelete(db *gorm.DB, filter *ypb.DeleteAISessionFilter, deleteAll bool) ([]string, error) {
	if db == nil {
		return nil, utils.Errorf("database is nil")
	}

	query := db.Model(&schema.AISession{})
	if !deleteAll {
		if filter == nil {
			return nil, utils.Errorf("filter is required unless delete_all is true")
		}

		sessionIDs := make([]string, 0, len(filter.GetSessionID()))
		seen := make(map[string]struct{}, len(filter.GetSessionID()))
		for _, sid := range filter.GetSessionID() {
			sid = strings.TrimSpace(sid)
			if sid == "" {
				continue
			}
			if _, ok := seen[sid]; ok {
				continue
			}
			seen[sid] = struct{}{}
			sessionIDs = append(sessionIDs, sid)
		}
		query = bizhelper.ExactQueryStringArrayOr(query, "session_id", sessionIDs)
		if filter.GetAfterTimestamp() > 0 {
			query = query.Where("updated_at > ?", time.Unix(filter.GetAfterTimestamp(), 0))
		}
		if filter.GetBeforeTimestamp() > 0 {
			query = query.Where("updated_at < ?", time.Unix(filter.GetBeforeTimestamp(), 0))
		}
		sources := normalizeAISessionFilterStrings(filter.GetSource())
		if len(filter.GetSource()) > 0 {
			query = applyAISessionSourceFilter(query, filter.GetSource())
		}
		if len(sessionIDs) == 0 && filter.GetAfterTimestamp() <= 0 && filter.GetBeforeTimestamp() <= 0 && len(sources) == 0 {
			return nil, utils.Errorf("at least one filter condition is required")
		}
	}

	var sessionIDs []string
	if err := query.Pluck("session_id", &sessionIDs).Error; err != nil {
		return nil, err
	}
	return sessionIDs, nil
}
