package minirehs

import (
	"fmt"
	"math/rand"
	"regexp"
	"regexp/syntax"
	"testing"
	"time"
)

// mvsExistIDs 扫描 data, 返回命中的 pattern ID 集合 (存在性, 忽略偏移与重复).
func mvsExistIDs(tb testing.TB, db Database, data []byte) map[PatternID]struct{} {
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

// mvsAssertSameIDSet 断言两个命中 ID 集合相等.
func mvsAssertSameIDSet(tb testing.TB, got, oracle map[PatternID]struct{}, ctx string) {
	tb.Helper()
	for id := range oracle {
		if _, ok := got[id]; !ok {
			tb.Errorf("%s: mvs MISSED rule id=%d (oracle matched)", ctx, id)
		}
	}
	for id := range got {
		if _, ok := oracle[id]; !ok {
			tb.Errorf("%s: mvs EXTRA rule id=%d (oracle did not match)", ctx, id)
		}
	}
}

// buildNFAFor 解析 expr 并尝试编入 mvsNFA; 返回 nil 表示该 expr 走兜底 (非本核处理范围).
func buildNFAFor(t *testing.T, expr string) *mvsNFA {
	t.Helper()
	parsed, err := syntax.Parse(expr, syntax.Perl)
	if err != nil {
		t.Fatalf("parse %q: %v", expr, err)
	}
	nfa, ok := compileMVSNFA(parsed.Simplify())
	if !ok {
		return nil
	}
	return nfa
}

// TestMVSNFADirect 直接验证位并行 NFA 的存在性判定与 stdlib regexp.Match 完全一致.
// 选取的 pattern 都应能编入 NFA (全 ASCII 位置), 以真正覆盖 NFA 路径而非兜底.
func TestMVSNFADirect(t *testing.T) {
	cases := []struct {
		expr   string
		inputs []string
	}{
		{`AKIA[0-9A-Z]{16}`, []string{"AKIAABCDEFGHIJKLMNOP", "xxAKIAABCDEFGHIJKLMNOPyy", "AKIAshort", "akiaABCDEFGHIJKLMNOP"}},
		{`Druid`, []string{"Druid", "xDruidy", "druid", "Dru"}},
		{`swagger-ui\.html`, []string{"/swagger-ui.html", "swagger-uiXhtml", "swagger-ui.htm"}},
		{`[0-9]{3,}`, []string{"12", "123", "99999", "ab123cd", "x12y"}},
		{`(GET|POST|PUT)`, []string{"GET /", "a POST b", "PUTx", "PATCH", "PU"}},
		{`rememberMe=`, []string{"rememberMe=deleteMe", "rememberme=", "x rememberMe= y"}},
		{`\d{1,3}\.\d{1,3}`, []string{"10.0", "255.255", "1.2.3.4", "abc", ".5", "12."}},
		{`^GET`, []string{"GET /x", "xGET", "GE", " GET"}},
		{`END$`, []string{"the END", "ENDx", "END", "ENDED"}},
		{`ab+c`, []string{"abc", "abbbbc", "ac", "xabcy"}},
		{`a[bc]*d`, []string{"ad", "abcbcd", "abx d", "aXd"}},
		{`(?i)druid`, []string{"DRUID", "DrUiD", "druid", "drui"}},
		{`colou?r`, []string{"color", "colour", "colouur"}},
		{`a.c`, []string{"abc", "axc", "a\nc", "ac", "aXcY"}},        // . 任意字符(排除\n)
		{`<[^>]+>`, []string{"<a>", "<abc>", "<>", "x<tag>y", "<a"}}, // 负类
		{`https?://`, []string{"http://x", "https://y", "htt://", "xhttps://"}},
		{`\bGET\b`, []string{"a GET b", "GETTER", "xGETy", "GET"}}, // \b 含词边界 -> 预期兜底(nil)
	}
	for _, c := range cases {
		re := regexp.MustCompile(c.expr)
		nfa := buildNFAFor(t, c.expr)
		for _, in := range c.inputs {
			b := []byte(in)
			want := re.Match(b)
			if nfa == nil {
				// 该 expr 走兜底, 不在本测试断言 NFA 结果; 仅记录.
				t.Logf("expr=%q routed to fallback (no NFA), input=%q oracle=%v", c.expr, in, want)
				continue
			}
			got := nfa.existsIn(b)
			if got != want {
				t.Errorf("expr=%q input=%q: nfa=%v oracle=%v", c.expr, in, got, want)
			}
		}
	}
}

// genRE 生成一个合法的 (ASCII 子集) RE2 表达式片段, 深度受限.
func genRE(r *rand.Rand, depth int) string {
	atom := func() string {
		switch r.Intn(11) {
		case 0:
			return string(rune('a' + r.Intn(4)))
		case 1:
			return "[a-c]"
		case 2:
			return "[abz]"
		case 3:
			return `\d`
		case 4:
			return `\w`
		case 5:
			if depth > 0 {
				return "(?:" + genRE(r, depth-1) + ")"
			}
			return "d"
		case 6:
			return "[0-9]"
		case 7:
			return "." // 任意字符 (rune 级, 排除 \n)
		case 8:
			return "[^abc]" // 负类
		case 9:
			return `[^\d]`
		default:
			return `\.` // 字面点
		}
	}
	quant := func(s string) string {
		switch r.Intn(6) {
		case 0:
			return s
		case 1:
			return s + "*"
		case 2:
			return s + "+"
		case 3:
			return s + "?"
		case 4:
			return s + fmt.Sprintf("{%d,%d}", r.Intn(2), 1+r.Intn(3))
		default:
			return s + fmt.Sprintf("{%d,}", r.Intn(3))
		}
	}
	seq := func() string {
		out := ""
		for k := 0; k < 1+r.Intn(3); k++ {
			out += quant(atom())
		}
		if out == "" {
			out = "a"
		}
		return out
	}
	expr := seq()
	if r.Intn(3) == 0 {
		expr = expr + "|" + seq()
	}
	if r.Intn(4) == 0 {
		expr = "^" + expr
	}
	if r.Intn(4) == 0 {
		expr = expr + "$"
	}
	return expr
}

// TestMVSNFARandomDifferential 随机生成大量 RE2, 对能编入 NFA 的, 在随机字节输入上比对
// existsIn 与 stdlib regexp.Match. ASCII 闸保证字节级 NFA 在任意字节上与 RE2 一致.
func TestMVSNFARandomDifferential(t *testing.T) {
	r := rand.New(rand.NewSource(0x5EED))
	const iters = 20000
	tested, skipped := 0, 0
	for it := 0; it < iters; it++ {
		expr := genRE(r, 2)
		re, err := regexp.Compile(expr)
		if err != nil {
			continue
		}
		parsed, err := syntax.Parse(expr, syntax.Perl)
		if err != nil {
			continue
		}
		nfa, ok := compileMVSNFA(parsed.Simplify())
		if !ok {
			skipped++
			continue
		}
		tested++
		// 几组随机输入: 含 pattern 字母表 + 任意字节 + 边界.
		for s := 0; s < 6; s++ {
			n := r.Intn(20)
			in := make([]byte, n)
			for i := range in {
				switch r.Intn(3) {
				case 0:
					in[i] = byte("abcd0123.z"[r.Intn(10)])
				case 1:
					in[i] = byte(r.Intn(128))
				default:
					in[i] = byte(r.Intn(256))
				}
			}
			want := re.Match(in)
			got := nfa.existsIn(in)
			if got != want {
				t.Fatalf("expr=%q input=%q: nfa=%v oracle=%v", expr, in, got, want)
			}
		}
	}
	t.Logf("random differential: tested(NFA)=%d skipped(fallback)=%d", tested, skipped)
	if tested < 1000 {
		t.Fatalf("too few NFA-eligible patterns exercised: %d", tested)
	}
}

// TestMVSExistenceVsOracleSynthetic 用固定 pattern + 随机语料做存在性 ID 集合差分:
// mvs 后端命中的规则集合必须与 stdlib oracle 完全一致.
func TestMVSExistenceVsOracleSynthetic(t *testing.T) {
	patterns := fixedPatterns()
	mvs, err := Compile(patterns, WithBackend(BackendMVS))
	if err != nil {
		t.Fatalf("compile mvs: %v", err)
	}
	defer mvs.Close()
	oracle, err := Compile(patterns, WithBackend(BackendStdlib))
	if err != nil {
		t.Fatalf("compile oracle: %v", err)
	}
	defer oracle.Close()

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
		got := mvsExistIDs(t, mvs, data)
		ora := mvsExistIDs(t, oracle, data)
		mvsAssertSameIDSet(t, got, ora, fmt.Sprintf("synthetic#%d(len=%d)", i, size))
		if t.Failed() {
			t.Fatalf("data=%q", data)
		}
	}
}

