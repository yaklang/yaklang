//go:build cgo && minirehs_mvs

package minirehs

import (
	"fmt"
	"math/rand"
	"regexp"
	"regexp/syntax"
	"strings"
	"testing"
)

// 本文件是纯 C99 运行期内核 (native/mvscan) 的差分护栏, 仅在 `cgo && minirehs_mvs` 构建运行.
// 目标: 证明 C 内核与 Go 参考执行器 (existsIn / merged scanExist) 逐位一致, 且端到端与 stdlib
// oracle 一致. 触及 C 内核 / blob 契约 / utf8 解码 / 字母表者必须重跑本文件 + 真实流量差分.
//
// 关键词: mvscan, cgo, 差分验证, bit-identical, C kernel

// getMVSDB 从已编译的 Database 取出内部 *mvsDB (database -> compositeDB -> mvsDB).
func getMVSDB(t *testing.T, db Database) *mvsDB {
	t.Helper()
	d, ok := db.(*database)
	if !ok {
		t.Fatalf("db is not *database: %T", db)
	}
	c, ok := d.primary.(*compositeDB)
	if !ok {
		t.Fatalf("primary is not *compositeDB: %T", d.primary)
	}
	m, ok := c.primary.(*mvsDB)
	if !ok {
		t.Fatalf("composite.primary is not *mvsDB: %T", c.primary)
	}
	return m
}

// TestMVSKernelAvailable 确认本构建确实编入了 C 内核 (否则差分测试无意义).
func TestMVSKernelAvailable(t *testing.T) {
	if !mvsKernelAvailable() {
		t.Fatal("expected C kernel available under cgo && minirehs_mvs build")
	}
}

// TestMVSKernelAnchoredBatchBackendOracle 接通 C anchored-many 的端到端 A/B 路径。
// 默认生产开关保持关闭，直到 C 侧能把 literal trigger 与局部 verifier 融为一趟；此测试
// 保证未来重新评估时，该批处理路径仍和 stdlib oracle 完全一致。
func TestMVSKernelAnchoredBatchBackendOracle(t *testing.T) {
	old := anchorCBatchEnabled
	anchorCBatchEnabled = true
	defer func() { anchorCBatchEnabled = old }()

	patterns, _ := compilableMITMPatterns(t)
	db, err := Compile(patterns, WithBackend(BackendMVS))
	if err != nil {
		t.Fatalf("compile mvs: %v", err)
	}
	defer db.Close()
	oracle, err := Compile(patterns, WithBackend(BackendStdlib))
	if err != nil {
		t.Fatalf("compile oracle: %v", err)
	}
	defer oracle.Close()
	records, _ := loadCorpus(t)
	if testing.Short() && len(records) > 200 {
		records = records[:200]
	}
	for i, rec := range records {
		got := mvsExistIDs(t, db, rec)
		want := mvsExistIDs(t, oracle, rec)
		mvsAssertSameIDSet(t, got, want, fmt.Sprintf("anchored-c-batch-record#%d", i))
	}
}

// BenchmarkMVSCAnchoredBatchAB 衡量 C anchored-many 与默认 Go 单字 gap-jump 的净差异。
// 该 A/B 只在 cgo+mvs 构建中存在，避免将尚未验收的 C 调度混入主基准。
func BenchmarkMVSCAnchoredBatchAB(b *testing.B) {
	patterns := re2OnlyMITMPatterns(b)
	records, _ := loadCorpusB(b)
	old := anchorCBatchEnabled
	defer func() { anchorCBatchEnabled = old }()
	for _, tc := range []struct {
		name    string
		enabled bool
	}{
		{"GoGapJump", false},
		{"CBatchGapJump", true},
	} {
		b.Run(tc.name, func(b *testing.B) {
			anchorCBatchEnabled = tc.enabled
			benchScanRecordsOpts(b, patterns, records, WithBackend(BackendMVS), WithReportLocation(false))
		})
	}
}

