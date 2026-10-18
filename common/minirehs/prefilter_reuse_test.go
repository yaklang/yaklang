//go:build cgo

package minirehs

import (
	"bytes"
	"reflect"
	"sort"
	"testing"
)

func TestCGOPrefilterDenseScratchReuse(t *testing.T) {
	for _, literals := range [][]string{{"api-key", "api-key=", "key="}, {"api-key", "api-key=", "key=", "@"}} {
		li := liFromLiterals(literals)
		pf := newCGOPrefilter(li)
		if pf == nil {
			t.Fatal("C prefilter unavailable")
		}
		defer pf.release()
		sc := &scratch{}
		for _, data := range [][]byte{
			bytes.Repeat([]byte("api-key=@ "), 3000),
			[]byte("api-key=@ "),
			bytes.Repeat([]byte("api-key=@ "), 3000),
			nil, []byte("unrelated"),
		} {
			got := pf.scanHits(data, sc)
			want := newScalarPrefilter(li).scanHits(data, &scratch{})
			if !reflect.DeepEqual(normalizedHits(got), normalizedHits(want)) {
				t.Fatalf("bytes=%d hits=%d want=%d", len(data), len(got), len(want))
			}
			if len(data) > 0 && len(sc.cpairs) != cap(sc.cpairs)/2*2 {
				t.Fatal("retained output capacity was hidden from C")
			}
		}
		// A large no-hit record must not allocate in proportion to the record
		// length merely because the warm path can use a larger existing buffer.
		fresh := &scratch{}
		if got := pf.scanHits(bytes.Repeat([]byte("z"), 1<<20), fresh); len(got) != 0 {
			t.Fatal("unexpected literal hit")
		}
		if cap(fresh.cpairs) > 2*cgoPrefilterInitialPairCapacity {
			t.Fatal("fresh scratch lost its bounded initial allocation")
		}
	}
}

func normalizedHits(hits []litHit) []litHit {
	out := append([]litHit(nil), hits...)
	// Literal scanners may order equal end positions differently.
	sort.Slice(out, func(i, j int) bool {
		if out[i].end != out[j].end {
			return out[i].end < out[j].end
		}
		return out[i].litID < out[j].litID
	})
	return out
}
