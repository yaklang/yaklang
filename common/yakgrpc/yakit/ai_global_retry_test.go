package yakit

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func TestGlobalRetryPersistenceAndRuntime(t *testing.T) {
	db := setupAIGlobalConfigTestDB(t)
	for _, enabled := range []bool{true, false} {
		_, err := SetAIGlobalConfig(db, &ypb.AIGlobalConfig{RetryForever: enabled, RoutingPolicy: "performance"})
		require.NoError(t, err)
		saved, err := GetAIGlobalConfig(db)
		require.NoError(t, err)
		require.Equal(t, enabled, saved.GetRetryForever())
		require.NoError(t, ApplyAIGlobalConfig(db, saved))
		require.Equal(t, enabled, GetCachedAIGlobalConfig().GetRetryForever())
		require.Equal(t, enabled, consts.IsAIRetryForeverEnabled())
		cloned := GetCachedAIGlobalConfig()
		cloned.RetryForever = !enabled
		require.Equal(t, enabled, GetCachedAIGlobalConfig().GetRetryForever())
	}
}

func TestGlobalNetworkDebugPersistenceAndRuntime(t *testing.T) {
	db := setupAIGlobalConfigTestDB(t)
	for _, enabled := range []bool{false, true, false} {
		_, err := SetAIGlobalConfig(db, &ypb.AIGlobalConfig{NetworkDebug: enabled, RetryForever: true, RoutingPolicy: "performance"})
		require.NoError(t, err)
		saved, err := GetAIGlobalConfig(db)
		require.NoError(t, err)
		require.Equal(t, enabled, saved.GetNetworkDebug())
		require.NoError(t, ApplyAIGlobalConfig(db, saved))
		require.Equal(t, enabled, consts.IsAINetworkDebugEnabled())
		cloned := GetCachedAIGlobalConfig()
		require.Equal(t, enabled, cloned.GetNetworkDebug())
		require.True(t, cloned.GetRetryForever())
		cloned.NetworkDebug = !enabled
		require.Equal(t, enabled, GetCachedAIGlobalConfig().GetNetworkDebug())
	}
}

func TestScriptProviderGlobalConfigPersistence(t *testing.T) {
	db := setupAIGlobalConfigTestDB(t)
	source := `chat = func(req, emit) { return req.Prompt }`
	config := &ypb.AIGlobalConfig{IntelligentModels: []*ypb.AIModelConfig{{
		ModelName: "script-label", Provider: &ypb.ThirdPartyApplicationConfig{Type: "yak-script", Script: source, EnableNativeToolCalling: true},
	}}}
	_, err := SetAIGlobalConfig(db, config)
	require.NoError(t, err)
	saved, err := GetAIGlobalConfig(db)
	require.NoError(t, err)
	require.Equal(t, source, saved.IntelligentModels[0].Provider.Script)
	require.True(t, saved.IntelligentModels[0].Provider.EnableNativeToolCalling)
	require.Empty(t, saved.IntelligentModels[0].Provider.ExtraParams)
	require.Empty(t, saved.IntelligentModels[0].Provider.BaseURL, "ordinary provider defaults must not populate script configuration")
	require.False(t, migrateThirdPartyConfigBaseURL(saved.IntelligentModels[0].Provider))
	require.False(t, modelNeedsLegacyRecovery(saved.IntelligentModels[0]))
	require.NoError(t, ApplyAIGlobalConfig(db, saved))
	require.Equal(t, source, GetCachedAIGlobalConfig().IntelligentModels[0].Provider.Script)
	require.Equal(t, source, consts.GetTieredAIConfig().IntelligentConfigs[0].Provider.Script)
}
