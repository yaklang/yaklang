package ssaconfig

import (
	"runtime"

	"github.com/yaklang/yaklang/common/utils/memorybudget"
)

// One concurrent compile worker is budgeted this much RAM when deriving the
// default compile_concurrency from the machine. AST parse still has its own
// slot window; this floor only stops a 32-core box with 2GiB from opening 31
// PreHandler / library-body workers.
const compileWorkerMemoryBytes = 256 << 20

// DefaultCPUConcurrency is the CPU-only budget: max(1, GOMAXPROCS-1).
// Scan concurrency defaults to this. Compile concurrency also applies the
// machine memory cap via CompileConcurrencyFor.
func DefaultCPUConcurrency() int {
	c := runtime.GOMAXPROCS(0)
	if c <= 1 {
		return 1
	}
	return c - 1
}

func defaultCompileConcurrency() int {
	return CompileConcurrencyFor(runtime.GOMAXPROCS(0), memorybudget.Total())
}

// CompileConcurrencyFor caps compile workers by CPU and by usable RAM.
// cpuCount is GOMAXPROCS (or an explicit test value). memoryBytes 0 skips
// the memory cap so CPU-only tests stay stable.
func CompileConcurrencyFor(cpuCount int, memoryBytes int64) int {
	cpu := cpuCount - 1
	if cpu < 1 {
		cpu = 1
	}
	if memoryBytes <= 0 {
		return cpu
	}
	usable := memorybudget.UsableFrom(memoryBytes)
	memWorkers := int(usable / compileWorkerMemoryBytes)
	if memWorkers < 1 {
		memWorkers = 1
	}
	if cpu > memWorkers {
		return memWorkers
	}
	return cpu
}

// DefaultCompileMemoryLimit is 80% of cgroup/host RAM, the same budget
// compile uses for Go's soft memory limit when the caller does not set one.
func DefaultCompileMemoryLimit() int64 {
	return memorybudget.Usable()
}
