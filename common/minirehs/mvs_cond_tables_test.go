//go:build cgo

package minirehs

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func checkNativeConditionalTables(t *testing.T, k *mvsKernel, n *mvsNFA) {
	t.Helper()
	if n.nword != 1 {
		return
	}
	count := 0
	for _, entries := range n.condFollow {
		count += len(entries)
	}
	for truth := 0; truth < 64; truth++ {
		for pos := 0; pos < n.npos; pos++ {
			var want [4]uint64
			if count != 0 {
				want[0] = 1
			}
			for _, row := range []struct {
				entries []guardedBits
				slot    int
			}{{n.condFirst, 1}, {n.condFollow[pos], 2}, {n.condAccept, 3}} {
				for _, entry := range row.entries {
					if int(entry.g)&truth == int(entry.g) {
						want[row.slot] |= entry.bits[0]
					}
				}
			}
			got, ok := k.assertTables1(0, truth, pos)
			if !ok || got != want {
				t.Fatalf("P=%d truth=%d pos=%d got=%x want=%x ok=%v", n.npos, truth, pos, got, want, ok)
			}
		}
	}
}

func TestNativeConditionalTables(t *testing.T) {
	var exprs []string
	for _, p := range []int{1, 16, 63, 64, 65, 128} {
		exprs = append(exprs, `\b`+strings.Repeat("a", p)+`\b`)
	}
	exprs = append(exprs, `a\Bb`, `a\b!`, `(?:a{64}\b)+`)
	for _, expr := range exprs {
		t.Run(expr, func(t *testing.T) {
			n, ok := buildAssertNFA(t, expr)
			if !ok || !n.hasAssert {
				t.Fatal("must reach assertion NFA")
			}
			blob := buildMVSBlob([]*mvsNFA{n}, nil, nil, nil)
			k := openMVSKernel(blob, 1)
			if k == nil {
				t.Fatal("native open")
			}
			defer k.close()
			checkNativeConditionalTables(t, k, n)
			word := []byte(strings.Repeat("a", n.npos))
			inputs := [][]byte{nil, word, append([]byte(" "), append(bytes.Clone(word), ' ')...), append([]byte{0xff, 0}, append(bytes.Clone(word), 0xc0)...), []byte("ab a! aaaa")}
			for _, locate := range []bool{false, true} {
				patterns := []Pattern{{ID: 0, Expr: expr}, {ID: 2, Expr: `[ax]{2}`}}
				db, err := Compile(patterns, WithBackend(BackendMVS), WithReportLocation(locate), WithLogger(silentLogger{}))
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
				for _, src := range []Database{db, loaded} {
					m := src.(*database).imageMVS()
					checkNativeConditionalTables(t, m.kernel, m.nfas[0])
					for _, data := range inputs {
						got, want := programScan(t, src, data, 0), programOracle(patterns, data, BackendMVS, locate, 0)
						if err := programCompare(got, want, false); err != nil {
							t.Fatal(err)
						}
						// All single-word readers must tolerate the omitted table;
						// the online single-pattern API itself is multiword-only.
						a, e := m.kernel.nfaExistsAssertSelfErr(0, data, nil)
						b, f := m.kernel.nfaExistsAssertErr(0, data, computeBoundaries(data, nil))
						if e != nil || f != nil || a != n.existsIn(data) || b != a {
							t.Fatalf("native readers: %v %v %v %v", a, b, e, f)
						}
						if n.single {
							many := m.kernel.nfaExistsAssertMany([]int32{0}, data, &scratch{})
							_, combined, err := m.kernel.combinedScanErr(data, []int32{0}, false, &scratch{})
							if len(many) != 1 || int8(many[0]) < 0 || (many[0] != 0) != a || err != nil || (len(combined) != 0) != a {
								t.Fatalf("native many/combined: %v %v %v expected=%v", many, combined, err, a)
							}
						} else {
							c, err := m.kernel.nfaExistsAssertOnlineErr(0, data)
							if err != nil || c != a {
								t.Fatalf("multiword online: %v %v expected=%v", c, err, a)
							}
						}
						for _, stop := range []int{1, 2} {
							if err := programCompare(programScan(t, src, data, stop), programScan(t, db, data, stop), true); err != nil {
								t.Fatal(err)
							}
						}
					}
				}
				again, err := loaded.MarshalImage()
				if err != nil || !bytes.Equal(again, image) {
					t.Fatal("changed image on load", err)
				}
				loaded.Close()
				db.Close()
			}
		})
	}
}

