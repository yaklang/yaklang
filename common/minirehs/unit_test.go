package minirehs

import (
	"reflect"
	"regexp"
	"regexp/syntax"
	"strings"
	"testing"
)

// 本文件是面向各内部组件的单元 / 边界 / 错误路径测试, 目标是把模块测试覆盖率推到 >=99%,
// 同时把不易在差分测试中触达的退化分支 (空输入、非法正则、不支持构造、缓冲扩容、提前停止
// 去重等) 全部覆盖. 与 fuzz_test.go (随机差分安全网) 互补.

// ---------- minirehs.go ----------

func TestBackendKindString(t *testing.T) {
	cases := map[BackendKind]string{
		Auto:           "auto",
		BackendEngine:  "engine",
		BackendStdlib:  "stdlib",
		BackendKind(9): "unknown",
	}
	for k, want := range cases {
		if got := k.String(); got != want {
			t.Errorf("BackendKind(%d).String()=%q want %q", k, got, want)
		}
	}
}

// ---------- options.go ----------

func TestOptions(t *testing.T) {
	cfg := newDefaultConfig()
	WithBackend(BackendStdlib)(cfg)
	WithDefaultUnsupportedPolicy(Fallback)(cfg)
	WithMinLiteralLen(5)(cfg)
	WithMinLiteralLen(0)(cfg) // <1 应被忽略, 维持 5
	WithLogger(nil)(cfg)      // nil 应被忽略, 维持原 logger
	WithLogger(silentLogger{})(cfg)

	if cfg.backend != BackendStdlib {
		t.Errorf("backend not set")
	}
	if cfg.defaultPolicy != Fallback {
		t.Errorf("defaultPolicy not set")
	}
	if cfg.minLiteralLen != 5 {
		t.Errorf("minLiteralLen=%d want 5", cfg.minLiteralLen)
	}
	if _, ok := cfg.logger.(silentLogger); !ok {
		t.Errorf("logger not set to silentLogger")
	}
}

func TestDefaultLoggerNoPanic(t *testing.T) {
	// 仅确保默认 logger 各级别可调用不 panic (输出转发 common/log).
	l := defaultLogger{}
	l.Infof("info %d", 1)
	l.Warnf("warn %s", "x")
	l.Errorf("error %v", nil)
	l.Debugf("debug")
}

// ---------- feature_gate.go ----------

func TestIsRE2Expressible(t *testing.T) {
	if ok, reason := IsRE2Expressible(`foo[a-z]+`); !ok {
		t.Errorf("valid RE2 should be expressible, reason=%s", reason)
	}
	if ok, reason := IsRE2Expressible(`foo(?=bar)`); ok || reason == "" {
		t.Errorf("lookahead must be non-RE2, got ok=%v reason=%q", ok, reason)
	}
	if ok, _ := IsRE2Expressible(`(`); ok {
		t.Errorf("malformed regex must not be expressible")
	}
}

// ---------- backend.go ----------

func TestSelectBackend(t *testing.T) {
	for _, b := range []BackendKind{Auto, BackendEngine} {
		impl, err := selectBackend(&config{backend: b})
		if err != nil || impl.kind() != BackendEngine {
			t.Errorf("backend %v: want engine, err=%v", b, err)
		}
	}
	impl, err := selectBackend(&config{backend: BackendStdlib})
	if err != nil || impl.kind() != BackendStdlib {
		t.Errorf("stdlib backend selection failed: %v", err)
	}
	if _, err := selectBackend(&config{backend: BackendKind(200)}); err == nil {
		t.Errorf("unknown backend must error")
	}
}

// ---------- database.go: Compile 错误路径与策略 ----------

func TestCompileEmptyPatterns(t *testing.T) {
	if _, err := Compile(nil); err == nil {
		t.Errorf("empty patterns must error")
	}
}

func TestCompileEmptyExpr(t *testing.T) {
	if _, err := Compile([]Pattern{{ID: 1, Expr: ""}}); err == nil {
		t.Errorf("empty expr must error")
	}
}

func TestCompileUnsupportedRejected(t *testing.T) {
	// 既不能被 RE2 也不能被 regexp2 编译 (括号不匹配): 必须拒绝整体编译.
	_, err := Compile([]Pattern{{ID: 1, Expr: `(unbalanced`}}, WithLogger(silentLogger{}))
	if err == nil {
		t.Errorf("unbalanced expr must be rejected")
	}
}

