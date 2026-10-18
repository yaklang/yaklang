//go:build cgo

package minirehs

import (
	"errors"
	"fmt"
	"reflect"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
)

// This exercises the Go bridge/owner replay with injected native statuses.
// Actual malloc/calloc failures in the C kernel are tested separately with a
// frontend-generated blob; a status hook is not a native allocation test.
func TestNativeExecutionErrorRoutes(t *testing.T) {
	oldProcs := runtime.GOMAXPROCS(0)
	oldPrecheck := gateSupersetPrecheck
	oldAnchorBatch := anchorCBatchEnabled
	gateSupersetPrecheck = true
	defer func() {
		runtime.GOMAXPROCS(oldProcs)
		gateSupersetPrecheck = oldPrecheck
		anchorCBatchEnabled = oldAnchorBatch
	}()
	type route struct {
		name, expr, stage           string
		more                        []Pattern
		locate, global, anchorBatch bool
		procs                       []int
	}
	cases := []route{
		{name: "candidate_batch", expr: `[ab]*abc[ab]*`, stage: "exists", procs: []int{1, 2}},
		{name: "window_located", expr: `abc[0-9]{2}`, stage: "exists", locate: true, procs: []int{1, 2}},
		{name: "anchored", expr: `abc[0-9]+`, stage: "anchored", procs: []int{1, 2}},
		{name: "anchored_many", expr: `abc[0-9]+`, stage: "anchored_many", anchorBatch: true, procs: []int{1, 2}},
		{name: "assert_single", expr: `\b[ab]{3}\b`, stage: "assert", procs: []int{1, 2}},
		{name: "assert_wide", expr: `\b[ab]{600}\b`, stage: "assert_online", procs: []int{1, 2}},
		{name: "merged", expr: `[ab]{600}`, stage: "merged", global: true, procs: []int{1, 2}},
		{name: "locate", expr: `[ab]{3}`, stage: "locate", locate: true, procs: []int{1, 2}},
		{name: "gated_worker", expr: `abc[ab]{600}(?=!)`, stage: "exists", procs: []int{1, 2}},
		{name: "combined", expr: `[ab]{3}`, stage: "combined", more: []Pattern{{ID: 7, Expr: `\b[ab]{3}\b`}}, global: true, procs: []int{1}},
		{name: "async_merged", expr: `[ab]{3}`, stage: "merged", more: []Pattern{{ID: 7, Expr: `\b[ab]{3}\b`}}, global: true, procs: []int{2}},
		{name: "async_assert", expr: `\b[ab]{3}\b`, stage: "assert_many", more: []Pattern{{ID: 7, Expr: `[ab]{3}`}}, procs: []int{2}},
	}
	data := []byte("token abc42 aaa abc" + strings.Repeat("a", 600) + "! " + strings.Repeat("a", 600) + "!")
	for _, tc := range cases {
		for _, procs := range tc.procs {
			for _, loaded := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/P%d/load%v", tc.name, procs, loaded), func(t *testing.T) {
					runtime.GOMAXPROCS(procs)
					anchorCBatchEnabled = tc.anchorBatch
					patterns := append([]Pattern{{ID: 99, Expr: `token`}, {ID: 0, Expr: tc.expr}}, tc.more...)
					db, err := Compile(patterns, WithBackend(BackendMVS), WithReportLocation(tc.locate), WithLogger(silentLogger{}))
					if err != nil {
						t.Fatal(err)
					}
					if loaded {
						image, imageErr := db.MarshalImage()
						db.Close()
						if imageErr != nil {
							t.Fatal(imageErr)
						}
						db, err = LoadImage(image)
						if err != nil {
							t.Fatal(err)
						}
					}
					defer db.Close()
					m := db.(*database).imageMVS()
					if m.kernel == nil {
						t.Fatal("native backend unavailable")
					}
					m.pf = programOrderedHits{prefilter: m.pf}
					sc, err := db.NewScratch()
					if err != nil {
						t.Fatal(err)
					}
					defer sc.Close()
					var baseline []Match
					if err := db.Scan(data, sc, func(match Match) bool { baseline = append(baseline, match); return true }); err != nil {
						t.Fatal(err)
					}
					if len(baseline) < 2 || baseline[0].ID != 99 {
						t.Fatalf("missing ordered baseline: %v", baseline)
					}
					for _, code := range []int{-1, -2, -3} {
						var reached atomic.Int64
						m.kernel.testStatus = func(stage string, idx, status int) int {
							if stage == tc.stage && (tc.global || idx == 1) {
								reached.Add(1)
								return code
							}
							return status
						}
						for _, early := range []bool{false, true, false} {
							var got []Match
							failure := db.Scan(data, sc, func(match Match) bool { got = append(got, match); return !early })
							if len(got) == 0 || len(got) > len(baseline) || !reflect.DeepEqual(got, baseline[:len(got)]) {
								t.Fatalf("invalid callback prefix: %v baseline=%v", got, baseline)
							}
							if early {
								if failure != nil || len(got) != 1 {
									t.Fatalf("earlier stop observed a later failure: %v %v", got, failure)
								}
								continue
							}
							var native *ScanError
							if !errors.As(failure, &native) || native.NativeCode == nil || *native.NativeCode != code || !native.Incomplete || native.Record != -1 || native.Stage != "native_"+tc.stage || native.HasRule == tc.global || !tc.global && native.RuleID != 0 {
								t.Fatalf("lost native failure attributes: %#v / %v", native, failure)
							}
							wantCategory := map[int]string{-1: "invalid_dispatch", -2: "overflow", -3: "out_of_memory"}[code]
							if native.Category != wantCategory {
								t.Fatalf("category=%s want=%s", native.Category, wantCategory)
							}
						}
						if reached.Load() == 0 {
							t.Fatalf("test never reached %s", tc.stage)
						}
						m.kernel.testStatus = nil // every Scan has already joined its readers
						var recovered []Match
						if err := db.Scan(data, sc, func(match Match) bool { recovered = append(recovered, match); return true }); err != nil || !reflect.DeepEqual(recovered, baseline) {
							t.Fatalf("scratch not reusable after native error: %v", err)
						}
					}
				})
			}
		}
	}
}

