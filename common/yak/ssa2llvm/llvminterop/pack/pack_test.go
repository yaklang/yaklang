package pack

import (
	"os"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/yak/ssa2llvm/llvminterop/plugin"
)

func TestManifestValidate_Valid(t *testing.T) {
	m := &Manifest{
		Name:           "test-pack",
		LLVMVersionMin: 14,
		Plugins: []plugin.Descriptor{
			{Name: "flatten", Kind: plugin.KindNewPM, Path: "/usr/lib/flatten.so", Passes: []string{"function(flatten)"}},
		},
	}
	require.NoError(t, m.Validate())
}

func TestManifestValidate_EmptyName(t *testing.T) {
	m := &Manifest{
		LLVMVersionMin: 14,
		Plugins: []plugin.Descriptor{
			{Name: "a", Kind: plugin.KindNewPM, Path: "/a.so"},
		},
	}
	require.Error(t, m.Validate())
}

func TestManifestValidate_NoPlugins(t *testing.T) {
	m := &Manifest{
		Name:           "test",
		LLVMVersionMin: 14,
	}
	err := m.Validate()
	require.Error(t, err)
	require.Contains(t, err.Error(), "at least one plugin")
}

func TestManifestCompatible(t *testing.T) {
	m := &Manifest{
		Name:           "test",
		LLVMVersionMin: 14,
		LLVMVersionMax: 17,
		Plugins:        []plugin.Descriptor{{Name: "a", Path: "/a.so"}},
	}

	require.False(t, m.Compatible(13))
	require.True(t, m.Compatible(14))
	require.True(t, m.Compatible(15))
	require.True(t, m.Compatible(17))
	require.False(t, m.Compatible(18))
}

func TestManifestCompatible_NoUpperBound(t *testing.T) {
	m := &Manifest{
		Name:           "test",
		LLVMVersionMin: 14,
		Plugins:        []plugin.Descriptor{{Name: "a", Path: "/a.so"}},
	}

	require.True(t, m.Compatible(14))
	require.True(t, m.Compatible(99))
}

func TestManifestSaveLoad(t *testing.T) {
	tmpFile := t.TempDir() + "/manifest.json"

	original := &Manifest{
		Name:           "test-pack",
		Description:    "A test pack",
		LLVMVersionMin: 14,
		LLVMVersionMax: 18,
		Plugins: []plugin.Descriptor{
			{Name: "flatten", Kind: plugin.KindNewPM, Path: "/usr/lib/flatten.so", Passes: []string{"function(flatten)"}},
		},
		KnownLimitations: []string{"Does not support opaque pointers"},
	}

	require.NoError(t, SaveManifest(tmpFile, original))

	loaded, err := LoadManifest(tmpFile)
	require.NoError(t, err)
	require.Equal(t, original.Name, loaded.Name)
	require.Equal(t, original.Description, loaded.Description)
	require.Equal(t, original.LLVMVersionMin, loaded.LLVMVersionMin)
	require.Equal(t, original.LLVMVersionMax, loaded.LLVMVersionMax)
	require.Len(t, loaded.Plugins, 1)
	require.Equal(t, "flatten", loaded.Plugins[0].Name)
	require.Len(t, loaded.KnownLimitations, 1)
}

func TestLoadManifest_NotFound(t *testing.T) {
	_, err := LoadManifest("/tmp/nonexistent_manifest.json")
	require.Error(t, err)
}

func TestLoadManifest_InvalidJSON(t *testing.T) {
	tmpFile := t.TempDir() + "/bad.json"
	require.NoError(t, os.WriteFile(tmpFile, []byte("not json"), 0644))

	_, err := LoadManifest(tmpFile)
	require.Error(t, err)
}

func TestRegistryBasics(t *testing.T) {
	reg := NewRegistry()

	m := &Manifest{
		Name:           "pack-a",
		LLVMVersionMin: 14,
		Plugins:        []plugin.Descriptor{{Name: "a", Path: "/a.so"}},
	}
	require.NoError(t, reg.Register(m))

	got, ok := reg.Get("pack-a")
	require.True(t, ok)
	require.Equal(t, "pack-a", got.Name)

	_, ok = reg.Get("nonexistent")
	require.False(t, ok)
}

func TestRegistryListCompatible(t *testing.T) {
	reg := NewRegistry()

	_ = reg.Register(&Manifest{
		Name: "old-pack", LLVMVersionMin: 10, LLVMVersionMax: 13,
		Plugins: []plugin.Descriptor{{Name: "a", Path: "/a.so"}},
	})
	_ = reg.Register(&Manifest{
		Name: "new-pack", LLVMVersionMin: 14,
		Plugins: []plugin.Descriptor{{Name: "b", Path: "/b.so"}},
	})

	compat := reg.ListCompatible(15)
	require.Len(t, compat, 1)
	require.Equal(t, "new-pack", compat[0].Name)
}

func TestLookupBuiltin(t *testing.T) {
	manifest, ok := LookupBuiltin("instcombine-simplifycfg")
	require.True(t, ok)
	require.NotNil(t, manifest)
	require.Equal(t, "instcombine-simplifycfg", manifest.Name)
	require.NotEmpty(t, manifest.Plugins)
}
