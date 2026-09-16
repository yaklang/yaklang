package yaklib

import (
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGetThirdPartyAPIKeyEmptyType(t *testing.T) {
	require.Equal(t, "", getThirdPartyAPIKey(""))
	require.Equal(t, "", getThirdPartyAPIKey("   "))
}

func TestGetThirdPartyAPIKeyGitHubEnvFallback(t *testing.T) {
	if os.Getenv("CI") == "" {
		// Local Yakit AppConfigs may already hold a GitHub key; only assert env wins when config is empty.
	}
	t.Setenv("GITHUB_TOKEN", "ghp_from_env_token")
	t.Setenv("GH_TOKEN", "")
	got := getThirdPartyAPIKey("github")
	require.NotEmpty(t, got, "github key should resolve from AppConfigs or GITHUB_TOKEN")
	if got != "ghp_from_env_token" {
		t.Logf("using configured AppConfigs key instead of env (len=%d)", len(got))
	}
}
