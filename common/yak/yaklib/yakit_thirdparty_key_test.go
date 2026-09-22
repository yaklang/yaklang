package yaklib

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func isolateThirdPartyAppConfigs(t *testing.T) {
	t.Helper()
	original := consts.AllThirdPartyApplicationConfig()
	t.Cleanup(func() {
		consts.ClearThirdPartyApplicationConfig()
		for _, cfg := range original {
			consts.UpdateThirdPartyApplicationConfig(cfg)
		}
	})
	consts.ClearThirdPartyApplicationConfig()
}

func TestGetGitHubAPIKeyEmpty(t *testing.T) {
	isolateThirdPartyAppConfigs(t)
	t.Setenv("GITHUB_TOKEN", "")
	t.Setenv("GH_TOKEN", "")
	require.Equal(t, "", getGitHubAPIKey())
}

func TestGetGitHubAPIKeyEnvFallback(t *testing.T) {
	isolateThirdPartyAppConfigs(t)
	t.Setenv("GITHUB_TOKEN", "ghp_from_env_token")
	t.Setenv("GH_TOKEN", "should_not_use")
	require.Equal(t, "ghp_from_env_token", getGitHubAPIKey())
}

func TestGetGitHubAPIKeyGHTokenFallback(t *testing.T) {
	isolateThirdPartyAppConfigs(t)
	t.Setenv("GITHUB_TOKEN", "")
	t.Setenv("GH_TOKEN", "gh_from_env_token")
	require.Equal(t, "gh_from_env_token", getGitHubAPIKey())
}

func TestGetGitHubAPIKeyAppConfigWins(t *testing.T) {
	isolateThirdPartyAppConfigs(t)
	t.Setenv("GITHUB_TOKEN", "ghp_from_env_token")
	consts.UpdateThirdPartyApplicationConfig(&ypb.ThirdPartyApplicationConfig{
		Type:   "github",
		APIKey: "ghp_from_app_config",
	})
	require.Equal(t, "ghp_from_app_config", getGitHubAPIKey())
}

func TestGetGiteeAPIKeyEmpty(t *testing.T) {
	isolateThirdPartyAppConfigs(t)
	t.Setenv("GITEE_TOKEN", "")
	require.Equal(t, "", getGiteeAPIKey())
}

func TestGetGiteeAPIKeyEnvFallback(t *testing.T) {
	isolateThirdPartyAppConfigs(t)
	t.Setenv("GITEE_TOKEN", "gitee_from_env_token")
	require.Equal(t, "gitee_from_env_token", getGiteeAPIKey())
}

func TestGetGiteeAPIKeyAppConfigWins(t *testing.T) {
	isolateThirdPartyAppConfigs(t)
	t.Setenv("GITEE_TOKEN", "gitee_from_env_token")
	consts.UpdateThirdPartyApplicationConfig(&ypb.ThirdPartyApplicationConfig{
		Type:   "gitee",
		APIKey: "gitee_from_app_config",
	})
	require.Equal(t, "gitee_from_app_config", getGiteeAPIKey())
}
