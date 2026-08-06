//go:build cgo

package minirehs

import (
	"bytes"
	"strings"
	"testing"
)

// 本文件在 CGO 构建 (CGO_ENABLED=1, Teddy 默认化后无需额外 tag) 下编译, 专门覆盖 cgoPrefilter
// 的构造、扫描、缓冲扩容重扫与释放等本地路径, 把 CGO 构建的覆盖率也推到生产级.

func TestCGOPrefilterBasic(t *testing.T) {
	li := buildLiteralIndex([]*compiledPattern{
		{id: 1, idx: 0, literals: []string{"abc"}},
		{id: 2, idx: 1, literals: []string{"xyz"}},
	})
	pf := newCGOPrefilter(li)
	if pf == nil {
		t.Fatal("newCGOPrefilter returned nil for non-empty index")
	}
	defer pf.release()

	if !pf.simd() {
		t.Errorf("cgo prefilter must report simd=true")
	}

	sc := &scratch{}
	hits := pf.scanHits([]byte("__abc__xyz__ABC"), sc)
	if len(hits) != 3 {
		t.Fatalf("cgo scanHits expected 3 hits, got %d: %v", len(hits), hits)
	}

	// 空数据快速返回.
	if h := pf.scanHits(nil, sc); len(h) != 0 {
		t.Errorf("empty data must yield 0 hits, got %v", h)
	}
}

func TestCGOPrefilterRealloc(t *testing.T) {
	// 极稠密命中: 命中数远超初始 capPairs(len/8+64), 触发扩容重扫路径.
	li := buildLiteralIndex([]*compiledPattern{{id: 1, idx: 0, literals: []string{"ab"}}})
	pf := newCGOPrefilter(li)
	if pf == nil {
		t.Fatal("nil prefilter")
	}
	defer pf.release()

	reps := 3000
	data := []byte(strings.Repeat("ab", reps)) // 每个 "ab" 命中一次.
	sc := &scratch{}
	hits := pf.scanHits(data, sc)
	if len(hits) != reps {
		t.Fatalf("dense scan expected %d hits, got %d", reps, len(hits))
	}
	pairs := &sc.cpairs[0]
	hits = pf.scanHits(data, sc)
	if len(hits) != reps {
		t.Fatalf("reused dense scan expected %d hits, got %d", reps, len(hits))
	}
	if pairs != &sc.cpairs[0] {
		t.Fatal("dense scan did not reuse the exact-size pair buffer")
	}
}

func TestCGOPrefilterCleanLargeInputUsesBoundedInitialPairs(t *testing.T) {
	li := buildLiteralIndex([]*compiledPattern{{id: 1, idx: 0, literals: []string{"authorization"}}})
	pf := newCGOPrefilter(li)
	if pf == nil {
		t.Fatal("nil prefilter")
	}
	defer pf.release()

	sc := &scratch{}
	if hits := pf.scanHits(bytes.Repeat([]byte("z"), 256<<10), sc); len(hits) != 0 {
		t.Fatalf("clean scan unexpectedly matched: %v", hits)
	}
	if got, max := cap(sc.cpairs), cgoPrefilterInitialPairCapacity*2; got > max {
		t.Fatalf("clean scan pair buffer is not bounded: cap=%d max=%d", got, max)
	}
}

func TestCGOPrefilterEmptyFallback(t *testing.T) {
	// 空字面量索引 -> newCGOPrefilter 返回 nil; newPrefilter 退化到标量实现.
	if newCGOPrefilter(&literalIndex{}) != nil {
		t.Errorf("empty index must yield nil cgo prefilter")
	}
	pf := newPrefilter(&literalIndex{})
	if _, ok := pf.(*scalarPrefilter); !ok {
		t.Errorf("newPrefilter on empty index must fall back to scalar, got %T", pf)
	}
}

func TestCGOSimdReported(t *testing.T) {
	// SIMD 构建下, Compile 出的 db 应报告 simd=true / tier=2.
	db, err := Compile([]Pattern{{ID: 1, Expr: `foobar`}}, WithLogger(silentLogger{}))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	info := db.Info()
	if !info.SIMD || info.Tier != 2 {
		t.Errorf("cgo build expected simd=true tier=2, got simd=%v tier=%d", info.SIMD, info.Tier)
	}
}