// TestMVSExistenceVsOracleMITM 用真实 MITM 规则集 + 真实流量做存在性 ID 集合差分.
func TestMVSExistenceVsOracleMITM(t *testing.T) {
	patterns, _ := compilableMITMPatterns(t)
	mvs, err := Compile(patterns, WithBackend(BackendMVS))
	if err != nil {
		t.Fatalf("compile mvs: %v", err)
	}
	defer mvs.Close()
	oracle, err := Compile(patterns, WithBackend(BackendStdlib))
	if err != nil {
		t.Fatalf("compile oracle: %v", err)
	}
	defer oracle.Close()

	info := mvs.Info()
	t.Logf("mvs backend=%s patterns=%d always_on=%d", info.Backend, info.NumPatterns, info.NumAlwaysOn)

	records, joined := loadCorpus(t)
	t.Logf("corpus: %d records, %d bytes joined", len(records), len(joined))
	if testing.Short() && len(records) > 200 {
		records = records[:200]
	}
	for i, rec := range records {
		got := mvsExistIDs(t, mvs, rec)
		ora := mvsExistIDs(t, oracle, rec)
		mvsAssertSameIDSet(t, got, ora, fmt.Sprintf("record#%d(len=%d)", i, len(rec)))
		if t.Failed() {
			t.FailNow()
		}
	}
	if testing.Short() {
		return
	}
	got := mvsExistIDs(t, mvs, joined)
	ora := mvsExistIDs(t, oracle, joined)
	mvsAssertSameIDSet(t, got, ora, "joined-corpus")
}

