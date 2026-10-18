package minirehs

import (
	"math/rand"
	"reflect"
	"testing"
)

func TestMVSScalarViewsPreserveTransitions(t *testing.T) {
	rng := rand.New(rand.NewSource(349))
	for _, npos := range []int{1, 2, 63, 64, 65, 127, 128, 255} {
		for _, dense := range []bool{false, true} {
			nw := (npos + 63) / 64
			n := &mvsNFA{npos: npos, nword: nw, nsym: 4,
				first: bsNew(nw), lastAny: bsNew(nw), lastEnd: bsNew(nw),
				follow: make([][]uint64, npos), reach: make([][]uint64, 4), condFollow: make([][]guardedBits, npos)}
			bsSet(n.first, 0)
			bsSet(n.lastAny, npos-1)
			bsSet(n.lastEnd, npos/2)
			for p := range n.follow {
				n.follow[p] = bsNew(nw)
				if p+1 < npos {
					bsSet(n.follow[p], p+1)
				}
				if dense || p%13 == 0 {
					for q := 0; q < npos; q++ {
						if rng.Intn(3) == 0 {
							bsSet(n.follow[p], q)
						}
					}
				}
				if p%7 == 0 {
					n.condFollow[p] = []guardedBits{{}}
				}
			}
			for s := range n.reach {
				n.reach[s] = bsNew(nw)
				for p := 0; p < npos; p++ {
					if rng.Intn(2) == 0 {
						bsSet(n.reach[s], p)
					}
				}
			}
			originalFollow := make([][]uint64, npos)
			for p := range n.follow {
				originalFollow[p] = append([]uint64(nil), n.follow[p]...)
			}
			// Reinitialization must be stable as well as preserving all edges.
			for repeat := 0; repeat < 2; repeat++ {
				n.initScalar()
				exceptions := 0
				for p, original := range originalFollow {
					if !reflect.DeepEqual(n.follow[p], original) {
						t.Fatal("initialization modified source transitions")
					}
					rebuilt := bsNew(nw)
					copy(rebuilt, n.excFollow[p])
					if p+1 < npos && bsTest(n.chainTarget, p+1) {
						bsSet(rebuilt, p+1)
					}
					if !reflect.DeepEqual(rebuilt, original) {
						t.Fatalf("npos=%d dense=%v position=%d lost transition", npos, dense, p)
					}
					if n.excFollow[p] != nil {
						exceptions++
						if !bsTest(n.excMask, p) || bsIsZero(n.excFollow[p]) {
							t.Fatal("exception mask disagrees with transitions")
						}
					}
					if n.single && (n.follow1[p] != original[0] || n.excFollow1[p] != rebuiltException(n.excFollow[p])) {
						t.Fatal("scalar and generic views disagree")
					}
				}
				if n.excCount != exceptions {
					t.Fatalf("npos=%d count=%d want=%d", npos, n.excCount, exceptions)
				}
				if n.single {
					if n.first1 != n.first[0] || n.lastAny1 != n.lastAny[0] || n.lastEnd1 != n.lastEnd[0] {
						t.Fatal("scalar start/accept views differ")
					}
					for s := range n.reach {
						if n.reach1[s] != n.reach[s][0] {
							t.Fatal("scalar alphabet view differs")
						}
					}
					for p := 0; p < npos; p++ {
						if (n.condFollowMask1&(uint64(1)<<uint(p)) != 0) != (len(n.condFollow[p]) > 0) {
							t.Fatal("conditional transition mask differs")
						}
					}
				}
			}
		}
	}
}

func rebuiltException(row []uint64) uint64 {
	if len(row) == 0 {
		return 0
	}
	return row[0]
}