func TestNativeInvalidDispatcherIsNotNoMatch(t *testing.T) {
	db, err := Compile([]Pattern{{ID: 0, Expr: `abc[0-9]{2}`}}, WithBackend(BackendMVS), WithLogger(silentLogger{}))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	m := db.(*database).imageMVS()
	// A structurally valid native DB with no executable member: this produces
	// the real C dispatch code -1, without a status hook.
	m.kernel.close()
	m.kernel = openMVSKernel(buildMVSBlob([]*mvsNFA{nil}, nil, nil, nil), 1)
	if m.kernel == nil {
		t.Fatal("could not open dispatch fixture")
	}
	var got []Match
	err = db.Scan([]byte("abc42"), nil, func(match Match) bool { got = append(got, match); return true })
	var failure *ScanError
	if !errors.As(err, &failure) || failure.RuleID != 0 || !failure.HasRule || failure.NativeCode == nil || *failure.NativeCode != -1 || len(got) != 0 {
		t.Fatalf("C dispatcher failure flattened: %v %v", got, err)
	}
}

func TestNativeBatchFailureRecordAndEarlyStop(t *testing.T) {
	old := runtime.GOMAXPROCS(0)
	defer runtime.GOMAXPROCS(old)
	for _, procs := range []int{1, 2} {
		runtime.GOMAXPROCS(procs)
		db, err := Compile([]Pattern{{ID: 99, Expr: `token`}, {ID: 0, Expr: `[ab]*abc[ab]*`}},
			WithBackend(BackendMVS), WithReportLocation(false), WithLogger(silentLogger{}))
		if err != nil {
			t.Fatal(err)
		}
		m := db.(*database).imageMVS()
		m.pf = programOrderedHits{prefilter: m.pf}
		m.kernel.testStatus = func(stage string, idx, status int) int {
			if stage == "exists" && idx == 1 {
				return -3
			}
			return status
		}
		sc, err := db.NewScratch()
		if err != nil {
			t.Fatal(err)
		}
		records := make([][]byte, 17)
		for i := range records {
			records[i] = []byte("token abc")
		}
		for _, early := range []bool{false, true, false} {
			calls := 0
			err := db.ScanBatch(records, sc, func(record int, match Match) bool {
				calls++
				if record != 0 || match.ID != 99 {
					t.Errorf("callback beyond failing record: record=%d match=%v", record, match)
				}
				return !early
			})
			var failure *ScanError
			if early && err != nil || !early && (!errors.As(err, &failure) || failure.Record != 0 || failure.RuleID != 0 || failure.NativeCode == nil || *failure.NativeCode != -3) || calls != 1 {
				t.Fatalf("P%d early=%v calls=%d failure=%v", procs, early, calls, err)
			}
		}
		sc.Close()
		db.Close()
	}
}
