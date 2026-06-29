package aisessioncleanup

import (
	"context"
	"strings"

	"github.com/jinzhu/gorm"
	"github.com/samber/lo"
	"github.com/yaklang/yaklang/common/ai/aid/aimem"
	"github.com/yaklang/yaklang/common/ai/rag/vectorstore"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
)

const (
	aiMemoryRAGCollectionPrefix   = "ai-memory-"
	aiMemoryEntityDeleteBatchSize = 200
)

// Result summarizes orphan cleanup after database init.
type Result struct {
	DeletedMemoryEntities    int64
	DeletedMemoryCollections int64
	DeletedRAGCollections    int
	DeletedWorkDirs          int
}

//func init() {
//	yakit.RegisterPostInitDatabaseFunction(func() error {
//		db := consts.GetGormProjectDatabase()
//		if db == nil {
//			return nil
//		}
//		_, err := ReconcileOrphanArtifacts(db)
//		if err != nil {
//			log.Errorf("reconcile orphan ai session artifacts failed: %v", err)
//		}
//		return err
//	}, "reconcile-orphan-ai-session-artifacts")
//}

// ReconcileOrphanArtifacts removes AI memory rows and aispace directories
// that are no longer associated with any persisted AISession.
func ReconcileOrphanArtifacts(db *gorm.DB) (*Result, error) {
	result := &Result{}
	if db == nil {
		return result, utils.Errorf("database is nil")
	}

	validSessionIDs, err := queryValidAISessionIDs(db)
	if err != nil {
		return result, err
	}

	result.DeletedMemoryEntities, err = deleteOrphanAIMemoryEntities(db, validSessionIDs)
	if err != nil {
		return result, err
	}

	result.DeletedMemoryCollections, err = deleteOrphanAIMemoryCollections(db, validSessionIDs)
	if err != nil {
		return result, err
	}

	result.DeletedRAGCollections, err = deleteOrphanAIMemoryRAGCollections(db, validSessionIDs)
	if err != nil {
		return result, err
	}

	result.DeletedWorkDirs, err = yakit.CleanupOrphanAISpaceWorkDirs(db)
	if err != nil {
		return result, err
	}

	if result.DeletedMemoryEntities > 0 || result.DeletedMemoryCollections > 0 ||
		result.DeletedRAGCollections > 0 || result.DeletedWorkDirs > 0 {
		log.Infof(
			"reconciled orphan ai session artifacts: memory_entities=%d memory_collections=%d rag_collections=%d workdirs=%d",
			result.DeletedMemoryEntities,
			result.DeletedMemoryCollections,
			result.DeletedRAGCollections,
			result.DeletedWorkDirs,
		)
	}
	return result, nil
}

func queryValidAISessionIDs(db *gorm.DB) (map[string]struct{}, error) {
	var sessionIDs []string
	if err := db.Model(&schema.AISession{}).Pluck("session_id", &sessionIDs).Error; err != nil {
		if isMissingTableErr(err) {
			return map[string]struct{}{}, nil
		}
		return nil, err
	}
	valid := make(map[string]struct{}, len(sessionIDs))
	for _, sessionID := range sessionIDs {
		sessionID = strings.TrimSpace(sessionID)
		if sessionID == "" {
			continue
		}
		valid[sessionID] = struct{}{}
	}
	return valid, nil
}

func deleteOrphanAIMemoryEntities(db *gorm.DB, validSessionIDs map[string]struct{}) (int64, error) {
	ctx := context.Background()
	validIDs := lo.Keys(validSessionIDs)
	var deleted int64
	var lastID uint

	for {
		var entities []schema.AIMemoryEntity
		q := db.Model(&schema.AIMemoryEntity{}).
			Select("id, memory_id, session_id, potential_questions").
			Order("id asc").
			Limit(aiMemoryEntityDeleteBatchSize)
		if lastID > 0 {
			q = q.Where("id > ?", lastID)
		}
		if len(validIDs) > 0 {
			q = q.Where("session_id NOT IN (?)", validIDs)
		}
		if err := q.Find(&entities).Error; err != nil {
			if isMissingTableErr(err) {
				return deleted, nil
			}
			return deleted, err
		}
		if len(entities) == 0 {
			return deleted, nil
		}

		entityIDs := make([]uint, 0, len(entities))
		for _, entity := range entities {
			entityIDs = append(entityIDs, entity.ID)
			lastID = entity.ID
		}

		if err := aimem.DeleteMemoryVectorArtifacts(ctx, db, entities); err != nil {
			return deleted, err
		}
		res := db.Model(&schema.AIMemoryEntity{}).
			Where("id IN (?)", entityIDs).
			Unscoped().
			Delete(&schema.AIMemoryEntity{})
		if res.Error != nil {
			if isMissingTableErr(res.Error) {
				return deleted, nil
			}
			return deleted, res.Error
		}
		deleted += res.RowsAffected
	}
}

func deleteOrphanAIMemoryCollections(db *gorm.DB, validSessionIDs map[string]struct{}) (int64, error) {
	validIDs := lo.Keys(validSessionIDs)
	q := db.Model(&schema.AIMemoryCollection{})
	if len(validIDs) > 0 {
		q = q.Where("session_id NOT IN (?)", validIDs)
	}
	res := q.Unscoped().Delete(&schema.AIMemoryCollection{})
	if res.Error != nil {
		if isMissingTableErr(res.Error) {
			return 0, nil
		}
		return 0, res.Error
	}
	return res.RowsAffected, nil
}

func deleteOrphanAIMemoryRAGCollections(db *gorm.DB, validSessionIDs map[string]struct{}) (int, error) {
	deleted := 0
	for _, collectionName := range vectorstore.ListCollections(db) {
		if !strings.HasPrefix(collectionName, aiMemoryRAGCollectionPrefix) {
			continue
		}
		sessionID := strings.TrimSpace(strings.TrimPrefix(collectionName, aiMemoryRAGCollectionPrefix))
		if sessionID == "" {
			continue
		}
		if _, ok := validSessionIDs[sessionID]; ok {
			continue
		}
		if err := vectorstore.DeleteCollection(db, collectionName); err != nil {
			log.Warnf("delete orphan ai memory rag collection failed: %s: %v", collectionName, err)
			continue
		}
		deleted++
		log.Infof("deleted orphan ai memory rag collection: %s", collectionName)
	}
	return deleted, nil
}

func isMissingTableErr(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "no such table") || strings.Contains(msg, "doesn't exist")
}
