package yakgrpc

import (
	"context"
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
	require.Len(t, got.IntelligentModels, 1)
	assert.NotZero(t, got.IntelligentModels[0].ProviderId)
	assert.NotNil(t, got.IntelligentModels[0].Provider)

	providers, err := client.ListAIProviders(ctx, &ypb.Empty{})
	require.NoError(t, err)
	assert.GreaterOrEqual(t, len(providers.Providers), 2)

	upsert, err := client.UpsertAIProvider(ctx, &ypb.UpsertAIProviderRequest{
		Provider: &ypb.AIProvider{
			Config: &ypb.ThirdPartyApplicationConfig{
				Type:   "custom",
				APIKey: "custom-key",
				Domain: "custom.example.com",
			},
		},
	})
	require.NoError(t, err)
	require.NotNil(t, upsert.GetProvider())
	assert.NotZero(t, upsert.Provider.Id)

	_, err = client.DeleteAIProvider(ctx, &ypb.DeleteAIProviderRequest{Id: upsert.Provider.Id})
	require.NoError(t, err)
}
