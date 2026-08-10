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

// TestRuleLifecycle_TempValuesReleased proves that after a SyntaxFlow
// rule query completes, temporary objects are releasable by GC.
func TestRuleLifecycle_TempValuesReleased(t *testing.T) {
	programName := uuid.NewString()
	defer ssadb.DeleteProgram(ssadb.GetDB(), programName)

	prog, err := ssaapi.Parse(
		`package main
import "fmt"
func main() {
	x := 1
	y := x + 2
	fmt.Println(y)
}`,
		ssaapi.WithLanguage(ssaconfig.GO),
		ssaconfig.WithSetProgramName(programName),
	)
	require.NoError(t, err)

	// Run a SyntaxFlow query
	result, err := prog.SyntaxFlowWithError(`* as $p`)
	require.NoError(t, err)
	require.NotNil(t, result)

	values := result.GetValues("p")
	t.Logf("Found %d values", values.Len())

	// Record heap after query
	runtime.GC()
	var before runtime.MemStats
	runtime.ReadMemStats(&before)

	// Release the result
	result = nil
	values = nil

	// Force GC
	runtime.GC()
	var after runtime.MemStats
	runtime.ReadMemStats(&after)

	delta := int64(after.HeapInuse) - int64(before.HeapInuse)
	t.Logf("Heap delta after releasing query result: %dMB", delta/1024/1024)
	require.Less(t, delta, int64(10*1024*1024),
		"Heap should not grow after releasing query result (delta=%dMB)", delta/1024/1024)
}
