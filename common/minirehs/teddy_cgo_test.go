//go:build cgo

package minirehs

import (
	"fmt"
	"math/rand"
	"testing"
)

// 本文件验证真正的 Teddy SIMD 多字面量预过滤 (native/teddy.c): SIMD 分发结果必须与标量孪生
// 逐项一致, 且与独立的纯 Go Aho-Corasick (scalarPrefilter) 命中集合完全相等. 预过滤允许假阳、
// 绝无假阴, 故 Teddy(指纹+confirm) 与 AC 应产出同一精确命中集.

func liFromLiterals(lits []string) *literalIndex {
	cps := make([]*compiledPattern, 0, len(lits))
	for i, l := range lits {
		cps = append(cps, &compiledPattern{id: PatternID(i + 1), idx: i, literals: []string{l}})
	}
	return buildLiteralIndex(cps)
}

func hitSet(hits []litHit) map[litHit]struct{} {
	m := make(map[litHit]struct{}, len(hits))
	for _, h := range hits {
		m[h] = struct{}{}
	}
	return m
}

func assertSameHits(t *testing.T, got, want map[litHit]struct{}, ctx string) {
	t.Helper()
	for h := range want {
		if _, ok := got[h]; !ok {
			t.Errorf("%s: MISSING hit litID=%d end=%d", ctx, h.litID, h.end)
		}
	}
	for h := range got {
		if _, ok := want[h]; !ok {
			t.Errorf("%s: EXTRA hit litID=%d end=%d", ctx, h.litID, h.end)
		}
	}
}

// TestTeddyEnabled 验证字面量长度 >=2 时启用 Teddy, 指纹长度 = min(最短字面量, 4).
func TestTeddyEnabled(t *testing.T) {
	cases := []struct {
		lits []string
		m    int
	}{
		{[]string{"abc", "defg", "hijkl"}, 3},
		{[]string{"ab", "abcdef"}, 2},
		{[]string{"abcdef", "ghijkl", "mnopqr"}, 4},
		{[]string{"swagger-ui.html", "rememberme="}, 4},
	}
	for _, c := range cases {
		pf := newCGOPrefilter(liFromLiterals(c.lits))
		if pf == nil {
			t.Fatalf("nil prefilter for %v", c.lits)
		}
		if !pf.useTeddy() {
			t.Errorf("expected Teddy enabled for %v", c.lits)
		}
		if pf.teddyM() != c.m {
			t.Errorf("lits %v: teddyM=%d want %d", c.lits, pf.teddyM(), c.m)
		}
		pf.release()
	}

	// 含长度 1 字面量 -> Teddy 关闭, 走 AC 回退 (仍正确).
	pf := newCGOPrefilter(liFromLiterals([]string{"a", "bcd"}))
	if pf == nil {
		t.Fatal("nil prefilter")
	}
	if pf.useTeddy() {
		t.Errorf("Teddy must be disabled when a length-1 literal exists")
	}
	pf.release()
}

func randLowerLiteral(r *rand.Rand) string {
	const alpha = "abcdefghijklmnopqrstuvwxyz0123456789_.-=:/@"
	n := 2 + r.Intn(7)
	b := make([]byte, n)
	for i := range b {
		b[i] = alpha[r.Intn(len(alpha))]
	}
	return string(b)
}

func randCorpusWith(r *rand.Rand, lits []string, n int) []byte {
	const alpha = "abcdefghijklmnopqrstuvwxyz0123456789 _.-=:/@\n\t"
	buf := make([]byte, 0, n)
	for len(buf) < n {
		if len(lits) > 0 && r.Intn(6) == 0 {
			buf = append(buf, lits[r.Intn(len(lits))]...)
			continue
		}
		if r.Intn(30) == 0 {
			buf = append(buf, byte(0x80+r.Intn(0x40))) // 非 ASCII 字节
			continue
		}
		buf = append(buf, alpha[r.Intn(len(alpha))])
	}
	return buf[:n]
}

// TestTeddyDifferentialRandom 随机字面量集 + 随机数据: Teddy SIMD == Teddy 标量孪生 == 纯 Go AC.
func TestTeddyDifferentialRandom(t *testing.T) {
	r := rand.New(rand.NewSource(0x7EDD7))
	teddySets, checks := 0, 0
	for iter := 0; iter < 400; iter++ {
		nlit := 1 + r.Intn(40)
		lits := make([]string, 0, nlit)
		for i := 0; i < nlit; i++ {
			lits = append(lits, randLowerLiteral(r))
		}
		li := liFromLiterals(lits)
		cpf := newCGOPrefilter(li)
		if cpf == nil {
			continue
		}
		gopf := newScalarPrefilter(li)
		if cpf.useTeddy() {
			teddySets++
		}
		for j := 0; j < 12; j++ {
			size := r.Intn(2048)
			data := randCorpusWith(r, lits, size)
			scA, scB, scC := &scratch{}, &scratch{}, &scratch{}
			simd := hitSet(cpf.scanHits(data, scA))
			scal := hitSet(cpf.scanHitsScalar(data, scB))
			goac := hitSet(gopf.scanHits(data, scC))
			ctx := fmt.Sprintf("iter=%d j=%d teddy=%v m=%d nlit=%d size=%d", iter, j, cpf.useTeddy(), cpf.teddyM(), nlit, size)
			assertSameHits(t, simd, scal, "simd-vs-scalar "+ctx)
			assertSameHits(t, simd, goac, "simd-vs-goAC "+ctx)
			checks++
			if t.Failed() {
				t.Fatalf("DIVERGE %s\nlits=%v\ndata=%q", ctx, lits, data)
			}
		}
		cpf.release()
	}
	t.Logf("teddy differential: %d checks, teddy-enabled sets=%d (all consistent)", checks, teddySets)
	if teddySets < 50 {
		t.Fatalf("too few Teddy-enabled sets (%d)", teddySets)
	}
}

// TestTeddyRealTrafficVsGoAC 用真实 MITM 规则提取的字面量 + 真实流量, 对照 Teddy 与纯 Go AC.
func TestTeddyRealTrafficVsGoAC(t *testing.T) {
	patterns, _ := compilableMITMPatterns(t)
	cfg := newDefaultConfig()
	var lits []string
	for _, p := range patterns {
		expr := buildExprWithFlags(p)
		ls := extractRequiredLiteralsApprox(expr, cfg.minLiteralLen)
		lits = append(lits, ls...)
	}
	if len(lits) == 0 {
		t.Skip("no literals extracted")
	}
	li := liFromLiterals(lits)
	cpf := newCGOPrefilter(li)
	if cpf == nil {
		t.Fatal("nil prefilter")
	}
	defer cpf.release()
	gopf := newScalarPrefilter(li)
	t.Logf("real literals=%d teddy=%v m=%d", len(li.literals), cpf.useTeddy(), cpf.teddyM())

	records, _ := loadCorpus(t)
	for i, rec := range records {
		scA, scB := &scratch{}, &scratch{}
		teddy := hitSet(cpf.scanHits(rec, scA))
		goac := hitSet(gopf.scanHits(rec, scB))
		assertSameHits(t, teddy, goac, fmt.Sprintf("record#%d", i))
		if t.Failed() {
			t.FailNow()
		}
	}
}