// openSingleNFAKernel 把一条 NFA 序列化为 blob 并打开 C 内核 (pattern 槽位仅 idx 0).
func openSingleNFAKernel(t *testing.T, nfa *mvsNFA) *mvsKernel {
	t.Helper()
	blob := buildMVSBlob([]*mvsNFA{nfa}, nil, nil)
	k := openMVSKernel(blob, 1)
	if k == nil {
		t.Fatal("openMVSKernel returned nil for valid single-NFA blob")
	}
	return k
}

// TestMVSKernelExistsDirect 用固定 pattern + 定向输入, 直接比对 C nfaExists 与 Go existsIn
// 及 stdlib oracle 三方一致.
func TestMVSKernelExistsDirect(t *testing.T) {
	cases := []struct {
		expr   string
		inputs []string
	}{
		{`AKIA[0-9A-Z]{16}`, []string{"AKIAABCDEFGHIJKLMNOP", "xxAKIAABCDEFGHIJKLMNOPyy", "AKIAshort"}},
		{`Druid`, []string{"Druid", "xDruidy", "druid", "Dru"}},
		{`swagger-ui\.html`, []string{"/swagger-ui.html", "swagger-uiXhtml", "swagger-ui.htm"}},
		{`[0-9]{3,}`, []string{"12", "123", "99999", "ab123cd"}},
		{`(GET|POST|PUT)`, []string{"GET /", "a POST b", "PUTx", "PATCH"}},
		{`\d{1,3}\.\d{1,3}`, []string{"10.0", "255.255", "1.2.3.4", "abc", ".5"}},
		{`^GET`, []string{"GET /x", "xGET", "GE", " GET"}},
		{`END$`, []string{"the END", "ENDx", "END"}},
		{`ab+c`, []string{"abc", "abbbbc", "ac", "xabcy"}},
		{`a[bc]*d`, []string{"ad", "abcbcd", "abx d", "aXd"}},
		{`(?i)druid`, []string{"DRUID", "DrUiD", "druid", "drui"}},
		{`colou?r`, []string{"color", "colour", "colouur"}},
		{`a.c`, []string{"abc", "axc", "a\nc", "ac"}},
		{`<[^>]+>`, []string{"<a>", "<abc>", "<>", "x<tag>y"}},
		{`https?://`, []string{"http://x", "https://y", "htt://", "xhttps://"}},
		// 多字节 / 非 ASCII: 校验 C utf8 解码与字母表压缩.
		{`[\x{4e00}-\x{9fff}]+`, []string{"中文", "abc中def", "abc", "测试123"}},
		{`café`, []string{"café", "cafe", "xcaféy"}},
	}
	for _, c := range cases {
		re := regexp.MustCompile(c.expr)
		nfa := buildNFAFor(t, c.expr)
		if nfa == nil {
			t.Logf("expr=%q routed to fallback (no NFA)", c.expr)
			continue
		}
		k := openSingleNFAKernel(t, nfa)
		for _, in := range c.inputs {
			b := []byte(in)
			want := re.Match(b)
			goHit := nfa.existsIn(b)
			cHit := k.nfaExists(0, b)
			if goHit != want {
				t.Errorf("expr=%q input=%q: go=%v oracle=%v", c.expr, in, goHit, want)
			}
			if cHit != want {
				t.Errorf("expr=%q input=%q: C=%v oracle=%v", c.expr, in, cHit, want)
			}
		}
		k.close()
	}
}

// TestMVSKernelRandomDifferential 随机生成大量 RE2, 在随机字节 (含非法 UTF-8) 上比对
// C nfaExists == Go existsIn == stdlib oracle. 这是 C utf8 解码 / 字母表 / 位并行递推的主护栏.
func TestMVSKernelRandomDifferential(t *testing.T) {
	r := rand.New(rand.NewSource(0x5EED7))
	iters := diffIters(t, defaultDiffIters)
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
		k := openSingleNFAKernel(t, nfa)
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
			goHit := nfa.existsIn(in)
			cHit := k.nfaExists(0, in)
			if goHit != want {
				t.Fatalf("GO MISMATCH expr=%q input=%q go=%v oracle=%v", expr, in, goHit, want)
			}
			if cHit != goHit {
				t.Fatalf("C MISMATCH expr=%q input=%q C=%v go=%v", expr, in, cHit, goHit)
			}
		}
		k.close()
	}
	t.Logf("kernel random differential: tested(NFA)=%d skipped(fallback)=%d", tested, skipped)
	if tested < 1000 {
		t.Fatalf("too few NFA-eligible patterns exercised: %d", tested)
	}
}