// TestMVSCoverageMITM 报告 MITM 规则中有多少条能编入 NFA (本核加速), 多少条走兜底.
func TestMVSCoverageMITM(t *testing.T) {
	patterns, _ := compilableMITMPatterns(t)
	nfa, fallback := 0, 0
	for _, p := range patterns {
		expr := buildExprWithFlags(p)
		parsed, err := syntax.Parse(expr, syntax.Perl)
		if err != nil {
			fallback++
			continue
		}
		if _, ok := compileMVSNFA(parsed.Simplify()); ok {
			nfa++
		} else {
			fallback++
		}
	}
	t.Logf("MITM coverage: total=%d nfa=%d (%.1f%%) fallback=%d",
		len(patterns), nfa, 100*float64(nfa)/float64(max1(len(patterns))), fallback)
}

func max1(n int) int {
	if n == 0 {
		return 1
	}
	return n
}

// TestMVSQuantifySpeedup 量化"纯算法 (无 SIMD/cgo)"相对 stdlib 逐条匹配的加速比.
func TestMVSQuantifySpeedup(t *testing.T) {
	if testing.Short() {
		t.Skip("skip speedup quantify in -short")
	}
	patterns, _ := compilableMITMPatterns(t)
	records, joined := loadCorpus(t)
	_ = joined

	mvs, err := Compile(patterns, WithBackend(BackendMVS))
	if err != nil {
		t.Fatalf("compile mvs: %v", err)
	}
	defer mvs.Close()
	std, err := Compile(patterns, WithBackend(BackendStdlib))
	if err != nil {
		t.Fatalf("compile stdlib: %v", err)
	}
	defer std.Close()

	total := 0
	for _, r := range records {
		total += len(r)
	}

	measure := func(db Database) time.Duration {
		sc, _ := db.NewScratch()
		defer sc.Close()
		start := time.Now()
		for _, rec := range records {
			_ = db.Scan(rec, sc, func(Match) bool { return true })
		}
		return time.Since(start)
	}

	// 预热.
	_ = measure(mvs)
	_ = measure(std)

	tm := measure(mvs)
	ts := measure(std)
	mbps := func(d time.Duration) float64 { return float64(total) / 1e6 / d.Seconds() }
	t.Logf("corpus=%d bytes over %d records", total, len(records))
	t.Logf("mvs(pure-go):   %v  %.2f MB/s", tm, mbps(tm))
	t.Logf("stdlib(loop):   %v  %.2f MB/s", ts, mbps(ts))
	t.Logf("speedup mvs/stdlib = %.1fx", float64(ts)/float64(tm))
}

// BenchmarkMVSFullRuleset 基准: 真实 MITM 规则集 + 真实流量上 mvs / engine / stdlib 三方吞吐对比.
func BenchmarkMVSFullRuleset(b *testing.B) {
	patterns, _ := compilableMITMPatterns(b)
	records, joined := loadCorpusB(b)
	b.Logf("rules: %d, records: %d, corpus bytes: %d", len(patterns), len(records), len(joined))
	b.Run("MVS", func(b *testing.B) {
		benchScanRecords(b, patterns, BackendMVS, records)
	})
	b.Run("Engine", func(b *testing.B) {
		benchScanRecords(b, patterns, BackendEngine, records)
	})
	b.Run("StdlibLoop", func(b *testing.B) {
		benchScanRecords(b, patterns, BackendStdlib, records)
	})
}
