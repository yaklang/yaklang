package yakit

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/yaklang/gorm"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/bizhelper"
)

// SaveAITrajectory keeps one terminal row per task within a persistent session. Compression
// keys identify committed operations and are immutable. The return value says
// whether the stored content changed, allowing downstream evaluation to skip
// repeated completion notifications.
func SaveAITrajectory(db *gorm.DB, record *schema.AITrajectory) (bool, error) {
	if db == nil || record == nil {
		return false, fmt.Errorf("trajectory requires a database and record")
	}
	if strings.TrimSpace(record.PersistentSessionID) == "" {
		return false, fmt.Errorf("trajectory requires a persistent session ID")
	}
	switch record.Kind {
	case schema.AITrajectoryTerminal:
		if record.TaskID == "" {
			return false, fmt.Errorf("terminal trajectory requires a task ID")
		}
		record.TrajectoryKey = schema.AITrajectoryTerminal + ":" + record.TaskID
	case schema.AITrajectoryCompression:
		if record.TrajectoryKey == "" || record.TrajectoryKey == schema.AITrajectoryTerminal {
			return false, fmt.Errorf("compression trajectory requires its own key")
		}
	default:
		return false, fmt.Errorf("unknown trajectory kind %q", record.Kind)
	}
	content := *record
	content.Model = gorm.Model{}
	content.CapturedAt = time.Time{}
	content.ContentHash = ""
	content.EvaluationStatus = ""
	content.ValueScore, content.EvaluatedAt = nil, nil
	content.EvaluationReason, content.EvaluationModel, content.EvaluationError = "", "", ""
	raw, err := json.Marshal(content)
	if err != nil {
		return false, err
	}
	record.ContentHash = fmt.Sprintf("%x", sha256.Sum256(raw))
	record.EvaluationStatus = schema.AITrajectoryEvaluationPending
	record.ValueScore, record.EvaluatedAt = nil, nil
	record.EvaluationReason, record.EvaluationModel, record.EvaluationError = "", "", ""
	if record.CapturedAt.IsZero() {
		record.CapturedAt = time.Now()
	}
	changed := false
	err = utils.GormTransaction(db, func(tx *gorm.DB) error {
		// The native conflict helper makes the first operation a write, reserving
		// SQLite's writer without a handwritten lock statement or read/write race.
		candidate := *record
		candidate.Model = gorm.Model{}
		insert := tx.OnConflictDoNothing("persistent_session_id,trajectory_key").Create(&candidate)
		if insert.Error != nil {
			return insert.Error
		}
		var owners int64
		ownerQuery := bizhelper.ExactQueryString(tx.Model(&schema.AIAgentRuntime{}), "persistent_session", record.PersistentSessionID)
		ownerQuery = bizhelper.ExactQueryString(ownerQuery, "uuid", record.RuntimeID)
		if err := ownerQuery.Count(&owners).Error; err != nil {
			return err
		}
		if owners == 0 {
			return fmt.Errorf("trajectory session %q has no runtime", record.PersistentSessionID)
		}
		if insert.RowsAffected > 0 {
			*record, changed = candidate, true
			return nil
		}
		query := bizhelper.ExactQueryString(tx, "persistent_session_id", record.PersistentSessionID)
		query = bizhelper.ExactQueryString(query, "trajectory_key", record.TrajectoryKey)
		var existing schema.AITrajectory
		if err := query.First(&existing).Error; err != nil {
			return err
		}
		if record.Kind == schema.AITrajectoryTerminal && record.CapturedAt.Before(existing.CapturedAt) {
			*record = existing
			return nil
		}
		if existing.ContentHash == record.ContentHash {
			// Keep the capture watermark even when the latest prompt repeats.
			// A delayed intermediate prompt must not replace that newer receipt.
			if record.Kind == schema.AITrajectoryTerminal && record.CapturedAt.After(existing.CapturedAt) {
				if err := tx.Model(&existing).UpdateColumn("captured_at", record.CapturedAt).Error; err != nil {
					return err
				}
				existing.CapturedAt = record.CapturedAt
			}
			*record = existing
			return nil
		}
		if record.Kind == schema.AITrajectoryCompression {
			return fmt.Errorf("compression trajectory %q is immutable", record.TrajectoryKey)
		}
		record.Model = existing.Model
		changed = true
		// Save includes zero values, clearing a superseded prompt's score.
		return tx.Save(record).Error
	})
	return changed && err == nil, err
}

func QueryAITrajectories(db *gorm.DB, persistentSessionID string) ([]*schema.AITrajectory, error) {
	if db == nil || strings.TrimSpace(persistentSessionID) == "" {
		return nil, fmt.Errorf("trajectory query requires a database and persistent session ID")
	}
	if !db.HasTable(&schema.AITrajectory{}) {
		return nil, nil
	}
	var records []*schema.AITrajectory
	query := bizhelper.ExactQueryString(db.Model(&schema.AITrajectory{}), "persistent_session_id", persistentSessionID)
	query = bizhelper.QueryOrder(query, "captured_at", "asc")
	query = bizhelper.QueryOrder(query, "id", "asc")
	err := query.Find(&records).Error
	return records, err
}

