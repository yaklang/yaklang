package ssaapi

import (
	"github.com/stretchr/testify/require"
	"testing"
)

func TestAutomaticCompileMemoryLimitRespectsExplicitPolicies(t *testing.T) {
	for _, tc := range []struct {
		name, runtimeLimit, compileLimit, adaptive string
		automatic                                  bool
	}{
		{name: "default", automatic: true},
		{name: "runtime limit", runtimeLimit: "8GiB"},
		{name: "runtime explicitly disabled", runtimeLimit: "off"},
		{name: "compile limit", compileLimit: "4GiB"},
		{name: "compile explicitly disabled", compileLimit: "off"},
		{name: "adaptive default limit", adaptive: "1"},
		{name: "adaptive off", adaptive: "0", automatic: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("GOMEMLIMIT", tc.runtimeLimit)
			t.Setenv(ssaCompileMemLimitEnv, tc.compileLimit)
			t.Setenv(ssaCompileAdaptiveGCEnv, tc.adaptive)
			limit, enabled := automaticSSACompileMemoryLimit(30 * 1024 * 1024 * 1024)
			require.Equal(t, tc.automatic, enabled)
			if enabled {
				require.Equal(t, int64(24*1024*1024*1024), limit)
			} else {
				require.Zero(t, limit)
			}
			_, enabled = automaticSSACompileMemoryLimit(0)
			require.False(t, enabled)
		})
	}
}
