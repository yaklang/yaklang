package scannode

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestPluginCacheManagerSyncPersistsArtifact(t *testing.T) {
	t.Parallel()

	baseDir := t.TempDir()
	sourcePath := filepath.Join(baseDir, "source.yak")
	content := []byte(`println("hello legion")`)
	if err := os.WriteFile(sourcePath, content, 0o644); err != nil {
		t.Fatalf("write source artifact: %v", err)
	}
	digest := sha256.Sum256(content)

	manager := newPluginCacheManager(PluginCacheManagerConfig{
		NodeID:  "node-a",
		BaseDir: baseDir,
	})
	result, err := manager.Sync(context.Background(), PluginSyncInput{
		PluginID:          "plugin-a",
		ReleaseID:         "release-a",
		Version:           "2026.03.28",
		EntryKind:         "yak_script",
		ArtifactURI:       sourcePath,
		ArtifactSHA256:    hex.EncodeToString(digest[:]),
		ArtifactSizeBytes: int64(len(content)),
	})
	if err != nil {
		t.Fatalf("sync plugin: %v", err)
	}
	if result.Status != pluginCacheStatusReady {
		t.Fatalf("unexpected status: %s", result.Status)
	}
	if _, err := os.Stat(result.LocalPath); err != nil {
		t.Fatalf("stat cached artifact: %v", err)
	}

	scriptContent, err := manager.LoadScriptContent("release-a")
	if err != nil {
		t.Fatalf("load cached script: %v", err)
	}
	if scriptContent != string(content) {
		t.Fatalf("unexpected script content: %s", scriptContent)
	}
}

func TestPluginCacheManagerSyncRejectsHashMismatch(t *testing.T) {
	t.Parallel()

	baseDir := t.TempDir()
	sourcePath := filepath.Join(baseDir, "source.yak")
	if err := os.WriteFile(sourcePath, []byte(`println("bad hash")`), 0o644); err != nil {
		t.Fatalf("write source artifact: %v", err)
	}

	manager := newPluginCacheManager(PluginCacheManagerConfig{
		NodeID:  "node-a",
		BaseDir: baseDir,
	})
	_, err := manager.Sync(context.Background(), PluginSyncInput{
		PluginID:          "plugin-a",
		ReleaseID:         "release-a",
		Version:           "2026.03.28",
		EntryKind:         "yak_script",
		ArtifactURI:       sourcePath,
		ArtifactSHA256:    stringsRepeat("0", 64),
		ArtifactSizeBytes: 19,
	})
	if !errors.Is(err, ErrArtifactHashMismatch) {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestPluginCacheManagerLoadScriptContentReturnsCacheMiss(t *testing.T) {
	t.Parallel()

	manager := newPluginCacheManager(PluginCacheManagerConfig{
		NodeID:  "node-a",
		BaseDir: t.TempDir(),
	})
	_, err := manager.LoadScriptContent("release-missing")
	if !errors.Is(err, ErrPluginCacheMiss) {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestNormalizePluginEntryKindRejectsLegacyDispatchKind(t *testing.T) {
	t.Parallel()

	_, err := normalizePluginEntryKind("yak_script_name")
	if !errors.Is(err, ErrUnsupportedPluginEntryKind) {
		t.Fatalf("unexpected error: %v", err)
	}
}

func stringsRepeat(value string, count int) string {
	result := ""
	for i := 0; i < count; i++ {
		result += value
	}
	return result
}
