package aiconfig

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func saveAndRestore(t *testing.T) {
	t.Helper()
	orig := consts.GetTieredAIConfig()
	t.Cleanup(func() {
		consts.SetTieredAIConfig(orig)
		ResetConfigLoaded()
		ResetNetworkConfigGetter()
		ResetAIGlobalConfigGetter()
	})
}

func setupTempYakitHome(t *testing.T) string {
	t.Helper()
	tmpDir := t.TempDir()
	t.Setenv("YAKIT_HOME", tmpDir)
	baseDir := filepath.Join(tmpDir, "base")
	require.NoError(t, os.MkdirAll(baseDir, 0o755))
	return tmpDir
}

func writeConfigFile(t *testing.T, yakitHome string, content string) string {
	t.Helper()
	configPath := filepath.Join(yakitHome, "base", "tiered-ai-config.yaml")
	require.NoError(t, os.WriteFile(configPath, []byte(content), 0o644))
	return configPath
}

func TestIsConfigLoaded(t *testing.T) {
	saveAndRestore(t)
	ResetConfigLoaded()
	assert.False(t, IsConfigLoaded())
}

func TestResetConfigLoaded(t *testing.T) {
	saveAndRestore(t)
	consts.SetTieredAIConfig(&consts.TieredAIConfig{Enabled: true})
	ResetConfigLoaded()
	assert.False(t, IsConfigLoaded())
}

func TestLoadTieredConfigFromNetworkConfig_Enabled(t *testing.T) {
	saveAndRestore(t)
	consts.SetTieredAIConfig(nil)
	ResetConfigLoaded()

	networkConfig := &ypb.GlobalNetworkConfig{
		EnableTieredAIModelConfig: true,
		TieredAIModelConfig: &ypb.TieredAIModelConfigDescriptor{
			ModelRoutingPolicy:                "performance",
			DisableFallbackToLightweightModel: true,
		},
		IntelligentAIModelConfig: []*ypb.ThirdPartyApplicationConfig{
			{Type: "aibalance", APIKey: "intelligent-key"},
		},
		LightweightAIModelConfig: []*ypb.ThirdPartyApplicationConfig{
			{Type: "aibalance", APIKey: "lightweight-key"},
		},
		VisionAIModelConfig: []*ypb.ThirdPartyApplicationConfig{
			{Type: "aibalance", APIKey: "vision-key"},
		},
	}

	loadTieredConfigFromNetworkConfig(networkConfig)

	cfg := consts.GetTieredAIConfig()
	require.NotNil(t, cfg)
	assert.True(t, cfg.Enabled)
	assert.Equal(t, consts.PolicyPerformance, cfg.RoutingPolicy)
	assert.True(t, cfg.DisableFallback)
	assert.Len(t, cfg.IntelligentConfigs, 1)
	assert.Len(t, cfg.LightweightConfigs, 1)
	assert.Len(t, cfg.VisionConfigs, 1)
	assert.True(t, IsConfigLoaded())
}

func TestLoadTieredConfigFromNetworkConfig_Disabled(t *testing.T) {
	saveAndRestore(t)
	consts.SetTieredAIConfig(nil)
	ResetConfigLoaded()

	networkConfig := &ypb.GlobalNetworkConfig{
		EnableTieredAIModelConfig: false,
		TieredAIModelConfig: &ypb.TieredAIModelConfigDescriptor{
			ModelRoutingPolicy: "balance",
		},
	}

	loadTieredConfigFromNetworkConfig(networkConfig)

	cfg := consts.GetTieredAIConfig()
	require.NotNil(t, cfg)
	assert.False(t, cfg.Enabled, "DB says disabled, must be respected")
	assert.True(t, IsConfigLoaded())
	assert.False(t, consts.IsTieredAIModelConfigEnabled())
}

func TestLoadTieredConfigFromNetworkConfig_EmptyPolicy(t *testing.T) {
	saveAndRestore(t)
	consts.SetTieredAIConfig(nil)
	ResetConfigLoaded()

	networkConfig := &ypb.GlobalNetworkConfig{
		EnableTieredAIModelConfig: true,
		IntelligentAIModelConfig: []*ypb.ThirdPartyApplicationConfig{
			{Type: "aibalance", APIKey: "test"},
		},
	}

	loadTieredConfigFromNetworkConfig(networkConfig)

	cfg := consts.GetTieredAIConfig()
	require.NotNil(t, cfg)
	assert.Equal(t, consts.PolicyBalance, cfg.RoutingPolicy)
}

