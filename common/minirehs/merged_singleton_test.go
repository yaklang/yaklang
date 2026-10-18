//go:build cgo

package minirehs

import (
	"bytes"
	"fmt"
	"runtime"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
)

func TestMergedSingletonBoundaries(t *testing.T) {
	for _, positions := range []int{1, 16, 63, 64, 65, 127, 128, 129} {
		for _, locate := range []bool{false, true} {
			pats := []Pattern{{ID: 99, Expr: `token`}, {ID: 17, Expr: fmt.Sprintf(`[ab]{%d}`, positions)}}
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
				idx, ok := singleMergedMember(m)
				qualified := positions <= 64 && !locate
				if ok != qualified || (ok && (idx != 1 || m.kernel.mergedSingleton != idx)) || (!ok && m.kernel.mergedSingleton != -1) {
					t.Fatalf("P%d locate%v qualified=%v idx=%d kernel=%d", positions, locate, ok, idx, m.kernel.mergedSingleton)
				}
				for _, data := range [][]byte{nil, []byte("token a"), []byte("token " + strings.Repeat("ab", positions) + " \xff\x00K中"), {0xff, 0, 'a', 'b', 0xfe}} {
					for _, scalar := range []bool{true, false} {
						sc := &scratch{}
						hits, err := m.kernel.mergedScanImpl(data, sc, scalar)
						if err != nil {
							t.Fatal(err)
						}
						var expected []int
						if m.nfas[1].existsIn(data) {
							expected = []int{1}
						}
						if !slices.Equal(hits, expected) {
							t.Fatalf("P%d scalar%v: hits%v want%v", positions, scalar, hits, expected)
						}
						sc.Close()
					}
					want := programOracle(pats, data, BackendMVS, locate, 0)
					if err := programCompare(programScan(t, current, data, 0), want, false); err != nil {
						t.Fatal(err)
					}
					for _, stop := range []int{1, 2} {
						m.kernel.mergedSingleton = -1
						ref := programScan(t, current, data, stop)
						m.kernel.configureMergedSingleton(m)
						if err := programCompare(programScan(t, current, data, stop), ref, true); err != nil {
							t.Fatal(err)
						}
					}
				}
			}
			loaded.Close()
			db.Close()
		}
	}
}

func TestMergedSingletonQualificationRejectsDifferentTables(t *testing.T) {
	db, err := Compile([]Pattern{{ID: 17, Expr: `[ab]{3}`}}, WithBackend(BackendMVS), WithReportLocation(false), WithLogger(silentLogger{}))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	m := db.(*database).imageMVS()
	if _, ok := singleMergedMember(m); !ok {
		t.Fatal("positive qualification fixture did not qualify")
	}
	for _, mutate := range []func() func(){
		func() func() { m.mergedCount++; return func() { m.mergedCount-- } },
		func() func() { m.merged.follow[0][0] ^= 1; return func() { m.merged.follow[0][0] ^= 1 } },
		func() func() { m.merged.reach[0][0] ^= 1; return func() { m.merged.reach[0][0] ^= 1 } },
		func() func() { m.merged.firstUnanchored[0] ^= 1; return func() { m.merged.firstUnanchored[0] ^= 1 } },
		func() func() { m.merged.posPat[2] = -1; return func() { m.merged.posPat[2] = 0 } },
		func() func() { m.merged.asciiSym[0]++; return func() { m.merged.asciiSym[0]-- } },
		func() func() { m.legacyInvalidUTF = true; return func() { m.legacyInvalidUTF = false } },
	} {
		restore := mutate()
		if _, ok := singleMergedMember(m); ok {
			t.Fatal("mismatched/legacy plan incorrectly qualified")
		}
		restore()
		if _, ok := singleMergedMember(m); !ok {
			t.Fatal("restored qualification lost")
		}
	}
	for _, expr := range []string{`^[ab]{3}`, `[ab]{3}$`, `\b[ab]{3}\b`, `[ab]+(?=!)`} {
		other, err := Compile([]Pattern{{ID: 17, Expr: expr}}, WithBackend(BackendMVS), WithReportLocation(false), WithLogger(silentLogger{}))
		if err != nil {
			t.Fatal(err)
		}
		if _, ok := singleMergedMember(other.(*database).imageMVS()); ok {
			t.Fatalf("unsafe qualification: %s", expr)
		}
		other.Close()
	}
}