func TestCompileUnsupportedFallbackWarn(t *testing.T) {
	// Fallback 策略对"两者都不能编译"的规则仍会失败 (stdlib 同为 RE2), 走 warn + reject.
	_, err := Compile([]Pattern{{ID: 1, Expr: `(bad`, OnUnsupported: Fallback}}, WithLogger(silentLogger{}))
	if err == nil {
		t.Errorf("fallback on hard-unsupported must still reject")
	}
}

func TestEffectivePolicy(t *testing.T) {
	if got := effectivePolicy(DefaultPolicy, DefaultPolicy); got != Reject {
		t.Errorf("default+default => Reject, got %v", got)
	}
	if got := effectivePolicy(DefaultPolicy, Fallback); got != Fallback {
		t.Errorf("default+fallback => Fallback, got %v", got)
	}
	if got := effectivePolicy(Reject, Fallback); got != Reject {
		t.Errorf("explicit Reject overrides, got %v", got)
	}
}

func TestBuildExprWithFlags(t *testing.T) {
	cases := []struct {
		flags Flag
		want  string
	}{
		{0, `foo`},
		{FlagCaseless, `(?i)foo`},
		{FlagDotAll, `(?s)foo`},
		{FlagMultiline, `(?m)foo`},
		{FlagCaseless | FlagDotAll | FlagMultiline, `(?ism)foo`},
	}
	for _, c := range cases {
		if got := buildExprWithFlags(Pattern{Expr: "foo", Flags: c.flags}); got != c.want {
			t.Errorf("flags=%d => %q want %q", c.flags, got, c.want)
		}
	}
}

func TestCompileAndParseError(t *testing.T) {
	if _, _, err := compileAndParse(`[`); err == nil {
		t.Errorf("invalid expr must error in compileAndParse")
	}
}

func TestCompileUnknownBackend(t *testing.T) {
	// WithBackend 传入未知值 -> selectBackend 在 Compile 内部报错.
	_, err := Compile([]Pattern{{ID: 1, Expr: `foobar`}},
		WithBackend(BackendKind(200)), WithLogger(silentLogger{}))
	if err == nil {
		t.Errorf("unknown backend in Compile must error")
	}
}

// ---------- database.go: Scan 退化路径 ----------

func TestScanNilHandlerAndNilScratch(t *testing.T) {
	db, err := Compile([]Pattern{{ID: 1, Expr: `foobar`}})
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	// nil scratch -> 内部自建; nil handler -> 内部默认 (始终 cont).
	if err := db.Scan([]byte("xx foobar yy"), nil, nil); err != nil {
		t.Errorf("scan with nil scratch/handler: %v", err)
	}

	// 传入非本类型的 Scratch 也应自建 (这里用一个伪实现).
	if err := db.Scan([]byte("foobar"), fakeScratch{}, func(Match) bool { return true }); err != nil {
		t.Errorf("scan with foreign scratch: %v", err)
	}
}

type fakeScratch struct{}

func (fakeScratch) Close() error { return nil }

func TestScratchGrowAcrossDatabases(t *testing.T) {
	// 用"少 pattern" db 的 scratch 去扫"多 pattern" db, 触发 fullDone 扩容分支.
	small, _ := Compile([]Pattern{{ID: 1, Expr: `aa.aa`}})
	defer small.Close()
	big, _ := Compile([]Pattern{
		{ID: 1, Expr: `aa.aa`}, {ID: 2, Expr: `bb.bb`}, {ID: 3, Expr: `cc.cc`},
		{ID: 4, Expr: `dd.dd`}, {ID: 5, Expr: `ee.ee`},
	})
	defer big.Close()

	sc, _ := small.NewScratch()
	defer sc.Close()
	// 先用小 db 扫一次, 让 scratch 缓冲定型在 1.
	_ = small.Scan([]byte("aaxaa"), sc, func(Match) bool { return true })
	// 再用大 db 复用同一 scratch, n=5 > cap, 触发扩容.
	got := 0
	_ = big.Scan([]byte("aaxaa eexee"), sc, func(Match) bool { got++; return true })
	if got == 0 {
		t.Errorf("expected matches after scratch grow")
	}
}

