package yakit

import (
	"testing"

	"github.com/jinzhu/gorm"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func setupAIGlobalConfigTestDB(t *testing.T) *gorm.DB {
	db, err := utils.CreateTempTestDatabaseInMemory()
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&schema.GeneralStorage{}, &schema.AIThirdPartyConfig{}).Error)
	return db
}

func TestSetAndGetAIGlobalConfig(t *testing.T) {
	db := setupAIGlobalConfigTestDB(t)
	defer db.Close()

	cfg := &ypb.AIGlobalConfig{
		Enabled:         true,
		RoutingPolicy:   "performance",
		DisableFallback: true,
		DefaultModelId:  "default-model",
		GlobalWeight:    0.75,
		IntelligentModels: []*ypb.AIModelConfig{
			{
				ModelName: "gpt-4o",
				Provider: &ypb.ThirdPartyApplicationConfig{
					Type:   "openai",
					APIKey: "key-1",
					Domain: "api.openai.com",
					ExtraParams: []*ypb.KVPair{
						{Key: "region", Value: "us"},
					},
				},
				ExtraParams: []*ypb.KVPair{{Key: "temperature", Value: "0.1"}},
			},
		},
		LightweightModels: []*ypb.AIModelConfig{
			{
				ModelName: "gpt-4o-mini",
				Provider: &ypb.ThirdPartyApplicationConfig{
					Type:   "openai",
					APIKey: "key-2",
					Domain: "api.openai.com",
				},
			},
		},
	}

	saved, err := SetAIGlobalConfig(db, cfg)
	require.NoError(t, err)
	require.NotNil(t, saved)

	loaded, err := GetAIGlobalConfig(db)
	require.NoError(t, err)
	require.NotNil(t, loaded)
	assert.True(t, loaded.Enabled)
	assert.Equal(t, "performance", loaded.RoutingPolicy)
	assert.True(t, loaded.DisableFallback)
	assert.Equal(t, "default-model", loaded.DefaultModelId)
	assert.Equal(t, 0.75, loaded.GlobalWeight)

	require.Len(t, loaded.IntelligentModels, 1)
	assert.NotZero(t, loaded.IntelligentModels[0].ProviderId)
	assert.NotNil(t, loaded.IntelligentModels[0].Provider)
	assert.Equal(t, "openai", loaded.IntelligentModels[0].Provider.Type)

	providers, err := ListAIProviders(db)
	require.NoError(t, err)
	assert.Len(t, providers, 2)
}

func TestApplyAIGlobalConfig(t *testing.T) {
	original := consts.GetTieredAIConfig()
	t.Cleanup(func() {
		consts.SetTieredAIConfig(original)
	})

	db := setupAIGlobalConfigTestDB(t)
	defer db.Close()

	cfg := &ypb.AIGlobalConfig{
		Enabled:         true,
		RoutingPolicy:   "cost",
		DisableFallback: true,
		DefaultModelId:  "default-model",
		GlobalWeight:    0.42,
		IntelligentModels: []*ypb.AIModelConfig{
			{
				ModelName: "gpt-4o",
				Provider: &ypb.ThirdPartyApplicationConfig{
					Type:   "openai",
					APIKey: "key-1",
					Domain: "api.openai.com",
				},
			},
		},
		LightweightModels: []*ypb.AIModelConfig{
			{
				ModelName: "gpt-4o-mini",
				Provider: &ypb.ThirdPartyApplicationConfig{
					Type:   "openai",
					APIKey: "key-2",
					Domain: "api.openai.com",
				},
			},
		},
	}

	_, err := SetAIGlobalConfig(db, cfg)
	require.NoError(t, err)
	require.NoError(t, ApplyAIGlobalConfig(db, cfg))

	applied := consts.GetTieredAIConfig()
	require.NotNil(t, applied)
	assert.True(t, applied.Enabled)
	assert.True(t, applied.DisableFallback)
	assert.Equal(t, consts.PolicyCost, applied.RoutingPolicy)
	assert.Equal(t, "default-model", applied.DefaultModelID)
	assert.Equal(t, 0.42, applied.GlobalWeight)
	assert.Len(t, applied.IntelligentConfigs, 1)
	assert.Len(t, applied.LightweightConfigs, 1)
	assert.Equal(t, "gpt-4o", lookupExtraParam(applied.IntelligentConfigs[0], "model"))
	assert.Equal(t, "gpt-4o-mini", lookupExtraParam(applied.LightweightConfigs[0], "model"))
}

func TestSetAIGlobalConfigRequiresProvider(t *testing.T) {
	db := setupAIGlobalConfigTestDB(t)
	defer db.Close()

	cfg := &ypb.AIGlobalConfig{
		Enabled:       true,
		RoutingPolicy: "balance",
		IntelligentModels: []*ypb.AIModelConfig{
			{
				ModelName: "missing-provider",
			},
		},
	}

	_, err := SetAIGlobalConfig(db, cfg)
	assert.Error(t, err)
}

func lookupExtraParam(cfg *ypb.ThirdPartyApplicationConfig, key string) string {
	if cfg == nil {
		return ""
	}
	for _, kv := range cfg.GetExtraParams() {
		if kv.GetKey() == key {
			return kv.GetValue()
		}
	}
	return ""
}
