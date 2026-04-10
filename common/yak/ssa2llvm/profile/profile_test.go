package profile_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	_ "github.com/yaklang/yaklang/common/yak/ssa2llvm/obfuscation/builtin"
	"github.com/yaklang/yaklang/common/yak/ssa2llvm/profile"
)

func ptrFloat(v float64) *float64 { return &v }
func ptrInt(v int) *int           { return &v }

func TestBuiltinProfilesExist(t *testing.T) {
	names := profile.Names()
	require.GreaterOrEqual(t, len(names), 3)
	assert.Contains(t, names, "resilience-lite")
	assert.Contains(t, names, "resilience-hybrid")
	assert.Contains(t, names, "resilience-max")
}

func TestGetProfile(t *testing.T) {
	tests := []struct {
		name  string
		found bool
	}{
		{name: "resilience-lite", found: true},
		{name: "Resilience-Lite", found: true},
		{name: " resilience-hybrid ", found: true},
		{name: "resilience-max", found: true},
		{name: "missing", found: false},
	}

	for _, tt := range tests {
		p, ok := profile.Get(tt.name)
		if tt.found {
			require.True(t, ok)
			require.NotNil(t, p)
		} else {
			require.False(t, ok)
		}
	}
}

func TestLiteProfileProperties(t *testing.T) {
	p, ok := profile.Get("resilience-lite")
	require.True(t, ok)

	assert.Equal(t, profile.SeedNone, p.NormalizedSeedPolicy())
	assert.Equal(t, []string{"addsub", "xor", "callret"}, p.ObfuscatorNames())
	require.NoError(t, p.Validate())
}

func TestHybridProfileProperties(t *testing.T) {
	p, ok := profile.Get("resilience-hybrid")
	require.True(t, ok)

	assert.Equal(t, profile.SeedPerBuild, p.NormalizedSeedPolicy())
	assert.Equal(t, []string{"addsub", "xor", "callret", "mba", "opaque", "virtualize"}, p.ObfuscatorNames())

	var virtualize profile.ObfEntry
	for _, entry := range p.Obfuscators {
		if entry.Name == "virtualize" {
			virtualize = entry
			break
		}
	}
	assert.Equal(t, profile.CategoryBodyReplace, virtualize.EffectiveCategory())
	require.NotNil(t, virtualize.Selector.Ratio)
	assert.InDelta(t, 0.3, *virtualize.Selector.Ratio, 0.001)
	require.NoError(t, p.Validate())
}

func TestMaxProfileProperties(t *testing.T) {
	p, ok := profile.Get("resilience-max")
	require.True(t, ok)

	assert.Equal(t, profile.SeedPerBuild, p.NormalizedSeedPolicy())
	assert.Equal(t, []string{"addsub", "xor", "callret", "mba", "opaque", "virtualize"}, p.ObfuscatorNames())
	require.NoError(t, p.Validate())
}

func TestValidateBadObfuscator(t *testing.T) {
	p := &profile.Profile{
		Name: "broken",
		Obfuscators: []profile.ObfEntry{
			{Name: "missing"},
		},
	}
	err := p.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "missing")
}

func TestValidateFixedSeed(t *testing.T) {
	p := &profile.Profile{
		Name:         "fixed",
		SeedPolicy:   profile.SeedFixed,
		BuildSeedHex: "00112233445566778899aabbccddeeff00112233445566778899aabbccddeeff",
		Obfuscators: []profile.ObfEntry{
			{Name: "addsub", Category: profile.CategoryLocal},
		},
	}
	require.NoError(t, p.Validate())

	seed, err := p.FixedBuildSeed()
	require.NoError(t, err)
	require.Len(t, seed, 32)
}

func TestValidateFixedSeedRequiresHex(t *testing.T) {
	p := &profile.Profile{
		Name:       "fixed",
		SeedPolicy: profile.SeedFixed,
		Obfuscators: []profile.ObfEntry{
			{Name: "addsub", Category: profile.CategoryLocal},
		},
	}
	err := p.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "build_seed_hex")
}

