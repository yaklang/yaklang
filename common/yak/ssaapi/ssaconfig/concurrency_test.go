package ssaconfig_test

import (
	"runtime"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
)

// TestDefaultCPUConcurrencyIsCountMinusOne proves that the default CPU
// concurrency is max(1, GOMAXPROCS-1), not GOMAXPROCS/2.
func TestDefaultCPUConcurrencyIsCountMinusOne(t *testing.T) {
	got := ssaconfig.DefaultCPUConcurrency()
	expected := runtime.GOMAXPROCS(0) - 1
	if expected < 1 {
		expected = 1
	}
	require.Equal(t, expected, got,
		"DefaultCPUConcurrency must be max(1, GOMAXPROCS-1), got %d on %d CPUs",
		got, runtime.GOMAXPROCS(0))

	// On 32 CPU machine, must be 31
	if runtime.GOMAXPROCS(0) == 32 {
		require.Equal(t, 31, got, "on 32 CPUs, DefaultCPUConcurrency must be 31")
	}
}

// TestCompileAndScanConcurrencyBothUseCPUConcurrency proves that both
// compile and scan concurrency default to the unified CPU concurrency.
func TestCompileAndScanConcurrencyBothUseCPUConcurrency(t *testing.T) {
	cfg, err := ssaconfig.New(ssaconfig.ModeSyntaxFlowScan)
	require.NoError(t, err)

	cpu := ssaconfig.DefaultCPUConcurrency()
	compile := cfg.GetCompileConcurrency()
	scan := int(cfg.GetScanConcurrency())

	// With default config (no override), both should use CPU concurrency
	require.Equal(t, cpu, compile,
		"compile concurrency must default to CPU concurrency")
	require.Equal(t, cpu, scan,
		"scan concurrency must default to CPU concurrency (not 5)")
}