// TestMVSKernelExistsMITM 用真实 MITM 规则 + 真实流量, 逐 pattern 逐记录比对 C nfaExists
// 与 Go existsIn (复用同一个 db 的 C 内核, 即真实集成路径).
func TestMVSKernelExistsMITM(t *testing.T) {
	patterns, _ := compilableMITMPatterns(t)
	db, err := Compile(patterns, WithBackend(BackendMVS))
	if err != nil {
		t.Fatalf("compile mvs: %v", err)
	}
	defer db.Close()
	mdb := getMVSDB(t, db)
	if mdb.kernel == nil {
		t.Fatal("expected C kernel active in mvsDB under minirehs_mvs build")
	}

	records, joined := loadCorpus(t)
	if testing.Short() && len(records) > 200 {
		records = records[:200]
	}
	nfaCount := 0
	for _, nfa := range mdb.nfas {
		if nfa != nil {
			nfaCount++
		}
	}
	t.Logf("MITM: %d patterns, %d nfa, %d records", len(patterns), nfaCount, len(records))

	checks := 0
	for ri, rec := range records {
		for idx, nfa := range mdb.nfas {
			if nfa == nil || nfa.hasAssert {
				// 断言 NFA (零宽断言) 按契约不入 C blob, 由 Go existsInAssert 兜底;
				// 其正确性由 mvs_assert_test.go 的差分/oracle 守护, 此处不与 C 比对.
				continue
			}
			goHit := nfa.existsIn(rec)
			cHit := mdb.kernel.nfaExists(idx, rec)
			if cHit != goHit {
				t.Fatalf("record#%d idx=%d expr=%q: C=%v go=%v", ri, idx, mdb.all[idx].expr, cHit, goHit)
			}
			checks++
		}
	}
	// joined 整段也比对 (覆盖更长输入).
	for idx, nfa := range mdb.nfas {
		if nfa == nil || nfa.hasAssert {
			continue
		}
		if mdb.kernel.nfaExists(idx, joined) != nfa.existsIn(joined) {
			t.Fatalf("joined idx=%d expr=%q: C/go mismatch", idx, mdb.all[idx].expr)
		}
	}
	t.Logf("kernel MITM exists checks: %d (all C==Go)", checks)
}

// TestMVSKernelExistsManyMITM 守护 Phase 2 批处理入口: nfaExistsMany (一次 cgo 多条) 的逐 idx
// 结果, 必须与逐条 nfaExists / Go existsIn 完全一致. 把全部非断言 NFA 的 idx 一次性入批比对.
func TestMVSKernelExistsManyMITM(t *testing.T) {
	patterns, _ := compilableMITMPatterns(t)
	db, err := Compile(patterns, WithBackend(BackendMVS))
	if err != nil {
		t.Fatalf("compile mvs: %v", err)
	}
	defer db.Close()
	mdb := getMVSDB(t, db)
	if mdb.kernel == nil {
		t.Fatal("expected C kernel active")
	}
	mscratch := &scratch{}

	// 全部非断言 NFA 的 idx (即 batchable 集合).
	var idxs []int32
	for idx, nfa := range mdb.nfas {
		if nfa != nil && !nfa.hasAssert {
			idxs = append(idxs, int32(idx))
		}
	}

	records, joined := loadCorpus(t)
	if testing.Short() && len(records) > 200 {
		records = records[:200]
	}
	check := func(data []byte, ctx string) {
		res := mdb.kernel.nfaExistsMany(idxs, data, mscratch)
		if len(res) != len(idxs) {
			t.Fatalf("%s: nfaExistsMany returned %d results, want %d", ctx, len(res), len(idxs))
		}
		for i, idx := range idxs {
			many := res[i] == 1
			single := mdb.kernel.nfaExists(int(idx), data)
			goHit := mdb.nfas[idx].existsIn(data)
			if many != single || many != goHit {
				t.Fatalf("%s idx=%d expr=%q: many=%v single=%v go=%v", ctx, idx, mdb.all[idx].expr, many, single, goHit)
			}
		}
	}
	for ri, rec := range records {
		check(rec, fmt.Sprintf("record#%d", ri))
		if t.Failed() {
			t.FailNow()
		}
	}
	check(joined, "joined")
	t.Logf("nfaExistsMany MITM: %d idx x %d records (all many==single==go)", len(idxs), len(records))
}

