package yakit

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jinzhu/gorm"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func TestAISessionMetaCRUD(t *testing.T) {
	db, err := utils.CreateTempTestDatabaseInMemory()
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&schema.AISession{}).Error)

	created, err := CreateOrUpdateAISessionMeta(db, "sess-1", "first title")
	require.NoError(t, err)
	require.Equal(t, "sess-1", created.SessionID)
	require.Equal(t, "first title", created.Title)

	updated, err := CreateOrUpdateAISessionMeta(db, "sess-1", "updated title")
	require.NoError(t, err)
	require.Equal(t, created.ID, updated.ID)
	require.Equal(t, "updated title", updated.Title)

	got, err := GetAISessionMetaBySessionID(db, "sess-1")
	require.NoError(t, err)
	require.Equal(t, "updated title", got.Title)
	require.Equal(t, emptyRelatedRuntimeIDsJSON, got.RelatedRuntimeIDS)

	list, err := QueryAISessionMeta(db, "updated", 10, 0)
	require.NoError(t, err)
	require.Len(t, list, 1)

	affected, err := UpdateAISessionMetaTitle(db, "sess-1", "final title")
	require.NoError(t, err)
	require.Equal(t, int64(1), affected)

	got, err = GetAISessionMetaBySessionID(db, "sess-1")
	require.NoError(t, err)
	require.Equal(t, "final title", got.Title)

	affected, err = DeleteAISessionMetaBySessionID(db, "sess-1")
	require.NoError(t, err)
	require.Equal(t, int64(1), affected)
}

func TestEnsureAISessionMetaDefaultTitle(t *testing.T) {
	db, err := utils.CreateTempTestDatabaseInMemory()
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&schema.AISession{}).Error)

	created, err := EnsureAISessionMeta(db, "sess-default-title")
	require.NoError(t, err)
	require.Equal(t, "sess-default-title", created.SessionID)

	got, err := GetAISessionMetaBySessionID(db, "sess-default-title")
	require.NoError(t, err)
	require.Equal(t, defaultAISessionTitle, got.Title)
	require.False(t, got.TitleInitialized)
	require.Equal(t, emptyRelatedRuntimeIDsJSON, got.RelatedRuntimeIDS)
}

func TestEnsureAISessionMetaNotOverrideExistingTitle(t *testing.T) {
	db, err := utils.CreateTempTestDatabaseInMemory()
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&schema.AISession{}).Error)

	_, err = CreateOrUpdateAISessionMeta(db, "sess-keep-title", "自定义标题")
	require.NoError(t, err)

	_, err = EnsureAISessionMeta(db, "sess-keep-title")
	require.NoError(t, err)

	got, err := GetAISessionMetaBySessionID(db, "sess-keep-title")
	require.NoError(t, err)
	require.Equal(t, "自定义标题", got.Title)
	require.True(t, got.TitleInitialized)
	require.Equal(t, emptyRelatedRuntimeIDsJSON, got.RelatedRuntimeIDS)
}

func TestAppendAISessionMetaRelatedRuntimeID(t *testing.T) {
	db, err := utils.CreateTempTestDatabaseInMemory()
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&schema.AISession{}).Error)

	sessionID := "sess-runtime-ids"
	runtimeID1 := "runtime-a"
	runtimeID2 := "runtime-b"
	_, err = CreateOrUpdateAISessionMeta(db, sessionID, "title")
	require.NoError(t, err)

	err = AppendAISessionMetaRelatedRuntimeID(db, sessionID, runtimeID1)
	require.NoError(t, err)
	err = AppendAISessionMetaRelatedRuntimeID(db, sessionID, " "+runtimeID1+" ")
	require.NoError(t, err)
	err = AppendAISessionMetaRelatedRuntimeID(db, sessionID, runtimeID2)
	require.NoError(t, err)

	got, err := GetAISessionMetaBySessionID(db, sessionID)
	require.NoError(t, err)
	require.Equal(t, `["`+runtimeID1+`","`+runtimeID2+`"]`, got.RelatedRuntimeIDS)
}

