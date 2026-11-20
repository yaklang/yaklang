package yakgrpc

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"google.golang.org/grpc/codes"
	grpcstatus "google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
)

func TestAIModelContextConfiguredIdentityAndBounds(t *testing.T) {
	client, server, err := NewLocalClientAndServerWithTempDatabase(t)
	require.NoError(t, err)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	config := &ypb.AIModelConfig{ModelName: "same-name", Provider: &ypb.ThirdPartyApplicationConfig{Type: "openai", BaseURL: "https://a.example/v1", APIKey: "test-only"}}
	_, err = yakit.SetAIGlobalConfig(server.GetProfileDatabase(), &ypb.AIGlobalConfig{IntelligentModels: []*ypb.AIModelConfig{config}})
	require.NoError(t, err)
	initial, err := client.GetAIModelContext(ctx, &ypb.GetAIModelContextRequest{Config: config})
	require.NoError(t, err)
	require.EqualValues(t, 500000, initial.ThresholdTokens)
	require.EqualValues(t, 100000, initial.MinimumThresholdTokens)
	require.NotEmpty(t, initial.ContextKey)
	identity := &ypb.GetAIModelContextRequest{Provider: initial.Provider, Model: initial.Model, ContextKey: initial.ContextKey}
	for _, tokens := range []int64{-1, 0, 99999, 1000001} {
		_, err := client.SetAIModelContext(ctx, &ypb.SetAIModelContextRequest{Config: config, ThresholdTokens: tokens})
		require.Equal(t, codes.InvalidArgument, grpcstatus.Code(err))
		unchanged, err := client.GetAIModelContext(ctx, identity)
		require.NoError(t, err)
		require.True(t, proto.Equal(initial, unchanged))
	}
	for _, tokens := range []int64{100000, 100001, 102400, 600000, 1000000} {
		saved, err := client.SetAIModelContext(ctx, &ypb.SetAIModelContextRequest{Provider: initial.Provider, Model: initial.Model, ContextKey: initial.ContextKey, ThresholdTokens: tokens})
		require.NoError(t, err)
		require.EqualValues(t, tokens, saved.ThresholdTokens)
		readByConfig, err := client.GetAIModelContext(ctx, &ypb.GetAIModelContextRequest{Config: config})
		require.NoError(t, err)
		require.True(t, proto.Equal(saved, readByConfig))
	}
	for _, field := range []string{"address", "account", "api", "model"} {
		other := proto.Clone(config).(*ypb.AIModelConfig)
		switch field {
		case "address":
			other.Provider.BaseURL = "https://b.example/v1"
		case "account":
			other.Provider.APIKey = "another-account"
		case "api":
			other.Provider.APIType = "responses"
		case "model":
			other.ModelName = "another-model"
		}
		value, err := client.GetAIModelContext(ctx, &ypb.GetAIModelContextRequest{Config: other})
		require.NoError(t, err)
		require.NotEqual(t, initial.ContextKey, value.ContextKey)
		require.EqualValues(t, 500000, value.ThresholdTokens, field)
	}
	disabled := true
	saved, err := client.SetAIModelContext(ctx, &ypb.SetAIModelContextRequest{Config: config, DisableAutoAdjust: &disabled})
	require.NoError(t, err)
	require.True(t, saved.DisableAutoAdjust)
	require.EqualValues(t, 1000000, saved.ThresholdTokens)
	for _, request := range []*ypb.GetAIModelContextRequest{{}, {Provider: "openai", Model: "same-name"}, {Provider: "openai", Model: "same-name", ContextKey: "v2:invalid"}} {
		_, err := client.GetAIModelContext(ctx, request)
		require.Equal(t, codes.InvalidArgument, grpcstatus.Code(err))
	}
	_, err = yakit.SetAIGlobalConfig(server.GetProfileDatabase(), &ypb.AIGlobalConfig{})
	require.NoError(t, err)
	_, err = client.GetAIModelContext(ctx, identity)
	require.Equal(t, codes.NotFound, grpcstatus.Code(err))
	_, err = client.SetAIModelContext(ctx, &ypb.SetAIModelContextRequest{Config: config, ThresholdTokens: 200000})
	require.Equal(t, codes.NotFound, grpcstatus.Code(err))

}