// TestMVSKernelMergedMITM 比对合并 always-on 自动机: C mergedScan 命中集合 == Go scanExist
// 命中集合 (真实流量 + joined). 覆盖混合锚点 / 多字 / 多字节合并自动机.
func TestMVSKernelMergedMITM(t *testing.T) {
	patterns, _ := compilableMITMPatterns(t)
	db, err := Compile(patterns, WithBackend(BackendMVS))
	if err != nil {
		t.Fatalf("compile mvs: %v", err)
	}
	defer db.Close()
	mdb := getMVSDB(t, db)
	if mdb.merged == nil {
		t.Skip("no merged always-on NFA in this ruleset")
	}
	if mdb.kernel == nil {
		t.Fatal("expected C kernel active")
	}
	sc := &scratch{}

	records, joined := loadCorpus(t)
	if testing.Short() && len(records) > 200 {
		records = records[:200]
	}
	inputs := append(append([][]byte{}, records...), joined)
	for ri, rec := range inputs {
		cSet := toIntSet(mdb.kernel.mergedScan(rec, sc))
		goSeen := make([]bool, mdb.n)
		goSet := toIntSet(mdb.merged.scanExist(rec, goSeen, nil))
		if !sameIntSet(cSet, goSet) {
			t.Fatalf("merged input#%d(len=%d): C=%v go=%v", ri, len(rec), cSet, goSet)
		}
	}
	t.Logf("kernel merged MITM: %d inputs, C==Go hit sets", len(inputs))
}

// TestMVSKernelMergedRandom 合成随机 always-on NFA 合并自动机, 随机字节上比对 C/Go 命中集合.
func TestMVSKernelMergedRandom(t *testing.T) {
	r := rand.New(rand.NewSource(0xBEEF))
	// 收集若干无锚 always-on NFA 作为成员.
	var members []mergeMember
	idx := 0
	for len(members) < 12 && idx < 4000 {
		expr := genRE(r, 2)
		parsed, err := syntax.Parse(expr, syntax.Perl)
		idx++
		if err != nil {
			continue
		}
		nfa, ok := compileMVSNFA(parsed.Simplify())
		if !ok {
			continue
		}
		members = append(members, mergeMember{idx: len(members), nfa: nfa})
	}
	if len(members) < 4 {
		t.Skipf("too few members: %d", len(members))
	}
	merged := buildMergedNFA(members)
	npat := len(members)
	nfas := make([]*mvsNFA, npat)
	for _, m := range members {
		nfas[m.idx] = m.nfa
	}
	blob := buildMVSBlob(nfas, merged, nil)
	k := openMVSKernel(blob, npat)
	if k == nil {
		t.Fatal("openMVSKernel nil")
	}
	defer k.close()
	sc := &scratch{}

	for it := 0; it < 4000; it++ {
		n := r.Intn(40)
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
		cSet := toIntSet(k.mergedScan(in, sc))
		goSeen := make([]bool, npat)
		goSet := toIntSet(merged.scanExist(in, goSeen, nil))
		if !sameIntSet(cSet, goSet) {
			t.Fatalf("merged random input=%q: C=%v go=%v", in, cSet, goSet)
		}
	}
}

