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
