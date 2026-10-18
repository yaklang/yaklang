package minirehs

import (
	"fmt"
	"math/rand"
	"testing"

	regexp_utils "github.com/yaklang/yaklang/common/utils/regexp-utils"
)

// scanAllSet 扫描 data, 返回以 (id,from,to) 为键的命中集合 (用于差分比较).
func scanAllSet(tb testing.TB, db Database, data []byte) map[matchKey]struct{} {
	tb.Helper()
	sc, err := db.NewScratch()
	if err != nil {
		tb.Fatalf("new scratch: %v", err)
	}
	defer sc.Close()
	set := make(map[matchKey]struct{})
	if err := db.Scan(data, sc, func(m Match) bool {
		set[matchKey{id: m.ID, from: m.From, to: m.To}] = struct{}{}
		return true
	}); err != nil {
		tb.Fatalf("scan: %v", err)
	}
	return set
}

func assertSameMatchSet(tb testing.TB, engine, oracle map[matchKey]struct{}, ctx string) {
	tb.Helper()
	if len(engine) != len(oracle) {
		tb.Errorf("%s: match count differs engine=%d oracle=%d", ctx, len(engine), len(oracle))
	}
	for k := range oracle {
		if _, ok := engine[k]; !ok {
			tb.Errorf("%s: engine MISSED match id=%d [%d,%d)", ctx, k.id, k.from, k.to)
		}
	}
	for k := range engine {
		if _, ok := oracle[k]; !ok {
			tb.Errorf("%s: engine EXTRA match id=%d [%d,%d)", ctx, k.id, k.from, k.to)
		}
	}
}

// fixedPatterns 是一组覆盖多种构造的代表性 RE2 正则: 含必需字面量的、大小写无关的、
// 交替分支的、以及无字面量的 always-on 正则.
func fixedPatterns() []Pattern {
	return []Pattern{
		{ID: 1, Expr: `password\s*=\s*\S+`},
		{ID: 2, Expr: `AKIA[0-9A-Z]{16}`},
		{ID: 3, Expr: `Druid`, Flags: FlagCaseless},
		{ID: 4, Expr: `(swagger-ui\.html|swaggerVersion|swaggerUi)`},
		{ID: 5, Expr: `eyJ[A-Za-z0-9_-]{6,}\.[A-Za-z0-9._-]{6,}`},
		{ID: 6, Expr: `[0-9]{3,}`},          // always-on
		{ID: 7, Expr: `\b(GET|POST|PUT)\b`}, // 交替, 短字面量
		{ID: 8, Expr: `rememberMe=`},        // 纯字面量
		{ID: 9, Expr: `(?i)content-type:\s*\S+`},
		{ID: 10, Expr: `\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3}`}, // always-on (IP)
	}
}

// randomCorpus 生成随机数据, 其中以一定概率植入 pattern 的字面量, 以触发真实命中.
func randomCorpus(r *rand.Rand, tokens []string, n int) []byte {
	const alphabet = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789 =:./\\\"'\t\n{}[]_-"
	buf := make([]byte, 0, n)
	for len(buf) < n {
		if len(tokens) > 0 && r.Intn(8) == 0 {
			buf = append(buf, tokens[r.Intn(len(tokens))]...)
			continue
		}
		buf = append(buf, alphabet[r.Intn(len(alphabet))])
	}
	return buf[:n]
}

