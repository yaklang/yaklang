package yakit

import (
	"errors"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/gorm"
	"github.com/yaklang/yaklang/common/ai/aispec"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"google.golang.org/protobuf/proto"
)

func contextStorageFixture(t *testing.T) (*gorm.DB, *ypb.AIGlobalConfig) {
	t.Helper()
	db, err := utils.CreateTempTestDatabaseInMemory()
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })
	require.NoError(t, db.AutoMigrate(&schema.GeneralStorage{}).Error)
	provider := func() *ypb.AIModelConfig {
		return &ypb.AIModelConfig{ModelName: "model-a", Provider: &ypb.ThirdPartyApplicationConfig{Type: "openai", BaseURL: "https://example.test/v1", APIKey: "test-only"}}
	}
	cfg, err := SetAIGlobalConfig(db, &ypb.AIGlobalConfig{RoutingPolicy: "balance", IntelligentModels: []*ypb.AIModelConfig{provider(), provider()}})
	require.NoError(t, err)
	require.NotEqual(t, cfg.IntelligentModels[0].ContextOwnerId, cfg.IntelligentModels[1].ContextOwnerId)
	return db, cfg
}

func TestAIModelContextProviderLifecycle(t *testing.T) {
	db, cfg := contextStorageFixture(t)
	first, other := cfg.IntelligentModels[0], cfg.IntelligentModels[1]
	owner := first.ContextOwnerId
	keyA, keyOther := aispec.ModelContextIdentity(first), aispec.ModelContextIdentity(other)
	require.NoError(t, SetAIModelContextSettings(db, "openai", "model-a", 200000, nil, keyA))
	require.NoError(t, SetAIModelContextSettings(db, "openai", "model-a", 600000, nil, keyOther))
	// Changing the selected model keeps the same provider entry and prior settings.
	first.ModelName = "model-b"
	_, err := SetAIGlobalConfig(db, cfg)
	require.NoError(t, err)
	require.Equal(t, owner, first.ContextOwnerId)
	keyB := aispec.ModelContextIdentity(first)
	require.EqualValues(t, 500000, GetAIModelContext(db, "openai", "model-b", keyB).ThresholdTokens)
	require.NoError(t, SetAIModelContextSettings(db, "openai", "model-b", 300000, nil, keyB))
	first.ModelName = "model-a"
	_, err = SetAIGlobalConfig(db, cfg)
	require.NoError(t, err)
	require.Equal(t, keyA, aispec.ModelContextIdentity(first))
	require.EqualValues(t, 200000, GetAIModelContext(db, "openai", "model-a", keyA).ThresholdTokens)
	require.EqualValues(t, 300000, GetAIModelContext(db, "openai", "model-b", keyB).ThresholdTokens)
	// Reordering/moving an entry between tiers is not deleting its owner.
	cfg.IntelligentModels = []*ypb.AIModelConfig{other}
	cfg.LightweightModels = []*ypb.AIModelConfig{first}
	_, err = SetAIGlobalConfig(db, cfg)
	require.NoError(t, err)
	require.EqualValues(t, 200000, GetAIModelContext(db, "openai", "model-a", keyA).ThresholdTokens)
	late := GetAIModelContext(db, "openai", "model-a", keyA)
	late.Source, late.ThresholdTokens = "learned", 150000
	// Removing the whole entry deletes all models it has ever selected, atomically.
	cfg.LightweightModels = nil
	_, err = SetAIGlobalConfig(db, cfg)
	require.NoError(t, err)
	for _, key := range []string{keyA, keyB} {
		require.ErrorIs(t, ValidateAIModelContextOwner(db, key), ErrAIModelContextOwnerDeleted)
	}
	require.Empty(t, GetKey(db, aiModelContextKey("openai", "model-a", keyA)))
	require.Empty(t, GetKey(db, aiModelContextKey("openai", "model-b", keyB)))
	require.EqualValues(t, 600000, GetAIModelContext(db, "openai", "model-a", keyOther).ThresholdTokens, "identical credentials in another provider entry must be retained")
	require.ErrorIs(t, SaveAIModelContext(db, "openai", "model-a", late, keyA), ErrAIModelContextOwnerDeleted)
	require.ErrorIs(t, SetAIModelContextSettings(db, "openai", "model-b", 400000, nil, keyB), ErrAIModelContextOwnerDeleted)
	require.Empty(t, GetKey(db, aiModelContextKey("openai", "model-a", keyA)), "late success cannot resurrect a deleted owner")
	// A deliberate new Add gets a fresh binding even with the same credentials/name.
	readded := proto.Clone(first).(*ypb.AIModelConfig)
	readded.ContextOwnerId = ""
	cfg.IntelligentModels = append(cfg.IntelligentModels, readded)
	_, err = SetAIGlobalConfig(db, cfg)
	require.NoError(t, err)
	require.NotEqual(t, owner, readded.ContextOwnerId)
	require.EqualValues(t, 500000, GetAIModelContext(db, "openai", "model-a", aispec.ModelContextIdentity(readded)).ThresholdTokens)
	require.ErrorIs(t, SaveAIModelContext(db, "openai", "model-a", late, keyA), ErrAIModelContextOwnerDeleted)
	loaded, err := GetAIGlobalConfig(db)
	require.NoError(t, err)
	require.Equal(t, readded.ContextOwnerId, loaded.IntelligentModels[1].ContextOwnerId)
	require.Equal(t, readded.ContextOwnerId, cloneAIGlobalConfig(cfg).IntelligentModels[1].ContextOwnerId)
}