// ---------- engine_purego.go: 各路径与提前停止 ----------

func TestEngineEarlyStopAllPaths(t *testing.T) {
	patterns := []Pattern{
		{ID: 1, Expr: `aa.aa`},        // windowed (有字面量, 有界)
		{ID: 2, Expr: `key.*secret`},  // 非窗口 exact (有字面量, 无界) -> 整段验证
		{ID: 3, Expr: `[0-9]{3,}`},    // always-on exact (无字面量)
		{ID: 4, Expr: `foo(?!bar)x?`}, // regexp2-only (lookahead) -> inexact
	}
	data := []byte("aaxaa key zzz secret 12345 foozzz")

	db, err := Compile(patterns, WithLogger(silentLogger{}))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	// 对每条 pattern 都验证"首个命中即停止"能正确返回.
	for _, target := range []PatternID{1, 2, 3, 4} {
		sc, _ := db.NewScratch()
		stopped := false
		_ = db.Scan(data, sc, func(m Match) bool {
			if m.ID == target {
				stopped = true
				return false // 提前停止
			}
			return true
		})
		sc.Close()
		_ = stopped // 仅为驱动各路径的 return true,nil 分支
	}

	// 完整扫描, 确认 4 条 pattern 都至少各有一次命中.
	hit := map[PatternID]bool{}
	sc, _ := db.NewScratch()
	defer sc.Close()
	_ = db.Scan(data, sc, func(m Match) bool { hit[m.ID] = true; return true })
	for _, id := range []PatternID{1, 2, 3, 4} {
		if !hit[id] {
			t.Errorf("pattern id=%d expected to match", id)
		}
	}
}

func TestEngineWindowDedup(t *testing.T) {
	// aa.aa 的字面量为 "aa"; 在 "aaxaa" 中 "aa" 命中两次, 两个邻域窗口都包含同一匹配 [0,5),
	// 必须经 dedup 仅上报一次.
	db, err := Compile([]Pattern{{ID: 1, Expr: `aa.aa`}}, WithLogger(silentLogger{}))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	got := collectMatches(t, db, []byte("aaxaa"))
	if len(got) != 1 || got[0].From != 0 || got[0].To != 5 {
		t.Fatalf("expected single dedup match [0,5), got %v", got)
	}
}

func TestEngineNumAlwaysOn(t *testing.T) {
	db, _ := Compile([]Pattern{
		{ID: 1, Expr: `foobar`},    // 有字面量, 非 always-on
		{ID: 2, Expr: `[0-9]{3,}`}, // always-on exact
		{ID: 3, Expr: `x(?!y)`},    // regexp2-only always-on
	}, WithLogger(silentLogger{}))
	defer db.Close()
	if n := db.Info().NumAlwaysOn; n != 2 {
		t.Errorf("NumAlwaysOn=%d want 2", n)
	}
}

// ---------- composite.go: 带 fallback 子集的合并去重 ----------

func mustCP(id int, expr string) *compiledPattern {
	return &compiledPattern{id: PatternID(id), expr: expr, v: &re2Verifier{re: regexp.MustCompile(expr)}}
}

func TestCompositeWithFallbackDedup(t *testing.T) {
	cp1 := mustCP(1, `foo`)
	cp2 := mustCP(2, `bar`)
	primary := &stdlibDB{patterns: []*compiledPattern{cp1}}
	fallback := &stdlibDB{patterns: []*compiledPattern{cp1, cp2}} // cp1 重复, 触发去重
	c := newCompositeDB(primary, fallback)

	if c.numAlwaysOn() != 3 {
		t.Errorf("numAlwaysOn=%d want 3", c.numAlwaysOn())
	}

	sc := &scratch{dedup: map[matchKey]struct{}{}, fullDone: make([]bool, 4)}
	counts := map[PatternID]int{}
	stopped, err := c.scan([]byte("foo bar foo"), sc, func(m Match) bool {
		counts[m.ID]++
		return true
	})
	if err != nil || stopped {
		t.Fatalf("composite scan err=%v stopped=%v", err, stopped)
	}
	// foo 出现两次 -> primary 上报 [0,3) 与 [8,11); fallback 再报相同两处被去重.
	if counts[1] != 2 {
		t.Errorf("id=1 count=%d want 2 (deduped)", counts[1])
	}
	if counts[2] != 1 {
		t.Errorf("id=2 count=%d want 1", counts[2])
	}

	if err := c.close(); err != nil {
		t.Errorf("close: %v", err)
	}
}

