package yakgrpc

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func TestCleanBootstrapConfigurationPassesGuard(t *testing.T) {
	var skipped bool
	t.Run("clean isolated bootstrap", func(st *testing.T) {
		defer func() { skipped = st.Skipped() }()
		yakit.GuardConfigurationForTest(st)
	})
	require.False(t, skipped, "clean configuration tests must execute, rather than all being silently skipped")
}

func TestConfigurationGuardSkipsBeforeChangingExistingAPIKeys(t *testing.T) {
	yakit.GuardConfigurationForTest(t)
	db := consts.GetGormProfileDatabase()
	// This credential is deliberately synthetic and is seeded only after the
	// outer precondition has proved that the shared database is disposable.
	const private = `{"Enabled":true,"IntelligentModels":[{"Provider":{"Type":"openai","APIKey":"private-test-sentinel"},"ModelName":"test-model"}]}`
	require.NoError(t, yakit.SetKey(db, consts.AI_GLOBAL_CONFIG_KEY, private))
	var skipped bool
	t.Run("pre-existing settings", func(st *testing.T) {
		defer func() { skipped = st.Skipped() }()
		yakit.GuardConfigurationForTest(st)
		st.Fatal("configuration writer should have been skipped")
	})
	require.True(t, skipped)
	require.True(t, yakit.GetKey(db, consts.AI_GLOBAL_CONFIG_KEY) == private, "skipping must preserve the original API key")
}

func TestConfigurationGuardSkipsPrivateRuntimeCache(t *testing.T) {
	yakit.GuardConfigurationForTest(t)
	private := &ypb.AIGlobalConfig{IntelligentModels: []*ypb.AIModelConfig{{
		Provider: &ypb.ThirdPartyApplicationConfig{Type: "openai", APIKey: "private-test-sentinel"}, ModelName: "test-model",
	}}}
	yakit.SetCachedAIGlobalConfigForTest(private)
	var skipped bool
	t.Run("pre-existing runtime key", func(st *testing.T) {
		defer func() { skipped = st.Skipped() }()
		yakit.GuardConfigurationForTest(st)
		st.Fatal("configuration writer should have been skipped")
	})
	require.True(t, skipped)
	require.True(t, yakit.GetCachedAIGlobalConfig().GetIntelligentModels()[0].GetProvider().GetAPIKey() == "private-test-sentinel")
}
