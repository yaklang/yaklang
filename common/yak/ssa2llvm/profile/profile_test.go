package profile_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	// Ensure all built-in obfuscators are registered so Validate works.
	_ "github.com/yaklang/yaklang/common/yak/ssa2llvm/obfuscation/builtin"
	"github.com/yaklang/yaklang/common/yak/ssa2llvm/profile"
)

func TestBuiltinProfilesExist(t *testing.T) {
	names := profile.Names()
	require.GreaterOrEqual(t, len(names), 3, "expected at least 3 built-in profiles")
	assert.Contains(t, names, "resilience-lite")
	assert.Contains(t, names, "resilience-hybrid")
	assert.Contains(t, names, "resilience-max")
}

func TestGetProfile(t *testing.T) {
	tests := []struct {
		name  string
		found bool
	}{
		{"resilience-lite", true},
		{"Resilience-Lite", true},
		{"  resilience-hybrid  ", true},
		{"resilience-max", true},
		{"nonexistent", false},
	}
	for _, tt := range tests {
		p, ok := profile.Get(tt.name)
		if tt.found {
			require.True(t, ok, "expected to find %q", tt.name)
			require.NotNil(t, p)
		} else {
			require.False(t, ok, "expected not to find %q", tt.name)
		}
	}
}

func TestLiteProfileProperties(t *testing.T) {
	p, ok := profile.Get("resilience-lite")
	require.True(t, ok)

	assert.Equal(t, profile.LevelLite, p.Level)
	assert.Equal(t, profile.SeedNone, p.SeedPolicy)

	names := p.ObfuscatorNames()
	assert.Contains(t, names, "addsub")
	assert.Contains(t, names, "callret")
	assert.Contains(t, names, "xor")
}

func TestHybridProfileProperties(t *testing.T) {
	p, ok := profile.Get("resilience-hybrid")
	require.True(t, ok)

	assert.Equal(t, profile.LevelHybrid, p.Level)
	assert.Equal(t, profile.SeedPerBuild, p.SeedPolicy)

	names := p.ObfuscatorNames()
	assert.Contains(t, names, "mba")
	assert.Contains(t, names, "opaque")
	assert.Contains(t, names, "virtualize")
	assert.Contains(t, names, "callret")
}

func TestMaxProfileProperties(t *testing.T) {
	p, ok := profile.Get("resilience-max")
	require.True(t, ok)

	assert.Equal(t, profile.LevelMax, p.Level)
	assert.Equal(t, profile.SeedPerBuild, p.SeedPolicy)

	// Uses "*" glob - should list all registered passes
	names := p.ObfuscatorNames()
	assert.Contains(t, names, "*")
}

func TestValidateLite(t *testing.T) {
	p, _ := profile.Get("resilience-lite")
	require.NoError(t, p.Validate())
}

func TestValidateHybrid(t *testing.T) {
	p, _ := profile.Get("resilience-hybrid")
	require.NoError(t, p.Validate())
}

func TestValidateMax(t *testing.T) {
	p, _ := profile.Get("resilience-max")
	require.NoError(t, p.Validate())
}

func TestValidateBadObfuscator(t *testing.T) {
	p := &profile.Profile{
		Name:        "broken",
		Obfuscators: []string{"nonexistent-pass"},
	}
	err := p.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "nonexistent-pass")
}

func TestRegisterCustomProfile(t *testing.T) {
	custom := &profile.Profile{
		Name:        "custom-test",
		Level:       profile.LevelLite,
		Obfuscators: []string{"addsub"},
		SeedPolicy:  profile.SeedNone,
		Description: "test-only profile",
	}
	profile.Register(custom)

	p, ok := profile.Get("custom-test")
	require.True(t, ok)
	assert.Equal(t, "custom-test", p.Name)
	assert.Equal(t, "test-only profile", p.Description)
}

func TestListReturnsAllProfiles(t *testing.T) {
	all := profile.List()
	require.GreaterOrEqual(t, len(all), 3)

	// Verify sorted order
	for i := 1; i < len(all); i++ {
		assert.LessOrEqual(t, all[i-1].Name, all[i].Name,
			"profiles should be sorted by name")
	}
}

func TestNilProfileValidate(t *testing.T) {
	var p *profile.Profile
	err := p.Validate()
	require.Error(t, err)
}

func TestHybridDefaultPolicy(t *testing.T) {
	p, ok := profile.Get("resilience-hybrid")
	require.True(t, ok)

	pol := p.DefaultPolicy()
	require.NotNil(t, pol, "hybrid profile should have a default policy")

	require.Len(t, pol.Obfuscators, 1)
	assert.Equal(t, "virtualize", pol.Obfuscators[0].Name)
	require.NotNil(t, pol.Obfuscators[0].Selector.Ratio)
	assert.InDelta(t, 0.3, *pol.Obfuscators[0].Selector.Ratio, 0.001)
}

func TestMaxDefaultPolicy(t *testing.T) {
	p, ok := profile.Get("resilience-max")
	require.True(t, ok)

	pol := p.DefaultPolicy()
	require.NotNil(t, pol, "max profile should have a default policy")

	require.Len(t, pol.Obfuscators, 1)
	assert.Equal(t, "virtualize", pol.Obfuscators[0].Name)
	require.NotNil(t, pol.Obfuscators[0].Selector.Ratio)
	assert.InDelta(t, 1.0, *pol.Obfuscators[0].Selector.Ratio, 0.001)
}

func TestLiteHasNoDefaultPolicy(t *testing.T) {
	p, ok := profile.Get("resilience-lite")
	require.True(t, ok)

	pol := p.DefaultPolicy()
	assert.Nil(t, pol, "lite profile should have no default policy")
}

func TestNilDefaultPolicy(t *testing.T) {
	var p *profile.Profile
	assert.Nil(t, p.DefaultPolicy())
}
