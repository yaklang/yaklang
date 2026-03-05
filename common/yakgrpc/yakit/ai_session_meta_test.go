package yakit

import (
	"testing"

	"github.com/jinzhu/gorm"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
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
	require.True(t, got.TitleInitialized)
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