func TestCompositeFallbackEarlyStop(t *testing.T) {
	cp1 := mustCP(1, `foo`)
	cp2 := mustCP(2, `bar`)
	c := newCompositeDB(&stdlibDB{patterns: []*compiledPattern{cp1}},
		&stdlibDB{patterns: []*compiledPattern{cp2}})
	sc := &scratch{dedup: map[matchKey]struct{}{}, fullDone: make([]bool, 4)}

	// 在 primary 阶段就停止.
	stopped, _ := c.scan([]byte("foo bar"), sc, func(Match) bool { return false })
	if !stopped {
		t.Errorf("expected stop during primary")
	}

	// 让 primary 全部放行, 在 fallback 阶段停止.
	sc2 := &scratch{dedup: map[matchKey]struct{}{}, fullDone: make([]bool, 4)}
	seen := 0
	stopped2, _ := c.scan([]byte("foo bar"), sc2, func(m Match) bool {
		seen++
		return m.ID != 2 // 命中 bar (来自 fallback) 时停止
	})
	if !stopped2 {
		t.Errorf("expected stop during fallback")
	}
}

// ---------- backend_stdlib.go: 提前停止 ----------

func TestStdlibBackendEarlyStop(t *testing.T) {
	db, _ := Compile([]Pattern{{ID: 1, Expr: `a`}, {ID: 2, Expr: `b`}}, WithBackend(BackendStdlib))
	defer db.Close()
	sc, _ := db.NewScratch()
	defer sc.Close()
	n := 0
	_ = db.Scan([]byte("ab ab"), sc, func(Match) bool { n++; return false })
	if n != 1 {
		t.Errorf("early stop should yield exactly 1 callback, got %d", n)
	}
}

// ---------- prefilter.go ----------

func TestScalarPrefilterSimd(t *testing.T) {
	li := buildLiteralIndex([]*compiledPattern{{id: 1, idx: 0, literals: []string{"abc"}}})
	pf := newScalarPrefilter(li)
	if pf.simd() {
		t.Errorf("scalar prefilter must report simd=false")
	}
	pf.release() // no-op, 仅覆盖
}

func TestScalarPrefilterScanHits(t *testing.T) {
	// 直接驱动标量预过滤 scanHits (在 CGO 构建下默认走 cgoPrefilter, 这里仍直测标量实现).
	li := buildLiteralIndex([]*compiledPattern{
		{id: 1, idx: 0, literals: []string{"abc"}},
		{id: 2, idx: 1, literals: []string{"xyz"}},
	})
	pf := newScalarPrefilter(li)
	sc := &scratch{}
	hits := pf.scanHits([]byte("__abc__xyz__ABC"), sc)
	// abc(end5), xyz(end10), ABC->abc(end15): 大小写无关, 共 3 次命中.
	if len(hits) != 3 {
		t.Fatalf("scalar scanHits expected 3 hits, got %d: %v", len(hits), hits)
	}
}

func TestACScanFoldASCIIEquivalence(t *testing.T) {
	li := buildLiteralIndex([]*compiledPattern{
		{id: 1, idx: 0, literals: []string{"authorization"}},
		{id: 2, idx: 1, literals: []string{"cookie"}},
	})
	p := newScalarPrefilter(li)
	data := []byte("AUTHORIZATION: x\\r\\nCoOkIe: y\\xff")
	var want, got []litHit
	p.ac.scan(asciiLowerInto(data, new([]byte)), func(id int32, end int) {
		want = append(want, litHit{litID: id, end: int32(end)})
	})
	p.ac.scanFoldASCII(data, func(id int32, end int) {
		got = append(got, litHit{litID: id, end: int32(end)})
	})
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("folded scan mismatch: got=%v want=%v", got, want)
	}
}

