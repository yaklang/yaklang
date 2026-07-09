package minirehs

import (
	"math/rand"
	"regexp"
	"regexp/syntax"
	"testing"
	"unicode/utf8"
)

// buildAssertNFA 编译 expr 为断言扩展 NFA; ok=false 表示该 expr 不在断言核处理范围 (交兜底).
func buildAssertNFA(tb testing.TB, expr string) (*mvsNFA, bool) {
	tb.Helper()
	parsed, err := syntax.Parse(expr, syntax.Perl)
	if err != nil {
		return nil, false
	}
	return compileMVSNFAAssert(parsed.Simplify())
}

// TestMVSAssertDirect 直接验证含零宽断言的 pattern: existsInAssert 与 stdlib regexp 存在性逐例一致.
func TestMVSAssertDirect(t *testing.T) {
	cases := []struct {
		expr   string
		inputs []string
	}{
		{`\bGET\b`, []string{"GET", "GETX", "XGET", "a GET b", "GET/", "GETTER", "/GET ", "get", "  GET"}},
		{`\bword`, []string{"word", "a word", "sword", "_word", "1word", " word", "word ", "wordy"}},
		{`word\b`, []string{"word", "word ", "wordy", "a word!", "words", "word_"}},
		{`\Bint\B`, []string{"print", "int", "ints", "aintb", "in t", "xintx", " int "}},
		{`(?m)^foo`, []string{"foo", "x\nfoo", "xfoo", "a\nb\nfoo", "\nfoo", "barfoo"}},
		{`bar(?m)$`, []string{"bar", "bar\n", "barx", "x\nbar\ny", "a bar", "bar\nbaz"}},
		{`^abc`, []string{"abc", "xabc", "abcd", " abc"}},
		{`abc$`, []string{"abc", "abcx", "xabc", "abc\n"}},
		{`[^0-9](\d{2}$|\d{3}x)[^0-9]`, []string{"a12", "a12 ", " 123x ", "a999xb", "x12y", "  45"}},
		{`(^mac|[^a-z]mac)`, []string{"mac", "xmac", " mac", "Amac", "macx", "1mac2"}},
		{`\b(file|path|url)\s*=`, []string{"file=", "path =", "url= x", "profile=", "myurl=", " file =", "xpath="}},
		{`\b[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\.[A-Za-z]{2,}\b`, []string{
			"a@b.com", "x a@b.com y", "name.surname@mail.co", "@b.com", "a@b.", "foo@bar.commm.", "u_v@w-x.io"}},
	}
	for _, tc := range cases {
		nfa, ok := buildAssertNFA(t, tc.expr)
		if !ok {
			t.Errorf("expr %q expected to compile into assert NFA but bailed", tc.expr)
			continue
		}
		if !nfa.hasAssert {
			t.Errorf("expr %q compiled but hasAssert=false", tc.expr)
		}
		re := regexp.MustCompile(tc.expr)
		for _, in := range tc.inputs {
			got := nfa.existsInAssert([]byte(in))
			want := re.MatchString(in)
			if got != want {
				t.Errorf("expr %q input %q: existsInAssert=%v want=%v", tc.expr, in, got, want)
			}
		}
	}
}

// randAssertRegex 生成一条含零宽断言的随机正则 (受控文法, 同时被 syntax.Parse 与 stdlib 接受).
func randAssertRegex(r *rand.Rand) string {
	terms := []string{
		"a", "b", "c", "x", "y", "z", "0", "1", "_", ":", "=", "@", ".", " ",
		"[a-z]", "[0-9]", "[A-Za-z0-9_]", "\\w", "\\d", "\\s", ".", "[^0-9]", "[^a-z ]",
	}
	quants := []string{"", "", "?", "*", "+", "{1,3}", "{2}"}
	branch := func() string {
		k := 1 + r.Intn(4)
		s := ""
		for i := 0; i < k; i++ {
			t := terms[r.Intn(len(terms))]
			q := quants[r.Intn(len(quants))]
			s += t + q
			// 偶尔在 term 间插入中缀断言 (制造 follow-guard, 含可能的死分支如 $X).
			if r.Intn(6) == 0 {
				s += []string{"\\b", "\\B", "$", "^"}[r.Intn(4)]
			}
		}
		return s
	}
	body := branch()
	if r.Intn(2) == 0 {
		alts := 1 + r.Intn(2)
		for i := 0; i < alts; i++ {
			body += "|" + branch()
		}
		body = "(" + body + ")"
	}
	prefix := []string{"", "\\b", "\\B", "^", "(?m)^", "\\A"}[r.Intn(6)]
	suffix := []string{"", "\\b", "\\B", "$", "(?m)$", "\\z"}[r.Intn(6)]
	expr := prefix + body + suffix
	if r.Intn(3) == 0 {
		expr = "(?m)" + expr
	}
	return expr
}

