//go:build cgo

package minirehs

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestMVSKernelMergedBatchMemberBoundary(t *testing.T) {
	for _, count := range []int{1, 255, 256, 257, 300} {
		t.Run(fmt.Sprint(count), func(t *testing.T) {
			patterns := make([]Pattern, count)
			for i := range patterns {
				patterns[i] = Pattern{ID: PatternID(i + 1), Expr: `[a-z]`}
			}
			db, err := Compile(patterns, WithBackend(BackendMVS), WithReportLocation(false), WithLogger(silentLogger{}))
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			m := digMVSDB(t, db)
			if m.kernel == nil || m.merged == nil {
				t.Fatal("expected actual merged native route")
			}
			if count == 300 && os.Getenv("MINIREHS_EVIDENCE_DIR") != "" {
				dir := programEvidenceDir(t)
				if err := os.WriteFile(filepath.Join(dir, "merged-300.blob"), m.blob, 0600); err != nil {
					t.Fatal(err)
				}
			}
			pairs, err := m.kernel.mergedScanBatch([]byte("a!a"), []int32{0, 1, 2, 3}, &scratch{})
			if err != nil {
				t.Fatal(err)
			}
			if len(pairs) != 2*count {
				t.Fatalf("complete pair count=%d want=%d", len(pairs), 2*count)
			}
			seen := make(map[[2]int32]int)
			for _, p := range pairs {
				seen[p]++
			}
			for _, record := range []int32{0, 2} {
				for member := 0; member < count; member++ {
					if n := seen[[2]int32{record, int32(member)}]; n != 1 {
						t.Fatalf("record=%d member=%d multiplicity=%d want=1", record, member, n)
					}
				}
			}
		})
	}
}

func TestMVSKernelMergedBatchInvalidOffsets(t *testing.T) {
	db, err := Compile([]Pattern{{ID: 1, Expr: `[a-z]`}}, WithBackend(BackendMVS), WithReportLocation(false), WithLogger(silentLogger{}))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	m := digMVSDB(t, db)
	if m.kernel == nil || m.merged == nil {
		t.Fatal("expected merged native route")
	}
	sc := &scratch{}
	for _, offsets := range [][]int32{{-1, 1}, {1, 0}, {0, 2}} {
		if pairs, err := m.kernel.mergedScanBatch([]byte("a"), offsets, sc); err == nil || len(pairs) != 0 {
			t.Fatalf("invalid offsets=%v returned pairs=%v err=%v", offsets, pairs, err)
		}
	}
	if _, err := m.kernel.mergedScanBatch([]byte("a"), []int32{0, 1}, nil); err == nil {
		t.Fatal("nil scratch accepted")
	}
	if pairs, err := m.kernel.mergedScanBatch(nil, []int32{0, 0}, sc); err != nil || len(pairs) != 0 {
		t.Fatalf("empty record returned pairs=%v err=%v", pairs, err)
	}
	pairs, err := m.kernel.mergedScanBatch([]byte("a"), []int32{0, 1}, sc)
	if err != nil || len(pairs) != 1 || pairs[0] != [2]int32{0, 0} {
		t.Fatalf("scratch reuse after invalid batch: pairs=%v err=%v", pairs, err)
	}
}

func TestMVSKernelMergedAnchoredRecordMask(t *testing.T) {
	patterns := []Pattern{{ID: 11, Expr: `^[0-9]{2}$`}, {ID: 22, Expr: `[a-z]{2}`}}
	db, image := compileImageTest(t, patterns, false)
	loaded, err := LoadImage(image)
	if err != nil {
		t.Fatal(err)
	}
	defer loaded.Close()
	for _, input := range [][]byte{[]byte("12"), []byte("aa"), []byte("x12"), nil, []byte("\xff12")} {
		want := programOracle(patterns, input, BackendMVS, false, 0)
		for _, current := range []Database{db, loaded} {
			got := programScan(t, current, input, 0)
			if err := programCompare(got, want, false); err != nil {
				t.Fatalf("input=%q: %s", input, err)
			}
		}
	}
	m := digMVSDB(t, db)
	pairs, err := m.kernel.mergedScanBatch([]byte("12aa!1"), []int32{0, 2, 4, 6}, &scratch{})
	if err != nil || len(pairs) != 2 || pairs[0] != [2]int32{0, 0} || pairs[1] != [2]int32{1, 1} {
		t.Fatalf("anchored/unanchored batch got=%v err=%v", pairs, err)
	}
}

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

	pairs, err := mdb.kernel.mergedScanBatch([]byte("a"), []int32{0, 1}, sc.(*scratch))
	if err != nil {
		t.Fatal(err)
	}
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