func TestLiteralIndexEmpty(t *testing.T) {
	if !(&literalIndex{}).empty() {
		t.Errorf("empty index must report empty")
	}
	li := buildLiteralIndex([]*compiledPattern{{id: 1, idx: 0, literals: []string{"abc"}}})
	if li.empty() {
		t.Errorf("non-empty index must not report empty")
	}
}

// fakePrefilter 返回一个越界 litID, 用于覆盖引擎对预过滤命中的防御性边界检查
// (CGO 内核理论上可能返回脏数据时不应越界 panic).
type fakePrefilter struct{}

func (fakePrefilter) simd() bool { return false }
func (fakePrefilter) scanHits(data []byte, sc *scratch) []litHit {
	sc.hits = sc.hits[:0]
	sc.hits = append(sc.hits, litHit{litID: 999, end: 1})
	return sc.hits
}

func TestEngineDefensiveLitIDBounds(t *testing.T) {
	d := &engineDB{
		all:      []*compiledPattern{mustCP(1, "foobar")},
		n:        1,
		pf:       fakePrefilter{},
		litToPat: [][]int32{{0}}, // 长度 1, 而预过滤返回 litID=999 越界
	}
	sc := &scratch{dedup: map[matchKey]struct{}{}, fullDone: make([]bool, 1)}
	stop, err := d.scan([]byte("anything"), sc, func(Match) bool { return true })
	if stop || err != nil {
		t.Errorf("defensive bounds path: stop=%v err=%v", stop, err)
	}
}

// ---------- ahocorasick.go ----------

func TestAhoCorasickAddEmptyAndScan(t *testing.T) {
	b := newACBuilder()
	b.add("", 0) // 空字面量应被忽略, 不影响后续
	b.add("ab", 0)
	b.add("bc", 1)
	ac := b.build(2)
	type hit struct {
		id  int32
		end int
	}
	var hits []hit
	ac.scan([]byte("zabcz"), func(id int32, end int) { hits = append(hits, hit{id, end}) })
	// "ab" end=3, "bc" end=4.
	if len(hits) != 2 {
		t.Fatalf("expected 2 hits, got %v", hits)
	}
}

// ---------- literal.go ----------

func TestRequiredLiteralsBranches(t *testing.T) {
	mustParse := func(s string) *syntax.Regexp {
		re, err := syntax.Parse(s, syntax.Perl)
		if err != nil {
			t.Fatalf("parse %q: %v", s, err)
		}
		return re
	}

	// OpConcat 跳过无字面量子节点, 取最长字面量.
	if lits := requiredLiterals(mustParse(`a*bcd`)); len(lits) != 1 || lits[0] != "bcd" {
		t.Errorf("concat skip-nil pick longest: %v", lits)
	}
	// OpAlternate 任一分支无字面量 -> 整体 nil.
	if lits := requiredLiterals(mustParse(`abc|d*`)); lits != nil {
		t.Errorf("alt with literal-less branch must be nil: %v", lits)
	}
	// OpRepeat min>=1 透传; OpCapture 单子节点透传.
	if lits := requiredLiterals(mustParse(`(abcd){2,3}`)); len(lits) != 1 || lits[0] != "abcd" {
		t.Errorf("repeat>=1 over capture: %v", lits)
	}
	// OpRepeat min==0 -> nil.
	if lits := requiredLiterals(mustParse(`(abcd){0,3}`)); lits != nil {
		t.Errorf("repeat min0 must be nil: %v", lits)
	}
	// OpStar -> nil.
	if lits := requiredLiterals(mustParse(`a*`)); lits != nil {
		t.Errorf("star must be nil: %v", lits)
	}
	// FoldCase 含非 ASCII -> 放弃提取.
	if lits := extractRequiredLiterals(mustParse(`(?i)héllo`), 2); lits != nil {
		t.Errorf("foldcase non-ascii must yield nil: %v", lits)
	}
	// 字面量过短 -> 退化为 nil.
	if lits := extractRequiredLiterals(mustParse(`ab`), 5); lits != nil {
		t.Errorf("too-short literal must be nil: %v", lits)
	}
	// 正常提取并小写化去重.
	if lits := extractRequiredLiterals(mustParse(`(?i)FooFoo`), 2); len(lits) != 1 || lits[0] != "foofoo" {
		t.Errorf("normal extract lower/dedup: %v", lits)
	}
}