// randAssertInput 生成偏向触发断言的随机输入 (单词字符/空白/换行/标点混合, 含少量非法 UTF-8).
func randAssertInput(r *rand.Rand) []byte {
	alphabet := []byte("abcxyz019_ :=@.\n\t,/[]")
	n := r.Intn(40)
	buf := make([]byte, 0, n)
	for i := 0; i < n; i++ {
		if r.Intn(25) == 0 {
			buf = append(buf, byte(0x80+r.Intn(0x40))) // 孤立续字节: 非法 UTF-8
			continue
		}
		buf = append(buf, alphabet[r.Intn(len(alphabet))])
	}
	return buf
}

// TestMVSAssertRandomDifferential 随机生成含断言的正则 + 随机输入, 凡能编入断言 NFA 的,
// existsInAssert 必须与 stdlib regexp 存在性逐例一致 (这是断言门控的安全性核心: 不得有假阴/假阳).
func TestMVSAssertRandomDifferential(t *testing.T) {
	r := rand.New(rand.NewSource(0xA55E27))
	compiled, checks := 0, 0
	for iter := 0; iter < diffIters(t, 800); iter++ {
		expr := randAssertRegex(r)
		nfa, ok := buildAssertNFA(t, expr)
		if !ok {
			continue
		}
		re, err := regexp.Compile(expr)
		if err != nil {
			continue // stdlib 不接受则跳过 (本核也不应据此判定)
		}
		compiled++
		for j := 0; j < 40; j++ {
			data := randAssertInput(r)
			got := nfa.existsInAssert(data)
			want := re.Match(data)
			checks++
			if got != want {
				t.Fatalf("DIVERGE expr=%q data=%q existsInAssert=%v stdlib=%v", expr, data, got, want)
			}
		}
	}
	t.Logf("assert random differential: compiled=%d regexes, checks=%d (all consistent)", compiled, checks)
	if compiled < 100 {
		t.Fatalf("too few assert regexes compiled (%d); generator/coverage problem", compiled)
	}
}

// TestMVSAssertRecoversFallback 验证真实 MITM 规则中 7 条由零宽断言导致的 fallback 现已被
// 断言扩展救回 (\b ×5, 中缀 ^/$ 各 1), 且每条在真实流量上 existsInAssert 与 stdlib 存在性一致.
func TestMVSAssertRecoversFallback(t *testing.T) {
	patterns, names := compilableMITMPatterns(t)
	records, _ := loadCorpus(t)

	recovered := 0
	for _, p := range patterns {
		expr := buildExprWithFlags(p)
		parsed, err := syntax.Parse(expr, syntax.Perl)
		if err != nil {
			continue
		}
		s := parsed.Simplify()
		if _, ok := compileMVSNFA(s); ok {
			continue // lean 已覆盖, 非本扩展目标
		}
		nfa, ok := compileMVSNFAAssert(s)
		if !ok {
			continue // 真正不可表达 (regexp2-only)
		}
		recovered++
		re, err := regexp.Compile(expr)
		if err != nil {
			t.Fatalf("rule %q stdlib compile: %v", names[p.ID], err)
		}
		for i, rec := range records {
			got := nfa.existsInAssert(rec)
			want := re.Match(rec)
			if got != want {
				t.Fatalf("rule=%q (id=%d) record#%d: existsInAssert=%v stdlib=%v\nexpr=%q",
					names[p.ID], p.ID, i, got, want, expr)
			}
		}
	}
	t.Logf("assert extension recovered %d fallback rules (verified on %d real records)", recovered, len(records))
	if recovered < 7 {
		t.Fatalf("expected >=7 assertion fallbacks recovered, got %d", recovered)
	}
}

