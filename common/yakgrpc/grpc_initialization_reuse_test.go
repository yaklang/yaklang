package yakgrpc

import (
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils/yakitenv"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
)

func TestLocalClientInitializationReusesDatabasesAndBuiltinRecords(t *testing.T) {
	yakit.GuardConfigurationForTest(t)
	if info, err := os.Stat(yakitenv.TestRoot()); err == nil {
		t.Logf("elapsed since private test root creation (includes package cold initialization): %s", time.Since(info.ModTime()))
	}
	client, err := NewLocalClient()
	require.NoError(t, err)
	profile, project := consts.GetGormProfileDatabase(), consts.GetGormProjectDatabase()
	counts := func() []int64 {
		var result []int64
		for _, model := range []any{&schema.YakScript{}, &schema.AIYakTool{}, &schema.Payload{}, &schema.PluginGroup{}} {
			var count int64
			require.NoError(t, profile.Model(model).Count(&count).Error)
			result = append(result, count)
		}
		return result
	}
	before := counts()
	for _, count := range before {
		require.Positive(t, count, "one initialization pass must finish importing every builtin category")
	}
	start := time.Now()
	for i := 0; i < 100; i++ {
		reused, err := NewLocalClient()
		require.NoError(t, err)
		require.Same(t, client, reused)
		require.Same(t, profile, consts.GetGormProfileDatabase())
		require.Same(t, project, consts.GetGormProjectDatabase())
	}
	t.Logf("100 shared client/database lookups: %s", time.Since(start))
	for i := 0; i < 2; i++ {
		start := time.Now()
		independent, err := NewLocalClientForceNew()
		require.NoError(t, err)
		require.Same(t, profile, independent.(*Client).GetProfileDatabase())
		require.Same(t, project, independent.(*Client).GetProjectDatabase())
		require.NoError(t, independent.(*Client).Close())
		require.Equal(t, before, counts(), "new local transports must not duplicate builtin records")
		t.Logf("independent transport using existing databases: %s", time.Since(start))
	}
	for i := 0; i < 2; i++ {
		start := time.Now()
		require.NoError(t, yakit.CallPostInitDatabase())
		require.Equal(t, before, counts(), "replaying post-init hooks must not append duplicate builtin records")
		t.Logf("explicit post-init replay %d: %s (builtin counts %v)", i+1, time.Since(start), before)
	}
}
