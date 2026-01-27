package yakit

import (
	"strings"

	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/bizhelper"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func FilterAIMemoryEntity(db *gorm.DB, filter *ypb.AIMemoryEntityFilter) *gorm.DB {
	db = db.Model(&schema.AIMemoryEntity{})
	if filter == nil {
		return db
	}

	if filter.GetSessionID() != "" {
		db = db.Where("session_id = ?", filter.GetSessionID())
	}
	db = bizhelper.ExactQueryStringArrayOr(db, "memory_id", filter.GetMemoryID())

	if kw := strings.TrimSpace(filter.GetContentKeyword()); kw != "" {
		db = bizhelper.FuzzSearchEx(db, []string{"content"}, kw, false)
	}

	if kw := strings.TrimSpace(filter.GetPotentialQuestionKeyword()); kw != "" {
		db = db.Where("potential_questions LIKE ?", "%"+kw+"%")
	}

	db = bizhelper.QueryByFloatRange(db, "c_score", filter.GetCScore())
	db = bizhelper.QueryByFloatRange(db, "o_score", filter.GetOScore())
	db = bizhelper.QueryByFloatRange(db, "r_score", filter.GetRScore())
	db = bizhelper.QueryByFloatRange(db, "e_score", filter.GetEScore())
	db = bizhelper.QueryByFloatRange(db, "p_score", filter.GetPScore())
	db = bizhelper.QueryByFloatRange(db, "a_score", filter.GetAScore())
	db = bizhelper.QueryByFloatRange(db, "t_score", filter.GetTScore())

	db = bizhelper.QueryByTimeRangeUnix(db, "created_at", filter.GetCreatedAt())
	db = bizhelper.QueryByTimeRangeUnix(db, "updated_at", filter.GetUpdatedAt())

	if filter.GetTagMatchAll() {
		db = bizhelper.FuzzQueryArrayStringAndLike(db, "tags", filter.GetTags())
	} else {
		db = bizhelper.FuzzQueryArrayStringOrLike(db, "tags", filter.GetTags())
	}

	return db
}

func QueryAIMemoryEntityPaging(db *gorm.DB, filter *ypb.AIMemoryEntityFilter, paging *ypb.Paging) (*bizhelper.Paginator, []*schema.AIMemoryEntity, error) {
	db = FilterAIMemoryEntity(db, filter)

	if paging == nil {
		paging = &ypb.Paging{Page: 1, Limit: 10, OrderBy: "created_at", Order: "desc"}
	}

	var ret []*schema.AIMemoryEntity
	pag, db := bizhelper.YakitPagingQuery(db, paging, &ret)
	if db.Error != nil {
		return nil, nil, utils.Errorf("paging failed: %s", db.Error)
	}
	return pag, ret, nil
}

func GetAIMemoryEntity(db *gorm.DB, sessionID, memoryID string) (*schema.AIMemoryEntity, error) {
	if sessionID == "" {
		return nil, utils.Errorf("session_id is required")
	}
	if memoryID == "" {
		return nil, utils.Errorf("memory_id is required")
	}

	var entity schema.AIMemoryEntity
	if err := db.Where("session_id = ? AND memory_id = ?", sessionID, memoryID).First(&entity).Error; err != nil {
		return nil, err
	}
	return &entity, nil
}

func DeleteAIMemoryEntity(db *gorm.DB, filter *ypb.AIMemoryEntityFilter) (int64, error) {
	db = FilterAIMemoryEntity(db, filter).Model(&schema.AIMemoryEntity{})
	if db := db.Unscoped().Delete(&schema.AIMemoryEntity{}); db.Error != nil {
		return 0, db.Error
	} else {
		return db.RowsAffected, nil
	}
}