func TestAppendAISessionMetaRelatedRuntimeID_NotFoundIgnored(t *testing.T) {
	db, err := utils.CreateTempTestDatabaseInMemory()
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&schema.AISession{}).Error)

	err = AppendAISessionMetaRelatedRuntimeID(db, "missing-session", uuid.NewString())
	require.NoError(t, err)
}

func TestAppendAISessionMetaRelatedRuntimeID_InvalidStoredJSON(t *testing.T) {
	db, err := utils.CreateTempTestDatabaseInMemory()
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&schema.AISession{}).Error)

	sessionID := "sess-invalid-runtime-json"
	_, err = CreateOrUpdateAISessionMeta(db, sessionID, "title")
	require.NoError(t, err)
	require.NoError(t, db.Model(&schema.AISession{}).
		Where("session_id = ?", sessionID).
		UpdateColumn("related_runtime_ids", `{invalid-json}`).Error)

	err = AppendAISessionMetaRelatedRuntimeID(db, sessionID, uuid.NewString())
	require.Error(t, err)
}

func TestAppendAISessionMetaRelatedRuntimeID_PlainString(t *testing.T) {
	db, err := utils.CreateTempTestDatabaseInMemory()
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&schema.AISession{}).Error)

	sessionID := "sess-plain-runtime-id"
	_, err = CreateOrUpdateAISessionMeta(db, sessionID, "title")
	require.NoError(t, err)

	err = AppendAISessionMetaRelatedRuntimeID(db, sessionID, "not-a-uuid")
	require.NoError(t, err)

	got, err := GetAISessionMetaBySessionID(db, sessionID)
	require.NoError(t, err)
	require.Equal(t, `["not-a-uuid"]`, got.RelatedRuntimeIDS)
}

func TestEnsureAISessionMetaSetsSource(t *testing.T) {
	db, err := utils.CreateTempTestDatabaseInMemory()
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&schema.AISession{}).Error)

	meta, err := EnsureAISessionMeta(db, "sess-src-new", "ide")
	require.NoError(t, err)
	require.Equal(t, "ide", meta.Source)

	got, err := GetAISessionMetaBySessionID(db, "sess-src-new")
	require.NoError(t, err)
	require.Equal(t, "ide", got.Source)

	// Second start with a different source must not overwrite an existing value.
	_, err = EnsureAISessionMeta(db, "sess-src-new", "cli")
	require.NoError(t, err)
	got, err = GetAISessionMetaBySessionID(db, "sess-src-new")
	require.NoError(t, err)
	require.Equal(t, "ide", got.Source)
}

func TestEnsureAISessionMetaBackfillSource(t *testing.T) {
	db, err := utils.CreateTempTestDatabaseInMemory()
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&schema.AISession{}).Error)

	_, err = CreateOrUpdateAISessionMeta(db, "sess-backfill", "t")
	require.NoError(t, err)
	got, err := GetAISessionMetaBySessionID(db, "sess-backfill")
	require.NoError(t, err)
	require.Equal(t, "", got.Source)

	_, err = EnsureAISessionMeta(db, "sess-backfill", "yak")
	require.NoError(t, err)
	got, err = GetAISessionMetaBySessionID(db, "sess-backfill")
	require.NoError(t, err)
	require.Equal(t, "yak", got.Source)
}