func TestValidateSelectorRules(t *testing.T) {
	p := &profile.Profile{
		Name: "bad",
		Obfuscators: []profile.ObfEntry{
			{
				Name:     "addsub",
				Category: profile.CategoryLocal,
				Selector: profile.Selector{
					Ratio: ptrFloat(0.5),
					Count: ptrInt(1),
				},
			},
		},
	}
	err := p.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "mutually exclusive")
}

func TestParseProfileFile(t *testing.T) {
	raw := []byte(`{
		"name": "file-profile",
		"seed_policy": "none",
		"obfuscators": [
			{"name": "callret", "category": "callflow", "selector": {"allow_entry": true}}
		]
	}`)
	p, err := profile.Parse(raw)
	require.NoError(t, err)
	assert.Equal(t, "file-profile", p.Name)
	require.Len(t, p.Obfuscators, 1)
	assert.Equal(t, "callret", p.Obfuscators[0].Name)
}

func TestLoadFile(t *testing.T) {
	p := &profile.Profile{
		Name:        "disk-profile",
		Description: "from disk",
		Obfuscators: []profile.ObfEntry{
			{Name: "addsub", Category: profile.CategoryLocal},
		},
	}
	data, err := json.Marshal(p)
	require.NoError(t, err)

	path := filepath.Join(t.TempDir(), "profile.json")
	require.NoError(t, os.WriteFile(path, data, 0o644))

	loaded, err := profile.LoadFile(path)
	require.NoError(t, err)
	assert.Equal(t, "disk-profile", loaded.Name)
	assert.Equal(t, "from disk", loaded.Description)
}

func TestLoadRefBuiltinWinsOverPath(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "resilience-lite"), []byte(`not-json`), 0o644))
	cwd, err := os.Getwd()
	require.NoError(t, err)
	require.NoError(t, os.Chdir(dir))
	t.Cleanup(func() {
		_ = os.Chdir(cwd)
	})

	loaded, err := profile.LoadRef("resilience-lite")
	require.NoError(t, err)
	require.NotNil(t, loaded)
	assert.Equal(t, "resilience-lite", loaded.Name)
}

func TestLoadRefFilePath(t *testing.T) {
	data := []byte(`{
		"name": "file-profile",
		"obfuscators": [{"name": "addsub", "category": "llvm-local"}]
	}`)
	path := filepath.Join(t.TempDir(), "custom.json")
	require.NoError(t, os.WriteFile(path, data, 0o644))

	loaded, err := profile.LoadRef(path)
	require.NoError(t, err)
	assert.Equal(t, "file-profile", loaded.Name)
}

func TestRegisterCustomProfileClonesInput(t *testing.T) {
	custom := &profile.Profile{
		Name:        "custom-test",
		Description: "test only",
		Obfuscators: []profile.ObfEntry{
			{Name: "addsub", Category: profile.CategoryLocal},
		},
	}
	profile.Register(custom)
	custom.Obfuscators[0].Name = "mutated"

	loaded, ok := profile.Get("custom-test")
	require.True(t, ok)
	require.NotNil(t, loaded)
	assert.Equal(t, "addsub", loaded.Obfuscators[0].Name)
}

func TestDefaultCategoryForObfuscator(t *testing.T) {
	assert.Equal(t, profile.CategoryBodyReplace, profile.DefaultCategoryForObfuscator("virtualize"))
	assert.Equal(t, profile.CategoryCallflow, profile.DefaultCategoryForObfuscator("callret"))
	assert.Equal(t, profile.CategoryLocal, profile.DefaultCategoryForObfuscator("addsub"))
}

func TestNilProfileValidate(t *testing.T) {
	var p *profile.Profile
	err := p.Validate()
	require.Error(t, err)
}