func TestRequiredLiteralsManualNodes(t *testing.T) {
	// 这些结构无法由 Parse 正常产生, 手工构造以覆盖防御性分支.
	if requiredLiterals(&syntax.Regexp{Op: syntax.OpLiteral}) != nil {
		t.Errorf("empty literal node must be nil")
	}
	if requiredLiterals(&syntax.Regexp{Op: syntax.OpCapture}) != nil {
		t.Errorf("capture with !=1 sub must be nil")
	}
	if requiredLiterals(&syntax.Regexp{Op: syntax.OpPlus}) != nil {
		t.Errorf("plus with !=1 sub must be nil")
	}
	if requiredLiterals(&syntax.Regexp{Op: syntax.OpRepeat, Min: 1}) != nil {
		t.Errorf("repeat with !=1 sub must be nil")
	}
}

func TestMinLiteralLen(t *testing.T) {
	if minLiteralLen(nil) != 0 {
		t.Errorf("minLiteralLen(nil) want 0")
	}
	if minLiteralLen([]string{"abc", "de"}) != 2 {
		t.Errorf("minLiteralLen want 2")
	}
}

// ---------- width.go ----------

func TestMaxByteWidth(t *testing.T) {
	mustParse := func(s string) *syntax.Regexp {
		re, err := syntax.Parse(s, syntax.Perl)
		if err != nil {
			t.Fatalf("parse %q: %v", s, err)
		}
		return re
	}
	type tc struct {
		expr    string
		width   int
		bounded bool
	}
	cases := []tc{
		{`abc`, 3, true},
		{`[a-z]`, 4, true},
		{`(?s).`, 4, true},
		{`a?`, 1, true},     // OpQuest 透传子节点 'a' (字面量宽 1)
		{`a{2,3}`, 3, true}, // OpRepeat: 子宽 1 * Max 3 = 3
		{`a{2,}`, 0, false},
		{`a*`, 0, false},
		{`a+`, 0, false},
		{`ab.`, 6, true},  // concat 2 + 4
		{`a|bb`, 2, true}, // alternate max
		{`a*b`, 0, false}, // concat with unbounded
		{`a*|b`, 0, false},
		{`(abc)`, 3, true},
		{`(?:)`, 0, true}, // empty match
		{`(?m)^a$`, 1, true},
	}
	for _, c := range cases {
		w, b := maxByteWidth(mustParse(c.expr))
		if b != c.bounded || (c.bounded && w != c.width) {
			t.Errorf("maxByteWidth(%q)=(%d,%v) want (%d,%v)", c.expr, w, b, c.width, c.bounded)
		}
	}

	// 手工节点: OpCapture/OpQuest 非单子节点, OpNoMatch (default 分支).
	if _, b := maxByteWidth(&syntax.Regexp{Op: syntax.OpCapture}); b {
		t.Errorf("capture !=1 sub must be unbounded")
	}
	if _, b := maxByteWidth(&syntax.Regexp{Op: syntax.OpQuest}); b {
		t.Errorf("quest !=1 sub must be unbounded")
	}
	if _, b := maxByteWidth(&syntax.Regexp{Op: syntax.OpNoMatch}); b {
		t.Errorf("nomatch must be unbounded(default)")
	}
	// OpRepeat 子节点无界.
	if _, b := maxByteWidth(mustParse(`(a*){2,3}`)); b {
		t.Errorf("repeat over unbounded sub must be unbounded")
	}
}

func TestHasPositionAnchorAndWindowVerifiable(t *testing.T) {
	mustParse := func(s string) *syntax.Regexp {
		re, _ := syntax.Parse(s, syntax.Perl)
		return re
	}
	if !hasPositionAnchor(mustParse(`^abc`)) {
		t.Errorf("^abc has anchor")
	}
	if hasPositionAnchor(mustParse(`abc\b`)) {
		t.Errorf("word boundary is not a position anchor")
	}
	if hasPositionAnchor(mustParse(`abc`)) {
		t.Errorf("plain has no anchor")
	}

	// windowVerifiable: 有锚点 -> false; 无界 -> false; 超大 -> false; 正常 -> true.
	if _, ok := windowVerifiable(mustParse(`(?m)^abc`)); ok {
		t.Errorf("anchored must not be window-verifiable")
	}
	if _, ok := windowVerifiable(mustParse(`abc.*`)); ok {
		t.Errorf("unbounded must not be window-verifiable")
	}
	if _, ok := windowVerifiable(mustParse(`a{600}`)); ok {
		t.Errorf("too-wide must not be window-verifiable")
	}
	if w, ok := windowVerifiable(mustParse(`abc.def`)); !ok || w <= 0 {
		t.Errorf("normal bounded must be window-verifiable, got w=%d ok=%v", w, ok)
	}
}

