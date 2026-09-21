package ssaconfig_test

import (
	"runtime"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils/memorybudget"
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

// TestCompileAndScanConcurrencyBothUseCPUConcurrency proves that scan
// defaults to the CPU budget and compile defaults to that budget capped by RAM.
func TestCompileAndScanConcurrencyBothUseCPUConcurrency(t *testing.T) {
	cfg, err := ssaconfig.New(ssaconfig.ModeSyntaxFlowScan)
	require.NoError(t, err)

	cpu := ssaconfig.DefaultCPUConcurrency()
	compile := cfg.GetCompileConcurrency()
	scan := int(cfg.GetScanConcurrency())

	require.Equal(t, cpu, scan,
		"scan concurrency must default to CPU concurrency (not 5)")
	require.LessOrEqual(t, compile, cpu,
		"compile concurrency must not exceed the CPU budget")
	require.GreaterOrEqual(t, compile, 1)
}

func TestCompileConcurrencyForCapsByMemory(t *testing.T) {
	require.Equal(t, 31, ssaconfig.CompileConcurrencyFor(32, 0))
	require.Equal(t, 31, ssaconfig.CompileConcurrencyFor(32, 32<<30))
	require.Equal(t, 6, ssaconfig.CompileConcurrencyFor(32, 2<<30))
	require.Equal(t, 1, ssaconfig.CompileConcurrencyFor(2, 32<<30))
	require.Equal(t, 1, ssaconfig.CompileConcurrencyFor(1, 1<<20))
}

func TestCompileMemoryLimitUsesUsableRAM(t *testing.T) {
	require.Equal(t, int64(24<<30), memorybudget.UsableFrom(30<<30))
	cfg, err := ssaconfig.New(ssaconfig.ModeSSACompile)
	require.NoError(t, err)
	require.Equal(t, ssaconfig.DefaultCompileMemoryLimit(), cfg.GetCompileMemoryLimit())
	require.NoError(t, ssaconfig.WithCompileMemoryLimit(4<<30)(cfg))
	require.Equal(t, int64(4<<30), cfg.GetCompileMemoryLimit())
}