// TestMVSKernelSIMDScalarTwin (P4-M3) 验证 SIMD 加速档与标量孪生逐位一致: 选取 nword>=2
// 的多字 NFA (SIMD 与标量在字向量 OR/AND/COPY 上走不同实现), 在随机/定向输入上比对
// nfaExists (默认 SIMD 分发) == nfaExistsScalar (强制标量) == Go existsIn == stdlib oracle.
func TestMVSKernelSIMDScalarTwin(t *testing.T) {
	// 这些 expr 展开后位置数 >64, 触发 nword>=2 (走 SIMD 字向量路径).
	exprs := []string{
		`[a-z]{70}`,
		`[0-9a-f]{80}`,
		`(abc|def|ghi){25}`,
		`\d{1,3}(\.\d{1,3}){25}`,
		`[A-Za-z0-9_]{65,90}`,
		`(GET|POST|PUT|DELETE)[a-z ]{40,60}`,
	}
	r := rand.New(rand.NewSource(0x51 + 0xD))
	multiWord, checks := 0, 0
	for _, expr := range exprs {
		nfa := buildNFAFor(t, expr)
		if nfa == nil {
			t.Logf("expr=%q fallback (skip)", expr)
			continue
		}
		if nfa.nword < 2 {
			t.Logf("expr=%q nword=%d (<2, skip)", expr, nfa.nword)
			continue
		}
		multiWord++
		re := regexp.MustCompile(expr)
		k := openSingleNFAKernel(t, nfa)
		if !k.simdEnabled() {
			t.Skip("SIMD tier not compiled on this arch; scalar-only build")
		}
		for s := 0; s < 200; s++ {
			n := r.Intn(160)
			in := make([]byte, n)
			for i := range in {
				switch r.Intn(3) {
				case 0:
					in[i] = byte("abcdef0123.GETPOST _"[r.Intn(20)])
				case 1:
					in[i] = byte(r.Intn(128))
				default:
					in[i] = byte(r.Intn(256))
				}
			}
			want := re.Match(in)
			simd := k.nfaExists(0, in)
			scal := k.nfaExistsScalar(0, in)
			goHit := nfa.existsIn(in)
			checks++
			if simd != scal {
				t.Fatalf("SIMD!=scalar expr=%q nword=%d input=%q simd=%v scalar=%v", expr, nfa.nword, in, simd, scal)
			}
			if simd != goHit {
				t.Fatalf("C!=Go expr=%q input=%q C=%v go=%v", expr, in, simd, goHit)
			}
			if goHit != want {
				t.Fatalf("Go!=oracle expr=%q input=%q go=%v oracle=%v", expr, in, goHit, want)
			}
		}
		k.close()
	}
	t.Logf("SIMD/scalar twin: multiWord NFAs=%d checks=%d (all bit-identical)", multiWord, checks)
	if multiWord < 3 {
		t.Fatalf("too few multi-word (nword>=2) NFAs exercised: %d", multiWord)
	}
}

// TestMVSKernelMergedSIMDScalarTwin (P4-M3) 在真实 MITM 合并自动机 (nword>=2) 上比对
// SIMD 分发 mergedScan 与强制标量 mergedScanScalar 的命中集合逐条一致.
func TestMVSKernelMergedSIMDScalarTwin(t *testing.T) {
	patterns, _ := compilableMITMPatterns(t)
	db, err := Compile(patterns, WithBackend(BackendMVS))
	if err != nil {
		t.Fatalf("compile mvs: %v", err)
	}
	defer db.Close()
	mdb := getMVSDB(t, db)
	if mdb.merged == nil || mdb.kernel == nil {
		t.Skip("no merged NFA / kernel")
	}
	if !mdb.kernel.simdEnabled() {
		t.Skip("scalar-only build")
	}
	t.Logf("merged nword=%d simd=%v", mdb.merged.nword, mdb.kernel.simdEnabled())
	sc1, sc2 := &scratch{}, &scratch{}
	records, joined := loadCorpus(t)
	inputs := append(append([][]byte{}, records...), joined)
	for ri, rec := range inputs {
		simd := toIntSet(mdb.kernel.mergedScan(rec, sc1))
		scal := toIntSet(mdb.kernel.mergedScanScalar(rec, sc2))
		if !sameIntSet(simd, scal) {
			t.Fatalf("merged input#%d(len=%d): simd=%v scalar=%v", ri, len(rec), simd, scal)
		}
	}
	t.Logf("merged SIMD/scalar twin: %d inputs bit-identical", len(inputs))
}