func TestLoadTieredConfigFromNetworkConfig_NilConfig(t *testing.T) {
	saveAndRestore(t)
	consts.SetTieredAIConfig(nil)
	ResetConfigLoaded()

	loadTieredConfigFromNetworkConfig(nil)
	assert.Nil(t, consts.GetTieredAIConfig())
}

// DB returns enabled config with performance policy.
func TestEnsureConfigLoaded_DBEnabled(t *testing.T) {
	saveAndRestore(t)
	setupTempYakitHome(t)

	SetNetworkConfigGetter(func() *ypb.GlobalNetworkConfig {
		return &ypb.GlobalNetworkConfig{
			EnableTieredAIModelConfig: true,
			TieredAIModelConfig: &ypb.TieredAIModelConfigDescriptor{
				ModelRoutingPolicy: "performance",
			},
			IntelligentAIModelConfig: []*ypb.ThirdPartyApplicationConfig{
				{Type: "aibalance", APIKey: "db-key"},
			},
		}
	})
	consts.SetTieredAIConfig(nil)
	ResetConfigLoaded()

	EnsureConfigLoaded()

	cfg := consts.GetTieredAIConfig()
	require.NotNil(t, cfg)
	assert.True(t, cfg.Enabled)
	assert.Equal(t, consts.PolicyPerformance, cfg.RoutingPolicy)
	assert.True(t, IsConfigLoaded())
	assert.True(t, consts.IsTieredAIModelConfigEnabled())
}

// DB returns disabled config. Must be respected -- defaults must NOT override.
func TestEnsureConfigLoaded_DBDisabled(t *testing.T) {
	saveAndRestore(t)
	setupTempYakitHome(t)

	SetNetworkConfigGetter(func() *ypb.GlobalNetworkConfig {
		return &ypb.GlobalNetworkConfig{
			EnableTieredAIModelConfig: false,
			TieredAIModelConfig: &ypb.TieredAIModelConfigDescriptor{
				ModelRoutingPolicy: "balance",
			},
		}
	})
	consts.SetTieredAIConfig(nil)
	ResetConfigLoaded()

	EnsureConfigLoaded()

	cfg := consts.GetTieredAIConfig()
	require.NotNil(t, cfg)
	assert.False(t, cfg.Enabled, "DB disabled config must NOT be overridden by defaults")
	assert.True(t, IsConfigLoaded())
	assert.False(t, consts.IsTieredAIModelConfigEnabled())
}

// No DB config, no in-memory config -> built-in defaults should be loaded.
func TestEnsureConfigLoaded_NoConfig_LoadsDefaults(t *testing.T) {
	saveAndRestore(t)
	setupTempYakitHome(t)

	SetNetworkConfigGetter(func() *ypb.GlobalNetworkConfig { return nil })
	consts.SetTieredAIConfig(nil)
	ResetConfigLoaded()

	EnsureConfigLoaded()

	cfg := consts.GetTieredAIConfig()
	require.NotNil(t, cfg)
	assert.True(t, cfg.Enabled, "built-in defaults should have enabled: true")
	assert.True(t, IsConfigLoaded())
	assert.True(t, consts.IsTieredAIModelConfigEnabled())
	assert.NotEmpty(t, cfg.IntelligentConfigs)
	assert.NotEmpty(t, cfg.LightweightConfigs)
	assert.NotEmpty(t, cfg.VisionConfigs)
}

