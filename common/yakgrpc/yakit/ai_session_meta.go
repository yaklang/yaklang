package yakit

import (
	"encoding/json"
	"strings"
	"time"

	"github.com/jinzhu/gorm"
	"github.com/samber/lo"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

const aiSessionMetaMigrationKey = "yakit.ai_session_meta.migrated.v1"
const defaultAISessionTitle = "<未命名>"
const emptyRelatedRuntimeIDsJSON = "[]"

// RegisterAIAgentSession ensures project DB has a row for a yak/aim ReAct session id.
// source is optional (e.g. yak, cli) and is written when the session row is first created.
func RegisterAIAgentSession(db *gorm.DB, sessionID string, source ...string) error {
	if db == nil {
		return utils.Errorf("database is nil")
	}
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return utils.Errorf("session_id is empty")
	}
	record, err := CreateOrUpdateAISessionMeta(db, sessionID, defaultAISessionTitle)
	if err != nil {
		return err
	}
	src := ""
	if len(source) > 0 {
		src = strings.TrimSpace(source[0])
	}
	if src == "" || record == nil {
		return nil
	}
	if strings.TrimSpace(record.Source) != "" {
		return nil
	}
	return db.Model(&schema.AISession{}).
		Where("session_id = ?", sessionID).
		UpdateColumn("source", src).Error
}

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

func marshalAISessionStartParams(params *ypb.AIStartParams) (string, error) {
	if params == nil {
		return "", nil
	}
	raw, err := protojson.Marshal(params)
	if err != nil {
		return "", utils.Errorf("marshal ai start params failed: %v", err)
	}
	return string(raw), nil
}

func UnmarshalAISessionStartParams(raw string) (*ypb.AIStartParams, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	params := &ypb.AIStartParams{}
	if err := (protojson.UnmarshalOptions{DiscardUnknown: true}).Unmarshal([]byte(raw), params); err != nil {
		return nil, utils.Errorf("unmarshal ai start params failed: %v", err)
	}
	return params, nil
}

func UpdateAISessionMetaStartParams(db *gorm.DB, sessionID string, params *ypb.AIStartParams) (int64, error) {
	if db == nil {
		return 0, utils.Errorf("database is nil")
	}
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return 0, utils.Errorf("session_id is empty")
	}
	raw, err := marshalAISessionStartParams(params)
	if err != nil {
		return 0, err
	}

	result := db.Model(&schema.AISession{}).
		Where("session_id = ?", sessionID).
		UpdateColumn("start_params", raw)
	return result.RowsAffected, result.Error
}

func CreateOrUpdateAISessionMetaStartParams(db *gorm.DB, sessionID string, params *ypb.AIStartParams) (*schema.AISession, error) {
	if db == nil {
		return nil, utils.Errorf("database is nil")
	}
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return nil, utils.Errorf("session_id is empty")
	}
	if _, err := EnsureAISessionMeta(db, sessionID); err != nil {
		return nil, err
	}
	if _, err := UpdateAISessionMetaStartParams(db, sessionID, params); err != nil {
		return nil, err
	}
	return GetAISessionMetaBySessionID(db, sessionID)
}

func CreateOrUpdateAISessionMetaOnStart(db *gorm.DB, sessionID string, params *ypb.AIStartParams, lastUsedAt time.Time) (*schema.AISession, error) {
	if db == nil {
		return nil, utils.Errorf("database is nil")
	}
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return nil, utils.Errorf("session_id is empty")
	}
	if lastUsedAt.IsZero() {
		lastUsedAt = time.Now()
	}
	raw, err := marshalAISessionStartParams(params)
	if err != nil {
		return nil, err
	}
	if _, err := EnsureAISessionMeta(db, sessionID); err != nil {
		return nil, err
	}
	if err := db.Model(&schema.AISession{}).
		Where("session_id = ?", sessionID).
		UpdateColumns(map[string]any{
			"start_params": raw,
			"last_used_at": lastUsedAt,
			"updated_at":   lastUsedAt,
		}).Error; err != nil {
		return nil, err
	}
	return GetAISessionMetaBySessionID(db, sessionID)
}

func UpdateAISessionMetaLastUsedAt(db *gorm.DB, sessionID string, lastUsedAt time.Time) (int64, error) {
	if db == nil {
		return 0, utils.Errorf("database is nil")
	}
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return 0, utils.Errorf("session_id is empty")
	}
	if lastUsedAt.IsZero() {
		lastUsedAt = time.Now()
	}

	result := db.Model(&schema.AISession{}).
		Where("session_id = ?", sessionID).
		UpdateColumns(map[string]any{
			"last_used_at": lastUsedAt,
			"updated_at":   lastUsedAt,
		})
	return result.RowsAffected, result.Error
}

