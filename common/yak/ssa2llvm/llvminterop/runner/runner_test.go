package runner

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/yak/ssa2llvm/llvminterop/plugin"
)

func TestParseVersion(t *testing.T) {
	output := `LLVM (http://llvm.org/):
  LLVM version 17.0.6
  Optimizations enabled`

	info := &VersionInfo{}
	err := parseVersion(output, info)
	require.NoError(t, err)
	require.Equal(t, 17, info.Major)
	require.Equal(t, 0, info.Minor)
	require.Equal(t, 6, info.Patch)
}

func TestParseVersion_OlderFormat(t *testing.T) {
	output := `LLVM version 14.0.0`

	info := &VersionInfo{}
	err := parseVersion(output, info)
	require.NoError(t, err)
	require.Equal(t, 14, info.Major)
}

func TestParseVersion_Invalid(t *testing.T) {
	info := &VersionInfo{}
	err := parseVersion("no version here", info)
	require.Error(t, err)
}

func TestDetectCapabilities(t *testing.T) {
	tests := []struct {
		major              int
		expectNewPM        bool
		expectLegacyPM     bool
		expectLoadPassPlug bool
	}{
		{12, false, true, false},
		{13, true, true, true},
		{15, true, true, true},
		{17, true, false, true},
		{18, true, false, true},
	}

	for _, tt := range tests {
		v := &VersionInfo{Major: tt.major}
		caps := DetectCapabilities(v)
		require.Equal(t, tt.expectNewPM, caps.HasNewPM, "major=%d NewPM", tt.major)
		require.Equal(t, tt.expectLegacyPM, caps.HasLegacyPM, "major=%d LegacyPM", tt.major)
		require.Equal(t, tt.expectLoadPassPlug, caps.SupportsLoadPassPlugin, "major=%d LoadPassPlugin", tt.major)
	}
}

func TestDetectCapabilities_Nil(t *testing.T) {
	caps := DetectCapabilities(nil)
	require.False(t, caps.HasNewPM)
	require.False(t, caps.HasLegacyPM)
}

func TestRunNilConfig(t *testing.T) {
	_, err := Run(nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "nil config")
}

func TestRunNilPlugin(t *testing.T) {
	_, err := Run(&Config{})
	require.Error(t, err)
	require.Contains(t, err.Error(), "nil plugin")
}

func TestRunMissingInput(t *testing.T) {
	desc := plugin.Descriptor{Name: "test", Kind: plugin.KindNewPM, Path: "/nonexistent.so"}
	_, err := Run(&Config{
		Plugin:     &desc,
		InputFile:  "/tmp/nonexistent_input.ll",
		OutputFile: "/tmp/out.ll",
	})
	require.Error(t, err)
}
