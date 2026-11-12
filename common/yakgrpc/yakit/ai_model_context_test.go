package yakit

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
)

func TestAIModelContextDecimalUnitsPreserveStoredTokens(t *testing.T) {
	db, err := utils.CreateTempTestDatabaseInMemory()
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })
	require.NoError(t, db.AutoMigrate(&schema.GeneralStorage{}).Error)
	initial := GetAIModelContext(db, "p", "new")
	require.EqualValues(t, 500000, initial.ThresholdTokens)
	require.EqualValues(t, 100000, MinimumAIModelContextTokens)
	require.EqualValues(t, 1000000, AIModelContextMaximumThreshold(initial))
	for _, saved := range []AIModelContext{
		{ContextTokens: 512000, ThresholdTokens: 512000, Source: "manual"},
		{ContextTokens: 512000, ThresholdTokens: 1024000, Source: "manual"},
		{ContextTokens: 131072, ThresholdTokens: 104857, Source: "learned"},
		{ContextTokens: 64000, ThresholdTokens: 51200, Source: "learned"},
	} {
		// Write the old storage payload directly: upgrading must not rescale,
		// round or clamp historical settings, even above the new default 1M.
		raw, err := json.Marshal(saved)
		require.NoError(t, err)
		key := aiModelContextKey("p", "legacy")
		require.NoError(t, SetKey(db, key, string(raw)))
		require.Equal(t, saved, GetAIModelContext(db, "p", "legacy"))
		require.Equal(t, string(raw), GetKey(db, key), "reading must not rewrite saved tokens")
		disabled := true
		require.NoError(t, SetAIModelContextSettings(db, "p", "legacy", 0, &disabled))
		got := GetAIModelContext(db, "p", "legacy")
		require.Equal(t, saved.ThresholdTokens, got.ThresholdTokens)
		require.Equal(t, saved.ContextTokens, got.ContextTokens)
	}
}

func TestAIModelContextPersistence(t *testing.T) {
	db, err := utils.CreateTempTestDatabaseInMemory()
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })
	require.NoError(t, db.AutoMigrate(&schema.GeneralStorage{}).Error)
	require.Equal(t, AIModelContext{ContextTokens: 500 * 1000, ThresholdTokens: 500 * 1000, Source: "default"}, GetAIModelContext(db, "provider", "model"))
	learned := AIModelContext{ContextTokens: 128000, ThresholdTokens: 102400, Source: "learned"}
	require.NoError(t, SaveAIModelContext(db, "provider", "model", learned))
	require.Equal(t, learned, GetAIModelContext(db, "provider", "model"))
	require.Equal(t, "default", GetAIModelContext(db, "other-provider", "model").Source)
	require.Equal(t, "default", GetAIModelContext(db, "provider", "other-model").Source)
	// A slower, older probe cannot raise a newly discovered smaller budget.
	require.NoError(t, SaveAIModelContext(db, "provider", "model", AIModelContext{ContextTokens: 200000, ThresholdTokens: 160000, Source: "learned"}))
	require.Equal(t, learned, GetAIModelContext(db, "provider", "model"))
	manual := AIModelContext{ContextTokens: 600 * 1000, ThresholdTokens: 400 * 1000, Source: "manual"}
	require.NoError(t, SaveAIModelContext(db, "provider", "model", manual))
	manual.ManualRevision = GetAIModelContext(db, "provider", "model").ManualRevision
	require.NotEmpty(t, manual.ManualRevision)
	require.Equal(t, manual, GetAIModelContext(db, "provider", "model"))
}

func TestAIModelContextManualBoundsAndAutoAdjustPreference(t *testing.T) {
	db, err := utils.CreateTempTestDatabaseInMemory()
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })
	require.NoError(t, db.AutoMigrate(&schema.GeneralStorage{}).Error)
	initial := GetAIModelContext(db, "p", "m")
	require.False(t, initial.DisableAutoAdjust, "legacy and new models default to automatic adjustment")
	disabled := true
	require.NoError(t, SetAIModelContextSettings(db, "p", "m", 0, &disabled))
	off := GetAIModelContext(db, "p", "m")
	require.True(t, off.DisableAutoAdjust)
	require.Equal(t, initial.ThresholdTokens, off.ThresholdTokens)
	require.NoError(t, SetAIModelContextSettings(db, "p", "m", 1000*1000, nil))
	atMax := GetAIModelContext(db, "p", "m")
	require.True(t, atMax.DisableAutoAdjust, "a threshold-only edit must preserve the switch")
	disabled = false
	require.Error(t, SetAIModelContextSettings(db, "p", "m", 1000*1000+1, &disabled))
	require.Equal(t, atMax, GetAIModelContext(db, "p", "m"), "an invalid edit must change neither threshold nor preference")
	require.Error(t, SaveAIModelContext(db, "p", "m", AIModelContext{ContextTokens: 500 * 1000, ThresholdTokens: 1200 * 1000, Source: "manual"}))
	require.False(t, GetAIModelContext(db, "other", "m").DisableAutoAdjust)
	require.False(t, GetAIModelContext(db, "p", "other").DisableAutoAdjust)
	require.NoError(t, SetAIModelContextSettings(db, "p", "m", 0, &disabled))
	require.False(t, GetAIModelContext(db, "p", "m").DisableAutoAdjust)
	require.EqualValues(t, 1000*1000, GetAIModelContext(db, "p", "m").ThresholdTokens)
	require.NoError(t, SaveAIModelContext(db, "p", "small", AIModelContext{ContextTokens: 64 * 1000, ThresholdTokens: 51 * 1000, Source: "learned"}))
	disabled = true
	require.NoError(t, SetAIModelContextSettings(db, "p", "small", 0, &disabled))
	require.EqualValues(t, 51*1000, GetAIModelContext(db, "p", "small").ThresholdTokens)
	require.True(t, GetAIModelContext(db, "p", "small").DisableAutoAdjust)
}
