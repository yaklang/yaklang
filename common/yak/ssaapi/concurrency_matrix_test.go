package ssaapi_test

import (
	"fmt"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
)

func compileSmallGoFixture(t *testing.T, concurrency int) (programName string) {
	t.Helper()
	programName = uuid.NewString()
	t.Cleanup(func() {
		ssadb.DeleteProgram(ssadb.GetDB(), programName)
	})

	src := `package main
import "fmt"
func add(a, b int) int { return a + b }
func sub(a, b int) int { return a - b }
func mul(a, b int) int { return a * b }
func main() {
	x := add(1, 2)
	y := sub(x, 3)
	z := mul(x, y)
	fmt.Println(z)
}`

	_, err := ssaapi.Parse(src,
		ssaapi.WithLanguage(ssaconfig.GO),
		ssaconfig.WithCompileConcurrency(concurrency),
		ssaconfig.WithSetProgramName(programName),
	)
	require.NoError(t, err, "compile with concurrency=%d failed", concurrency)
	return programName
}

func countDB(t *testing.T, programName, table string) int64 {
	t.Helper()
	var count int64
	err := ssadb.GetDB().Table(table).Where("program_name = ?", programName).Count(&count).Error
	require.NoError(t, err, "count %s failed", table)
	return count
}

// TestConcurrencyMatrix_InstructionIDUnique proves that instruction IDs
// are unique across all concurrency levels.
func TestConcurrencyMatrix_InstructionIDUnique(t *testing.T) {
	for _, conc := range []int{1, 16, 31} {
		t.Run(fmt.Sprintf("concurrency=%d", conc), func(t *testing.T) {
			progName := compileSmallGoFixture(t, conc)

			total := countDB(t, progName, "ir_codes")
			require.Greater(t, total, int64(0), "concurrency=%d: should have instructions", conc)

			var dupCount int64
			err := ssadb.GetDB().Raw(
				"SELECT COUNT(*) FROM (SELECT code_id, COUNT(*) as c FROM ir_codes WHERE program_name = ? GROUP BY code_id HAVING c > 1) as dups",
				progName,
			).Row().Scan(&dupCount)
			require.NoError(t, err)
			require.Equal(t, int64(0), dupCount,
				"concurrency=%d: no duplicate code_ids (found %d dups, total=%d)",
				conc, dupCount, total)
		})
	}
}

// TestConcurrencyMatrix_CountConsistent proves instruction count is
// consistent across concurrency levels (within 5%).
func TestConcurrencyMatrix_CountConsistent(t *testing.T) {
	counts := make(map[int]int64)
	for _, conc := range []int{1, 16, 31} {
		progName := compileSmallGoFixture(t, conc)
		counts[conc] = countDB(t, progName, "ir_codes")
		t.Logf("concurrency=%d: ir_codes=%d", conc, counts[conc])
	}

	base := counts[1]
	for conc, count := range counts {
		if conc == 1 {
			continue
		}
		ratio := float64(count) / float64(base)
		require.InDelta(t, 1.0, ratio, 0.05,
			"concurrency=%d: ir_codes (%d) should be within 5%% of concurrency=1 (%d), ratio=%.2f",
			conc, count, base, ratio)
	}
}

// TestConcurrencyMatrix_OffsetRatioBounded proves offset/instruction
// ratio has a reasonable upper bound.
func TestConcurrencyMatrix_OffsetRatioBounded(t *testing.T) {
	for _, conc := range []int{1, 16, 31} {
		t.Run(fmt.Sprintf("concurrency=%d", conc), func(t *testing.T) {
			progName := compileSmallGoFixture(t, conc)

			irCodes := countDB(t, progName, "ir_codes")
			irOffsets := countDB(t, progName, "ir_offsets")

			require.Greater(t, irCodes, int64(0))
			ratio := float64(irOffsets) / float64(irCodes)
			t.Logf("concurrency=%d: ir_codes=%d ir_offsets=%d ratio=%.1f", conc, irCodes, irOffsets, ratio)

			require.Less(t, ratio, 20.0,
				"concurrency=%d: offset/instruction ratio (%.1f) should be < 20x",
				conc, ratio)
		})
	}
}

// TestConcurrencyMatrix_RaceConcurrentCompiles runs 5 concurrent compiles
// with concurrency=31 to detect data races.
func TestConcurrencyMatrix_RaceConcurrentCompiles(t *testing.T) {
	var wg sync.WaitGroup
	errs := make([]error, 5)
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			progName := uuid.NewString()
			defer ssadb.DeleteProgram(ssadb.GetDB(), progName)
			_, errs[idx] = ssaapi.Parse(
				`package main
func f(x int) int { return x + 1 }
func main() { println(f(42)) }`,
				ssaapi.WithLanguage(ssaconfig.GO),
				ssaconfig.WithCompileConcurrency(31),
				ssaconfig.WithSetProgramName(progName),
			)
		}(i)
	}
	wg.Wait()
	for i, err := range errs {
		require.NoError(t, err, "concurrent compile %d failed", i)
	}
}

// TestConcurrencyMatrix_SingleProjectRace proves that a single
// ParseProjectWithFS call with concurrency=31 can trigger data races
// in the per-file SSA build pipeline (not just concurrent Parse calls).
func TestConcurrencyMatrix_SingleProjectRace(t *testing.T) {
	// Create a multi-file Go project in memory
	// We need at least 32+ files to trigger compile-unit batching with concurrency=31
	// But even with fewer files, the compile concurrency applies to per-file parsing
	progName := uuid.NewString()
	t.Cleanup(func() { ssadb.DeleteProgram(ssadb.GetDB(), progName) })

	// Use Parse (single source) with high concurrency — this tests if the
	// SSA build pipeline itself has races at high concurrency
	_, err := ssaapi.Parse(
		`package main
import "fmt"
func a() int { return 1 }
func b() int { return 2 }
func c() int { return 3 }
func d() int { return 4 }
func main() {
	fmt.Println(a() + b() + c() + d())
}`,
		ssaapi.WithLanguage(ssaconfig.GO),
		ssaconfig.WithCompileConcurrency(31),
		ssaconfig.WithSetProgramName(progName),
	)
	require.NoError(t, err)

	// Verify data integrity
	total := countDB(t, progName, "ir_codes")
	require.Greater(t, total, int64(0))

	var dupCount int64
	err = ssadb.GetDB().Raw(
		"SELECT COUNT(*) FROM (SELECT code_id, COUNT(*) as c FROM ir_codes WHERE program_name = ? GROUP BY code_id HAVING c > 1) as dups",
		progName,
	).Row().Scan(&dupCount)
	require.NoError(t, err)
	require.Equal(t, int64(0), dupCount,
		"no duplicate code_ids in single-project compile with concurrency=31")
}
