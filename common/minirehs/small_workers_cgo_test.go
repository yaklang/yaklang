//go:build cgo

package minirehs

import (
	"runtime"
	"sync/atomic"
	"testing"
)

func TestSmallWorkerNativeErrorOrder(t *testing.T) {
	prior := runtime.GOMAXPROCS(4)
	defer runtime.GOMAXPROCS(prior)
	patterns := []Pattern{{ID: 99, Expr: `token`}, {ID: 0, Expr: `abc[0-9]+`}}
	db, err := Compile(patterns, WithBackend(BackendMVS), WithReportLocation(false), WithLogger(silentLogger{}))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	image, err := db.MarshalImage()
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadImage(image)
	if err != nil {
		t.Fatal(err)
	}
	defer loaded.Close()
	data := []byte("token abc42 tail")
	for _, current := range []Database{db, loaded} {
		m := current.(*database).imageMVS()
		m.pf = programOrderedHits{prefilter: m.pf}
		var reached atomic.Int64
		m.kernel.testStatus = func(stage string, idx, status int) int {
			if stage == "anchored" && idx == 1 {
				reached.Add(1)
				return -3
			}
			return status
		}
		for _, stop := range []int{0, 1, 2} {
			m.inlineSmallWorkers = false
			want := programScan(t, current, data, stop)
			m.inlineSmallWorkers = true
			got := programScan(t, current, data, stop)
			if err := programCompare(got, want, true); err != nil {
				t.Fatal(err)
			}
			if stop == 0 && (got.Error == nil || got.Completed || len(got.Callbacks) != 1 || got.Callbacks[0].ID != 99) {
				t.Fatalf("missing observable failure/prefix: %+v", got)
			}
			if stop == 1 && (got.Error != nil || !got.Stopped) {
				t.Fatalf("earlier stop exposed a later error: %+v", got)
			}
		}
		if reached.Load() == 0 {
			t.Fatal("short native worker error was not exercised")
		}
		m.kernel.testStatus = nil
		if err := programCompare(programScan(t, current, data, 0), programOracle(patterns, data, BackendMVS, false, 0), false); err != nil {
			t.Fatal(err)
		}
	}
}
