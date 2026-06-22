//go:build minirehs_vectorscan

package minirehs

import (
	"fmt"
	"math/rand"
	"testing"
)

// 本文件验证 Vectorscan 后端的"存在性"语义与 stdlib oracle 一致 (逐记录命中的规则 ID 集合
// 完全相同), 并覆盖 regexp2-only 规则经 fallback 承载的路径。仅在 -tags minirehs_vectorscan
// 构建且运行时能加载 libhs 时运行, 否则跳过 (不视为失败, 体现"环境缺失即退化")。

func requireVectorscan(tb testing.TB) {
	tb.Helper()
	if !vectorscanAvailable() {
		tb.Skip("vectorscan/libhs not loadable at runtime; backend gracefully unavailable")
	}
}

// existIDs 扫描 data, 返回命中的 pattern ID 集合 (存在性, 忽略偏移与重复)。
func existIDs(tb testing.TB, db Database, data []byte) map[PatternID]struct{} {
	tb.Helper()
	sc, err := db.NewScratch()
	if err != nil {
		tb.Fatalf("new scratch: %v", err)
	}
	defer sc.Close()
	set := make(map[PatternID]struct{})
	if err := db.Scan(data, sc, func(m Match) bool {
		set[m.ID] = struct{}{}
		return true
	}); err != nil {
		tb.Fatalf("scan: %v", err)
	}
	return set
}

func assertSameIDSet(tb testing.TB, vs, oracle map[PatternID]struct{}, ctx string) {
	tb.Helper()
	for id := range oracle {
		if _, ok := vs[id]; !ok {
			tb.Errorf("%s: vectorscan MISSED rule id=%d (oracle matched)", ctx, id)
		}
	}
	for id := range vs {
		if _, ok := oracle[id]; !ok {
			tb.Errorf("%s: vectorscan EXTRA rule id=%d (oracle did not match)", ctx, id)
		}
	}
}

func TestVectorscanAvailable(t *testing.T) {
	if !vectorscanAvailable() {
		t.Skip("vectorscan/libhs not loadable; nothing to report")
	}
	t.Logf("vectorscan available, libhs version: %s", hsVersion())
}

// TestVectorscanGracefulDegradation 验证"环境不可用即退化": 通过 MINIREHS_HS_DISABLE 强制
// 模拟 libhs 不可加载, 此时请求 BackendVectorscan 必须优雅退化为引擎, 且功能完全正常,
// 绝不报错或崩溃 (这是"分发不崩溃"的核心保证)。
func TestVectorscanGracefulDegradation(t *testing.T) {
	requireVectorscan(t) // 仅在本可用的环境里才有意义地验证"被强制禁用后的退化"

	t.Setenv("MINIREHS_HS_DISABLE", "1")
	if vectorscanAvailable() {
		t.Fatal("MINIREHS_HS_DISABLE=1 should force vectorscan unavailable")
	}

	patterns := []Pattern{{ID: 1, Expr: `foobar`}, {ID: 2, Expr: `[0-9]{3,}`}}
	db, err := Compile(patterns, WithBackend(BackendVectorscan), WithLogger(silentLogger{}))
	if err != nil {
		t.Fatalf("compile must not fail on degradation: %v", err)
	}
	defer db.Close()
	if db.Info().Backend != BackendEngine {
		t.Fatalf("expected graceful fallback to engine, got %s", db.Info().Backend)
	}
	// 退化后功能正常.
	got := existIDs(t, db, []byte("xx foobar 12345"))
	if _, ok := got[1]; !ok {
		t.Errorf("fallback engine should match id=1")
	}
	if _, ok := got[2]; !ok {
		t.Errorf("fallback engine should match id=2")
	}
}

// TestVectorscanExistenceVsOracleSynthetic 用随机 RE2 正则 + 随机语料, 比较 Vectorscan 后端
// 与 stdlib oracle 的命中规则 ID 集合。Vectorscan 无法编译者经 fallback 承载, 不影响一致性。
func TestVectorscanExistenceVsOracleSynthetic(t *testing.T) {
	requireVectorscan(t)

	rounds := 30
	if testing.Short() {
		rounds = 8
	}
	for round := 0; round < rounds; round++ {
		r := rand.New(rand.NewSource(int64(0x515C + round)))
		npat := 10 + r.Intn(20)
		var patterns []Pattern
		for i := 0; i < npat; i++ {
			patterns = append(patterns, Pattern{ID: PatternID(i + 1), Expr: genPattern(r), Flags: randFlags(r)})
		}

		vs, err := Compile(patterns, WithBackend(BackendVectorscan), WithLogger(silentLogger{}))
		if err != nil {
			t.Fatalf("round=%d compile vectorscan: %v", round, err)
		}
		oracle, err := Compile(patterns, WithBackend(BackendStdlib), WithLogger(silentLogger{}))
		if err != nil {
			vs.Close()
			t.Fatalf("round=%d compile oracle: %v", round, err)
		}
		if vs.Info().Backend != BackendVectorscan {
			vs.Close()
			oracle.Close()
			t.Fatalf("round=%d expected vectorscan backend, got %s", round, vs.Info().Backend)
		}

		plants := extractPlants(patternExprs(patterns))
		corpora := 12
		if testing.Short() {
			corpora = 4
		}
		for c := 0; c < corpora; c++ {
			data := genFuzzCorpus(r, plants, r.Intn(1500))
			a := existIDs(t, vs, data)
			b := existIDs(t, oracle, data)
			assertSameIDSet(t, a, b, fmt.Sprintf("round=%d corpus=%d len=%d", round, c, len(data)))
			if t.Failed() {
				for _, p := range patterns {
					t.Logf("  id=%d flags=%d expr=%q", p.ID, p.Flags, p.Expr)
				}
				t.Fatalf("round=%d corpus=%d data=%q", round, c, data)
			}
		}
		vs.Close()
		oracle.Close()
	}
}