func TestAISessionMetaStartParamsCRUD(t *testing.T) {
	db, err := utils.CreateTempTestDatabaseInMemory()
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&schema.AISession{}).Error)

	params := &ypb.AIStartParams{
		ReviewPolicy:              "ai",
		AIService:                 "openai",
		AIModelName:               "gpt-test",
		EnablePlan:                true,
		TimelineSessionID:         "sess-start-params",
		PreferSessionCachedConfig: true,
	}

	gotMeta, err := CreateOrUpdateAISessionMetaStartParams(db, "sess-start-params", params)
	require.NoError(t, err)
	require.NotEmpty(t, gotMeta.StartParams)

	got, err := GetAISessionMetaStartParamsBySessionID(db, "sess-start-params")
	require.NoError(t, err)
	require.Equal(t, "ai", got.GetReviewPolicy())
	require.Equal(t, "openai", got.GetAIService())
	require.Equal(t, "gpt-test", got.GetAIModelName())
	require.True(t, got.GetEnablePlan())
	require.True(t, got.GetPreferSessionCachedConfig())
}

func TestGetAISessionMetaStartParams_DiscardUnknownFields(t *testing.T) {
	db, err := utils.CreateTempTestDatabaseInMemory()
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&schema.AISession{}).Error)

	sessionID := "sess-unknown-start-field"
	_, err = CreateOrUpdateAISessionMeta(db, sessionID, "test")
	require.NoError(t, err)

	// Simulates start_params written by an older build that had UserPlanPrompt in AIStartParams.
	legacyJSON := `{"ReviewPolicy":"ai","AIService":"openai","UserPlanPrompt":"legacy plan hint"}`
	result := db.Model(&schema.AISession{}).
		Where("session_id = ?", sessionID).
		UpdateColumn("start_params", legacyJSON)
	require.NoError(t, result.Error)

	got, err := GetAISessionMetaStartParamsBySessionID(db, sessionID)
	require.NoError(t, err)
	require.Equal(t, "ai", got.GetReviewPolicy())
	require.Equal(t, "openai", got.GetAIService())
}

func TestTouchAISessionMetaLastUsedAt(t *testing.T) {
	db, err := utils.CreateTempTestDatabaseInMemory()
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&schema.AISession{}).Error)

	lastUsedAt := time.Unix(1716200000, 0)
	got, err := TouchAISessionMetaLastUsedAt(db, "sess-last-used", lastUsedAt)
	require.NoError(t, err)
	require.Equal(t, "sess-last-used", got.SessionID)
	require.Equal(t, lastUsedAt.Unix(), got.LastUsedAt.Unix())
	require.Equal(t, lastUsedAt.Unix(), got.UpdatedAt.Unix())

	got, err = GetAISessionMetaBySessionID(db, "sess-last-used")
	require.NoError(t, err)
	require.Equal(t, lastUsedAt.Unix(), got.LastUsedAt.Unix())
	require.Equal(t, lastUsedAt.Unix(), got.UpdatedAt.Unix())
}

func TestCreateOrUpdateAISessionMetaOnStart(t *testing.T) {
	db, err := utils.CreateTempTestDatabaseInMemory()
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&schema.AISession{}).Error)

	lastUsedAt := time.Unix(1716201234, 0)
	params := &ypb.AIStartParams{
		AIService:         "openai",
		AIModelName:       "gpt-test",
		TimelineSessionID: "sess-on-start",
	}

	got, err := CreateOrUpdateAISessionMetaOnStart(db, "sess-on-start", params, lastUsedAt)
	require.NoError(t, err)
	require.Equal(t, "sess-on-start", got.SessionID)
	require.Equal(t, lastUsedAt.Unix(), got.LastUsedAt.Unix())
	require.Equal(t, lastUsedAt.Unix(), got.UpdatedAt.Unix())
	require.NotEmpty(t, got.StartParams)

	savedParams, err := GetAISessionMetaStartParamsBySessionID(db, "sess-on-start")
	require.NoError(t, err)
	require.Equal(t, "openai", savedParams.GetAIService())
	require.Equal(t, "gpt-test", savedParams.GetAIModelName())
	require.Equal(t, "sess-on-start", savedParams.GetTimelineSessionID())
}

