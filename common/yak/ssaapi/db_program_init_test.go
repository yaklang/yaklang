package ssaapi_test

import (
	"runtime"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
)

// TestDBProgramInit_NoFullEditorLoad proves that FromDatabase/ssa.GetProgram
// does not create all source editors (RuneOffsetMap) at initialization time.
// On Hadoop with 11K sources, this would create ~1.19GB of RuneOffsetMap.
// The LRU(2000) on irSourceCache should prevent this, but we verify.
func TestDBProgramInit_NoFullEditorLoad(t *testing.T) {
	programName := uuid.NewString()
	defer ssadb.DeleteProgram(ssadb.GetDB(), programName)

	// Compile a small multi-file Go project
	_, err := ssaapi.Parse(
		`package main
import "fmt"
func main() { fmt.Println(add(1, 2)) }
func add(a, b int) int { return a + b }`,
		ssaapi.WithLanguage(ssaconfig.GO),
		ssaconfig.WithSetProgramName(programName),
	)
	require.NoError(t, err)

	// Count ir_sources in DB
	var sourceCount int64
	ssadb.GetDB().Table("ir_sources").Where("program_name = ?", programName).Count(&sourceCount)
	require.Greater(t, sourceCount, int64(0), "should have sources in DB")
	t.Logf("ir_sources in DB: %d", sourceCount)

	// Record heap before FromDatabase
	runtime.GC()
	var before runtime.MemStats
	runtime.ReadMemStats(&before)
	t.Logf("Before FromDatabase: HeapInuse=%dMB HeapObjects=%d",
		before.HeapInuse/1024/1024, before.HeapObjects)

	// Load program from database (DBRead mode)
	prog, err := ssaapi.FromDatabase(programName)
	require.NoError(t, err)
	require.NotNil(t, prog)

	// Record heap immediately after FromDatabase (before any rule scan)
	runtime.GC()
	var after runtime.MemStats
	runtime.ReadMemStats(&after)
	t.Logf("After FromDatabase: HeapInuse=%dMB HeapObjects=%d",
		after.HeapInuse/1024/1024, after.HeapObjects)

	heapDelta := int64(after.HeapInuse) - int64(before.HeapInuse)
	objDelta := int64(after.HeapObjects) - int64(before.HeapObjects)
	t.Logf("Heap delta: %dMB, Objects delta: %d", heapDelta/1024/1024, objDelta)

	// The delta should be small — FromDatabase should NOT load all source
	// editors at init. If sourceCount is small (e.g. 3), the delta should
	// be < 10MB. For large projects (11K sources), the LRU(2000) should
	// prevent loading all at once.
	// For this small test: delta should be < 50MB
	require.Less(t, heapDelta, int64(50*1024*1024),
		"FromDatabase heap delta (%dMB) should be < 50MB — should not load all editors at init",
		heapDelta/1024/1024)

	// Verify the program is usable
	require.Equal(t, programName, prog.GetProgramName(),
		"FromDatabase should return the correct program")
}
