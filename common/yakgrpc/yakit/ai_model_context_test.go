package yakit

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
)

func TestAIModelContextPersistence(t *testing.T) {
	db, err := utils.CreateTempTestDatabaseInMemory()
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })
	require.NoError(t, db.AutoMigrate(&schema.GeneralStorage{}).Error)
	require.Equal(t, AIModelContext{ContextTokens: 500 * 1024, ThresholdTokens: 500 * 1024, Source: "default"}, GetAIModelContext(db, "provider", "model"))
	learned := AIModelContext{ContextTokens: 128000, ThresholdTokens: 102400, Source: "learned"}
	require.NoError(t, SaveAIModelContext(db, "provider", "model", learned))
	require.Equal(t, learned, GetAIModelContext(db, "provider", "model"))
	require.Equal(t, "default", GetAIModelContext(db, "other-provider", "model").Source)
	require.Equal(t, "default", GetAIModelContext(db, "provider", "other-model").Source)
	// A slower, older probe cannot raise a newly discovered smaller budget.
	require.NoError(t, SaveAIModelContext(db, "provider", "model", AIModelContext{ContextTokens: 200000, ThresholdTokens: 160000, Source: "learned"}))
	require.Equal(t, learned, GetAIModelContext(db, "provider", "model"))
	manual := AIModelContext{ContextTokens: 600 * 1024, ThresholdTokens: 400 * 1024, Source: "manual"}
	require.NoError(t, SaveAIModelContext(db, "provider", "model", manual))
	manual.ManualRevision = GetAIModelContext(db, "provider", "model").ManualRevision
	require.NotEmpty(t, manual.ManualRevision)
	require.Equal(t, manual, GetAIModelContext(db, "provider", "model"))
}
