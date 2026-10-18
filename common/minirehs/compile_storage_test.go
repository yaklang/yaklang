package minirehs

import (
	"bytes"
	"fmt"
	"math/rand"
	"reflect"
	"strings"
	"testing"
)

func TestMergedStorageRows(t *testing.T) {
	rng := rand.New(rand.NewSource(1212))
	for _, p := range []int{1, 16, 63, 64, 65, 127, 128, 129, 512} {
		for _, mode := range []string{"empty", "chain", "sparse", "dense"} {
			m := &mvsMergedNFA{npos: p, nword: (p + 63) / 64, follow: make([][]uint64, p)}
			for i := range m.follow {
				row := bsNew(m.nword)
				if mode != "empty" && i+1 < p {
					bsSet(row, i+1)
				}
				if mode == "dense" || mode == "sparse" && i%11 == 0 {
					for q := 0; q < p; q++ {
						if rng.Intn(3) == 0 {
							bsSet(row, q)
						}
					}
				}
				m.follow[i] = row
			}
			want := storageReferenceLimEx(m)
			got := buildLimEx(m)
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("P%d/%s derived row differs", p, mode)
			}
			for i, row := range got.excFollow {
				rebuilt := bsNew(m.nword)
				copy(rebuilt, row)
				if i+1 < p && bsTest(got.chainTarget, i+1) {
					bsSet(rebuilt, i+1)
				}
				if !reflect.DeepEqual(rebuilt, m.follow[i]) {
					t.Fatal("chain/exception decomposition lost an original edge")
				}
				if row != nil && len(row) != cap(row) {
					t.Fatal("row append can edit adjacent storage")
				}
				_ = append(row, 123)
			}
			if !reflect.DeepEqual(got, want) || !reflect.DeepEqual(buildLimEx(m), want) {
				t.Fatal("append or repeated derivation changed source/adjacent rows")
			}
		}
	}
	if buildLimEx(nil) != nil {
		t.Fatal("nil merged input")
	}
}

func TestEncodedStorageSize(t *testing.T) {
	// Exercise every optional extension independently of producer eligibility.
	for bits := 0; bits < 8; bits++ {
		u := mvsUnit{npos: 3, nword: 1, nsym: 2, hasAssert: bits&1 != 0, hasLimEx: bits&2 != 0, hasDFA: bits&4 != 0,
			firstUnanchored: []uint64{1}, firstAnchored: []uint64{2}, lastAny: []uint64{4}, lastEnd: []uint64{4},
			follow: []uint64{2, 4, 0}, reach: []uint64{3, 4}, cuts: []int32{0, 1, 2}, asciiSym: []int32{0, 1}, posPat: []int32{-1, -1, 0},
			chainTarget: []uint64{6}, excMask: []uint64{1}, excFollow: []uint64{4, 0, 0},
			chainTarget1: 6, excMask1: 1, excFollow1Flat: []uint64{4, 0, 0}, condFollowMask1: 2,
			condFirstGuard: []byte{1, 2}, condFirstBits: []uint64{1, 2}, condFollowPos: []int32{1}, condFollowGuard: []byte{3}, condFollowBits: []uint64{4}, condAcceptGuard: []byte{4}, condAcceptBits: []uint64{4},
			dfaNstates: 2, dfaNext: []int32{0, 1, -1}, dfaAccept: []byte{0, 1}}
		want, got := storageReferenceUnit(u), encodeUnit(u)
		if !bytes.Equal(want, got) || len(got) != encodedUnitSize(u) || len(got) != cap(got) {
			t.Fatalf("extension bits%d size/output differs", bits)
		}
		b := make([]byte, 3, 3+encodedUnitSize(u))
		copy(b, "pre")
		base := &b[0]
		b = appendMVSUnit(b, u)
		if &b[0] != base || string(b[:3]) != "pre" || !bytes.Equal(b[3:], want) {
			t.Fatal("appended unit grew or edited earlier bytes")
		}
	}
	for _, args := range [][3]int{{int(^uint(0) >> 1), 1, 1}, {16, int(^uint(0) >> 1), 8}, {0, -1, 8}} {
		func() {
			defer func() {
				if recover() == nil {
					t.Error("encoded size arithmetic must not wrap")
				}
			}()
			addMVSEncodedSize(args[0], args[1], args[2])
		}()
	}
}

func TestCompiledStorageBytesAndOracle(t *testing.T) {
	pats := []Pattern{{ID: 0, Expr: `[ab][cd]`}, {ID: 1, Expr: `\b[a-z]{3}\b`}, {ID: 2, Expr: `[ab]{65}`}, {ID: 3, Expr: `(?:atoken|bword){2}`}, {ID: 4, Expr: `^\p{Han}{2}$`}, {ID: 5, Expr: `\b` + strings.Repeat(`[ab]`, 129) + `\b`}}
	for _, locate := range []bool{false, true} {
		db, err := Compile(pats, WithBackend(BackendMVS), WithReportLocation(locate), WithLogger(silentLogger{}))
		if err != nil {
			t.Fatal(err)
		}
		m := db.(*database).imageMVS()
		if !bytes.Equal(m.blob, storageReferenceBlob(m.nfas, m.merged, m.assertNecFactor, m.hasLiterals)) || len(m.blob) != cap(m.blob) {
			t.Fatal("direct final encoding changed native bytes or grew capacity")
		}
		image, err := db.MarshalImage()
		if err != nil {
			t.Fatal(err)
		}
		loaded, err := LoadImage(image)
		if err != nil {
			t.Fatal(err)
		}
		for _, data := range [][]byte{nil, []byte("c ac abc atokenbword"), []byte(strings.Repeat("a", 129)), []byte("中文"), {0xff, 0, 'b', 'c'}} {
			want := programOracle(pats, data, BackendMVS, locate, 0)
			for _, d := range []Database{db, loaded} {
				if err := programCompare(programScan(t, d, data, 0), want, false); err != nil {
					t.Fatal(err)
				}
			}
			for _, stop := range []int{1, 2} {
				if err := programCompare(programScan(t, db, data, stop), programScan(t, loaded, data, stop), true); err != nil {
					t.Fatal(err)
				}
			}
		}
		loaded.Close()
		db.Close()
	}
}

func TestProgramStorageCensus(t *testing.T) {
	dir := programEvidenceDir(t)
	for _, set := range []string{"GetRequest", "MITM"} {
		pats := getRequestImagePatterns(t)
		if set == "MITM" {
			pats, _ = compilableMITMPatterns(t)
		}
		db, err := Compile(pats, WithBackend(BackendMVS), WithLogger(silentLogger{}))
		if err != nil {
			t.Fatal(err)
		}
		m := db.(*database).imageMVS()
		oldBlob := storageReferenceBlob(m.nfas, m.merged, m.assertNecFactor, m.hasLiterals)
		if !bytes.Equal(oldBlob, m.blob) {
			t.Fatal("actual blob byte difference")
		}
		p, w, r := 0, 0, 0
		if m.merged != nil {
			p, w = m.merged.npos, m.merged.nword
			r = buildLimEx(m.merged).excCount
		}
		writeProgramJSON(t, dir, fmt.Sprintf("%s-storage.json", set), map[string]interface{}{"set": set, "patterns": len(pats), "blob_bytes": len(m.blob), "blob_cap": cap(m.blob), "merged_positions": p, "merged_words": w, "merged_retained_rows": r, "old_merged_row_clone_bytes": p * w * 8, "new_merged_retained_bytes": r * w * 8, "note": "Only Go merged derivation and temporary serialized unit copies changed; C tables, flattened Go units and persistent dense blob remain."})
		db.Close()
	}
}