func patternExprs(ps []Pattern) []string {
	out := make([]string, len(ps))
	for i, p := range ps {
		out[i] = p.Expr
	}
	return out
}

// TestVectorscanExistenceVsOracleMITM 用真实 MITM 规则集 + 真实流量, 比较 Vectorscan 后端
// 与 stdlib oracle 的逐记录命中规则 ID 集合, 必须完全一致。
func TestVectorscanExistenceVsOracleMITM(t *testing.T) {
	requireVectorscan(t)

	patterns, _ := compilableMITMPatterns(t)
	records, _ := loadCorpus(t)
	t.Logf("MITM rules: %d, records: %d, libhs: %s", len(patterns), len(records), hsVersion())

	vs, err := Compile(patterns, WithBackend(BackendVectorscan), WithLogger(silentLogger{}))
	if err != nil {
		t.Fatal(err)
	}
	defer vs.Close()
	oracle, err := Compile(patterns, WithBackend(BackendStdlib), WithLogger(silentLogger{}))
	if err != nil {
		t.Fatal(err)
	}
	defer oracle.Close()

	info := vs.Info()
	t.Logf("vectorscan backend: tier=%d simd=%v always_on(fallback)=%d", info.Tier, info.SIMD, info.NumAlwaysOn)

	if testing.Short() && len(records) > 200 {
		records = records[:200]
	}
	for i, rec := range records {
		a := existIDs(t, vs, rec)
		b := existIDs(t, oracle, rec)
		assertSameIDSet(t, a, b, fmt.Sprintf("record#%d(len=%d)", i, len(rec)))
		if t.Failed() {
			t.FailNow()
		}
	}
}

// TestVectorscanFallbackRegexp2 验证 regexp2-only 规则 (lookahead) 在 Vectorscan 后端下
// 经 fallback 正确承载存在性命中。
func TestVectorscanFallbackRegexp2(t *testing.T) {
	requireVectorscan(t)

	patterns := []Pattern{
		{ID: 1, Expr: `foobar`},     // hs 可编译
		{ID: 2, Expr: `foo(?!bar)`}, // regexp2-only -> fallback
	}
	db, err := Compile(patterns, WithBackend(BackendVectorscan), WithLogger(silentLogger{}))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	ids := existIDs(t, db, []byte("foobaz and foobar"))
	if _, ok := ids[1]; !ok {
		t.Errorf("id=1 (foobar) should match")
	}
	if _, ok := ids[2]; !ok {
		t.Errorf("id=2 (regexp2 lookahead) should match via fallback")
	}
}

// TestVectorscanEarlyStop 验证 handler 返回 false 时, hs 路径与 fallback 路径都能提前终止。
func TestVectorscanEarlyStop(t *testing.T) {
	requireVectorscan(t)

	// hs 路径提前终止: 两条 hs 正则都命中, handler 第一次即返回 false。
	hsDB, err := Compile([]Pattern{{ID: 1, Expr: `aaa`}, {ID: 2, Expr: `bbb`}},
		WithBackend(BackendVectorscan), WithLogger(silentLogger{}))
	if err != nil {
		t.Fatal(err)
	}
	defer hsDB.Close()
	sc, _ := hsDB.NewScratch()
	defer sc.Close()
	cnt := 0
	_ = hsDB.Scan([]byte("aaa bbb"), sc, func(Match) bool { cnt++; return false })
	if cnt != 1 {
		t.Errorf("hs path early-stop: want 1 callback, got %d", cnt)
	}

	// fallback 路径提前终止: 两条 regexp2-only 正则都命中, handler 第一次即返回 false。
	fbDB, err := Compile([]Pattern{{ID: 1, Expr: `foo(?!x)`}, {ID: 2, Expr: `bar(?!x)`}},
		WithBackend(BackendVectorscan), WithLogger(silentLogger{}))
	if err != nil {
		t.Fatal(err)
	}
	defer fbDB.Close()
	sc2, _ := fbDB.NewScratch()
	defer sc2.Close()
	cnt = 0
	_ = fbDB.Scan([]byte("foo bar"), sc2, func(Match) bool { cnt++; return false })
	if cnt != 1 {
		t.Errorf("fallback path early-stop: want 1 callback, got %d", cnt)
	}

	// 空数据扫描不应崩溃 (覆盖 empty-data 指针路径)。
	cnt = 0
	_ = hsDB.Scan(nil, sc, func(Match) bool { cnt++; return true })
}

// TestVectorscanConcurrentScan 验证同一 db 被多 goroutine 并发扫描 (各自独立 scratch +
// hs_scratch 空闲表) 不发生数据竞争 / 崩溃。
func TestVectorscanConcurrentScan(t *testing.T) {
	requireVectorscan(t)

	patterns, _ := compilableMITMPatterns(t)
	records, _ := loadCorpus(t)
	if len(records) > 300 {
		records = records[:300]
	}
	db, err := Compile(patterns, WithBackend(BackendVectorscan), WithLogger(silentLogger{}))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	const workers = 8
	done := make(chan int, workers)
	for w := 0; w < workers; w++ {
		go func() {
			sc, _ := db.NewScratch()
			defer sc.Close()
			n := 0
			for _, rec := range records {
				_ = db.Scan(rec, sc, func(Match) bool { n++; return true })
			}
			done <- n
		}()
	}
	first := -1
	for w := 0; w < workers; w++ {
		got := <-done
		if first < 0 {
			first = got
		} else if got != first {
			t.Errorf("worker hit count mismatch: %d vs %d (data race?)", got, first)
		}
	}
}
