package yakgrpc

import (
	"context"
	"github.com/bytedance/mockey"
	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/yak/yaklib"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func isCI() bool {
	ciEnvVars := []string{
		"CI",
		"GITHUB_ACTIONS",
		"GITLAB_CI",
		"CIRCLECI",
		"TRAVIS",
		"JENKINS_HOME",
		"BUILDKITE",
	}
	for _, envVar := range ciEnvVars {
		if os.Getenv(envVar) != "" {
			return true
		}
	}
	return false
}

func TestAIGlobalConfig_GRPC_Local(t *testing.T) {
	if isCI() {
		t.Skip("skip grpc ai config local test in CI environment")
	}

	client, server, err := NewLocalClientAndServerWithTempDatabase(t)
	require.NoError(t, err)
	require.NotNil(t, client)
	require.NotNil(t, server)
	t.Cleanup(func() {
		if server.profileDatabase != nil {
			_ = server.profileDatabase.Close()
		}
		if server.projectDatabase != nil {
			_ = server.projectDatabase.Close()
		}
	})
	ctx := context.Background()

	cfg := &ypb.AIGlobalConfig{
		Enabled:         true,
		RoutingPolicy:   "performance",
		DisableFallback: true,
		DefaultModelId:  "default-model",
		GlobalWeight:    0.88,
		AIPresetPrompt:  "respond in markdown when suitable",
		IntelligentModels: []*ypb.AIModelConfig{
			{
				ModelName: "gpt-4o",
				Provider: &ypb.ThirdPartyApplicationConfig{
					Type:   "openai",
					APIKey: "key-1",
					Domain: "api.openai.com",
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

	_, err = client.SetAIGlobalConfig(ctx, cfg)
	require.NoError(t, err)

	got, err := client.GetAIGlobalConfig(ctx, &ypb.Empty{})
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.True(t, got.Enabled)
	assert.Equal(t, "performance", got.RoutingPolicy)
	assert.True(t, got.DisableFallback)
	assert.Equal(t, "default-model", got.DefaultModelId)
	assert.Equal(t, 0.88, got.GlobalWeight)
	assert.Equal(t, "respond in markdown when suitable", got.GetAIPresetPrompt())
	require.Len(t, got.IntelligentModels, 1)
	assert.NotNil(t, got.IntelligentModels[0].Provider)

	providers, err := client.ListAIProviders(ctx, &ypb.Empty{})
	require.NoError(t, err)
	assert.GreaterOrEqual(t, len(providers.Providers), 2)
	require.NotEmpty(t, providers.Providers)
	selected := providers.Providers[0]

	_, err = client.UpsertAIProvider(ctx, &ypb.UpsertAIProviderRequest{
		Provider: &ypb.AIProvider{
			Id: selected.GetId(),
			Config: &ypb.ThirdPartyApplicationConfig{
				Type:   selected.GetConfig().GetType(),
				APIKey: "updated-key",
				Domain: selected.GetConfig().GetDomain(),
			},
		},
	})
	require.Error(t, err)

	queryResp, err := client.QueryAIProvider(ctx, &ypb.QueryAIProvidersRequest{
		Filter: &ypb.AIProviderFilter{
			Ids:    []int64{selected.GetId()},
			AIType: []string{selected.GetConfig().GetType()},
		},
		Pagination: &ypb.Paging{
			Page:    1,
			Limit:   10,
			OrderBy: "id",
			Order:   "asc",
		},
	})
	require.NoError(t, err)
	require.NotNil(t, queryResp)
	assert.Equal(t, int64(1), queryResp.Total)
	require.Len(t, queryResp.Providers, 1)
	assert.Equal(t, selected.GetId(), queryResp.Providers[0].Id)

	_, err = client.DeleteAIProvider(ctx, &ypb.DeleteAIProviderRequest{Id: selected.GetId()})
	require.Error(t, err)
}

func TestGetApiKey_ReplaceAPIKeys(t *testing.T) {
	if isCI() {
		t.Skip("skip in CI environment")
	}

	client, server, err := NewLocalClientAndServerWithTempDatabase(t)
	require.NoError(t, err)
	require.NotNil(t, client)
	require.NotNil(t, server)

	db := server.GetProfileDatabase()
	oldKey := "old-key-for-test"
	testRecord := &schema.AIThirdPartyConfig{
		Type:   "openai",
		APIKey: oldKey,
		Domain: "api.openai.com",
	}
	err = db.Create(testRecord).Error
	require.NoError(t, err)

	t.Cleanup(func() {
		if testRecord.ID != 0 {
			db.Delete(&schema.AIThirdPartyConfig{}, testRecord.ID)
		}
		_ = yakit.SetKey(db, consts.AI_GLOBAL_CONFIG_KEY, "")
		if server.profileDatabase != nil {
			_ = server.profileDatabase.Close()
		}
		if server.projectDatabase != nil {
			_ = server.projectDatabase.Close()
		}
	})

	ctx := context.Background()

	cfg := &ypb.AIGlobalConfig{
		Enabled:         true,
		RoutingPolicy:   "performance",
		DisableFallback: true,
		DefaultModelId:  "default-model",
		GlobalWeight:    0.88,
		AIPresetPrompt:  "respond in markdown when suitable",
		IntelligentModels: []*ypb.AIModelConfig{
			{
				ModelName: "model-intel",
				Provider: &ypb.ThirdPartyApplicationConfig{
					Type:    "aibalance",
					APIKey:  oldKey,
					BaseURL: "https://aibalance.yaklang.com/v1",
				},
			},
		},
		LightweightModels: []*ypb.AIModelConfig{
			{
				ModelName: "model-light",
				Provider: &ypb.ThirdPartyApplicationConfig{
					Type:    "aibalance",
					APIKey:  oldKey,
					BaseURL: "https://aibalance.yaklang.com/v1",
				},
			},
		},
		VisionModels: []*ypb.AIModelConfig{
			{
				ModelName: "model-vision",
				Provider: &ypb.ThirdPartyApplicationConfig{
					Type:    "aibalance",
					APIKey:  "not-free-user", // 不会替换
					BaseURL: "https://aibalance.yaklang.com/v1",
				},
			},
		},
	}

	_, err = client.SetAIGlobalConfig(ctx, cfg)
	require.NoError(t, err)

	mockey.PatchConvey("mock online client", t, func() {
		newAPIKey := "mf-mock-created-key"

		mockey.Mock(consts.GetGormProfileDatabase).To(func() *gorm.DB {
			return db
		}).Build()

		mockey.Mock((*yaklib.OnlineClient).GetAIApiKeyByOnline).
			To(func(_ *yaklib.OnlineClient, ctx context.Context, token string) (string, error) {
				assert.Equal(t, "test-token", token)
				return newAPIKey, nil
			}).
			Build()

		req := &ypb.GetApiKeyByOnlineRequest{Token: "test-token"}
		resp, err := server.GetApiKeyByOnline(context.Background(), req)
		require.NoError(t, err)
		require.NotNil(t, resp)
		assert.Equal(t, newAPIKey, resp.ApiKey)

		updatedCfg, err := yakit.GetAIGlobalConfig(db)
		require.NoError(t, err)
		require.NotNil(t, updatedCfg)

		// 未替换的
		for _, m := range updatedCfg.IntelligentModels {
			assert.Equal(t, newAPIKey, m.Provider.APIKey)
		}
		for _, m := range updatedCfg.LightweightModels {
			assert.Equal(t, newAPIKey, m.Provider.APIKey)
		}
		for _, m := range updatedCfg.VisionModels {
			assert.Equal(t, "not-free-user", m.Provider.APIKey) // 未被替换
		}

	})

}
