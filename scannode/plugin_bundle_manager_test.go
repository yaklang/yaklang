package scannode

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	commonbundle "github.com/yaklang/yaklang/common/pluginbundle"
)

func TestPluginBundleManagerInstallAuthenticatesAndCaches(t *testing.T) {
	archive := pluginBundleManagerTestArchive(t)
	digest := sha256.Sum256(archive)
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		requests++
		if request.Header.Get("Authorization") != "Bearer session-token" {
			t.Errorf("authorization = %q", request.Header.Get("Authorization"))
		}
		if request.URL.Query().Get("node_session_id") != "session-1" {
			t.Errorf("node_session_id = %q", request.URL.Query().Get("node_session_id"))
		}
		writer.WriteHeader(http.StatusOK)
		_, _ = writer.Write(archive)
	}))
	defer server.Close()
	manager, err := NewPluginBundleManager(PluginBundleManagerConfig{BaseDir: t.TempDir(), Client: server.Client()})
	if err != nil {
		t.Fatal(err)
	}
	input := PluginBundleInstallInput{
		BundleID:      "bundle-1",
		ArtifactURL:   server.URL + "/v1/plugin-bundles/bundle-1/artifact?node_session_id=session-1",
		SHA256:        hex.EncodeToString(digest[:]),
		SizeBytes:     int64(len(archive)),
		ItemCount:     1,
		SchemaVersion: commonbundle.ManifestSchemaVersion,
		NodeSessionID: "session-1",
		SessionToken:  "session-token",
	}
	installed, err := manager.Install(context.Background(), input)
	if err != nil {
		t.Fatalf("Install() error = %v", err)
	}
	if _, err := os.Stat(installed); err != nil {
		t.Fatalf("installed path: %v", err)
	}
	second, err := manager.Install(context.Background(), input)
	if err != nil {
		t.Fatalf("cached Install() error = %v", err)
	}
	if second != installed || requests != 1 {
		t.Fatalf("cache path=%q requests=%d", second, requests)
	}
}

func TestPluginBundleManagerInstallRejectsDigestMismatch(t *testing.T) {
	archive := pluginBundleManagerTestArchive(t)
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		_, _ = writer.Write(archive)
	}))
	defer server.Close()
	manager, err := NewPluginBundleManager(PluginBundleManagerConfig{BaseDir: t.TempDir(), Client: server.Client()})
	if err != nil {
		t.Fatal(err)
	}
	_, err = manager.Install(context.Background(), PluginBundleInstallInput{
		BundleID: "bundle-1", ArtifactURL: server.URL + "?node_session_id=session-1", SHA256: strings.Repeat("ab", 32),
		SizeBytes: int64(len(archive)), ItemCount: 1, SchemaVersion: commonbundle.ManifestSchemaVersion,
		NodeSessionID: "session-1", SessionToken: "session-token",
	})
	if err == nil || !strings.Contains(err.Error(), "sha256 mismatch") {
		t.Fatalf("error = %v, want digest mismatch", err)
	}
}

func pluginBundleManagerTestArchive(t *testing.T) []byte {
	t.Helper()
	content := "handle = func(result) { return }"
	contentSum := sha256.Sum256([]byte(content))
	memberRaw, err := json.Marshal(map[string]any{
		"schema_version": commonbundle.MemberSchemaVersion,
		"plugin_id":      "plugin-1", "release_id": "release-1", "name": "probe-a",
		"type": "port-scan", "version": "1.0.0", "entry_kind": "yak_script",
		"content": content, "enabled": true, "status": "published",
		"script_content_sha256": hex.EncodeToString(contentSum[:]), "script_size_bytes": len(content),
	})
	if err != nil {
		t.Fatal(err)
	}
	memberSum := sha256.Sum256(memberRaw)
	manifestRaw, err := json.Marshal(commonbundle.Manifest{
		SchemaVersion: commonbundle.ManifestSchemaVersion,
		BundleID:      "bundle-1", Name: "Bundle 1", ItemCount: 1,
		Items: []commonbundle.ManifestItem{{
			PluginID: "plugin-1", ReleaseID: "release-1", Name: "probe-a", Version: "1.0.0",
			EntryKind: "yak_script", Path: "plugins/release-1/plugin.json",
			ContentSHA256: hex.EncodeToString(memberSum[:]), SizeBytes: int64(len(memberRaw)),
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)
	for _, file := range []struct {
		name string
		raw  []byte
	}{
		{commonbundle.ManifestPath, manifestRaw}, {"plugins/release-1/plugin.json", memberRaw},
	} {
		header := &zip.FileHeader{Name: file.name, Method: zip.Store}
		header.SetMode(0o600)
		entry, err := writer.CreateHeader(header)
		if err != nil {
			t.Fatal(err)
		}
		_, _ = entry.Write(file.raw)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return buffer.Bytes()
}
