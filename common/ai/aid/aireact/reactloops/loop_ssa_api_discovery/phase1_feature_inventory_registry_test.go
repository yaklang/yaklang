package loop_ssa_api_discovery

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/loop_ssa_api_discovery/store"
)

func TestBuildFeatureInventoryFromRegistry(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "ssa_discovery"), 0o755))

	reg := CodeUnitRegistryV1{
		SchemaVersion: 1,
		Units: []CodeUnitEntry{
			{RelPath: "src/FooController.java", KindHint: codeUnitKindHintHTTPEntry},
			{RelPath: "src/BarService.java", KindHint: "service"},
		},
	}
	rb, _ := json.MarshalIndent(reg, "", "  ")
	require.NoError(t, os.WriteFile(store.CodeUnitRegistryPath(dir), rb, 0o644))

	rt := &Runtime{WorkDir: dir}
	inv, err := BuildFeatureInventoryFromRegistry(rt)
	require.NoError(t, err)
	require.Len(t, inv.Features, 1)
	require.Equal(t, SurfaceKindHTTPAPI, inv.Features[0].SurfaceKind)
	require.True(t, inv.Coverage.Complete)
	require.Equal(t, "code_unit_registry_http_entry", inv.Coverage.Policy)
}

func TestBackfillFeatureInventoryFromRegistry(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "ssa_discovery"), 0o755))

	reg := CodeUnitRegistryV1{
		SchemaVersion: 1,
		Units: []CodeUnitEntry{
			{RelPath: "mod/XController.java", KindHint: codeUnitKindHintHTTPEntry},
		},
	}
	rb, _ := json.MarshalIndent(reg, "", "  ")
	require.NoError(t, os.WriteFile(store.CodeUnitRegistryPath(dir), rb, 0o644))

	rt := &Runtime{WorkDir: dir}
	require.NoError(t, BackfillFeatureInventoryFromRegistry(rt))

	inv, err := loadFeatureInventory(dir)
	require.NoError(t, err)
	require.Len(t, inv.Features, 1)
}

func TestParseInput_SkipDirectoryAnalysis(t *testing.T) {
	p, err := extractUserInputFields(`Code path: /tmp/x
Target: http://127.0.0.1
skip directory analysis: yes`)
	require.NoError(t, err)
	require.True(t, p.SkipDirectoryAnalysis)

	p2, err := extractUserInputFields(`目录分析: 跳过
Code path: /tmp/x`)
	require.NoError(t, err)
	require.True(t, p2.SkipDirectoryAnalysis)
}
