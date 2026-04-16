package yakit

import (
	"encoding/json"
	"strings"

	"github.com/jinzhu/gorm"
	"github.com/samber/lo"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
)

const aiSessionMetaMigrationKey = "yakit.ai_session_meta.migrated.v1"
const defaultAISessionTitle = "<未命名>"
const emptyRelatedRuntimeIDsJSON = "[]"

func CreateOrUpdateAISessionMeta(db *gorm.DB, sessionID, title string) (*schema.AISession, error) {
	if db == nil {
		return nil, utils.Errorf("database is nil")
	}
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return nil, utils.Errorf("session_id is empty")
	}

	record := &schema.AISession{SessionID: sessionID}
	title = strings.TrimSpace(title)
	attrs := map[string]any{
		"related_runtime_ids": emptyRelatedRuntimeIDsJSON,
	}
	assignments := map[string]any{}
	if title != "" {
		assignments["title"] = title
		assignments["title_initialized"] = true
	}
	result := db.Model(&schema.AISession{}).
		Where("session_id = ?", sessionID).
		Attrs(attrs).
		Assign(assignments).
		FirstOrCreate(record)
	if result.Error != nil {
		return nil, result.Error
	}
	return record, nil
}

func GetAISessionMetaBySessionID(db *gorm.DB, sessionID string) (*schema.AISession, error) {
	if db == nil {
		return nil, utils.Errorf("database is nil")
	}
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return nil, utils.Errorf("session_id is empty")
	}

	var record schema.AISession
	if err := db.Model(&schema.AISession{}).Where("session_id = ?", sessionID).First(&record).Error; err != nil {
		return nil, err
	}
	return &record, nil
}

func QueryAISessionMeta(db *gorm.DB, titleKeyword string, limit, offset int) ([]*schema.AISession, error) {
	if db == nil {
		return nil, utils.Errorf("database is nil")
	}
	if limit <= 0 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}

	query := db.Model(&schema.AISession{}).Order("updated_at desc").Limit(limit).Offset(offset)
	if kw := strings.TrimSpace(titleKeyword); kw != "" {
		query = query.Where("title LIKE ?", "%"+kw+"%")
	}

	var records []*schema.AISession
	if err := query.Find(&records).Error; err != nil {
		return nil, err
	}
	return records, nil
}

func UpdateAISessionMetaTitle(db *gorm.DB, sessionID, title string) (int64, error) {
	if db == nil {
		return 0, utils.Errorf("database is nil")
	}
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return 0, utils.Errorf("session_id is empty")
	}
	title = strings.TrimSpace(title)

	result := db.Model(&schema.AISession{}).
		Where("session_id = ?", sessionID).
		Updates(map[string]any{
			"title":             title,
			"title_initialized": title != "",
		})
	return result.RowsAffected, result.Error
}

func AppendAISessionMetaRelatedRuntimeID(db *gorm.DB, sessionID, runtimeID string) error {
	if db == nil {
		return utils.Errorf("database is nil")
	}
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return utils.Errorf("session_id is empty")
	}
	runtimeID = strings.TrimSpace(runtimeID)
	if runtimeID == "" {
		return nil
	}

	var meta schema.AISession
	if err := db.Model(&schema.AISession{}).Where("session_id = ?", sessionID).First(&meta).Error; err != nil {
		if gorm.IsRecordNotFoundError(err) {
			return nil
		}
		return err
	}

	var runtimeIDs []string
	if strings.TrimSpace(meta.RelatedRuntimeIDS) != "" {
		if err := json.Unmarshal([]byte(meta.RelatedRuntimeIDS), &runtimeIDs); err != nil {
			return utils.Errorf("unmarshal related_runtime_ids failed: %v", err)
		}
	}


	normalized := lo.Uniq(append(runtimeIDs, runtimeID))

	raw, err := json.Marshal(normalized)
	if err != nil {
		return utils.Errorf("marshal related_runtime_ids failed: %v", err)
	}

	return db.Model(&schema.AISession{}).
		Where("session_id = ?", sessionID).
		UpdateColumn("related_runtime_ids", string(raw)).Error
}

func EnsureAISessionMeta(db *gorm.DB, sessionID string) (*schema.AISession, error) {
	if db == nil {
		return nil, utils.Errorf("database is nil")
	}
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return nil, utils.Errorf("session_id is empty")
	}

	record := &schema.AISession{SessionID: sessionID}
	result := db.Model(&schema.AISession{}).
		Where("session_id = ?", sessionID).
		Attrs(map[string]any{
			"title":               defaultAISessionTitle,
			"title_initialized":   false,
			"related_runtime_ids": emptyRelatedRuntimeIDsJSON,
		}).
		FirstOrCreate(record)
	if result.Error != nil {
		return nil, result.Error
	}
	return record, nil
}

func IsAISessionTitleInitialized(db *gorm.DB, sessionID string) (bool, error) {
	meta, err := GetAISessionMetaBySessionID(db, sessionID)
	if err != nil {
		return false, err
	}
	return meta.TitleInitialized, nil
}