func TestOverlayAISessionStartParams(t *testing.T) {
	base := &ypb.AIStartParams{
		ReviewPolicy:         "manual",
		AIService:            "deepseek",
		AIModelName:          "model-a",
		EnablePlan:           true,
		UserInteractLimit:    9,
		TimelineSessionID:    "sess-1",
		DisableToolUse:       true,
		DisableAISearchForge: true,
		UserPresetPrompt:     "cached",
	}
	patch := &ypb.AIStartParams{
		AIService:         "openai",
		AIModelName:       "model-b",
		ReviewPolicy:      "ai",
		UserInteractLimit: 3,
	}

	next := OverlayAISessionStartParams(base, patch)
	require.Equal(t, "openai", next.GetAIService())
	require.Equal(t, "model-b", next.GetAIModelName())
	require.Equal(t, "ai", next.GetReviewPolicy())
	require.Equal(t, int64(3), next.GetUserInteractLimit())
	require.True(t, next.GetEnablePlan())
	require.True(t, next.GetDisableToolUse())
	require.True(t, next.GetDisableAISearchForge())
	require.Equal(t, "cached", next.GetUserPresetPrompt())
	require.Equal(t, "sess-1", next.GetTimelineSessionID())
}

func TestMigrateAISessionMetaFromEvents(t *testing.T) {
	profileDB, err := utils.CreateTempTestDatabaseInMemory()
	require.NoError(t, err)
	projectDB, err := utils.CreateTempTestDatabaseInMemory()
	require.NoError(t, err)

	require.NoError(t, profileDB.AutoMigrate(&schema.GeneralStorage{}).Error)
	require.NoError(t, projectDB.AutoMigrate(&schema.AISession{}, &schema.AiOutputEvent{}).Error)

	// sess-1: should prefer first input event title over other event content.
	require.NoError(t, projectDB.Create(&schema.AiOutputEvent{
		SessionId: "sess-1",
		Type:      schema.EVENT_TYPE_STRUCTURED,
		Content:   []byte(`{"prompt":"this should NOT be selected for sess-1"}`),
	}).Error)
	require.NoError(t, projectDB.Create(&schema.AiOutputEvent{
		SessionId: "sess-1",
		Type:      schema.EVENT_TYPE_INPUT,
		Content:   []byte(`{"free_input":"   first input title should be truncated at twenty chars   "}`),
	}).Error)

	// sess-2: no input event, fallback to parse prompt/content in first 100 events.
	require.NoError(t, projectDB.Create(&schema.AiOutputEvent{
		SessionId: "sess-2",
		Type:      schema.EVENT_TYPE_STRUCTURED,
		Content:   []byte(`{"prompt":"second session prompt title with extra spaces   "}`),
	}).Error)

	err = MigrateAISessionMetaFromEvents(profileDB, projectDB)
	require.NoError(t, err)
	require.Equal(t, "done", GetKey(profileDB, aiSessionMetaMigrationKey))

	s1, err := GetAISessionMetaBySessionID(projectDB, "sess-1")
	require.NoError(t, err)
	require.Equal(t, truncateTitle("first input title should be truncated at twenty chars", 20), s1.Title)

	s2, err := GetAISessionMetaBySessionID(projectDB, "sess-2")
	require.NoError(t, err)
	require.Equal(t, truncateTitle("second session prompt title with extra spaces", 20), s2.Title)

	// idempotent check: migration key already set, new session from events should not be migrated.
	require.NoError(t, projectDB.Create(&schema.AiOutputEvent{
		SessionId: "sess-3",
		Type:      schema.EVENT_TYPE_INPUT,
		Content:   []byte(`{"free_input":"third session should be skipped because migration already done"}`),
	}).Error)

	err = MigrateAISessionMetaFromEvents(profileDB, projectDB)
	require.NoError(t, err)

	var s3 schema.AISession
	err = projectDB.Where("session_id = ?", "sess-3").First(&s3).Error
	require.True(t, gorm.IsRecordNotFoundError(err))
}