func TestMergedSingletonLateAssertionAndErrors(t *testing.T) {
	prior := runtime.GOMAXPROCS(4)
	defer runtime.GOMAXPROCS(prior)
	pats := []Pattern{{ID: 99, Expr: `token`}, {ID: 17, Expr: `[ab]{3}`}, {ID: 18, Expr: `\bZ[0-9]+\b`}}
	db, err := Compile(pats, WithBackend(BackendMVS), WithReportLocation(false), WithLogger(silentLogger{}))
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
	data := append([]byte("token abba "), bytes.Repeat([]byte("xx "), 256)...)
	data = append(data, []byte(" Z9")...)
	for _, current := range []Database{db, loaded} {
		m := current.(*database).imageMVS()
		if m.kernel.mergedSingleton != 1 || len(m.assertAlwaysOnCIdxs) != 1 {
			t.Fatal("fixture missed singleton or independent assertion")
		}
		for _, procs := range []int{1, 4} {
			runtime.GOMAXPROCS(procs)
			if err := programCompare(programScan(t, current, data, 0), programOracle(pats, data, BackendMVS, false, 0), false); err != nil {
				t.Fatal(err)
			}
		}
		runtime.GOMAXPROCS(4)
		var reached atomic.Int64
		m.kernel.testStatus = func(stage string, idx, status int) int {
			if stage == "merged" && idx == -1 {
				reached.Add(1)
				return -3
			}
			return status
		}
		for _, stop := range []int{0, 1, 2} {
			m.kernel.mergedSingleton = -1
			ref := programScan(t, current, data, stop)
			m.kernel.configureMergedSingleton(m)
			got := programScan(t, current, data, stop)
			if err := programCompare(got, ref, true); err != nil {
				t.Fatal(err)
			}
			if stop == 0 && (got.Error == nil || got.Error.Stage != "native_merged" || got.Error.HasRule || got.Completed) {
				t.Fatalf("lost original merged error attribution: %+v", got)
			}
			if stop == 1 && got.Error != nil {
				t.Fatalf("earlier token stop exposed later error: %+v", got)
			}
		}
		if reached.Load() == 0 {
			t.Fatal("error route was not executed")
		}
		m.kernel.testStatus = nil
	}
}

func TestProgramMergedSingletonEligibility(t *testing.T) {
	dir := programEvidenceDir(t)
	var rows []map[string]interface{}
	for _, set := range []string{"GetRequest", "MITM"} {
		pats := getRequestImagePatterns(t)
		if set == "MITM" {
			pats, _ = compilableMITMPatterns(t)
		}
		db, err := Compile(pats, WithBackend(BackendMVS), WithReportLocation(false), WithLogger(silentLogger{}))
		if err != nil {
			t.Fatal(err)
		}
		m := db.(*database).imageMVS()
		idx, ok := singleMergedMember(m)
		rows = append(rows, map[string]interface{}{"set": set, "patterns": len(pats), "members": m.mergedCount, "positions": m.merged.npos, "words": m.merged.nword, "qualified": ok, "index": idx, "kernel_index": m.kernel.mergedSingleton})
		if (set == "MITM") != ok || (ok && (m.all[idx].id != 28 || m.kernel.mergedSingleton != idx)) {
			t.Fatalf("actual %s did not meet expected coverage: idx%d eligible%v mergedS%d", set, idx, ok, m.merged.nsym)
		}
		db.Close()
	}
	writeProgramJSON(t, dir, "singleton-eligibility.json", rows)
}