// Config file on disk must be IGNORED by EnsureConfigLoaded.
// Even when DB says disabled, a file saying enabled must not take effect.
func TestEnsureConfigLoaded_ConfigFileIgnored(t *testing.T) {
	saveAndRestore(t)
	yakitHome := setupTempYakitHome(t)
	writeConfigFile(t, yakitHome, `
enabled: true
routing_policy: performance
intelligent_configs:
  - type: aibalance
    api_key: file-key
    domain: file.example.com
    model: file-model
`)

	SetNetworkConfigGetter(func() *ypb.GlobalNetworkConfig {
		return &ypb.GlobalNetworkConfig{
			EnableTieredAIModelConfig: false,
			TieredAIModelConfig: &ypb.TieredAIModelConfigDescriptor{
				ModelRoutingPolicy: "balance",
			},
		}
	})
	consts.SetTieredAIConfig(nil)
	ResetConfigLoaded()

	EnsureConfigLoaded()

	cfg := consts.GetTieredAIConfig()
	require.NotNil(t, cfg)
	assert.False(t, cfg.Enabled, "config file must NOT override DB config")
	assert.Equal(t, consts.PolicyBalance, cfg.RoutingPolicy, "policy from DB, not from file")
	assert.True(t, IsConfigLoaded())
}

// In-memory config already loaded (e.g. by ConfigureNetWork during DB init).
// EnsureConfigLoaded must not clobber it.
func TestEnsureConfigLoaded_AlreadyLoaded(t *testing.T) {
	saveAndRestore(t)

	SetNetworkConfigGetter(func() *ypb.GlobalNetworkConfig { return nil })
	consts.SetTieredAIConfig(&consts.TieredAIConfig{
		Enabled:       true,
		RoutingPolicy: consts.PolicyPerformance,
	})
	ResetConfigLoaded()

	EnsureConfigLoaded()

	cfg := consts.GetTieredAIConfig()
	require.NotNil(t, cfg)
	assert.True(t, cfg.Enabled)
	assert.Equal(t, consts.PolicyPerformance, cfg.RoutingPolicy)
	assert.True(t, IsConfigLoaded())
}

// AIGlobalConfig should take priority over GlobalNetworkConfig when present.
func TestEnsureConfigLoaded_AIGlobalConfigPriority(t *testing.T) {
	saveAndRestore(t)
	setupTempYakitHome(t)

	SetAIGlobalConfigGetter(func() (*ypb.AIGlobalConfig, error) {
		return &ypb.AIGlobalConfig{
			Enabled:         true,
			RoutingPolicy:   "cost",
			DisableFallback: true,
			DefaultModelId:  "default-model",
			GlobalWeight:    0.33,
		}, nil
	})
	SetNetworkConfigGetter(func() *ypb.GlobalNetworkConfig {
		return &ypb.GlobalNetworkConfig{
			EnableTieredAIModelConfig: false,
			TieredAIModelConfig: &ypb.TieredAIModelConfigDescriptor{
				ModelRoutingPolicy: "balance",
			},
		}
	})

	consts.SetTieredAIConfig(nil)
	ResetConfigLoaded()

	EnsureConfigLoaded()

	cfg := consts.GetTieredAIConfig()
	require.NotNil(t, cfg)
	assert.True(t, cfg.Enabled)
	assert.True(t, cfg.DisableFallback)
	assert.Equal(t, consts.PolicyCost, cfg.RoutingPolicy)
	assert.Equal(t, "default-model", cfg.DefaultModelID)
	assert.Equal(t, 0.33, cfg.GlobalWeight)
	assert.True(t, IsConfigLoaded())
}

// If AIGlobalConfig getter errors, fallback to GlobalNetworkConfig.
func TestEnsureConfigLoaded_AIGlobalConfigErrorFallback(t *testing.T) {
	saveAndRestore(t)
	setupTempYakitHome(t)

	SetAIGlobalConfigGetter(func() (*ypb.AIGlobalConfig, error) {
		return nil, errors.New("boom")
	})
	SetNetworkConfigGetter(func() *ypb.GlobalNetworkConfig {
		return &ypb.GlobalNetworkConfig{
			EnableTieredAIModelConfig: true,
			TieredAIModelConfig: &ypb.TieredAIModelConfigDescriptor{
				ModelRoutingPolicy: "performance",
			},
		}
	})

	consts.SetTieredAIConfig(nil)
	ResetConfigLoaded()

	EnsureConfigLoaded()

	cfg := consts.GetTieredAIConfig()
	require.NotNil(t, cfg)
	assert.True(t, cfg.Enabled)
	assert.Equal(t, consts.PolicyPerformance, cfg.RoutingPolicy)
	assert.True(t, IsConfigLoaded())
}
