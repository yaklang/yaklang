package resources_monitor

import (
	"embed"
	"testing"

	"github.com/stretchr/testify/assert"
)

//go:embed testdata/normal/**
var normalFS embed.FS

// compressed.tar.gz contains the following structure:
// content/
// └── file.txt (content: "mock file\n")
// This file is created during test setup or expected to exist in repo.
//
//go:embed testdata/compressed.tar.gz
var compressedFS embed.FS

// base-yak-plugin.tar.gz is built with --root-path so paths inside are "base-yak-plugin/xxx.yak".
//
//go:embed testdata/base-yak-plugin.tar.gz
var baseYakPluginFS embed.FS

// base-yak-plugin-noprefix.tar.gz is built without --root-path (paths inside are "plugin1.yak").
// Used to test prefix fallback: ReadFile("base-yak-plugin/plugin1.yak") retries as "plugin1.yak".
//
//go:embed testdata/base-yak-plugin-noprefix.tar.gz
var baseYakPluginNoPrefixFS embed.FS

func TestStandardResourceMonitor(t *testing.T) {
	monitor := NewStandardResourceMonitor(normalFS, "")

	content, err := monitor.ReadFile("testdata/normal/file1.txt")
	assert.NoError(t, err)
	// Embedded content matches echo output (with newline)
	assert.Equal(t, "content1\n", string(content))

	entries, err := monitor.ReadDir("testdata/normal")
	assert.NoError(t, err)
	assert.True(t, len(entries) >= 2)

	hash, err := monitor.GetHash()
	assert.NoError(t, err)
	assert.NotEmpty(t, hash)

	monitorExt := NewStandardResourceMonitor(normalFS, ".txt")
	hashExt, err := monitorExt.GetHash()
	assert.NoError(t, err)
	assert.NotEmpty(t, hashExt)
}

func TestGzipResourceMonitor(t *testing.T) {
	monitor := NewGzipResourceMonitor(&compressedFS, "testdata/compressed.tar.gz", "")

	notified := false
	monitor.SetNotify(func(p float64, msg string) {
		notified = true
	})

	// Test successful read of file inside tar.gz
	// The tar.gz contains "content/file.txt"
	content, err := monitor.ReadFile("content/file.txt")
	assert.NoError(t, err)
	assert.Equal(t, "mock file\n", string(content))

	// Test notification happened
	assert.True(t, notified, "Notification should have been triggered during init")

	// Test missing file
	_, err = monitor.ReadFile("nonexistent.txt")
	assert.Error(t, err)
}

// TestGzipResourceMonitorCorePluginPath tests that a gzip embed with "base-yak-plugin/xxx.yak"
// paths (same convention as coreplugin) works: ReadFile and ReadDir use the directory prefix.
func TestGzipResourceMonitorCorePluginPath(t *testing.T) {
	monitor := NewGzipResourceMonitor(&baseYakPluginFS, "testdata/base-yak-plugin.tar.gz", "base-yak-plugin")

	// Same path pattern as coreplugin.GetCorePluginData("plugin1") -> "base-yak-plugin/plugin1.yak"
	content, err := monitor.ReadFile("base-yak-plugin/plugin1.yak")
	assert.NoError(t, err)
	assert.Contains(t, string(content), "plugin1")

	// Same as coreplugin.GetAllCorePluginName() which does ReadDir("base-yak-plugin")
	entries, err := monitor.ReadDir("base-yak-plugin")
	assert.NoError(t, err)
	assert.GreaterOrEqual(t, len(entries), 1)
	var found bool
	for _, e := range entries {
		if e.Name() == "plugin1.yak" {
			found = true
			break
		}
	}
	assert.True(t, found, "base-yak-plugin/plugin1.yak should appear in ReadDir")
}

// TestGzipResourceMonitorPrefixFallback: tar without --root-path has paths like "plugin1.yak".
// With pathPrefix "base-yak-plugin", ReadFile("base-yak-plugin/plugin1.yak") falls back to "plugin1.yak".
func TestGzipResourceMonitorPrefixFallback(t *testing.T) {
	monitor := NewGzipResourceMonitor(&baseYakPluginNoPrefixFS, "testdata/base-yak-plugin-noprefix.tar.gz", "base-yak-plugin")

	content, err := monitor.ReadFile("base-yak-plugin/plugin1.yak")
	assert.NoError(t, err)
	assert.Contains(t, string(content), "plugin1")
}

func TestResourceMonitorInterface(t *testing.T) {
	var _ ResourceMonitor = (*standardResourceMonitor)(nil)
	var _ ResourceMonitor = (*gzipResourceMonitor)(nil)
}
