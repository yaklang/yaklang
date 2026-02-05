package yakit

import (
	"context"
	"sort"
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
	return DeleteAIMemoryEntityBatched(context.Background(), db, filter, defaultAIMemoryEntityDeleteBatchSize, nil)
}

const (
	defaultAIMemoryEntityDeleteBatchSize = 200
)

type DeleteAIMemoryEntityBatchHook func(ctx context.Context, db *gorm.DB, entities []schema.AIMemoryEntity) error

// DeleteAIMemoryEntityBatched hard-deletes AIMemoryEntity rows in small batches to avoid large-range query+delete stalls.
// If hook is provided, it is called once per batch (before deleting the entities).
func DeleteAIMemoryEntityBatched(ctx context.Context, db *gorm.DB, filter *ypb.AIMemoryEntityFilter, batchSize int, hook DeleteAIMemoryEntityBatchHook) (deletedEntities int64, err error) {
	if db == nil {
		return 0, utils.Errorf("database not initialized")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if batchSize <= 0 {
		batchSize = defaultAIMemoryEntityDeleteBatchSize
	}

	var lastID uint
	for {
		select {
		case <-ctx.Done():
			return deletedEntities, ctx.Err()
		default:
		}

		var entities []schema.AIMemoryEntity
		q := FilterAIMemoryEntity(db, filter).
			Select("id, memory_id, session_id, potential_questions").
			Order("id asc").
			Limit(batchSize)
		if lastID > 0 {
			q = q.Where("id > ?", lastID)
		}
		if err := q.Find(&entities).Error; err != nil {
			return deletedEntities, err
		}
		if len(entities) == 0 {
			return deletedEntities, nil
		}

		entityIDs := make([]uint, 0, len(entities))
		for _, entity := range entities {
			entityIDs = append(entityIDs, entity.ID)
			lastID = entity.ID
		}

		var batchDeletedEntities int64
		if hook != nil {
			if err := hook(ctx, db, entities); err != nil {
				return deletedEntities, err
			}
		}
		if err := utils.GormTransaction(db, func(tx *gorm.DB) error {
			res := tx.Model(&schema.AIMemoryEntity{}).
				Where("id IN (?)", entityIDs).
				Unscoped().
				Delete(&schema.AIMemoryEntity{})
			if res.Error != nil {
				return res.Error
			}
			batchDeletedEntities = res.RowsAffected
			return nil
		}); err != nil {
			return deletedEntities, err
		}

		deletedEntities += batchDeletedEntities
	}
}

func CountAIMemoryEntityTags(ctx context.Context, db *gorm.DB, sessionID string) ([]*ypb.TagsCode, error) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return nil, utils.Errorf("session_id is required")
	}

	db = db.Where("session_id = ?", sessionID)
	var memoryTagsMap = make(map[string]int)

	for i := range bizhelper.YieldModel[*schema.AIMemoryEntity](ctx, db) {
		for _, tag := range i.Tags {
			if _, ok := memoryTagsMap[tag]; ok {
				memoryTagsMap[tag] = memoryTagsMap[tag] + 1
			} else {
				memoryTagsMap[tag] = 1
			}

		}
	}

	ret := make([]*ypb.TagsCode, 0, len(memoryTagsMap))
	for tag, count := range memoryTagsMap {
		ret = append(ret, &ypb.TagsCode{Value: tag, Total: int32(count)})
	}

	sort.Slice(ret, func(i, j int) bool {
		if ret[i].Total == ret[j].Total {
			return ret[i].Value < ret[j].Value
		}
		return ret[i].Total > ret[j].Total
	})

	return ret, nil
}
