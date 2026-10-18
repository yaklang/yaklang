package minirehs

import (
	"bytes"
	"os"
	"runtime"
	"testing"
)

func programSmallWorkerMode(tb testing.TB, db Database) string {
	tb.Helper()
	mode := os.Getenv("MINIREHS_SMALL_WORKER_MODE")
	if mode == "" {
		mode = "ShortInline"
	}
	m := db.(*database).imageMVS()
	switch mode {
	case "Reference":
		m.inlineSmallWorkers = false
	case "ShortInline":
		m.inlineSmallWorkers = true
	default:
		tb.Fatal("set MINIREHS_SMALL_WORKER_MODE Reference/ShortInline")
	}
	return mode
}

func smallWorkerPatterns() []Pattern {
	return []Pattern{{ID: 0, Expr: `[0-9]{3}`}, {ID: 1, Expr: `\b[a-z]{3}\b`}, {ID: 2, Expr: `needle(?=!)`}, {ID: 3, Expr: `prefix.*[ab]`}, {ID: 4, Expr: `(?:[ab]*left|right[cd]*)`}, {ID: 5, Expr: `a[bc]token`}}
}

func TestSmallWorkerPlansEquivalent(t *testing.T) {
	prior := runtime.GOMAXPROCS(4)
	defer runtime.GOMAXPROCS(prior)
	pats := smallWorkerPatterns()
	for _, locate := range []bool{false, true} {
		db, err := Compile(pats, WithBackend(BackendMVS), WithReportLocation(locate), WithLogger(silentLogger{}))
		if err != nil {
			t.Fatal(err)
		}
		image, err := db.MarshalImage()
		if err != nil {
			t.Fatal(err)
		}
		loaded, err := LoadImage(image)
		if err != nil {
			t.Fatal(err)
		}
		for _, current := range []Database{db, loaded} {
			m := current.(*database).imageMVS()
			if !m.inlineSmallWorkers {
				t.Fatal("constructor did not derive the current runtime plan")
			}
			for _, size := range []int{0, 1, 31, 32, 127, 128, 511, 512, 513, 1024} {
				data := bytes.Repeat([]byte{' '}, size)
				copy(data, "abtoken needle! prefixaaa rightccc abc 123 \xff\x00K中")
				m.inlineSmallWorkers = false
				full := programScan(t, current, data, 0)
				if err := programCompare(full, programOracle(pats, data, BackendMVS, locate, 0), false); err != nil {
					t.Fatal(err)
				}
				for _, stop := range []int{0, 1, 2} {
					want := programScan(t, current, data, stop)
					m.inlineSmallWorkers = true
					got := programScan(t, current, data, stop)
					if err := programCompare(got, want, true); err != nil {
						t.Fatalf("size%d locate%v stop%d: %v", size, locate, stop, err)
					}
					m.inlineSmallWorkers = false
				}
			}
			m.inlineSmallWorkers = true
		}
		loaded.Close()
		db.Close()
	}
}

func TestSmallWorkerDispatchBoundary(t *testing.T) {
	prior := runtime.GOMAXPROCS(4)
	defer runtime.GOMAXPROCS(prior)
	db, err := Compile([]Pattern{{ID: 0, Expr: `token[0-9]+`}, {ID: 1, Expr: `abc(?=!)`}}, WithBackend(BackendMVS), WithReportLocation(false), WithLogger(silentLogger{}))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	m := db.(*database).imageMVS()
	for _, procs := range []int{2, 3, 4} {
		runtime.GOMAXPROCS(procs)
		for _, size := range []int{511, 512, 513} {
			for _, inline := range []bool{false, true} {
				m.inlineSmallWorkers = inline
				data := bytes.Repeat([]byte{' '}, size)
				copy(data, "token123 abc! ")
				sc := &scratch{}
				if err := db.Scan(data, sc, nil); err != nil {
					t.Fatal(err)
				}
				if len(sc.gateTasks) == 0 || len(sc.anchorBatch) == 0 {
					t.Fatal("fixture missed gate or anchor worker")
				}
				workers := sc.mergedTasks != nil
				if workers != (!inline || size >= 512 || procs < 4) {
					t.Fatalf("P%d size%d inline%v workers%v", procs, size, inline, workers)
				}
				sc.Close()
			}
		}
	}
}