// TestConsistencySyntheticEngineVsOracle 用固定 pattern + 大量随机语料做差分测试:
// 引擎 (含预过滤) 必须与 stdlib 逐条 oracle 产出完全相同的命中集合.
func TestConsistencySyntheticEngineVsOracle(t *testing.T) {
	patterns := fixedPatterns()
	engine, err := Compile(patterns, WithBackend(BackendEngine))
	if err != nil {
		t.Fatalf("compile engine: %v", err)
	}
	defer engine.Close()
	oracle, err := Compile(patterns, WithBackend(BackendStdlib))
	if err != nil {
		t.Fatalf("compile oracle: %v", err)
	}
	defer oracle.Close()

	t.Logf("engine backend=%s tier=%d simd=%v always_on=%d",
		engine.Info().Backend, engine.Info().Tier, engine.Info().SIMD, engine.Info().NumAlwaysOn)

	tokens := []string{
		"password = secret123", "AKIAABCDEFGHIJKLMNOP", "druid", "DRUID", "Druid",
		"swagger-ui.html", "swaggerVersion", "rememberMe=deleteMe",
		"eyJhbGciOi.eyJzdWIiOi", "Content-Type: application/json", "GET", "POST",
		"192.168.1.1", "12345",
	}

	r := rand.New(rand.NewSource(0xC0FFEE))
	for i := 0; i < 400; i++ {
		size := 64 + r.Intn(8192)
		data := randomCorpus(r, tokens, size)
		eng := scanAllSet(t, engine, data)
		ora := scanAllSet(t, oracle, data)
		assertSameMatchSet(t, eng, ora, fmt.Sprintf("synthetic#%d(len=%d)", i, size))
		if t.Failed() {
			t.Fatalf("data=%q", data)
		}
	}
}

// compilableMITMPatterns 返回可被 YakRegexpUtils 编译 (RE2 或 regexp2) 的 MITM 规则,
// 跳过个别本身就语法非法的规则 (例如括号不匹配的那条).
func compilableMITMPatterns(tb testing.TB) ([]Pattern, map[PatternID]string) {
	all, names := mitmPatterns(tb)
	var ok []Pattern
	for _, p := range all {
		if regexp_utils.NewYakRegexpUtils(buildExprWithFlags(p)).CanUse() {
			ok = append(ok, p)
		} else {
			tb.Logf("skip non-compilable rule id=%d name=%s expr=%q", p.ID, names[p.ID], p.Expr)
		}
	}
	return ok, names
}

// TestConsistencyMITMRealTraffic 用 rule4yak 的真实规则集 + 本地库导出的真实流量,
// 验证引擎与 oracle 在每条报文上的命中集合完全一致.
func TestConsistencyMITMRealTraffic(t *testing.T) {
	patterns, _ := compilableMITMPatterns(t)
	t.Logf("compilable MITM rules: %d", len(patterns))

	engine, err := Compile(patterns, WithBackend(BackendEngine))
	if err != nil {
		t.Fatalf("compile engine: %v", err)
	}
	defer engine.Close()
	oracle, err := Compile(patterns, WithBackend(BackendStdlib))
	if err != nil {
		t.Fatalf("compile oracle: %v", err)
	}
	defer oracle.Close()

	info := engine.Info()
	t.Logf("engine backend=%s tier=%d simd=%v patterns=%d always_on=%d",
		info.Backend, info.Tier, info.SIMD, info.NumPatterns, info.NumAlwaysOn)

	records, joined := loadCorpus(t)
	t.Logf("corpus: %d records, %d bytes joined", len(records), len(joined))

	// -short 模式下只验证前若干条 (oracle 逐条匹配很慢), 完整验证用普通模式.
	if testing.Short() && len(records) > 200 {
		records = records[:200]
	}

	for i, rec := range records {
		eng := scanAllSet(t, engine, rec)
		ora := scanAllSet(t, oracle, rec)
		assertSameMatchSet(t, eng, ora, fmt.Sprintf("record#%d(len=%d)", i, len(rec)))
		if t.Failed() {
			t.FailNow()
		}
	}

	if testing.Short() {
		return
	}

	// 再对整段拼接语料做一次整体差分.
	eng := scanAllSet(t, engine, joined)
	ora := scanAllSet(t, oracle, joined)
	assertSameMatchSet(t, eng, ora, "joined-corpus")
	t.Logf("joined corpus total matches: %d", len(ora))
}
