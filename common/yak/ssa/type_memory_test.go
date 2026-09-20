package ssa

import (
	"crypto/sha256"
	"fmt"
	"math"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestTypeFingerprintsPages(t *testing.T) {
	p := make(typeFingerprints)
	ids := []int64{1, 31, 32, 63, 64, math.MaxInt64}
	for _, id := range ids {
		_, ok := p.get(id)
		require.False(t, ok)
		fingerprint := sha256.Sum256([]byte(fmt.Sprint(id)))
		p.set(id, fingerprint)
		got, ok := p.get(id)
		require.True(t, ok)
		require.Equal(t, fingerprint, got)
	}
	require.Len(t, p, 4, "sparse IDs must allocate only their own page")
	_, ok := p.get(2)
	require.False(t, ok, "an allocated page does not imply a persisted type")
	p.set(1, [sha256.Size]byte{})
	got, ok := p.get(1)
	require.True(t, ok, "zero fingerprint is distinct from missing")
	require.Equal(t, [sha256.Size]byte{}, got)
}

func TestFullTypeNamesBoundedAndOwned(t *testing.T) {
	names := make([]string, 10000)
	for i := range names {
		names[i] = fmt.Sprintf("pkg.Type%d", i/2)
	}
	var target []string
	require.True(t, fullTypeNameSet(&target, names, nil))
	require.Equal(t, clean(names)[:maxFullTypeNameEntries], target)
	require.LessOrEqual(t, cap(target), 256, "truncation must not retain the unbounded input backing array")
	names[0] = "mutated"
	require.Equal(t, "pkg.Type0", target[0])
	require.False(t, fullTypeNameSet(&target, append([]string(nil), target...), nil))
	// Set historically retains empty strings; Add ignores them. Preserve this.
	require.True(t, fullTypeNameSet(&target, []string{"", "a", "", "b", "a"}, nil))
	require.Equal(t, []string{"", "a", "b"}, target)
	target[2] = "a"
	require.True(t, fullTypeNameSet(&target, target, nil))
	require.Equal(t, []string{"", "a"}, target)
	require.True(t, fullTypeNameSet(&target, nil, nil))
	require.Nil(t, target)
	require.False(t, fullTypeNameSet(&target, []string{}, nil))
}

func BenchmarkTypeFingerprintStorage(b *testing.B) {
	for _, paged := range []bool{false, true} {
		b.Run(fmt.Sprintf("paged=%t", paged), func(b *testing.B) {
			b.ReportAllocs()
			for n := 0; n < b.N; n++ {
				if paged {
					p := make(typeFingerprints)
					for id := int64(1); id <= 100000; id++ {
						p.set(id, [sha256.Size]byte{1})
					}
					if v, ok := p.get(100000); !ok || v[0] != 1 {
						b.Fatal("missing")
					}
				} else {
					p := make(map[int64][sha256.Size]byte)
					for id := int64(1); id <= 100000; id++ {
						p[id] = [sha256.Size]byte{1}
					}
					if p[100000][0] != 1 {
						b.Fatal("missing")
					}
				}
			}
		})
	}
}

func BenchmarkFullTypeNameSet(b *testing.B) {
	names := make([]string, 10000)
	for i := range names {
		names[i] = fmt.Sprintf("T%d", i/2)
	}
	b.Run("legacy", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			result := clean(names)
			if len(result) < 200 {
				b.Fatal("missing")
			}
			result = result[:200]
		}
	})
	b.Run("bounded", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			var target []string
			fullTypeNameSet(&target, names, nil)
			if len(target) != 200 {
				b.Fatal("missing")
			}
		}
	})
	b.Run("unchanged-small", func(b *testing.B) {
		target := []string{"a", "b", "c"}
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			fullTypeNameSet(&target, target, nil)
		}
	})
}