// InitAISessionTitleIfNeeded sets title only when title_initialized is false.
// It returns whether title was updated.
func InitAISessionTitleIfNeeded(db *gorm.DB, sessionID, title string) (bool, error) {
	if db == nil {
		return false, utils.Errorf("database is nil")
	}
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return false, utils.Errorf("session_id is empty")
	}
	title = strings.TrimSpace(title)
	if title == "" {
		return false, nil
	}

	result := db.Model(&schema.AISession{}).
		Where("session_id = ? AND (title_initialized = ? OR title_initialized IS NULL)", sessionID, false).
		Updates(map[string]any{
			"title":             title,
			"title_initialized": true,
		})
	if result.Error != nil {
		return false, result.Error
	}
	return result.RowsAffected > 0, nil
}

func DeleteAISessionMetaBySessionID(db *gorm.DB, sessionID string) (int64, error) {
	if db == nil {
		return 0, utils.Errorf("database is nil")
	}
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return 0, utils.Errorf("session_id is empty")
	}

	result := db.Model(&schema.AISession{}).Where("session_id = ?", sessionID).Unscoped().Delete(&schema.AISession{})
	return result.RowsAffected, result.Error
}

func DeleteAllAISessionMeta(db *gorm.DB) (int64, error) {
	if db == nil {
		return 0, utils.Errorf("database is nil")
	}
	deletedSessions, err := countRowsIgnoreMissingTable(db, &schema.AISession{})
	if err != nil {
		return 0, err
	}
	if err := schema.DropRecreateTable(db, &schema.AISession{}); err != nil {
		return deletedSessions, err
	}
	return deletedSessions, nil
}

// MigrateAISessionMetaFromEvents migrates session titles from ai_output_events to ai_sessions_v1.
// It is idempotent and guarded by a migration flag in profile DB via SetKey/GetKey.
func MigrateAISessionMetaFromEvents(profileDB, projectDB *gorm.DB) error {
	if profileDB == nil || projectDB == nil {
		return nil
	}
	if GetKey(profileDB, aiSessionMetaMigrationKey) != "" {
		return nil
	}

	var sessionIDs []string
	if err := projectDB.Model(&schema.AiOutputEvent{}).
		Where("session_id <> ''").
		Group("session_id").
		Pluck("session_id", &sessionIDs).Error; err != nil {
		return err
	}
	if len(sessionIDs) == 0 {
		return SetKey(profileDB, aiSessionMetaMigrationKey, "done")
	}

	for _, sid := range sessionIDs {
		var events []schema.AiOutputEvent
		if err := projectDB.Model(&schema.AiOutputEvent{}).
			Where("session_id = ?", sid).
			Order("id ASC").
			Limit(100).
			Find(&events).Error; err != nil {
			return err
		}
		title := findSessionTitleFromEvents(events)
		if title == "" {
			title = sid
		}
		title = truncateTitle(title, 20)
		if _, err := CreateOrUpdateAISessionMeta(projectDB, sid, title); err != nil {
			return err
		}
	}
	return SetKey(profileDB, aiSessionMetaMigrationKey, "done")
}

func findSessionTitleFromEvents(events []schema.AiOutputEvent) string {
	// Prefer the first explicit input event.
	for _, evt := range events {
		if evt.Type != schema.EVENT_TYPE_INPUT {
			continue
		}
		if title := extractTitleFromEventContent(evt.Content); title != "" {
			return title
		}
	}
	// Fallback: scan first 100 events for embedded user input fields.
	for _, evt := range events {
		if title := extractTitleFromEventContent(evt.Content); title != "" {
			return title
		}
	}
	return ""
}

func extractTitleFromEventContent(raw []byte) string {
	plain := normalizeTitle(string(raw))
	if plain == "" {
		return ""
	}
	var obj map[string]any
	if err := json.Unmarshal(raw, &obj); err != nil {
		return plain
	}
	for _, key := range []string{
		"free_input",
		"prompt",
		"content",
		"message",
		"input",
		"query",
		"react_user_input",
	} {
		if v, ok := obj[key]; ok {
			if s := normalizeTitle(utils.InterfaceToString(v)); s != "" {
				return s
			}
		}
	}
	return plain
}

func normalizeTitle(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	s = strings.ReplaceAll(s, "\r", " ")
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.Join(strings.Fields(s), " ")
	return s
}

func truncateTitle(s string, maxLen int) string {
	if maxLen <= 0 {
		return ""
	}
	r := []rune(s)
	if len(r) <= maxLen {
		return s
	}
	return string(r[:maxLen])
}

func init() {
	RegisterPostInitDatabaseFunction(func() error {
		err := MigrateAISessionMetaFromEvents(consts.GetGormProfileDatabase(), consts.GetGormProjectDatabase())
		if err != nil {
			log.Errorf("migrate ai session meta from events failed: %v", err)
		}
		return err
	}, "migrate-ai-session-meta-from-events")
}