func DeleteAITrajectories(db *gorm.DB, persistentSessionID string) error {
	if db == nil || strings.TrimSpace(persistentSessionID) == "" {
		return fmt.Errorf("trajectory deletion requires a database and persistent session ID")
	}
	if !db.HasTable(&schema.AITrajectory{}) {
		return nil
	}
	return bizhelper.ExactQueryString(db.Unscoped(), "persistent_session_id", persistentSessionID).Delete(&schema.AITrajectory{}).Error
}

func DeleteAllAITrajectories(db *gorm.DB) error {
	if db == nil {
		return fmt.Errorf("trajectory deletion requires a database")
	}
	if !db.HasTable(&schema.AITrajectory{}) {
		return nil
	}
	return db.Unscoped().Delete(&schema.AITrajectory{}).Error
}

// ClaimAITrajectoryEvaluation is an atomic compare-and-swap. Duplicate queue
// notifications and superseded terminal snapshots cannot issue another call.
func ClaimAITrajectoryEvaluation(db *gorm.DB, id uint, contentHash string) (*schema.AITrajectory, error) {
	if db == nil || id == 0 || contentHash == "" {
		return nil, fmt.Errorf("trajectory evaluation requires a database, ID and content hash")
	}
	query := trajectoryRevisionQuery(db, id, contentHash)
	result := bizhelper.ExactQueryString(query, "evaluation_status", schema.AITrajectoryEvaluationPending).
		Update("evaluation_status", schema.AITrajectoryEvaluationRunning)
	if result.Error != nil || result.RowsAffected == 0 {
		return nil, result.Error
	}
	var record schema.AITrajectory
	if err := trajectoryRevisionQuery(db, id, contentHash).First(&record).Error; err != nil {
		if gorm.IsRecordNotFoundError(err) {
			return nil, nil
		}
		return nil, err
	}
	return &record, nil
}

// UpdateAITrajectoryEvaluation updates an existing revision only. It never
// inserts, so a result arriving after session deletion cannot resurrect data.
func UpdateAITrajectoryEvaluation(db *gorm.DB, id uint, contentHash, status, model, reason, evaluationError string, score *int) (bool, error) {
	if db == nil || id == 0 || contentHash == "" {
		return false, fmt.Errorf("trajectory evaluation requires a database, ID and content hash")
	}
	if score != nil && (*score < 0 || *score > 10) {
		return false, fmt.Errorf("trajectory value score must be between 0 and 10")
	}
	if status != schema.AITrajectoryEvaluationCompleted && status != schema.AITrajectoryEvaluationFailed && status != schema.AITrajectoryEvaluationSkipped {
		return false, fmt.Errorf("invalid trajectory evaluation status %q", status)
	}
	if status == schema.AITrajectoryEvaluationCompleted && (score == nil || strings.TrimSpace(reason) == "") {
		return false, fmt.Errorf("completed trajectory evaluation requires a score and reason")
	}
	query := trajectoryRevisionQuery(db, id, contentHash)
	result := bizhelper.ExactQueryStringArrayOr(query, "evaluation_status",
		[]string{schema.AITrajectoryEvaluationPending, schema.AITrajectoryEvaluationRunning}).
		Updates(map[string]any{"evaluation_status": status, "evaluation_model": model,
			"evaluation_reason": reason, "evaluation_error": evaluationError, "value_score": score, "evaluated_at": time.Now()})
	return result.RowsAffected > 0, result.Error
}

// ResumeAITrajectoryEvaluations returns lightweight receipts, not full prompt
// bodies. Completed evaluations are never reissued. A running claim is only
// recovered after the evaluator's bounded call lifetime has elapsed.
func ResumeAITrajectoryEvaluations(db *gorm.DB, sessionID string) ([]*schema.AITrajectory, error) {
	if db == nil || sessionID == "" {
		return nil, fmt.Errorf("trajectory recovery requires a database and persistent session ID")
	}
	if !db.HasTable(&schema.AITrajectory{}) {
		return nil, nil
	}
	query := bizhelper.ExactQueryString(db.Model(&schema.AITrajectory{}), "persistent_session_id", sessionID)
	result := query.
		Where("evaluation_status = ? OR (evaluation_status = ? AND updated_at < ?)",
			schema.AITrajectoryEvaluationSkipped, schema.AITrajectoryEvaluationRunning, time.Now().Add(-time.Minute)).
		Updates(map[string]any{"evaluation_status": schema.AITrajectoryEvaluationPending, "evaluation_error": ""})
	if result.Error != nil {
		return nil, result.Error
	}
	var receipts []*schema.AITrajectory
	query = bizhelper.ExactQueryString(query, "evaluation_status", schema.AITrajectoryEvaluationPending)
	err := bizhelper.QueryOrder(query, "id", "asc").Select("id, persistent_session_id, content_hash").Find(&receipts).Error
	return receipts, err
}

// All scoring writes share the existing bizhelper exact-match filters. Empty
// identities are rejected by callers before a helper can omit a predicate.
func trajectoryRevisionQuery(db *gorm.DB, id uint, contentHash string) *gorm.DB {
	db = bizhelper.ExactQueryInt64(db.Model(&schema.AITrajectory{}), "id", int64(id))
	return bizhelper.ExactQueryString(db, "content_hash", contentHash)
}