// Wrap an actual frontend unit with the ordinary one-slot native header. The
// serializer stays production code; malformed fields are the only mutations.
func singleConditionalUnitBlob(u mvsUnit) []byte {
	raw := encodeUnit(u)
	b := append([]byte(nil), mvsBlobMagic[:]...)
	for _, v := range []uint32{mvsBlobVersion, 1, ^uint32(0), 1, 0, 32, uint32(len(raw))} {
		b = putU32(b, v)
	}
	return append(b, raw...)
}

func TestNativeConditionalMaskReject(t *testing.T) {
	for _, expr := range []string{`\baaa\b`, `a\Bb`, `(?:a{64}\b)+`} {
		n, ok := buildAssertNFA(t, expr)
		if !ok || !n.single {
			t.Fatal("assert frontend")
		}
		for _, change := range []string{"mask", "mask_zero", "pos_negative", "pos_out_of_range", "count_negative"} {
			u := unitFromAssertNFA(n, 0, necFactor{})
			switch change {
			case "mask":
				u.condFollowMask1 ^= uint64(1)
			case "mask_zero":
				if len(u.condFollowPos) == 0 {
					continue
				}
				u.condFollowMask1 = 0
			case "pos_negative":
				if len(u.condFollowPos) == 0 {
					continue
				}
				u.condFollowPos[0] = -1
			case "pos_out_of_range":
				if len(u.condFollowPos) == 0 {
					continue
				}
				u.condFollowPos[0] = int32(u.npos)
			}
			blob := singleConditionalUnitBlob(u)
			if change == "count_negative" {
				// Fixed table extent + assertion fixed fields + condFirst block.
				off := 32 + 16 + (4+u.npos+u.nsym)*u.nword*8 + (u.nsym+129+u.npos)*4 + (3+u.npos)*8 + 4 + len(u.condFirstGuard)*(1+8*u.nword)
				binary.LittleEndian.PutUint32(blob[off:], ^uint32(0))
			}
			if k := openMVSKernel(blob, 1); k != nil {
				k.close()
				t.Errorf("native accepted %s: %s", expr, change)
			}
			if _, err := decodeImageUnit(blob[32:], 1); !errors.Is(err, ErrCorruptImage) {
				t.Errorf("file decoder accepted %s: %s: %v", expr, change, err)
			}
		}
	}
}

func TestNativeConditionalConcurrent(t *testing.T) {
	db, err := Compile([]Pattern{{ID: 0, Expr: `\b[ab]{63}\b`}, {ID: 1, Expr: `[ab]{2}`}}, WithBackend(BackendMVS), WithReportLocation(false), WithLogger(silentLogger{}))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var wg sync.WaitGroup
	problems := make(chan error, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			sc, e := db.NewScratch()
			if e != nil {
				problems <- e
				return
			}
			defer sc.Close()
			for j := 0; j < 64; j++ {
				var got []Match
				e = db.Scan([]byte(" "+strings.Repeat("a", 63)+" "), sc, func(m Match) bool { got = append(got, m); return true })
				if e != nil || len(got) != 2 {
					problems <- fmt.Errorf("got=%v err=%v", got, e)
					return
				}
			}
		}()
	}
	wg.Wait()
	close(problems)
	for e := range problems {
		t.Error(e)
	}
}

func TestProgramConditionalFixtures(t *testing.T) {
	dir := programEvidenceDir(t)
	for _, p := range []int{1, 16, 63, 64, 65, 128} {
		n, ok := buildAssertNFA(t, `\b`+strings.Repeat("a", p)+`\b`)
		if !ok || n.npos != p {
			t.Fatal("fixture shape", p)
		}
		if err := os.WriteFile(filepath.Join(dir, fmt.Sprintf("assert-%d.blob", p)), buildMVSBlob([]*mvsNFA{n}, nil, nil, nil), 0600); err != nil {
			t.Fatal(err)
		}
	}
	for name, expr := range map[string]string{"mid": `a\Bb`, "bit63": `(?:a{64}\b)+`} {
		n, ok := buildAssertNFA(t, expr)
		if !ok || !n.single || n.condFollowMask1 == 0 {
			t.Fatal("nonzero follow fixture", name)
		}
		if err := os.WriteFile(filepath.Join(dir, "assert-follow-"+name+".blob"), buildMVSBlob([]*mvsNFA{n}, nil, nil, nil), 0600); err != nil {
			t.Fatal(err)
		}
	}
}
