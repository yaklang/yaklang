package main

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestExtensionOriginChrome(t *testing.T) {
	require.Equal(t, "chrome-extension://abcdefghijklmnopabcdefghijklmnop", extensionOrigin([]string{
		"chrome-extension://abcdefghijklmnopabcdefghijklmnop/", "1234",
	}))
}

func TestExtensionOriginFirefox(t *testing.T) {
	extensionID := "browser-agent@yaklang.com"
	manifestPath := filepath.Join(t.TempDir(), "com.yaklang.browser_agent.json")
	require.NoError(t, os.WriteFile(manifestPath, []byte(`{"allowed_extensions":["browser-agent@yaklang.com"]}`), 0o600))
	digest := sha256.Sum256([]byte(extensionID))
	require.Equal(t, "moz-extension://"+hex.EncodeToString(digest[:]), extensionOrigin([]string{manifestPath, extensionID}))
	require.Empty(t, extensionOrigin([]string{manifestPath, "attacker@example.com"}))
}

func TestExtensionOriginRejectsMalformedInput(t *testing.T) {
	require.Empty(t, extensionOrigin([]string{"chrome-extension://trusted@attacker.example"}))
	require.Empty(t, extensionOrigin([]string{"/missing/native-host.json", "browser-agent@yaklang.com"}))
}