// TestMVSAssertScalarEquivalence 守护 nword==1 断言 NFA 的标量快路径 (existsInAssertShared1 /
// existsInAssertAnchored1) 与多字通用版逐例一致。compileMVSNFAAssert 此前漏置 single, 致这两条
// 标量孪生形同虚设 (所有断言 NFA 恒走多字版); 现已启用 (见 initScalar), 故须专项护栏防回归 ——
// 随机断言正则 × 随机输入 (含非法 UTF-8), 标量与多字判定必须恒等, 否则启用标量即引入假阴/假阳。
func TestMVSAssertScalarEquivalence(t *testing.T) {
	r := rand.New(rand.NewSource(0x5CA1A2))
	singleN, checks := 0, 0
	for iter := 0; iter < diffIters(t, 800); iter++ {
		expr := randAssertRegex(r)
		nfa, ok := buildAssertNFA(t, expr)
		if !ok {
			continue
		}
		if !nfa.single {
			continue // 仅测标量快路径资格 (nword==1); 多字版由其它差分覆盖
		}
		singleN++
		for j := 0; j < 40; j++ {
			data := randAssertInput(r)
			bound := computeBoundaries(data, nil)
			// 共享存在性: 标量 == 多字 (同一 NFA, 同一边界).
			gotShared := nfa.existsInAssertShared1(data, bound)
			wantShared := nfa.existsInAssertShared(data, bound)
			checks++
			if gotShared != wantShared {
				t.Fatalf("SHARED DIVERGE expr=%q data=%q scalar=%v multi=%v", expr, data, gotShared, wantShared)
			}
			// 锚定存在性: 标量 == 多字 (全区间注入 spans=[0,len], 等价整段存在性).
			if len(data) > 0 {
				spans := []anchorSpan{{0, int32(len(data))}}
				prev := make([]uint64, nfa.nword)
				cand := make([]uint64, nfa.nword)
				gotAnc := nfa.existsInAssertAnchored1(data, bound, spans)
				wantAnc := nfa.existsInAssertAnchored(data, bound, spans, prev, cand)
				checks++
				if gotAnc != wantAnc {
					t.Fatalf("ANCHORED DIVERGE expr=%q data=%q scalar=%v multi=%v", expr, data, gotAnc, wantAnc)
				}
			}
		}
	}
	t.Logf("assert scalar equivalence: single=%d nfas, checks=%d (all consistent)", singleN, checks)
	if singleN < 50 {
		t.Fatalf("too few single (nword==1) assert NFAs (%d); coverage/generator problem", singleN)
	}
}

// computeBoundariesRef 是 computeBoundaries 的独立 rune 级参考实现 (即原 boundaryConds 逐 rune 版),
// 供 TestComputeBoundariesASCIIFastPath 对照新版 ASCII 快路径逐字节一致.
func computeBoundariesRef(data []byte, buf []uint8) []uint8 {
	n := len(data)
	if cap(buf) < n+1 {
		buf = make([]uint8, n+1)
	} else {
		buf = buf[:n+1]
	}
	prev := rune(-1)
	i := 0
	for i < n {
		r, size := utf8.DecodeRune(data[i:])
		buf[i] = boundaryConds(prev, r)
		prev = r
		i += size
	}
	buf[n] = boundaryConds(prev, -1)
	return buf
}

// TestComputeBoundariesASCIIFastPath 对照新版 computeBoundaries (含 ASCII 快路径) 与独立 rune 级
// 参考实现逐字节一致, 覆盖纯 ASCII / 混合多字节 / 非法 UTF-8 / 空输入 / 单字节各类边界条件.
func TestComputeBoundariesASCIIFastPath(t *testing.T) {
	r := rand.New(rand.NewSource(0xB01D))
	cases := [][]byte{
		nil,
		{},
		{'a'},
		{'_'},
		{'\n'},
		{0x80},        // 非法 UTF-8 首字节
		{0xC0, 0xAF},  // 非法 overlong
		[]byte("hello world\nfoo bar"),
		[]byte("  \t\n\n  "),
		[]byte("café résumé 日本語"),
		[]byte("abc\x80def\xC0\xAFghi"), // 非法 UTF-8 夹杂
		[]byte("_word_boundary_"),
		[]byte("line1\nline2\nline3\n"),
		[]byte("Ünïcödé"),
	}
	// 随机长输入: 纯 ASCII / 混合 / 含非法字节.
	for k := 0; k < 200; k++ {
		n := r.Intn(512)
		data := make([]byte, n)
		kind := r.Intn(3)
		for i := range data {
			switch kind {
			case 0: // 纯 ASCII (含控制/字母/数字/符号)
				data[i] = byte(r.Intn(128))
			case 1: // 混合: 多为 ASCII, 偶尔多字节首字节
				if r.Intn(8) == 0 {
					data[i] = byte(0xC0 + r.Intn(0x30)) // 可能非法续
				} else {
					data[i] = byte(r.Intn(128))
				}
			default: // 任意字节 (含非法 UTF-8)
				data[i] = byte(r.Intn(256))
			}
		}
		cases = append(cases, data)
	}
	var ref, got []byte
	mism := 0
	for i, data := range cases {
		ref = computeBoundariesRef(data, ref)
		got = computeBoundaries(data, got)
		if len(ref) != len(got) {
			t.Errorf("case %d len mismatch: ref=%d got=%d data=%q", i, len(ref), len(got), data)
			continue
		}
		for j := range ref {
			if ref[j] != got[j] {
				if mism < 16 {
					t.Errorf("case %d pos %d: ref=%08b got=%08b data=%q", i, j, ref[j], got[j], data)
				}
				mism++
			}
		}
	}
	if mism > 0 {
		t.Fatalf("computeBoundaries ASCII fast path: %d byte mismatches vs rune-level reference", mism)
	}
	t.Logf("computeBoundaries ASCII fast path: %d cases all byte-identical to rune-level reference", len(cases))
}
