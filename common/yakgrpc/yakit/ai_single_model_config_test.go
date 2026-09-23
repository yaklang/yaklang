package yakit

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func TestSingleModelSessionStartParamsRoundTrip(t *testing.T) {
	for _, local := range []bool{false, true} {
		original := &ypb.AIStartParams{SingleModelMode: local, UserQuery: "initial"}
		raw, err := marshalAISessionStartParams(original)
		require.NoError(t, err)
		cached, err := UnmarshalAISessionStartParams(raw)
		require.NoError(t, err)
		require.Equal(t, local, cached.GetSingleModelMode())
		merged := MergeCachedAISessionStartParams(cached, &ypb.AIStartParams{UserQuery: "reconnect"})
		require.Equal(t, local, merged.GetSingleModelMode())
		require.Equal(t, "reconnect", merged.GetUserQuery())
		require.Equal(t, "initial", cached.GetUserQuery())
		merged = MergeCachedAISessionStartParams(cached, &ypb.AIStartParams{SingleModelMode: true})
		require.True(t, merged.GetSingleModelMode())
		require.Equal(t, local, cached.GetSingleModelMode())
		overlaid := OverlayAISessionStartParams(cached, &ypb.AIStartParams{SingleModelMode: true})
		require.True(t, overlaid.GetSingleModelMode())
		require.Equal(t, local, cached.GetSingleModelMode())
	}
}

func TestSingleModelGlobalConfigRoundTrip(t *testing.T) {
	db := setupAIGlobalConfigTestDB(t)
	defer db.Close()
	previous, cached := consts.GetTieredAIConfig(), GetCachedAIGlobalConfig()
	t.Cleanup(func() { consts.SetTieredAIConfig(previous); setCachedAIGlobalConfig(cached) })
	old := &ypb.AIModelConfig{Provider: &ypb.ThirdPartyApplicationConfig{Type: "openai", APIKey: "old"}, ModelName: "old-model"}
	cfg := &ypb.AIGlobalConfig{SingleModelMode: true,
		IntelligentModels: []*ypb.AIModelConfig{{Provider: &ypb.ThirdPartyApplicationConfig{Type: "ollama", Domain: "localhost:11434", NoHttps: true}, ModelName: "local-model", IsOnline: true}},
		LightweightModels: []*ypb.AIModelConfig{old}, VisionModels: []*ypb.AIModelConfig{old}}
	_, err := SetAIGlobalConfig(db, cfg)
	require.NoError(t, err)
	loaded, err := GetAIGlobalConfig(db)
	require.NoError(t, err)
	require.True(t, loaded.GetSingleModelMode())
	require.Equal(t, "local-model", loaded.GetIntelligentModels()[0].GetModelName())
	require.NoError(t, ApplyAIGlobalConfig(db, loaded))
	require.True(t, GetCachedAIGlobalConfig().GetIntelligentModels()[0].GetIsOnline())
	require.True(t, consts.IsSingleAIModelMode())
	require.Equal(t, "local-model", consts.GetIntelligentAIConfigs()[0].ModelName)
	require.Equal(t, "old-model", consts.GetLightweightAIConfigs()[0].ModelName)
	providers, err := ListAIProviders(db)
	require.NoError(t, err)
	require.Len(t, providers, 2)
	loaded.SingleModelMode = false
	_, err = SetAIGlobalConfig(db, loaded)
	require.NoError(t, err)
	require.NoError(t, ApplyAIGlobalConfig(db, loaded))
	models := consts.GetLightweightAIConfigs()
	require.Equal(t, "old-model", models[0].ModelName)
	require.Equal(t, "local-model", GetCachedAIGlobalConfig().GetIntelligentModels()[0].GetModelName(), "keep the quality model when switched off")
	for _, bad := range []*ypb.AIModelConfig{nil, {}, {Provider: &ypb.ThirdPartyApplicationConfig{Type: "openai"}}} {
		_, err = SetAIGlobalConfig(db, &ypb.AIGlobalConfig{SingleModelMode: true, IntelligentModels: []*ypb.AIModelConfig{bad}})
		require.ErrorIs(t, err, consts.ErrInvalidSingleAIModel)
		require.Error(t, ApplyAIGlobalConfig(db, &ypb.AIGlobalConfig{SingleModelMode: true, IntelligentModels: []*ypb.AIModelConfig{bad}}))
		require.False(t, consts.IsSingleAIModelMode(), "invalid updates must not replace the active config")
	}
}