func TouchAISessionMetaLastUsedAt(db *gorm.DB, sessionID string, lastUsedAt time.Time) (*schema.AISession, error) {
	if db == nil {
		return nil, utils.Errorf("database is nil")
	}
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return nil, utils.Errorf("session_id is empty")
	}
	if _, err := EnsureAISessionMeta(db, sessionID); err != nil {
		return nil, err
	}
	if _, err := UpdateAISessionMetaLastUsedAt(db, sessionID, lastUsedAt); err != nil {
		return nil, err
	}
	return GetAISessionMetaBySessionID(db, sessionID)
}

func GetAISessionMetaStartParamsBySessionID(db *gorm.DB, sessionID string) (*ypb.AIStartParams, error) {
	meta, err := GetAISessionMetaBySessionID(db, sessionID)
	if err != nil {
		return nil, err
	}
	return UnmarshalAISessionStartParams(meta.StartParams)
}

func MergeCachedAISessionStartParams(cached, request *ypb.AIStartParams) *ypb.AIStartParams {
	if cached == nil && request == nil {
		return nil
	}
	if cached == nil {
		return proto.Clone(request).(*ypb.AIStartParams)
	}
	if request == nil {
		return proto.Clone(cached).(*ypb.AIStartParams)
	}

	merged := proto.Clone(cached).(*ypb.AIStartParams)
	merged.CoordinatorId = request.GetCoordinatorId()
	merged.Sequence = request.GetSequence()
	merged.UserQuery = request.GetUserQuery()
	merged.TimelineSessionID = request.GetTimelineSessionID()
	merged.McpServers = request.GetMcpServers()
	merged.ForgeParams = request.GetForgeParams()
	return merged
}

func OverlayAISessionStartParams(base, patch *ypb.AIStartParams) *ypb.AIStartParams {
	if base == nil && patch == nil {
		return nil
	}
	if base == nil {
		return proto.Clone(patch).(*ypb.AIStartParams)
	}
	if patch == nil {
		return proto.Clone(base).(*ypb.AIStartParams)
	}

	next := proto.Clone(base).(*ypb.AIStartParams)

	if patch.GetCoordinatorId() != "" {
		next.CoordinatorId = patch.GetCoordinatorId()
	}
	if patch.GetSequence() != 0 {
		next.Sequence = patch.GetSequence()
	}
	if patch.GetUserQuery() != "" {
		next.UserQuery = patch.GetUserQuery()
	}
	if len(patch.GetMcpServers()) > 0 {
		next.McpServers = patch.GetMcpServers()
	}
	if patch.GetEnableSystemFileSystemOperator() {
		next.EnableSystemFileSystemOperator = true
	}
	if patch.GetUseDefaultAIConfig() {
		next.UseDefaultAIConfig = true
	}
	if patch.GetForgeName() != "" {
		next.ForgeName = patch.GetForgeName()
	}
	if len(patch.GetForgeParams()) > 0 {
		next.ForgeParams = patch.GetForgeParams()
	}
	if patch.GetDisallowRequireForUserPrompt() {
		next.DisallowRequireForUserPrompt = true
	}
	if patch.GetReviewPolicy() != "" {
		next.ReviewPolicy = patch.GetReviewPolicy()
	}
	if patch.GetAIReviewRiskControlScore() > 0 {
		next.AIReviewRiskControlScore = patch.GetAIReviewRiskControlScore()
	}
	if patch.GetDisableToolUse() {
		next.DisableToolUse = true
	}
	if patch.GetAICallAutoRetry() > 0 {
		next.AICallAutoRetry = patch.GetAICallAutoRetry()
	}
	if patch.GetAITransactionRetry() > 0 {
		next.AITransactionRetry = patch.GetAITransactionRetry()
	}
	if patch.GetEnableAISearchTool() {
		next.EnableAISearchTool = true
	}
	if patch.GetDisableAISearchForge() {
		next.DisableAISearchForge = true
	}
	if patch.GetEnableAISearchInternet() {
		next.EnableAISearchInternet = true
	}
	if len(patch.GetIncludeSuggestedToolNames()) > 0 {
		next.IncludeSuggestedToolNames = patch.GetIncludeSuggestedToolNames()
	}
	if len(patch.GetIncludeSuggestedToolKeywords()) > 0 {
		next.IncludeSuggestedToolKeywords = patch.GetIncludeSuggestedToolKeywords()
	}
	if len(patch.GetExcludeToolNames()) > 0 {
		next.ExcludeToolNames = patch.GetExcludeToolNames()
	}
	if patch.GetEnableQwenNoThinkMode() {
		next.EnableQwenNoThinkMode = true
	}
	if patch.GetAllowPlanUserInteract() {
		next.AllowPlanUserInteract = true
	}
	if patch.GetPlanUserInteractMaxCount() > 0 {
		next.PlanUserInteractMaxCount = patch.GetPlanUserInteractMaxCount()
	}
	if patch.GetAllowGenerateReport() {
		next.AllowGenerateReport = true
	}
	if patch.GetTaskMaxContinueCount() > 0 {
		next.TaskMaxContinueCount = patch.GetTaskMaxContinueCount()
	}
	if patch.GetAIService() != "" {
		next.AIService = patch.GetAIService()
	}
	if patch.GetAIModelName() != "" {
		next.AIModelName = patch.GetAIModelName()
	}
	if patch.GetReActMaxIteration() > 0 {
		next.ReActMaxIteration = patch.GetReActMaxIteration()
	}
	if patch.GetTimelineItemLimit() > 0 {
		next.TimelineItemLimit = patch.GetTimelineItemLimit()
	}
	if patch.GetTimelineContentSizeLimit() > 0 {
		next.TimelineContentSizeLimit = patch.GetTimelineContentSizeLimit()
	}
	if patch.GetUserInteractLimit() > 0 {
		next.UserInteractLimit = patch.GetUserInteractLimit()
	}
	if patch.GetTimelineSessionID() != "" {
		next.TimelineSessionID = patch.GetTimelineSessionID()
	}
	if patch.GetAICallTokenLimit() > 0 {
		next.AICallTokenLimit = patch.GetAICallTokenLimit()
	}
	if patch.GetUserPresetPrompt() != "" {
		next.UserPresetPrompt = patch.GetUserPresetPrompt()
	}
	if patch.GetDisableToolIntervalReview() {
		next.DisableToolIntervalReview = true
	}
	if patch.GetSyncPerceptionTrigger() {
		next.SyncPerceptionTrigger = true
	}
	if patch.GetEnablePlan() {
		next.EnablePlan = true
	}
	if patch.GetEnableDetachedPlan() {
		next.EnableDetachedPlan = true
	}
	if patch.GetPreferSessionCachedConfig() {
		next.PreferSessionCachedConfig = true
	}
	if len(patch.GetEnabledCapabilities()) > 0 {
		next.EnabledCapabilities = overlayEnabledCapabilities(base, patch)
	}

	return next
}