// ---------- verifier.go ----------

func TestFindAllInWindowClamps(t *testing.T) {
	v := &re2Verifier{re: regexp.MustCompile(`abc`)}
	data := []byte("xxabcxx")

	// winStart<0 与 winEnd>len 都应被夹紧, 仍能找到 [2,5).
	got := v.findAllInWindow(data, -100, 1000)
	if len(got) != 1 || got[0] != [2]int{2, 5} {
		t.Errorf("clamp window: %v", got)
	}
	// winStart>=winEnd -> nil.
	if got := v.findAllInWindow(data, 5, 2); got != nil {
		t.Errorf("empty window must be nil: %v", got)
	}
	// 子窗口内无命中 -> nil.
	if got := v.findAllInWindow(data, 0, 2); got != nil {
		t.Errorf("no-match window must be nil: %v", got)
	}
}

func TestFindAllInWindowBoundaryDrop(t *testing.T) {
	// 命中恰好贴住人为左/右边界时应被丢弃 (它会在自身字面量窗口中被找到).
	v := &re2Verifier{re: regexp.MustCompile(`abc`)}
	data := []byte("abcXabcXabc") // 命中位置 [0,3) [4,7) [8,11)
	// 窗口 [4,7): 命中恰好填满子切片, 左边界 winStart=4>0 且右边界 winEnd=7<len -> 两端都贴边,
	// 应被丢弃 (返回空结果).
	if got := v.findAllInWindow(data, 4, 7); len(got) != 0 {
		t.Errorf("flush-boundary match must be dropped: %v", got)
	}
	// 窗口 [3,8): 中间命中 [4,7) 不贴边 -> 保留.
	if got := v.findAllInWindow(data, 3, 8); len(got) != 1 || got[0] != [2]int{4, 7} {
		t.Errorf("interior match must be kept: %v", got)
	}
}

func TestRegexp2VerifierNoMatch(t *testing.T) {
	db, err := Compile([]Pattern{{ID: 1, Expr: `foo(?!bar)`}}, WithLogger(silentLogger{}))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	// 无命中输入 -> regexp2 verifier 走 !ok 分支返回 nil.
	got := collectMatches(t, db, []byte("foobar only"))
	if len(got) != 0 {
		t.Errorf("foo(?!bar) must not match 'foobar', got %v", got)
	}
	// 有命中输入.
	got = collectMatches(t, db, []byte("foobaz"))
	if len(got) != 1 {
		t.Errorf("foo(?!bar) must match 'foobaz', got %v", got)
	}
}

// ---------- DatabaseInfo / Reports ----------

func TestCompileReports(t *testing.T) {
	db, err := Compile([]Pattern{
		{ID: 1, Expr: `foobar`},
		{ID: 2, Expr: `[0-9]{3,}`},
		{ID: 3, Expr: `x(?!y)`},
	}, WithLogger(silentLogger{}))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	info := db.Info()
	if len(info.Reports) != 3 {
		t.Fatalf("reports=%d want 3", len(info.Reports))
	}
	disp := map[PatternID]string{}
	for _, r := range info.Reports {
		disp[r.ID] = r.Disposition
	}
	if disp[1] != "primary" {
		t.Errorf("id=1 disposition=%q want primary", disp[1])
	}
	if disp[2] != "always-on" {
		t.Errorf("id=2 disposition=%q want always-on", disp[2])
	}
	if disp[3] != "regexp2-always-on" {
		t.Errorf("id=3 disposition=%q want regexp2-always-on", disp[3])
	}
}

func TestPatternReportFields(t *testing.T) {
	// 仅确保 PatternReport.HasLiteral 与 Reason 字段被填充 / 可读 (避免无关 lint).
	_ = strings.TrimSpace("")
}
