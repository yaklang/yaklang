package ssa

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils/memedit"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
)

// newOffsetTestProgram creates a minimal Program with a mem editor for offset tests.
func newOffsetTestProgram(t *testing.T) (*Program, *memedit.MemEditor) {
	t.Helper()
	cfg, _ := ssaconfig.New(ssaconfig.ModeSSACompile, ssaconfig.WithSetProgramName(t.Name()))
	prog := NewProgram(cfg, ProgramCacheMemory, Application, nil, "", 0)
	// 300-byte content so offsets 0..99 each map to a unique endOffset
	content := make([]byte, 300)
	for i := range content {
		content[i] = 'x'
	}
	editor := memedit.NewMemEditor(string(content))
	editor.SetFileName("test.yak")
	prog.PushEditor(editor)
	return prog, editor
}

// TestProgramSetOffsetValueConcurrent verifies that concurrent calls to
// SetOffsetValue on the same Program do not trigger
// "fatal error: concurrent map writes".
//
// Before the offsetStore fix, this test panics because OffsetMap and
// OffsetSortedSlice are unprotected plain map/slice. Values and ranges
// are pre-created (single-threaded) to isolate the test to offsetStore's
// locking correctness.
func TestProgramSetOffsetValueConcurrent(t *testing.T) {
	prog, editor := newOffsetTestProgram(t)

	builder := prog.GetAndCreateFunctionBuilder("", string(MainFunctionName))
	require.NotNil(t, builder)

	// Pre-create values and ranges (single-threaded) — builder.EmitConstInst
	// is not goroutine-safe, so we must not call it concurrently.
	type item struct {
		val Value
		r   *memedit.Range
	}
	items := make([]item, 100)
	for i := 0; i < 100; i++ {
		val := builder.EmitConstInst(i)
		r := memedit.NewRangeFromOffsets(editor, i, i+1)
		items[i] = item{val: val, r: r}
	}

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			prog.SetOffsetValue(items[n].val, items[n].r)
		}(i)
	}
	wg.Wait()
	// If we reach here without panic, the concurrent map writes bug is fixed.
}

// TestProgramSetOffsetValueAndReadConcurrent verifies no race between
// concurrent writes (SetOffsetValue) and reads (GetFrontValueByOffset /
// SearchIndexAndOffsetByOffset) on the same Program.
func TestProgramSetOffsetValueAndReadConcurrent(t *testing.T) {
	prog, editor := newOffsetTestProgram(t)

	builder := prog.GetAndCreateFunctionBuilder("", string(MainFunctionName))
	require.NotNil(t, builder)

	// Pre-create values and ranges (single-threaded)
	type item struct {
		val Value
		r   *memedit.Range
	}
	items := make([]item, 70)
	for i := 0; i < 70; i++ {
		val := builder.EmitConstInst(i)
		r := memedit.NewRangeFromOffsets(editor, i, i+1)
		items[i] = item{val: val, r: r}
	}

	// Seed some offsets first so readers have data
	for i := 0; i < 20; i++ {
		prog.SetOffsetValue(items[i].val, items[i].r)
	}

	var wg sync.WaitGroup
	// Concurrent writers (use pre-created items[20..69])
	for i := 20; i < 70; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			prog.SetOffsetValue(items[n].val, items[n].r)
		}(i)
	}
	// Concurrent readers
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			prog.GetFrontValueByOffset(n)
			prog.SearchIndexAndOffsetByOffset(n)
		}(i)
	}
	wg.Wait()
}

// TestOffsetStoreConcurrentSetIsAtomic verifies the offsetStore directly:
// concurrent setValue calls with distinct endOffsets must all be preserved
// (no lost writes due to race).
func TestOffsetStoreConcurrentSetIsAtomic(t *testing.T) {
	store := newOffsetStore()
	content := make([]byte, 300)
	for i := range content {
		content[i] = 'x'
	}
	editor := memedit.NewMemEditor(string(content))

	// Pre-create all ranges (single-threaded) to isolate offsetStore locking
	ranges := make([]*memedit.Range, 100)
	for i := 0; i < 100; i++ {
		ranges[i] = memedit.NewRangeFromOffsets(editor, i, i+1)
	}

	// Verify all endOffsets are unique (sanity check)
	endOffsets := make(map[int]bool, 100)
	for _, r := range ranges {
		endOffsets[r.GetEndOffset()] = true
	}
	require.Equal(t, 100, len(endOffsets), "precondition: all 100 ranges must have unique endOffsets")

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			store.setValue(nil, ranges[n], false)
		}(i)
	}
	wg.Wait()
	assert.Equal(t, 100, store.count(), "all 100 concurrent writes should be preserved")
}