// TestMVSKernelEndToEndOracle 端到端: minirehs_mvs 构建 (C 内核驱动存在性) 与 stdlib oracle
// 在真实流量上命中 ID 集合逐条一致 (这是把 C 内核接进 scan 后的最终护栏).
func TestMVSKernelEndToEndOracle(t *testing.T) {
	patterns, _ := compilableMITMPatterns(t)
	mvs, err := Compile(patterns, WithBackend(BackendMVS))
	if err != nil {
		t.Fatalf("compile mvs: %v", err)
	}
	defer mvs.Close()
	if getMVSDB(t, mvs).kernel == nil {
		t.Fatal("expected C kernel active")
	}
	oracle, err := Compile(patterns, WithBackend(BackendStdlib))
	if err != nil {
		t.Fatalf("compile oracle: %v", err)
	}
	defer oracle.Close()

	records, _ := loadCorpus(t)
	if testing.Short() && len(records) > 200 {
		records = records[:200]
	}
	for i, rec := range records {
		got := mvsExistIDs(t, mvs, rec)
		ora := mvsExistIDs(t, oracle, rec)
		mvsAssertSameIDSet(t, got, ora, fmt.Sprintf("kernel-record#%d(len=%d)", i, len(rec)))
		if t.Failed() {
			t.FailNow()
		}
	}
}

// toIntSet 把 idx 切片转为集合 (sameIntSet 定义在 mvs_merged_test.go, 此处复用).
func toIntSet(in []int) map[int]struct{} {
	s := make(map[int]struct{}, len(in))
	for _, v := range in {
		s[v] = struct{}{}
	}
	return s
}

// TestMVSKernelAnchoredDirect 用固定 pattern + 定向输入 + 手工 spans, 比对 C nfaExistsAnchored
// 与 Go existsInAnchored / existsInAnchored1 及 SIMD/标量孪生四方一致. 覆盖 nword==1 与 nword>=2.
func TestMVSKernelAnchoredDirect(t *testing.T) {
	cases := []struct {
		expr  string
		input string
		spans []anchorSpan
	}{
		{`token=\w+`, "xx token=abc123 yy", []anchorSpan{{4, 10}}},
		{`token=\w+`, "token=abc zz token=def", []anchorSpan{{0, 6}, {14, 20}}},
		{`token=\w+`, "no match here", []anchorSpan{{0, 4}}},
		{`pass[word]?\s*[:=]`, "x password: y", []anchorSpan{{2, 8}}},
		{`AKIA[0-9A-Z]{16}`, "xxAKIAABCDEFGHIJKLMNOPyy", []anchorSpan{{2, 7}}},
		{`a[bc]+d`, "xabcbcdy abcd", []anchorSpan{{1, 2}, {9, 10}}},
		{`https?://[a-z]+`, "visit http://abc or https://xyz", []anchorSpan{{6, 11}, {20, 26}}},
		{`[0-9]{4,6}`, "code 12345 ok 99", []anchorSpan{{5, 10}}},
		{`colou?r`, "color colour", []anchorSpan{{0, 6}, {6, 12}}},
		{`café`, "xcaféy", []anchorSpan{{2, 6}}},
		{`[\x{4e00}-\x{9fff}]+`, "x 中文 y", []anchorSpan{{2, 5}}},
		// nword>=2 (位置数>64) 走 SIMD 档
		{`[a-z]{70}`, strings.Repeat("x", 50) + strings.Repeat("a", 70) + strings.Repeat("y", 30), []anchorSpan{{50, 51}}},
		{`[0-9a-f]{80}`, strings.Repeat("z", 40) + strings.Repeat("0123456789abcdef", 8) + strings.Repeat("z", 40), []anchorSpan{{40, 41}}},
	}
	checks := 0
	for _, c := range cases {
		nfa := buildNFAFor(t, c.expr)
		if nfa == nil {
			t.Logf("expr=%q fallback (skip)", c.expr)
			continue
		}
		data := []byte(c.input)
		k := openSingleNFAKernel(t, nfa)
		var goHit bool
		if nfa.single {
			goHit = nfa.existsInAnchored1(data, c.spans)
		} else if nfa.nword == 2 {
			goHit = nfa.existsInAnchored2(data, c.spans)
		} else {
			prev := make([]uint64, nfa.nword)
			cand := make([]uint64, nfa.nword)
			active := make([]uint64, nfa.nword)
			goHit = nfa.existsInAnchored(data, c.spans, prev, cand, active)
		}
		cHit := k.nfaExistsAnchored(0, data, c.spans)
		cScalarHit := k.nfaExistsAnchoredScalar(0, data, c.spans)
		checks++
		if cHit != goHit {
			t.Errorf("C!=Go expr=%q input=%q spans=%v C=%v go=%v", c.expr, c.input, c.spans, cHit, goHit)
		}
		if cHit != cScalarHit {
			t.Errorf("SIMD!=scalar expr=%q input=%q spans=%v simd=%v scalar=%v", c.expr, c.input, c.spans, cHit, cScalarHit)
		}
		k.close()
	}
	t.Logf("anchored direct: checks=%d (all C==Go==scalar-twin)", checks)
}

