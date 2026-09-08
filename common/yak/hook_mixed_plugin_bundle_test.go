package yak

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/pluginbundle"
)

func TestMixPluginCallerLoadPluginBundleUsesStructuredMember(t *testing.T) {
	root, memberPath := writeMixCallerPluginBundle(t)
	caller, err := NewMixPluginCaller()
	require.NoError(t, err)
	caller.SetLoadPluginTimeout(2)

	loaded, err := caller.LoadPluginBundle(root)
	require.NoError(t, err)
	require.Equal(t, 1, loaded)

	raw, err := os.ReadFile(memberPath)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(memberPath, append(raw, ' '), 0o600))
	_, err = caller.LoadPluginBundle(root)
	require.ErrorContains(t, err, "digest or size mismatch")
}

func writeMixCallerPluginBundle(t *testing.T) (string, string) {
	t.Helper()
	root := t.TempDir()
	memberRelativePath := "plugins/release-1/plugin.json"
	memberPath := filepath.Join(root, filepath.FromSlash(memberRelativePath))
	require.NoError(t, os.MkdirAll(filepath.Dir(memberPath), 0o700))
	content := "handle = func(result) { return }"
	contentSum := sha256.Sum256([]byte(content))
	memberRaw, err := json.Marshal(map[string]any{
		"schema_version": pluginbundle.MemberSchemaVersion,
		"plugin_id":      "plugin-1", "release_id": "release-1", "name": "bundle-probe",
		"type": "port-scan", "version": "1.0.0", "entry_kind": "yak_script",
		"content": content, "enabled": true, "status": "published",
		"script_content_sha256": hex.EncodeToString(contentSum[:]), "script_size_bytes": len(content),
	})
	require.NoError(t, err)
	memberSum := sha256.Sum256(memberRaw)
	manifestRaw, err := json.Marshal(pluginbundle.Manifest{
		SchemaVersion: pluginbundle.ManifestSchemaVersion,
		BundleID:      "bundle-1", Name: "Bundle 1", ItemCount: 1,
		Items: []pluginbundle.ManifestItem{{
			PluginID: "plugin-1", ReleaseID: "release-1", Name: "bundle-probe", Version: "1.0.0",
			EntryKind: "yak_script", Path: memberRelativePath,
			ContentSHA256: hex.EncodeToString(memberSum[:]), SizeBytes: int64(len(memberRaw)),
		}},
	})
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(root, pluginbundle.ManifestPath), manifestRaw, 0o600))
	require.NoError(t, os.WriteFile(memberPath, memberRaw, 0o600))
	return root, memberPath
}

func TestMixPluginCallerLoadPluginBundleRejectsUnknownDirectoryFile(t *testing.T) {
	root, _ := writeMixCallerPluginBundle(t)
	require.NoError(t, os.WriteFile(filepath.Join(root, "untrusted.yak"), []byte("println(1)"), 0o600))
	caller, err := NewMixPluginCaller()
	require.NoError(t, err)
	_, err = caller.LoadPluginBundle(root)
	require.Error(t, err)
	require.True(t, strings.Contains(err.Error(), "undeclared"), err.Error())
}