func benchmarkCGOPrefilter256K(b *testing.B, coldScratch bool) {
	li := liFromLiterals([]string{
		"authorization", "api-key", "secret_access_key", "private-key",
		"access-token", "rememberme=", "swagger-ui.html", "x-auth-token",
	})
	pf := newCGOPrefilter(li)
	if pf == nil || !pf.useTeddy() {
		b.Fatal("expected Teddy-enabled cgo prefilter")
	}
	defer pf.release()

	chunk := []byte("GET /items?page=1 HTTP/1.1\r\nHost: example.test\r\nContent-Type: application/json\r\n\r\n{\"status\":\"ok\",\"items\":[]}")
	data := bytes.Repeat(chunk, 256*1024/len(chunk)+1)[:256*1024]
	b.SetBytes(int64(len(data)))
	b.ReportAllocs()
	b.ResetTimer()
	if coldScratch {
		for i := 0; i < b.N; i++ {
			_ = pf.scanHits(data, &scratch{})
		}
		return
	}
	sc := &scratch{}
	for i := 0; i < b.N; i++ {
		_ = pf.scanHits(data, sc)
	}
}

func BenchmarkCGOPrefilterWarmScratch256K(b *testing.B) {
	benchmarkCGOPrefilter256K(b, false)
}

func BenchmarkCGOPrefilterColdScratch256K(b *testing.B) {
	benchmarkCGOPrefilter256K(b, true)
}

func BenchmarkCGOPrefilterColdScratchCapacity256K(b *testing.B) {
	li := liFromLiterals([]string{
		"authorization", "api-key", "secret_access_key", "private-key",
		"access-token", "rememberme=", "swagger-ui.html", "x-auth-token",
	})
	pf := newCGOPrefilter(li)
	if pf == nil || !pf.useTeddy() {
		b.Fatal("expected Teddy-enabled cgo prefilter")
	}
	defer pf.release()

	chunk := []byte("GET /items?page=1 HTTP/1.1\r\nHost: example.test\r\nContent-Type: application/json\r\n\r\n{\"status\":\"ok\",\"items\":[]}")
	data := bytes.Repeat(chunk, 256*1024/len(chunk)+1)[:256*1024]
	for _, tc := range []struct {
		name            string
		maxInitialPairs int
	}{
		{name: "proportional", maxInitialPairs: 0},
		{name: "bounded", maxInitialPairs: cgoPrefilterInitialPairCapacity},
	} {
		b.Run(tc.name, func(b *testing.B) {
			b.SetBytes(int64(len(data)))
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_ = pf.scanHitsImplWithInitialPairCapacity(data, &scratch{}, false, tc.maxInitialPairs)
			}
		})
	}
}

func BenchmarkCGOPrefilterModerateHitsCapacity256K(b *testing.B) {
	li := liFromLiterals([]string{"api-key", "authorization", "secret_access_key"})
	pf := newCGOPrefilter(li)
	if pf == nil || !pf.useTeddy() {
		b.Fatal("expected Teddy-enabled cgo prefilter")
	}
	defer pf.release()

	const expectedHits = 4000
	data := bytes.Repeat([]byte("z"), 256<<10)
	for index := 0; index < expectedHits; index++ {
		copy(data[index*64:], "api-key")
	}
	for _, tc := range []struct {
		name            string
		maxInitialPairs int
	}{
		{name: "proportional", maxInitialPairs: 0},
		{name: "bounded", maxInitialPairs: cgoPrefilterInitialPairCapacity},
	} {
		b.Run(tc.name, func(b *testing.B) {
			b.SetBytes(int64(len(data)))
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				hits := pf.scanHitsImplWithInitialPairCapacity(data, &scratch{}, false, tc.maxInitialPairs)
				if len(hits) != expectedHits {
					b.Fatalf("expected %d hits, got %d", expectedHits, len(hits))
				}
			}
		})
	}
}
