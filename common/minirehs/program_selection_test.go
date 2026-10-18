package minirehs

import (
	"bytes"
	"fmt"
	"runtime"
	"sort"
	"strings"
	"testing"
)

type programOrderedHits struct {
	prefilter
	descending bool
}

func (p programOrderedHits) scanHits(data []byte, sc *scratch) []litHit {
	hits := append([]litHit(nil), p.prefilter.scanHits(data, sc)...)
	sort.SliceStable(hits, func(i, j int) bool {
		if p.descending {
			return hits[i].end > hits[j].end
		}
		return hits[i].end < hits[j].end
	})
	return hits
}

func TestGateMinimumEndIgnoresEventOrder(t *testing.T) {
	patterns := []Pattern{{ID: 17, Expr: `(?:\A1fooAAAAAAAAAAAA(?=Q)|[0-9]{3}foo(?=A))`}}
	oldProcs := runtime.GOMAXPROCS(0)
	defer runtime.GOMAXPROCS(oldProcs)
	for _, procs := range []int{1, 2} {
		runtime.GOMAXPROCS(procs)
		for _, locate := range []bool{false, true} {
			t.Run(fmt.Sprintf("procs=%d/loc=%v", procs, locate), func(t *testing.T) {
				db, image := compileImageTest(t, patterns, locate)
				loaded, err := LoadImage(image)
				if err != nil {
					t.Fatal(err)
				}
				defer loaded.Close()
				for _, current := range []Database{db, loaded} {
					m := current.(*database).imageMVS()
					if !m.hasLocalizedGates || m.gateHead[0] < 0 {
						t.Fatal("test did not enter a localized gate")
					}
					baseFilter, baseKernel := m.pf, m.kernel
					defer func() { m.pf, m.kernel = baseFilter, baseKernel }()
					for _, goOnly := range []bool{false, true} {
						m.kernel = baseKernel
						if goOnly {
							m.kernel = nil
						}
						for _, descending := range []bool{false, true} {
							m.pf = programOrderedHits{prefilter: baseFilter, descending: descending}
							for _, data := range []string{"001fooAAAAAAAAAAAA", "1fooAAAAAAAAAAAAQ", "zz1fooAAAAAAAAAAAA", "", "z 001fooA"} {
								for _, stop := range []int{0, 1} {
									got := programScan(t, current, []byte(data), stop)
									want := programOracle(patterns, []byte(data), BackendMVS, locate, stop)
									if err := programCompare(got, want, true); err != nil {
										t.Fatalf("Go=%v descending=%v data=%q: %s", goOnly, descending, data, err)
									}
								}
							}
						}
					}
					m.pf, m.kernel = baseFilter, baseKernel
				}
			})
		}
	}
}

func TestProgramPCREContextAndInvalidUTF(t *testing.T) {
	for _, expr := range []string{
		`\Gfoo`, `foo\Kbar`, `(?<=left)foo`, `foo(?=right)`,
		`a(*SKIP)(*F)|foo`, `\Afoo(?=!)`, `^foo(?=!)`, `(?m)^foo(?=!)`,
		`(?i)kfoo(?=!)`, `(?i)k\w{1,3}foo(?=!)`,
	} {
		t.Run(expr, func(t *testing.T) {
			patterns := []Pattern{{ID: 17, Expr: expr}}
			for _, locate := range []bool{false, true} {
				db, image := compileImageTest(t, patterns, locate)
				loaded, err := LoadImage(image)
				if err != nil {
					t.Fatal(err)
				}
				defer loaded.Close()
				inputs := [][]byte{
					nil, []byte("foo"), []byte("foobar"), []byte("leftfoo"), []byte("fooright"),
					[]byte("a foo"), []byte("x\nfoo!"), []byte("foo!"), []byte("Kfoo!"), []byte("Kéfoo!"),
					{'\xff', 'f', 'o', 'o', '!'}, {'f', 'o', 'o', '\xff', '!'},
					{0xc0, 0xaf, 'f', 'o', 'o', '!'}, {0xe2, 0x82, 'f', 'o', 'o', '!'},
					append(bytes.Repeat([]byte(" "), 1024), []byte("foo! leftfoo foobar fooright")...),
				}
				for _, current := range []Database{db, loaded} {
					if !gateContextCanLocalize(expr) && current.(*database).imageMVS().gateHead[0] >= 0 {
						t.Fatal("origin-sensitive expression received a localized gate")
					}
					for _, data := range inputs {
						for _, stop := range []int{0, 1} {
							got := programScan(t, current, data, stop)
							want := programOracle(patterns, data, BackendMVS, locate, stop)
							if err := programCompare(got, want, true); err != nil {
								t.Fatalf("locate=%v input=%q: %s", locate, data, err)
							}
						}
					}
				}
			}
		})
	}
}

func TestEngineGlobalFindAllSelection(t *testing.T) {
	for _, expr := range []string{`aa`, `a{2,3}`, `aa|aaa`, `(?:ab){1,3}`, `(?i)ss`, `\baa\b`} {
		for _, locate := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/loc=%v", expr, locate), func(t *testing.T) {
				patterns := []Pattern{{ID: 17, Expr: expr}}
				db, err := Compile(patterns, WithBackend(BackendEngine), WithReportLocation(locate), WithLogger(silentLogger{}))
				if err != nil {
					t.Fatal(err)
				}
				defer db.Close()
				for _, data := range []string{"", "aaaaaaaaaa", "xaaaaaaaaaax", strings.Repeat("ab", 40), "ſſſsss", " aa aa "} {
					for _, stop := range []int{0, 1, 2, 3} {
						got := programScan(t, db, []byte(data), stop)
						want := programOracle(patterns, []byte(data), BackendEngine, locate, stop)
						if err := programCompare(got, want, true); err != nil {
							t.Fatalf("data=%q stop=%d: %s", data, stop, err)
						}
					}
				}
			})
		}
	}
}
