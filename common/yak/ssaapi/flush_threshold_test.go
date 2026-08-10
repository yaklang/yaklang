package ssaapi

import (
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestFlushCompileUnitThresholdDefault verifies the default value when
// YAK_SSA_FLUSH_COMPILE_UNIT_THRESHOLD is not set.
func TestFlushCompileUnitThresholdDefault(t *testing.T) {
	os.Unsetenv("YAK_SSA_FLUSH_COMPILE_UNIT_THRESHOLD")
	require.Equal(t, defaultFlushCompileUnitThreshold, flushCompileUnitThreshold())
}

// TestFlushCompileUnitThresholdEnvVar verifies that the env var is read
// and parsed correctly.
func TestFlushCompileUnitThresholdEnvVar(t *testing.T) {
	t.Setenv("YAK_SSA_FLUSH_COMPILE_UNIT_THRESHOLD", "50000")
	require.Equal(t, 50000, flushCompileUnitThreshold())
}

// TestFlushCompileUnitThresholdInvalid verifies fallback on invalid values.
func TestFlushCompileUnitThresholdInvalid(t *testing.T) {
	t.Setenv("YAK_SSA_FLUSH_COMPILE_UNIT_THRESHOLD", "not-a-number")
	require.Equal(t, defaultFlushCompileUnitThreshold, flushCompileUnitThreshold())
}

// TestFlushCompileUnitThresholdNonPositive verifies fallback on non-positive values.
func TestFlushCompileUnitThresholdNonPositive(t *testing.T) {
	t.Setenv("YAK_SSA_FLUSH_COMPILE_UNIT_THRESHOLD", "0")
	require.Equal(t, defaultFlushCompileUnitThreshold, flushCompileUnitThreshold())

	t.Setenv("YAK_SSA_FLUSH_COMPILE_UNIT_THRESHOLD", "-100")
	require.Equal(t, defaultFlushCompileUnitThreshold, flushCompileUnitThreshold())
}