// TestMVSKernelAnchoredRandom 随机 pattern + 随机输入 + 随机 spans, 比对 C nfaExistsAnchored
// 与 Go existsInAnchored 逐位一致 (含 SIMD/标量孪生). 覆盖 nword==1 与 nword>=2, 含非法 UTF-8.
func TestMVSKernelAnchoredRandom(t *testing.T) {
	exprs := []string{
		`ab+c`, `[0-9]{3,}`, `https?://`, `colou?r`, `a[bc]*d`, `Druid`,
		`AKIA[0-9A-Z]{16}`, `\d{1,3}\.\d{1,3}`, `(GET|POST|PUT)`, `<[^>]+>`,
		`café`, `[\x{4e00}-\x{9fff}]+`,
		`[a-z]{70}`, `[0-9a-f]{80}`, `(abc|def|ghi){25}`,
	}
	r := rand.New(rand.NewSource(0xA7 + 0xC))
	checks, multiWord := 0, 0
	for _, expr := range exprs {
		nfa := buildNFAFor(t, expr)
		if nfa == nil {
			t.Logf("expr=%q fallback (skip)", expr)
			continue
		}
		if nfa.nword >= 2 {
			multiWord++
		}
		k := openSingleNFAKernel(t, nfa)
		for s := 0; s < 300; s++ {
			n := r.Intn(200)
			data := make([]byte, n)
			for i := range data {
				switch r.Intn(3) {
				case 0:
					data[i] = byte("abcdef0123.GETPOST _"[r.Intn(20)])
				case 1:
					data[i] = byte(r.Intn(128))
				default:
					data[i] = byte(r.Intn(256))
				}
			}
			nspan := 1 + r.Intn(3)
			spans := make([]anchorSpan, nspan)
			for i := range spans {
				lo := r.Intn(n + 1)
				hi := lo + r.Intn(n-lo+1)
				spans[i] = anchorSpan{int32(lo), int32(hi)}
			}
			spans = mergeAnchorSpans(spans)
			var goHit bool
			if nfa.single {
				goHit = nfa.existsInAnchored1(data, spans)
			} else if nfa.nword == 2 {
				goHit = nfa.existsInAnchored2(data, spans)
			} else {
				prev := make([]uint64, nfa.nword)
				cand := make([]uint64, nfa.nword)
				active := make([]uint64, nfa.nword)
				goHit = nfa.existsInAnchored(data, spans, prev, cand, active)
			}
			cHit := k.nfaExistsAnchored(0, data, spans)
			cScalar := k.nfaExistsAnchoredScalar(0, data, spans)
			checks++
			if cHit != cScalar {
				t.Fatalf("SIMD!=scalar expr=%q nword=%d data=%q spans=%v simd=%v scalar=%v", expr, nfa.nword, data, spans, cHit, cScalar)
			}
			if cHit != goHit {
				t.Fatalf("C!=Go expr=%q nword=%d data=%q spans=%v C=%v go=%v", expr, nfa.nword, data, spans, cHit, goHit)
			}
		}
		k.close()
	}
	t.Logf("anchored random diff: exprs=%d (multiWord=%d) checks=%d (all C==Go==scalar)", len(exprs), multiWord, checks)
	if multiWord < 2 {
		t.Fatalf("too few multi-word (nword>=2) NFAs exercised: %d", multiWord)
	}
}

