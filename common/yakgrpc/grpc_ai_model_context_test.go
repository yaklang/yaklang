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
)

func TestAIModelContextBeforeSession(t *testing.T) {
	client, server, err := NewLocalClientAndServerWithTempDatabase(t)
	require.NoError(t, err)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	identity := &ypb.GetAIModelContextRequest{Provider: "test", Model: "same-name"}
	initial, err := client.GetAIModelContext(ctx, identity)
	require.NoError(t, err)
	require.EqualValues(t, 500*1000, initial.ContextTokens)
	require.EqualValues(t, 500*1000, initial.ThresholdTokens)
	require.EqualValues(t, 100*1000, initial.MinimumThresholdTokens)
	for _, tokens := range []int64{-1, 0, 100*1000 - 1, 1000*1000 + 1} {
		_, err := client.SetAIModelContext(ctx, &ypb.SetAIModelContextRequest{Provider: identity.Provider, Model: identity.Model, ThresholdTokens: tokens})
		require.Equal(t, codes.InvalidArgument, grpcstatus.Code(err))
		unchanged, err := client.GetAIModelContext(ctx, identity)
		require.NoError(t, err)
		require.Equal(t, initial.String(), unchanged.String())
	}
	for _, tokens := range []int64{100 * 1000, 100001, 102400, 600 * 1000, 1000 * 1000} {
		saved, err := client.SetAIModelContext(ctx, &ypb.SetAIModelContextRequest{Provider: identity.Provider, Model: identity.Model, ThresholdTokens: tokens})
		require.NoError(t, err)
		require.EqualValues(t, tokens, saved.ThresholdTokens)
		require.EqualValues(t, 500*1000, saved.ContextTokens, "a manual threshold does not change capacity")
		require.Equal(t, "manual", saved.Source)
		require.Equal(t, tokens, yakit.GetAIModelContext(server.GetProfileDatabase(), identity.Provider, identity.Model).ThresholdTokens)
	}
	for _, other := range []*ypb.GetAIModelContextRequest{{Provider: "other", Model: identity.Model}, {Provider: identity.Provider, Model: "other"}} {
		value, err := client.GetAIModelContext(ctx, other)
		require.NoError(t, err)
		require.EqualValues(t, 500*1000, value.ThresholdTokens)
	}
	require.NoError(t, yakit.SaveAIModelContext(server.GetProfileDatabase(), "small", "small", yakit.AIModelContext{
		ContextTokens: 64 * 1000, ThresholdTokens: 51 * 1000, Source: "learned",
	}))
	small, err := client.GetAIModelContext(ctx, &ypb.GetAIModelContextRequest{Provider: "small", Model: "small"})
	require.NoError(t, err)
	require.True(t, small.TooSmall)
	require.EqualValues(t, 51*1000, small.ThresholdTokens, "learned budgets remain visible before a new session")
	for _, capacity := range []int64{102399, 100000, 99999} {
		require.NoError(t, yakit.SaveAIModelContext(server.GetProfileDatabase(), "small", "decimal-boundary", yakit.AIModelContext{
			ContextTokens: capacity, ThresholdTokens: capacity * 8 / 10, Source: "learned",
		}))
		value, err := client.GetAIModelContext(ctx, &ypb.GetAIModelContextRequest{Provider: "small", Model: "decimal-boundary"})
		require.NoError(t, err)
		require.Equal(t, capacity < 100000, value.TooSmall, "the small-context warning uses the decimal 100K boundary")
		require.Equal(t, capacity, value.ContextTokens, "provider evidence remains an exact token count")
	}
	for _, invalid := range []*ypb.GetAIModelContextRequest{{}, {Provider: "test", Model: " "}, {Model: "test"}} {
		_, err := client.GetAIModelContext(ctx, invalid)
		require.Equal(t, codes.InvalidArgument, grpcstatus.Code(err))
		_, err = client.SetAIModelContext(ctx, &ypb.SetAIModelContextRequest{Provider: invalid.Provider, Model: invalid.Model, ThresholdTokens: 200 * 1000})
		require.Equal(t, codes.InvalidArgument, grpcstatus.Code(err))
	}
}

func TestAIModelContextAutoAdjustSwitchBeforeSession(t *testing.T) {
	client, _, err := NewLocalClientAndServerWithTempDatabase(t)
	require.NoError(t, err)
	ctx := context.Background()
	disabled := true
	request := &ypb.SetAIModelContextRequest{Provider: "test", Model: "switch", DisableAutoAdjust: &disabled}
	saved, err := client.SetAIModelContext(ctx, request)
	require.NoError(t, err)
	require.True(t, saved.DisableAutoAdjust)
	require.EqualValues(t, 500*1000, saved.ThresholdTokens)
	saved, err = client.SetAIModelContext(ctx, &ypb.SetAIModelContextRequest{Provider: "test", Model: "switch", ThresholdTokens: 600 * 1000})
	require.NoError(t, err)
	require.True(t, saved.DisableAutoAdjust)
	disabled = false
	saved, err = client.SetAIModelContext(ctx, request)
	require.NoError(t, err)
	require.False(t, saved.DisableAutoAdjust)
	require.EqualValues(t, 600*1000, saved.ThresholdTokens)
}
