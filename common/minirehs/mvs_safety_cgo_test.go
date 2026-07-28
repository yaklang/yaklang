//go:build cgo

package minirehs

import (
	"bytes"
	"runtime"
	"testing"
)

func TestScanInternalScratchClosesWorkers(t *testing.T) {
	db, err := Compile([]Pattern{
		{ID: 1, Expr: `[a-z]{3}`},
		{ID: 2, Expr: `\b[a-z]{2}\b`},
	}, WithBackend(BackendMVS), WithReportLocation(false), WithLogger(silentLogger{}))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	mdb := digMVSDB(t, db)
	if mdb.kernel == nil {
		t.Skip("C kernel unavailable")
	}
	if mdb.merged == nil || len(mdb.assertAlwaysOnCIdxs) == 0 || len(mdb.assertAlwaysOnGoIdxs) != 0 {
		t.Fatalf("test patterns did not enable async workers: merged=%v assertC=%d assertGo=%d",
			mdb.merged != nil, len(mdb.assertAlwaysOnCIdxs), len(mdb.assertAlwaysOnGoIdxs))
	}

	data := bytes.Repeat([]byte("abc "), 256)
	before := runtime.NumGoroutine()
	for i := 0; i < 8; i++ {
		keepScanning := i%2 != 0
		if err := db.Scan(data, nil, func(Match) bool { return keepScanning }); err != nil {
			t.Fatal(err)
		}
	}
	runtime.GC()
	runtime.Gosched()
	if delta := runtime.NumGoroutine() - before; delta > 8 {
		t.Fatalf("Scan with internal scratch leaked goroutines: delta=%d", delta)
	}
}

func TestMVSKernelMergedScanBatchResizesOutput(t *testing.T) {
	const patternsN = 80
	patterns := make([]Pattern, patternsN)
	for i := range patterns {
		patterns[i] = Pattern{ID: PatternID(i + 1), Expr: `[a-z]`}
	}
	db, err := Compile(patterns, WithBackend(BackendMVS), WithReportLocation(false), WithLogger(silentLogger{}))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	mdb := digMVSDB(t, db)
	if mdb.kernel == nil || mdb.merged == nil {
		t.Skip("merged C kernel unavailable")
	}
	sc, err := db.NewScratch()
	if err != nil {
		t.Fatal(err)
	}
	defer sc.Close()

	pairs := mdb.kernel.mergedScanBatch([]byte("a"), []int32{0, 1}, sc.(*scratch))
	if len(pairs) != patternsN {
		t.Fatalf("merged batch result was truncated: got=%d want=%d", len(pairs), patternsN)
	}
	seen := make(map[int32]bool, patternsN)
	for _, pair := range pairs {
		if pair[0] != 0 {
			t.Fatalf("unexpected record index: %d", pair[0])
		}
		seen[pair[1]] = true
	}
	if len(seen) != patternsN {
		t.Fatalf("merged batch members were not complete: got=%d want=%d", len(seen), patternsN)
	}
}