func overlayEnabledCapabilities(base, patch *ypb.AIStartParams) []*ypb.AIEnabledCapability {
	if patch == nil || len(patch.GetEnabledCapabilities()) == 0 {
		return nil
	}
	merged := make([]*ypb.AIEnabledCapability, 0)
	seen := make(map[string]struct{})
	appendCap := func(item *ypb.AIEnabledCapability) {
		if item == nil {
			return
		}
		name := strings.TrimSpace(item.GetName())
		capType := strings.ToLower(strings.TrimSpace(item.GetType()))
		if name == "" || capType == "" {
			return
		}
		key := capType + ":" + name
		if _, ok := seen[key]; ok {
			return
		}
		seen[key] = struct{}{}
		merged = append(merged, &ypb.AIEnabledCapability{Name: name, Type: capType})
	}
	if base != nil {
		for _, item := range base.GetEnabledCapabilities() {
			appendCap(item)
		}
	}
	for _, item := range patch.GetEnabledCapabilities() {
		appendCap(item)
	}
	return merged
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

// EnsureAISessionMeta creates session meta if missing. Optional source (first
// variadic arg, trimmed) is written on insert and backfilled when the row
// exists but source is still empty.
func EnsureAISessionMeta(db *gorm.DB, sessionID string, sourceOpt ...string) (*schema.AISession, error) {
	if db == nil {
		return nil, utils.Errorf("database is nil")
	}
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return nil, utils.Errorf("session_id is empty")
	}

	source := ""
	if len(sourceOpt) > 0 {
		source = strings.TrimSpace(sourceOpt[0])
	}

	record := &schema.AISession{SessionID: sessionID}
	attrs := map[string]any{
		"title":               defaultAISessionTitle,
		"title_initialized":   false,
		"related_runtime_ids": emptyRelatedRuntimeIDsJSON,
	}
	if source != "" {
		attrs["source"] = source
	}
	result := db.Model(&schema.AISession{}).
		Where("session_id = ?", sessionID).
		Attrs(attrs).
		FirstOrCreate(record)
	if result.Error != nil {
		return nil, result.Error
	}

	if source != "" && strings.TrimSpace(record.Source) == "" {
		if err := db.Model(&schema.AISession{}).
			Where("session_id = ?", sessionID).
			Update("source", source).Error; err != nil {
			return nil, err
		}
		record.Source = source
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
