package ssaapi

import (
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestPathAEnabledDefault verifies Path A is disabled by default.
func TestPathAEnabledDefault(t *testing.T) {
	os.Unsetenv("YAK_SSA_PATH_A_RELOAD")
	require.False(t, PathAEnabled(), "Path A should be disabled by default")
}

// TestPathAEnabledPositive verifies Path A is enabled when env=1.
func TestPathAEnabledPositive(t *testing.T) {
	t.Setenv("YAK_SSA_PATH_A_RELOAD", "1")
	require.True(t, PathAEnabled())
}

// TestPathAEnabledNonPositive verifies Path A is disabled for non-positive values.
func TestPathAEnabledNonPositive(t *testing.T) {
	t.Setenv("YAK_SSA_PATH_A_RELOAD", "0")
	require.False(t, PathAEnabled())

	t.Setenv("YAK_SSA_PATH_A_RELOAD", "-1")
	require.False(t, PathAEnabled())
}

// TestPathAEnabledInvalid verifies Path A is disabled for invalid values.
func TestPathAEnabledInvalid(t *testing.T) {
	t.Setenv("YAK_SSA_PATH_A_RELOAD", "not-a-number")
	require.False(t, PathAEnabled())
}

// TestReloadProgramFromDatabaseNil verifies nil safety.
func TestReloadProgramFromDatabaseNil(t *testing.T) {
	result := ReloadProgramFromDatabase(nil)
	require.Nil(t, result)
}

// TestReloadProgramFromDatabaseEmptyName verifies fallback when program name is empty.
func TestReloadProgramFromDatabaseEmptyName(t *testing.T) {
	prog := &Program{
		Program: nil, // no underlying SSA program
	}
	result := ReloadProgramFromDatabase(prog)
	// Should return the original program (fallback) when name is empty
	require.NotNil(t, result, "should return original program on fallback")
}
