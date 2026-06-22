package minirehs

import (
	"fmt"
	"math/rand"
	"regexp"
	"regexp/syntax"
	"testing"
)

// nfaFindAll 用本核 NFA 枚举 data 上所有非重叠匹配区间, 便于与 oracle 对照.
func nfaFindAll(nfa *mvsNFA, data []byte) [][2]int {
	var out [][2]int
	nfa.findAllLoc(data, nil, func(from, to int) bool {
		out = append(out, [2]int{from, to})
		return true
	})
	return out
}

// longestFindAll 用 stdlib regexp 的 leftmost-longest (POSIX) 语义枚举所有非重叠匹配,
// 作为 NFA 定位的 oracle. NFA 的 findAllLoc 语义即 leftmost-longest, 故二者应逐字节一致.
func longestFindAll(re *regexp.Regexp, data []byte) [][2]int {
	locs := re.FindAllIndex(data, -1)
	out := make([][2]int, 0, len(locs))
	for _, l := range locs {
		out = append(out, [2]int{l[0], l[1]})
	}
	return out
}

func sameSpans(a, b [][2]int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// TestMVSFindLocDirect 直接验证 NFA 定位 (findAllLoc) 给出的匹配区间与内容, 与 stdlib
// leftmost-longest oracle 逐字节一致; 并断言每个上报区间的内容确实匹配该正则.
func TestMVSFindLocDirect(t *testing.T) {
	cases := []struct {
		expr   string
		inputs []string
	}{
		{`AKIA[0-9A-Z]{16}`, []string{"AKIAABCDEFGHIJKLMNOP", "xxAKIAABCDEFGHIJKLMNOPyy zzAKIA0123456789ABCDEF", "no key here"}},
		{`Druid`, []string{"Druid", "DruidDruid", "xDruidyDruidz", "nope"}},
		{`swagger-ui\.html`, []string{"GET /swagger-ui.html HTTP", "a/swagger-ui.html b/swagger-ui.html"}},
		{`[0-9]{3,}`, []string{"ab123cd456", "12 999999 7", "x12y"}},
		{`(GET|POST|PUT)`, []string{"GET /x POST /y PUT /z", "PATCH only"}},
		{`rememberMe=`, []string{"Cookie: rememberMe=deleteMe; rememberMe=again", "none"}},
		{`\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3}`, []string{"ip 192.168.1.1 and 10.0.0.255 end", "1.2.3"}},
		{`^GET`, []string{"GET /index", "xGET", "GET GET"}},
		{`END$`, []string{"the END", "ENDx", "x END"}},
		{`ab+c`, []string{"abc abbbbc ac", "xabbcy"}},
		{`a[bc]*d`, []string{"ad abcbcd aXd", "abbccd"}},
		{`(?i)druid`, []string{"DRUID DrUiD druid", "none"}},
		{`colou?r`, []string{"color and colour", "colouur"}},
		{`a.c`, []string{"abc axc a c", "a\nc"}},
		{`<[^>]+>`, []string{"x<a><bc>y<tag attr=1>z", "<>"}},
		{`https?://`, []string{"http://a https://b", "ftp://c"}},
		{`eyJ[A-Za-z0-9_-]{3,}\.[A-Za-z0-9._-]{3,}`, []string{"token eyJhbGciOi.eyJzdWIiOiX end", "eyJ.short"}},
	}
	for _, c := range cases {
		re := regexp.MustCompile(c.expr)
		re.Longest()
		nfa := buildNFAFor(t, c.expr)
		for _, in := range c.inputs {
			b := []byte(in)
			oracle := longestFindAll(re, b)
			if nfa == nil {
				t.Logf("expr=%q routed to fallback (no NFA); oracle spans=%v", c.expr, oracle)
				continue
			}
			got := nfaFindAll(nfa, b)
			if !sameSpans(got, oracle) {
				t.Errorf("expr=%q input=%q: nfa spans=%v oracle spans=%v", c.expr, in, got, oracle)
				continue
			}
			// 内容自洽: 每个上报区间确实是该正则的一个匹配.
			single := regexp.MustCompile(c.expr)
			for _, sp := range got {
				content := b[sp[0]:sp[1]]
				if loc := single.FindIndex(content); loc == nil || loc[0] != 0 || loc[1] != len(content) {
					t.Errorf("expr=%q reported span %v content=%q is not a full match", c.expr, sp, content)
				}
			}
		}
	}
}

// TestMVSFindLocRandomDifferential 随机生成大量可编入 NFA 的 RE2, 在随机字节输入上比对
// NFA 定位 (findAllLoc) 与 stdlib leftmost-longest oracle 的匹配区间序列, 逐字节一致.
func TestMVSFindLocRandomDifferential(t *testing.T) {
	r := rand.New(rand.NewSource(0x10C8))
	const iters = 20000
	tested, skipped, checks := 0, 0, 0
	for it := 0; it < iters; it++ {
		expr := genRE(r, 2)
		re, err := regexp.Compile(expr)
		if err != nil {
			continue
		}
		re.Longest()
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
		for s := 0; s < 6; s++ {
			n := r.Intn(24)
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
			oracle := longestFindAll(re, in)
			got := nfaFindAll(nfa, in)
			checks++
			if !sameSpans(got, oracle) {
				t.Fatalf("expr=%q input=%q: nfa spans=%v oracle spans=%v", expr, in, got, oracle)
			}
		}
	}
	t.Logf("loc random differential: tested(NFA)=%d skipped=%d checks=%d", tested, skipped, checks)
	if tested < 1000 {
		t.Fatalf("too few NFA-eligible patterns exercised: %d", tested)
	}
}

// TestMVSLocationMITMRealTraffic 用真实 MITM 规则中可编入 NFA 的那些, 在真实流量每条记录上,
// 验证 NFA 定位与 stdlib leftmost-longest oracle 的匹配区间逐字节一致. 这是定位能力在真实
// 数据上的强护栏.
func TestMVSLocationMITMRealTraffic(t *testing.T) {
	patterns, names := compilableMITMPatterns(t)
	type nfaRule struct {
		id   PatternID
		expr string
		nfa  *mvsNFA
		re   *regexp.Regexp
	}
	var rules []nfaRule
	for _, p := range patterns {
		expr := buildExprWithFlags(p)
		parsed, err := syntax.Parse(expr, syntax.Perl)
		if err != nil {
			continue
		}
		nfa, ok := compileMVSNFA(parsed.Simplify())
		if !ok {
			continue
		}
		re, err := regexp.Compile(expr)
		if err != nil {
			continue
		}
		re.Longest()
		rules = append(rules, nfaRule{id: p.ID, expr: expr, nfa: nfa, re: re})
	}
	t.Logf("NFA-eligible MITM rules for location check: %d", len(rules))

	records, joined := loadCorpus(t)
	if testing.Short() && len(records) > 200 {
		records = records[:200]
	}
	mismatches := 0
	for ri, rec := range records {
		for _, rule := range rules {
			oracle := longestFindAll(rule.re, rec)
			got := nfaFindAll(rule.nfa, rec)
			if !sameSpans(got, oracle) {
				mismatches++
				t.Errorf("record#%d rule id=%d name=%s expr=%q: nfa=%v oracle=%v",
					ri, rule.id, names[rule.id], rule.expr, got, oracle)
				if mismatches > 10 {
					t.FailNow()
				}
			}
		}
	}
	if testing.Short() {
		return
	}
	for _, rule := range rules {
		oracle := longestFindAll(rule.re, joined)
		got := nfaFindAll(rule.nfa, joined)
		if !sameSpans(got, oracle) {
			t.Errorf("joined rule id=%d name=%s expr=%q: nfa count=%d oracle count=%d",
				rule.id, names[rule.id], rule.expr, len(got), len(oracle))
		}
	}
}

// TestMVSScanReportsContentMITM 端到端验证 MVS 后端 Scan 上报的"匹配内容 + 位置": 对每条
// 命中且带精确偏移 (From>=0) 的结果, 断言 data[From:To] 确实是该规则的一个真实匹配; 同时
// 统计带偏移命中与存在性命中 (regexp2 兜底, From=-1) 的占比, 展示"匹配到哪儿/内容是啥".
func TestMVSScanReportsContentMITM(t *testing.T) {
	patterns, names := compilableMITMPatterns(t)
	db, err := Compile(patterns, WithBackend(BackendMVS))
	if err != nil {
		t.Fatalf("compile mvs: %v", err)
	}
	defer db.Close()

	// 每条规则的校验器 (stdlib RE2 或 regexp2), 用于复核上报内容确属该规则的匹配.
	res := make(map[PatternID]*regexp.Regexp)
	for _, p := range patterns {
		if re, err := regexp.Compile(buildExprWithFlags(p)); err == nil {
			res[p.ID] = re
		}
	}

	sc, err := db.NewScratch()
	if err != nil {
		t.Fatalf("new scratch: %v", err)
	}
	defer sc.Close()

	records, _ := loadCorpus(t)
	if len(records) > 300 {
		records = records[:300]
	}

	var located, existence, samples int
	for _, rec := range records {
		err := db.Scan(rec, sc, func(m Match) bool {
			if m.From < 0 || m.To < 0 {
				existence++
				return true
			}
			located++
			if m.From > m.To || m.To > len(rec) {
				t.Fatalf("rule id=%d invalid span [%d,%d) len=%d", m.ID, m.From, m.To, len(rec))
			}
			content := rec[m.From:m.To]
			if re := res[m.ID]; re != nil {
				if !re.Match(content) {
					t.Errorf("rule id=%d name=%s reported content=%q does not match its pattern",
						m.ID, names[m.ID], content)
				}
			}
			if samples < 8 {
				samples++
				t.Logf("hit rule id=%d name=%s at [%d,%d) content=%q",
					m.ID, names[m.ID], m.From, m.To, truncForLog(content))
			}
			return true
		})
		if err != nil {
			t.Fatalf("scan: %v", err)
		}
	}
	t.Logf("located hits (with offset+content)=%d, existence-only hits (regexp2 fallback)=%d", located, existence)
	if located == 0 {
		t.Fatal("expected at least some located hits with precise offset and content")
	}
}

// TestMVSReportLocationToggle 验证 WithReportLocation(false) 时 mvs 退回纯存在性 (From/To=-1),
// 且命中 ID 集合与默认 (带定位) 模式一致 (定位开关不改变"哪些规则命中").
func TestMVSReportLocationToggle(t *testing.T) {
	patterns := fixedPatterns()
	loc, err := Compile(patterns, WithBackend(BackendMVS))
	if err != nil {
		t.Fatalf("compile located: %v", err)
	}
	defer loc.Close()
	exist, err := Compile(patterns, WithBackend(BackendMVS), WithReportLocation(false))
	if err != nil {
		t.Fatalf("compile existence: %v", err)
	}
	defer exist.Close()

	data := []byte("AKIAABCDEFGHIJKLMNOP password = secret123 id 12345 ip 10.0.0.1 rememberMe=x Druid")

	// 存在性模式: NFA pattern 命中也只报 -1/-1.
	scE, _ := exist.NewScratch()
	defer scE.Close()
	sawOffset := false
	idsExist := map[PatternID]struct{}{}
	_ = exist.Scan(data, scE, func(m Match) bool {
		idsExist[m.ID] = struct{}{}
		if m.From >= 0 || m.To >= 0 {
			sawOffset = true
		}
		return true
	})
	if sawOffset {
		t.Fatal("WithReportLocation(false) should report only existence (-1,-1)")
	}

	// 默认 (定位) 模式的命中 ID 集合应与存在性模式一致.
	idsLoc := mvsExistIDs(t, loc, data)
	mvsAssertSameIDSet(t, idsLoc, idsExist, "report-location-toggle")
}

func truncForLog(b []byte) string {
	const maxN = 64
	if len(b) <= maxN {
		return string(b)
	}
	return string(b[:maxN]) + fmt.Sprintf("...(%d bytes)", len(b))
}
