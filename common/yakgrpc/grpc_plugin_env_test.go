package yakgrpc

import (
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"testing"
	"time"
)

func TestGRPC_PluginEnv(t *testing.T) {
	client, err := NewLocalClient()
	require.NoError(t, err)
	ctx := utils.TimeoutContext(10 * time.Second)

	t.Run("test set env", func(t *testing.T) {
		tokenKey := utils.RandStringBytes(10)
		tokenValue1 := utils.RandStringBytes(10)
		_, err := client.SetPluginEnv(ctx, &ypb.PluginEnvRequest{Value: tokenValue1, Key: tokenKey})
		require.NoError(t, err)
		defer yakit.DeletePluginEnvByKey(consts.GetGormProfileDatabase(), tokenKey)

		actualValue, err := yakit.GetPluginEnvByKey(consts.GetGormProfileDatabase(), tokenKey)
		require.NoError(t, err)
		require.Equal(t, tokenValue1, actualValue)

		tokenValue2 := utils.RandStringBytes(10)
		_, err = client.SetPluginEnv(ctx, &ypb.PluginEnvRequest{Value: tokenValue2, Key: tokenKey})
		require.NoError(t, err)

		actualValue, err = yakit.GetPluginEnvByKey(consts.GetGormProfileDatabase(), tokenKey)
		require.NoError(t, err)
		require.Equal(t, tokenValue2, actualValue)

	})

	t.Run("test get all env", func(t *testing.T) {
		tokenKey1 := utils.RandStringBytes(10)
		tokenKey2 := utils.RandStringBytes(10)
		tokenValue := utils.RandStringBytes(10)
		db := consts.GetGormProfileDatabase()

		err = yakit.CreatePluginEnv(db, tokenKey1, tokenValue)
		require.NoError(t, err)
		defer yakit.DeletePluginEnvByKey(db, tokenKey1)

		err = yakit.CreatePluginEnv(db, tokenKey2, tokenValue)
		require.NoError(t, err)
		defer yakit.DeletePluginEnvByKey(db, tokenKey2)

		env, err := client.GetAllPluginEnv(ctx, &ypb.Empty{})
		require.NoError(t, err)
		require.Greater(t, len(env.Env), 2)

		var check1, check2 bool
		for _, e := range env.GetEnv() {
			if e.Key == tokenKey1 {
				check1 = true
			}
			if e.Key == tokenKey2 {
				check2 = true
			}
		}
		require.True(t, check1 && check2)

	})

	t.Run("test delete env", func(t *testing.T) {
		tokenKey := utils.RandStringBytes(10)
		tokenValue := utils.RandStringBytes(10)
		err := yakit.CreatePluginEnv(consts.GetGormProfileDatabase(), tokenKey, tokenValue)
		require.NoError(t, err)

		_, err = client.DeletePluginEnv(ctx, &ypb.DeletePluginEnvRequest{Key: tokenKey})
		require.NoError(t, err)

		_, err = yakit.GetPluginEnvByKey(consts.GetGormProfileDatabase(), tokenKey)
		require.Error(t, err)
	})
}