func TestAIModelContextManualBoundsAndAutoAdjustPreference(t *testing.T) {
	db, cfg := contextStorageFixture(t)
	key := aispec.ModelContextIdentity(cfg.IntelligentModels[0])
	initial := GetAIModelContext(db, "openai", "model-a", key)
	require.EqualValues(t, 500000, initial.ThresholdTokens)
	require.False(t, initial.DisableAutoAdjust)
	disabled := true
	require.NoError(t, SetAIModelContextSettings(db, "openai", "model-a", 0, &disabled, key))
	for _, value := range []int64{100000, 100001, 102400, 500000, 1000000} {
		require.NoError(t, SetAIModelContextSettings(db, "openai", "model-a", value, nil, key))
		got := GetAIModelContext(db, "openai", "model-a", key)
		require.Equal(t, value, got.ThresholdTokens)
		require.True(t, got.DisableAutoAdjust)
	}
	before := GetAIModelContext(db, "openai", "model-a", key)
	disabled = false
	for _, value := range []int64{-1, 99999, 1000001} {
		require.Error(t, SetAIModelContextSettings(db, "openai", "model-a", value, &disabled, key))
		require.Equal(t, before, GetAIModelContext(db, "openai", "model-a", key))
	}
	require.NoError(t, SetAIModelContextSettings(db, "openai", "model-a", 0, &disabled, key))
	learned := GetAIModelContext(db, "openai", "model-a", key)
	learned.ContextTokens, learned.ThresholdTokens, learned.Source = 64000, 51200, "learned"
	require.NoError(t, SaveAIModelContext(db, "openai", "model-a", learned, key))
	require.EqualValues(t, 51200, GetAIModelContext(db, "openai", "model-a", key).ThresholdTokens)
	disabled = true
	require.NoError(t, SetAIModelContextSettings(db, "openai", "model-a", 0, &disabled, key))
	require.EqualValues(t, 51200, GetAIModelContext(db, "openai", "model-a", key).ThresholdTokens)
}

func TestAIModelContextDeleteAndLateWriteAreAtomic(t *testing.T) {
	db, cfg := contextStorageFixture(t)
	model := cfg.IntelligentModels[0]
	key := aispec.ModelContextIdentity(model)
	require.NoError(t, SetAIModelContextSettings(db, "openai", "model-a", 300000, nil, key))
	learned := GetAIModelContext(db, "openai", "model-a", key)
	learned.Source, learned.ThresholdTokens = "learned", 200000
	cfg.IntelligentModels = cfg.IntelligentModels[1:]
	var wg sync.WaitGroup
	start := make(chan struct{})
	results := make(chan error, 2)
	wg.Add(2)
	go func() { defer wg.Done(); <-start; _, err := SetAIGlobalConfig(db, cfg); results <- err }()
	go func() { defer wg.Done(); <-start; results <- SaveAIModelContext(db, "openai", "model-a", learned, key) }()
	close(start)
	wg.Wait()
	close(results)
	for err := range results {
		require.True(t, err == nil || errors.Is(err, ErrAIModelContextOwnerDeleted), "%v", err)
	}
	require.Empty(t, GetKey(db, aiModelContextKey("openai", "model-a", key)))
	require.ErrorIs(t, ValidateAIModelContextOwner(db, key), ErrAIModelContextOwnerDeleted)
}

func TestAIModelContextOwnerAssignmentIsStableAndUnique(t *testing.T) {
	db, cfg := contextStorageFixture(t)
	owner := cfg.IntelligentModels[0].ContextOwnerId
	cfg.IntelligentModels[1].ContextOwnerId = owner
	saved, err := SetAIGlobalConfig(db, cfg)
	require.NoError(t, err)
	require.Equal(t, owner, saved.IntelligentModels[0].ContextOwnerId)
	require.NotEqual(t, owner, saved.IntelligentModels[1].ContextOwnerId)
	for i := 0; i < 2; i++ {
		loaded, err := GetAIGlobalConfig(db)
		require.NoError(t, err)
		require.True(t, proto.Equal(saved, loaded))
	}
}

func TestAIModelContextDeletionFailureKeepsProviderAndSettings(t *testing.T) {
	db, cfg := contextStorageFixture(t)
	model := cfg.IntelligentModels[0]
	key := aispec.ModelContextIdentity(model)
	require.NoError(t, SetAIModelContextSettings(db, "openai", "model-a", 200000, nil, key))
	// Simulate a storage failure after the global config write inside the transaction.
	require.NoError(t, db.Exec(`CREATE TRIGGER reject_context_delete BEFORE DELETE ON general_storages
		WHEN OLD.key LIKE '"ai-model-context:%'
		BEGIN SELECT RAISE(ABORT, 'test context delete failure'); END`).Error)
	_, err := SetAIGlobalConfig(db, &ypb.AIGlobalConfig{})
	require.ErrorContains(t, err, "test context delete failure")
	require.NoError(t, ValidateAIModelContextOwner(db, key))
	loaded, err := GetAIGlobalConfig(db)
	require.NoError(t, err)
	require.True(t, proto.Equal(cfg, loaded), "failed cleanup must also roll back provider removal")
	require.EqualValues(t, 200000, GetAIModelContext(db, "openai", "model-a", key).ThresholdTokens)
}