// TestMVSKernelAnchoredRealTraffic 在真实 MITM 规则集 + 真实流量上, 对每条 anchorable lean pattern
// 比对 C nfaExistsAnchored 与 Go existsInAnchored 逐条一致. 用全报文宽 [0,n] 作保守 spans.
func TestMVSKernelAnchoredRealTraffic(t *testing.T) {
	patterns, _ := compilableMITMPatterns(t)
	db, err := Compile(patterns, WithBackend(BackendMVS), WithReportLocation(false))
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	defer db.Close()
	mdb := getMVSDB(t, db)
	if mdb.kernel == nil {
		t.Fatal("expected C kernel active")
	}
	records, _ := loadCorpus(t)
	if testing.Short() && len(records) > 200 {
		records = records[:200]
	}
	anchorCount, checks := 0, 0
	for idx := 0; idx < mdb.n; idx++ {
		if !mdb.anchorable[idx] {
			continue
		}
		nfa := mdb.nfas[idx]
		if nfa == nil || nfa.hasAssert {
			continue
		}
		anchorCount++
		for ri, rec := range records {
			n := len(rec)
			spans := []anchorSpan{{0, int32(n)}}
			var goHit bool
			if nfa.single {
				goHit = nfa.existsInAnchored1(rec, spans)
			} else {
				prev := make([]uint64, nfa.nword)
				cand := make([]uint64, nfa.nword)
				active := make([]uint64, nfa.nword)
				goHit = nfa.existsInAnchored(rec, spans, prev, cand, active)
			}
			cHit := mdb.kernel.nfaExistsAnchored(idx, rec, spans)
			checks++
			if cHit != goHit {
				t.Fatalf("C!=Go anchored idx=%d record#%d len=%d C=%v go=%v", idx, ri, n, cHit, goHit)
			}
		}
	}
	t.Logf("anchored real traffic: anchorable=%d records=%d checks=%d (all C==Go)", anchorCount, len(records), checks)
}

// TestMVSKernelAnchoredEndToEndOracle 端到端: C 内核 anchored 路径激活后, 整体命中集合与
// stdlib oracle 一致 (含 anchorable + biAnchorable 前向走 C 的路径).
func TestMVSKernelAnchoredEndToEndOracle(t *testing.T) {
	patterns, _ := compilableMITMPatterns(t)
	mvs, err := Compile(patterns, WithBackend(BackendMVS), WithReportLocation(false))
	if err != nil {
		t.Fatalf("compile mvs: %v", err)
	}
	defer mvs.Close()
	if getMVSDB(t, mvs).kernel == nil {
		t.Fatal("expected C kernel active")
	}
	oracle, err := Compile(patterns, WithBackend(BackendStdlib))
	if err != nil {
		t.Fatalf("compile oracle: %v", err)
	}
	defer oracle.Close()
	records, _ := loadCorpus(t)
	if testing.Short() && len(records) > 200 {
		records = records[:200]
	}
	for i, rec := range records {
		got := mvsExistIDs(t, mvs, rec)
		ora := mvsExistIDs(t, oracle, rec)
		mvsAssertSameIDSet(t, got, ora, fmt.Sprintf("anchored-e2e-record#%d(len=%d)", i, len(rec)))
		if t.Failed() {
			t.FailNow()
		}
	}
}
