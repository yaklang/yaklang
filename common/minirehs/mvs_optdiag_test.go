package minirehs

import (
	"math/bits"
	"regexp/syntax"
	"sort"
	"strings"
	"testing"
	"time"
	"unicode/utf8"
)

// nowNanos 返回当前单调纳秒 (诊断计时用).
func nowNanos() int64 { return time.Now().UnixNano() }

// requiredLiteralFactors 返回 pattern 的"必需字面量因子"合取范式: [][]string, 每个内层 []string
// 是一个 OR 集 (任一命中满足该因子), 多个因子之间是 AND (全部因子都需满足才可能命中).
// 与 extractRequiredLiterals (只取单个最长 OR 集) 不同, 它把 concat 各部分的必需字面量都作为
// 独立必要条件. 任一命中 => 所有因子都出现 (concat 语义), 故"任一因子缺失则跳过"绝不漏报.
func requiredLiteralFactors(re *syntax.Regexp, minLen int) [][]string {
	raw := factorsOf(re)
	var out [][]string
	seenFactor := make(map[string]struct{})
	for _, f := range raw {
		if len(f) == 0 {
			continue
		}
		norm := make([]string, 0, len(f))
		seen := make(map[string]struct{}, len(f))
		ok := true
		for _, l := range f {
			if len([]byte(l)) < minLen {
				ok = false
				break
			}
			low := strings.ToLower(l)
			if _, d := seen[low]; d {
				continue
			}
			seen[low] = struct{}{}
			norm = append(norm, low)
		}
		if !ok || len(norm) == 0 {
			continue
		}
		sort.Strings(norm)
		key := strings.Join(norm, "\x00")
		if _, d := seenFactor[key]; d {
			continue
		}
		seenFactor[key] = struct{}{}
		out = append(out, norm)
	}
	return out
}

// factorsOf 递归求合取因子 (未做 minLen/小写规整, 由 requiredLiteralFactors 后处理).
func factorsOf(re *syntax.Regexp) [][]string {
	switch re.Op {
	case syntax.OpLiteral:
		if len(re.Rune) == 0 {
			return nil
		}
		if re.Flags&syntax.FoldCase != 0 {
			for _, r := range re.Rune {
				if r > 127 {
					return nil
				}
			}
		}
		return [][]string{{string(re.Rune)}}
	case syntax.OpConcat:
		var all [][]string
		for _, sub := range re.Sub {
			all = append(all, factorsOf(sub)...)
		}
		return all
	case syntax.OpAlternate:
		// 跨分支只能保证"并集 OR 因子"为必要 (单一因子); 各分支必须都提供非空必需集.
		lits := requiredLiterals(re)
		if len(lits) == 0 {
			return nil
		}
		return [][]string{lits}
	case syntax.OpCapture, syntax.OpPlus:
		if len(re.Sub) == 1 {
			return factorsOf(re.Sub[0])
		}
		return nil
	case syntax.OpRepeat:
		if re.Min >= 1 && len(re.Sub) == 1 {
			return factorsOf(re.Sub[0])
		}
		return nil
	default:
		return nil
	}
}

// TestMVSOptDiag 数据驱动诊断: 在真实 MITM 规则 + 真实流量上, 统计每条 pattern 的工作量分布
// (字面量触发记录数 / 窗口扫描字节总量 / 窗口是否尾部无界 / always-on 全段扫描成本), 把
// profile 的 "C内核NFA / regexp2 / assert" 三块成本精确归因到具体 pattern, 指导优化方向.
//
// 仅诊断, 不改任何生产逻辑; 复用内部 mvsDB 结构. 运行: go test -run TestMVSOptDiag -v
func TestMVSOptDiag(t *testing.T) {
	requireDiag(t)
	patterns, names := compilableMITMPatterns(t)
	records, _ := loadCorpus(t)

	db, err := Compile(patterns, WithBackend(BackendMVS), WithReportLocation(false), WithLogger(silentLogger{}))
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	defer db.Close()
	d := digMVSDB(t, db)

	scr, _ := db.NewScratch()
	sc := scr.(*scratch)

	var totalBytes int64
	for _, r := range records {
		totalBytes += int64(len(r))
	}
	avgRec := float64(totalBytes) / float64(len(records))

	type stat struct {
		idx            int
		triggerRecords int   // 多少条记录触发了该 pattern 的字面量
		windowBytes    int64 // 窗口扫描字节总量 (batchable: union 窗口; 整段=tailUnbounded)
		nfaHits        int   // 窗口 existsIn 真命中数 (记录级)
		tailUnbounded  bool  // 该 pattern 的窗口 tail 是否无界 (=> hi=n 整段)
		headUnbounded  bool
		category       string
		minWidth       int // NFA 到达接受态的最小字节宽 (存在性下界)
	}
	stats := make([]*stat, d.n)
	for i := range stats {
		stats[i] = &stat{idx: i}
	}

	// 分类.
	for i := 0; i < d.n; i++ {
		s := stats[i]
		w := d.win[i]
		s.tailUnbounded = w.tail < 0
		s.headUnbounded = w.head < 0
		switch {
		case d.nfas[i] != nil && d.nfas[i].hasAssert && len(d.all[i].literals) == 0:
			s.category = "assert-always-on"
		case d.nfas[i] != nil && d.nfas[i].hasAssert:
			s.category = "assert-gated"
		case d.gate[i]:
			s.category = "gate(regexp2-recheck)"
		case d.nfas[i] == nil:
			s.category = "fallback(regexp2)"
		case len(d.all[i].literals) == 0:
			s.category = "merged-always-on"
		default:
			s.category = "lean-gated"
		}
		if d.nfas[i] != nil && !d.nfas[i].hasAssert {
			s.minWidth = nfaMinAcceptWidth(d.nfas[i])
		}
	}

	// 逐记录跑预过滤 + 窗口归因 (复刻 scan 的 batch 窗口逻辑).
	triggeredThisRec := make([]bool, d.n)
	for _, data := range records {
		n := len(data)
		if d.pf == nil {
			break
		}
		hits := d.pf.scanHits(data, sc)
		for i := range triggeredThisRec {
			triggeredThisRec[i] = false
		}
		// union 窗口按 idx 累积.
		winLo := make(map[int]int)
		winHi := make(map[int]int)
		for _, h := range hits {
			if int(h.litID) >= len(d.litToPat) {
				continue
			}
			for _, idx32 := range d.litToPat[h.litID] {
				idx := int(idx32)
				if !triggeredThisRec[idx] {
					triggeredThisRec[idx] = true
					stats[idx].triggerRecords++
				}
				lo, hi := d.litSpan(idx, int(h.end), n)
				if cur, ok := winLo[idx]; !ok || lo < cur {
					winLo[idx] = lo
				}
				if cur, ok := winHi[idx]; !ok || hi > cur {
					winHi[idx] = hi
				}
			}
		}
		for idx, lo := range winLo {
			hi := winHi[idx]
			stats[idx].windowBytes += int64(hi - lo)
			if d.nfas[idx] != nil && !d.nfas[idx].hasAssert {
				if d.nfas[idx].existsIn(data[lo:hi]) {
					stats[idx].nfaHits++
				}
			}
		}
	}

	// 报告.
	t.Logf("=== corpus: %d records, %.0f total bytes, avg %.0f B/rec ===", len(records), float64(totalBytes), avgRec)
	cat := map[string]int{}
	catWin := map[string]int64{}
	catTrig := map[string]int64{}
	for _, s := range stats {
		cat[s.category]++
		catWin[s.category] += s.windowBytes
		catTrig[s.category] += int64(s.triggerRecords)
	}
	t.Logf("=== category summary (count / total-window-bytes / total-trigger-records) ===")
	cats := []string{"lean-gated", "gate(regexp2-recheck)", "fallback(regexp2)", "assert-always-on", "assert-gated", "merged-always-on"}
	for _, c := range cats {
		t.Logf("  %-24s n=%-3d windowBytes=%-12d triggerRecords=%d", c, cat[c], catWin[c], catTrig[c])
	}
	t.Logf("  always-on(merged) members=%d, assertAlwaysOn=%d, otherAlwaysOn(fallback)=%d", d.mergedCount, len(d.assertAlwaysOn), len(d.otherAlwaysOn))

	// 尾部无界 vs 有界的窗口字节占比 (lean-gated).
	var tailUnbWin, boundedWin int64
	for _, s := range stats {
		if s.category != "lean-gated" {
			continue
		}
		if s.tailUnbounded || s.headUnbounded {
			tailUnbWin += s.windowBytes
		} else {
			boundedWin += s.windowBytes
		}
	}
	t.Logf("=== lean-gated window bytes: unbounded-side=%d  fully-bounded=%d ===", tailUnbWin, boundedWin)

	// Top patterns by window bytes.
	sort.Slice(stats, func(i, j int) bool { return stats[i].windowBytes > stats[j].windowBytes })
	t.Logf("=== top 25 patterns by window-scan bytes ===")
	for k := 0; k < 25 && k < len(stats); k++ {
		s := stats[k]
		if s.windowBytes == 0 && s.triggerRecords == 0 {
			continue
		}
		nm := names[d.all[s.idx].id]
		t.Logf("  [%-22s] win=%-11d trig=%-5d hits=%-5d tailUnb=%-5v headUnb=%-5v minW=%-3d %s :: %.70s",
			s.category, s.windowBytes, s.triggerRecords, s.nfaHits, s.tailUnbounded, s.headUnbounded, s.minWidth, nm, d.all[s.idx].expr)
	}
}

// TestMVSANDGatePotential 量化"多因子 AND 门控"相对当前单触发的 NFA 扫描削减.
// 当前: 字面量预过滤命中 (单一最长 OR 集) 即对该 pattern 跑窗口 NFA.
// AND 门控: 仅当该 pattern 的所有必需因子 (concat 各部分) 都在记录中出现, 才跑 NFA.
func TestMVSANDGatePotential(t *testing.T) {
	requireDiag(t)
	patterns, names := compilableMITMPatterns(t)
	records, _ := loadCorpus(t)

	db, err := Compile(patterns, WithBackend(BackendMVS), WithReportLocation(false), WithLogger(silentLogger{}))
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	defer db.Close()
	d := digMVSDB(t, db)
	scr, _ := db.NewScratch()
	sc := scr.(*scratch)

	// 为每个 idx 预计算因子 (仅 lean-gated: 有 NFA、非断言、有字面量).
	factors := make([][][]string, d.n)
	multiFactor := 0
	for i := 0; i < d.n; i++ {
		if d.nfas[i] == nil || d.nfas[i].hasAssert || len(d.all[i].literals) == 0 {
			continue
		}
		parsed, err := syntax.Parse(d.all[i].expr, syntax.Perl)
		if err != nil {
			continue
		}
		f := requiredLiteralFactors(parsed.Simplify(), 2)
		factors[i] = f
		if len(f) >= 2 {
			multiFactor++
		}
	}
	t.Logf("lean-gated patterns with >=2 AND factors: %d", multiFactor)

	var curScanRecords, andScanRecords int64
	var curScanBytes, andScanBytes int64
	perPat := make([]struct{ cur, and int64 }, d.n)

	for _, data := range records {
		n := len(data)
		lower := strings.ToLower(string(data))
		hits := d.pf.scanHits(data, sc)
		trig := make([]bool, d.n)
		winLo := make(map[int]int)
		winHi := make(map[int]int)
		for _, h := range hits {
			if int(h.litID) >= len(d.litToPat) {
				continue
			}
			for _, idx32 := range d.litToPat[h.litID] {
				idx := int(idx32)
				trig[idx] = true
				lo, hi := d.litSpan(idx, int(h.end), n)
				if cur, ok := winLo[idx]; !ok || lo < cur {
					winLo[idx] = lo
				}
				if cur, ok := winHi[idx]; !ok || hi > cur {
					winHi[idx] = hi
				}
			}
		}
		for idx := 0; idx < d.n; idx++ {
			if !trig[idx] || factors[idx] == nil {
				continue
			}
			wb := int64(winHi[idx] - winLo[idx])
			curScanRecords++
			curScanBytes += wb
			perPat[idx].cur += wb
			// AND 门控: 所有因子都需出现.
			allPresent := true
			for _, f := range factors[idx] {
				any := false
				for _, lit := range f {
					if strings.Contains(lower, lit) {
						any = true
						break
					}
				}
				if !any {
					allPresent = false
					break
				}
			}
			if allPresent {
				andScanRecords++
				andScanBytes += wb
				perPat[idx].and += wb
			}
		}
	}

	t.Logf("=== AND-gate vs current (lean-gated only) ===")
	t.Logf("  NFA scan records: current=%d  AND-gated=%d  (%.1f%% reduction)",
		curScanRecords, andScanRecords, 100*float64(curScanRecords-andScanRecords)/float64(curScanRecords))
	t.Logf("  NFA scan bytes:   current=%d  AND-gated=%d  (%.1f%% reduction)",
		curScanBytes, andScanBytes, 100*float64(curScanBytes-andScanBytes)/float64(curScanBytes))

	type pr struct {
		idx      int
		cur, and int64
		nfactors int
	}
	var prs []pr
	for i := 0; i < d.n; i++ {
		if factors[i] == nil || perPat[i].cur == 0 {
			continue
		}
		prs = append(prs, pr{i, perPat[i].cur, perPat[i].and, len(factors[i])})
	}
	sort.Slice(prs, func(a, b int) bool { return (prs[a].cur - prs[a].and) > (prs[b].cur - prs[b].and) })
	t.Logf("=== top 20 patterns by AND-gate byte savings ===")
	for k := 0; k < 20 && k < len(prs); k++ {
		p := prs[k]
		t.Logf("  save=%-10d cur=%-10d and=%-10d nfac=%d %s :: %.60s",
			p.cur-p.and, p.cur, p.and, p.nfactors, names[d.all[p.idx].id], d.all[p.idx].expr)
	}
}

// existsAnchoredSinglePass 是"锚定验证"原型 (单趟多点注入): 在 inject[] 为 true 的字节位置
// 注入 first (候选匹配起点), 单趟从 minInject 扫到 scanEnd, 过了最后一个注入位且无活跃线程即停.
// 返回是否命中与实际处理字节数. 这才是运行期形态 (一趟, 不按命中点重复跑).
//
// 正确性: 只找"起点落在某注入位的匹配". 调用方保证任一含触发字面量的匹配起点都被某命中点的
// 有界 head 范围覆盖 (head 有界), 故无假阴; 子串关系无假阳.
func existsAnchoredSinglePass(nfa *mvsNFA, data []byte, inject []bool, minInject, maxInject, scanEnd int) (bool, int) {
	if nfa.hasAssert || nfa.anchoredStart {
		return false, 0
	}
	nword := nfa.nword
	prev := make([]uint64, nword)
	cand := make([]uint64, nword)
	if minInject < 0 {
		minInject = 0
	}
	if scanEnd > len(data) {
		scanEnd = len(data)
	}
	i := minInject
	walked := 0
	for i < scanEnd {
		runeStart := i
		r, size := utf8.DecodeRune(data[i:])
		i += size
		walked += size
		sym := nfa.symbolOf(r)

		for w := range cand {
			cand[w] = 0
		}
		if runeStart < len(inject) && inject[runeStart] {
			copy(cand, nfa.first)
		}
		for w := 0; w < nword; w++ {
			pw := prev[w]
			for pw != 0 {
				p := w*64 + bits.TrailingZeros64(pw)
				fp := nfa.follow[p]
				for k := 0; k < nword; k++ {
					cand[k] |= fp[k]
				}
				pw &= pw - 1
			}
		}
		rc := nfa.reach[sym]
		var anyActive uint64
		for w := 0; w < nword; w++ {
			v := cand[w] & rc[w]
			prev[w] = v
			anyActive |= v
			if v&nfa.lastAny[w] != 0 {
				return true, walked
			}
		}
		if nfa.requireEnd && i == len(data) {
			for w := 0; w < nword; w++ {
				if prev[w]&nfa.lastEnd[w] != 0 {
					return true, walked
				}
			}
		}
		if anyActive == 0 && runeStart >= maxInject {
			return false, walked
		}
	}
	return false, walked
}

// TestMVSAnchoredPotential 量化"锚定验证"(只在 head 有界范围注入 first + 快速失败) 相对当前
// unanchored 整段窗口的字节削减. 仅对 head 有界的 lean-gated pattern 适用 (tail 无界也能受益).
func TestMVSAnchoredPotential(t *testing.T) {
	requireDiag(t)
	patterns, names := compilableMITMPatterns(t)
	records, _ := loadCorpus(t)
	db, err := Compile(patterns, WithBackend(BackendMVS), WithReportLocation(false), WithLogger(silentLogger{}))
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	defer db.Close()
	d := digMVSDB(t, db)
	scr, _ := db.NewScratch()
	sc := scr.(*scratch)

	var curBytes, ancBytes int64
	var curBytesHeadBounded, ancBytesHeadBounded int64
	headBoundedPats := 0
	perPat := make([]struct{ cur, anc int64 }, d.n)
	headBounded := make([]bool, d.n)
	for i := 0; i < d.n; i++ {
		// 真正走 C 内核 batch 全扫的: batchable(!hasAssert) 且非 windowable(无界宽) 且 head 有界、非锚.
		if d.nfas[i] != nil && !d.nfas[i].hasAssert && !d.nfas[i].anchoredStart &&
			!d.windowable[i] && d.win[i].head >= 0 && len(d.all[i].literals) > 0 {
			headBounded[i] = true
			headBoundedPats++
		}
	}
	t.Logf("head-bounded batch (non-windowable, anchored-eligible) patterns: %d", headBoundedPats)

	litLen := map[int]int{}
	injectBuf := make([]bool, 0, 1<<16)
	for _, data := range records {
		n := len(data)
		hits := d.pf.scanHits(data, sc)
		winLo := make(map[int]int)
		winHi := make(map[int]int)
		// 锚定单趟: 每个 idx 维护一个注入位 bool[]、minInject/maxInject、scanEnd(=union hi).
		type ainfo struct {
			inject              []bool
			minInj, maxInj, end int
			has                 bool
		}
		anc := map[int]*ainfo{}
		for _, h := range hits {
			if int(h.litID) >= len(d.litToPat) {
				continue
			}
			ll, ok := litLen[int(h.litID)]
			if !ok {
				ll = litLengthOf(d, int(h.litID))
				litLen[int(h.litID)] = ll
			}
			hitStart := int(h.end) - ll
			for _, idx32 := range d.litToPat[h.litID] {
				idx := int(idx32)
				lo, hi := d.litSpan(idx, int(h.end), n)
				if cur, ok := winLo[idx]; !ok || lo < cur {
					winLo[idx] = lo
				}
				if cur, ok := winHi[idx]; !ok || hi > cur {
					winHi[idx] = hi
				}
				if headBounded[idx] {
					a := anc[idx]
					if a == nil {
						buf := make([]bool, n)
						a = &ainfo{inject: buf, minInj: n, maxInj: 0, end: 0}
						anc[idx] = a
					}
					// 注入区间 [lo, hitStart] (匹配起点候选: head 有界).
					for p := lo; p <= hitStart && p < n; p++ {
						if p >= 0 && !a.inject[p] {
							a.inject[p] = true
						}
					}
					if lo < a.minInj {
						a.minInj = lo
					}
					if hitStart > a.maxInj {
						a.maxInj = hitStart
					}
					if hi > a.end {
						a.end = hi
					}
					a.has = true
				}
			}
		}
		_ = injectBuf
		for idx, lo := range winLo {
			wb := int64(winHi[idx] - lo)
			curBytes += wb
			perPat[idx].cur += wb
			if headBounded[idx] {
				curBytesHeadBounded += wb
			}
		}
		for idx, a := range anc {
			if !a.has {
				continue
			}
			_, walked := existsAnchoredSinglePass(d.nfas[idx], data, a.inject, a.minInj, a.maxInj, a.end)
			ancBytes += int64(walked)
			ancBytesHeadBounded += int64(walked)
			perPat[idx].anc += int64(walked)
		}
	}

	t.Logf("=== anchored vs current window (ALL lean-gated; head-unbounded keep current) ===")
	t.Logf("  head-bounded subset: current=%d  anchored=%d  (%.1f%% reduction)",
		curBytesHeadBounded, ancBytesHeadBounded,
		100*float64(curBytesHeadBounded-ancBytesHeadBounded)/float64(curBytesHeadBounded))

	type pr struct {
		idx      int
		cur, anc int64
	}
	var prs []pr
	for i := 0; i < d.n; i++ {
		if !headBounded[i] || perPat[i].cur == 0 {
			continue
		}
		prs = append(prs, pr{i, perPat[i].cur, perPat[i].anc})
	}
	sort.Slice(prs, func(a, b int) bool { return (prs[a].cur - prs[a].anc) > (prs[b].cur - prs[b].anc) })
	t.Logf("=== top 20 head-bounded patterns by anchored byte savings ===")
	for k := 0; k < 20 && k < len(prs); k++ {
		p := prs[k]
		t.Logf("  save=%-10d cur=%-10d anc=%-10d %s :: %.55s",
			p.cur-p.anc, p.cur, p.anc, names[d.all[p.idx].id], d.all[p.idx].expr)
	}
}

// litLengthOf 返回 litID 对应字面量的字节长度 (从 prefilter 的字面量索引).
func litLengthOf(d *mvsDB, litID int) int {
	if sp, ok := d.pf.(*scalarPrefilter); ok {
		if litID < len(sp.li.literals) {
			return len(sp.li.literals[litID])
		}
	}
	return 0
}

// TestMVSDispositionDump 精确打印每条 pattern 的运行期路径与成本归因 (gate/re2Loc/assert/
// windowable/batchable + 触发记录数 + 实际全段扫描记录数), 厘清 regexp2 / NFA / assert 各成本来源.
func TestMVSDispositionDump(t *testing.T) {
	requireDiag(t)
	patterns, names := compilableMITMPatterns(t)
	records, _ := loadCorpus(t)
	db, err := Compile(patterns, WithBackend(BackendMVS), WithReportLocation(false), WithLogger(silentLogger{}))
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	defer db.Close()
	d := digMVSDB(t, db)
	scr, _ := db.NewScratch()
	sc := scr.(*scratch)

	// 每条 pattern: 实际走哪条路径 + 触发记录数 (字面量命中) + 是否 gate 复核 regexp2.
	path := make([]string, d.n)
	regexp2Recheck := make([]bool, d.n)
	for i := 0; i < d.n; i++ {
		hasNFA := d.nfas[i] != nil
		hasAssert := hasNFA && d.nfas[i].hasAssert
		hasLit := len(d.all[i].literals) > 0
		switch {
		case !hasNFA && hasLit:
			path[i] = "lit->regexp2-fallback"
			regexp2Recheck[i] = true
		case !hasNFA && !hasLit:
			path[i] = "alwayson-regexp2-fallback"
			regexp2Recheck[i] = true
		case d.gate[i]:
			path[i] = "NFA-gate->regexp2-recheck"
			regexp2Recheck[i] = true
		case hasAssert && hasLit:
			path[i] = "lit->existsInAssert(full)"
		case hasAssert && !hasLit:
			path[i] = "alwayson-existsInAssert(full)"
		case d.windowable[i]:
			path[i] = "lit->windowExists(small)"
		case hasLit && d.batchable[i]:
			if d.win[i].head < 0 || d.win[i].tail < 0 {
				path[i] = "lit->batch-nfaExists(UNBOUNDED)"
			} else {
				path[i] = "lit->batch-nfaExists(bounded)"
			}
		case !hasLit:
			path[i] = "merged-alwayson"
		default:
			path[i] = "other"
		}
	}

	trigRecords := make([]int, d.n)
	triggered := make([]bool, d.n)
	for _, data := range records {
		hits := d.pf.scanHits(data, sc)
		for i := range triggered {
			triggered[i] = false
		}
		for _, h := range hits {
			if int(h.litID) >= len(d.litToPat) {
				continue
			}
			for _, idx32 := range d.litToPat[h.litID] {
				idx := int(idx32)
				if !triggered[idx] {
					triggered[idx] = true
					trigRecords[idx]++
				}
			}
		}
	}

	pathCount := map[string]int{}
	pathTrig := map[string]int{}
	for i := 0; i < d.n; i++ {
		pathCount[path[i]]++
		pathTrig[path[i]] += trigRecords[i]
	}
	t.Logf("=== execution path summary (count / total-trigger-records over %d records) ===", len(records))
	paths := make([]string, 0, len(pathCount))
	for p := range pathCount {
		paths = append(paths, p)
	}
	sort.Slice(paths, func(a, b int) bool { return pathTrig[paths[a]] > pathTrig[paths[b]] })
	for _, p := range paths {
		t.Logf("  %-36s n=%-3d trigRecords=%d", p, pathCount[p], pathTrig[p])
	}

	// regexp2 复核 / always-on regexp2 / always-on assert 是否每条记录都跑.
	t.Logf("=== regexp2-recheck patterns (gate/fallback) — each runs regexp2 on triggering records ===")
	for i := 0; i < d.n; i++ {
		if regexp2Recheck[i] {
			t.Logf("  trig=%-5d %-28s %s :: %.50s", trigRecords[i], path[i], names[d.all[i].id], d.all[i].expr)
		}
	}
	t.Logf("=== assert always-on (scan EVERY record, no literal gate) ===")
	for _, idx := range d.assertAlwaysOn {
		t.Logf("  %s :: %.60s", names[d.all[idx].id], d.all[idx].expr)
	}
	t.Logf("=== assert-gated (literal trigger then existsInAssert full-scan) ===")
	for i := 0; i < d.n; i++ {
		if d.nfas[i] != nil && d.nfas[i].hasAssert && len(d.all[i].literals) > 0 {
			t.Logf("  trig=%-5d gate=%-5v %s :: %.50s", trigRecords[i], d.gate[i], names[d.all[i].id], d.all[i].expr)
		}
	}
}

// TestMVSAssertWindowDiag 打印每条断言 NFA 的窗口化资格 (windowed/winW/gate/assertWindowable)
// 与触发记录数, 定位 existsInAssertShared 仍占 15% 的真凶 (是否 always-on / 是否未窗口化).
func TestMVSAssertWindowDiag(t *testing.T) {
	requireDiag(t)
	patterns, names := compilableMITMPatterns(t)
	records, _ := loadCorpus(t)
	db, err := Compile(patterns, WithBackend(BackendMVS), WithReportLocation(false), WithLogger(silentLogger{}))
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	defer db.Close()
	d := digMVSDB(t, db)
	scr, _ := db.NewScratch()
	sc := scr.(*scratch)

	trig := make([]int, d.n)
	for _, data := range records {
		hits := d.pf.scanHits(data, sc)
		seen := make([]bool, d.n)
		for _, h := range hits {
			if int(h.litID) >= len(d.litToPat) {
				continue
			}
			for _, idx32 := range d.litToPat[h.litID] {
				idx := int(idx32)
				if !seen[idx] {
					seen[idx] = true
					trig[idx]++
				}
			}
		}
	}

	t.Logf("=== assert NFA patterns: windowed? gate? assertWindowable? winW? trig? ===")
	assertAlwaysOnSet := map[int]bool{}
	for _, idx := range d.assertAlwaysOn {
		assertAlwaysOnSet[idx] = true
	}
	for i := 0; i < d.n; i++ {
		if d.nfas[i] == nil || !d.nfas[i].hasAssert {
			continue
		}
		cp := d.all[i]
		var mw int
		var bounded, anchor bool
		if re, parsed, err := compileAndParse(cp.expr); err == nil {
			_ = re
			mw, bounded = maxByteWidth(parsed)
			anchor = hasPositionAnchor(parsed)
		}
		t.Logf("  win=%-5v winW=%-4d gate=%-5v anchorable=%-5v alwaysOn=%-5v nLit=%-2d trig=%-5d maxW=%d bounded=%v anchor=%v\n      %s :: %s",
			cp.windowed, cp.winW, d.gate[i], d.anchorable[i], assertAlwaysOnSet[i], len(cp.literals), trig[i], mw, bounded, anchor, names[cp.id], cp.expr)
	}
}

// TestMVSGateLocalizeDiag 量化"门复核 regexp2 局部化"潜力: 对每条 gate / fallback (regexp2 复核)
// pattern, 统计触发记录数 / regexp2 真命中数 / 首个字面量命中位置分布, 并直接计时 "整段 regexp2"
// 与 "data[winLo:] 局部 regexp2" 的耗时, winLo = firstHitEnd - maxHead - 1 (从超集算 head).
// 这决定左截窗口能省多少: 若首个 :// 普遍靠后, 局部化收益大; 靠前则收益有限.
func TestMVSGateLocalizeDiag(t *testing.T) {
	requireDiag(t)
	patterns, names := compilableMITMPatterns(t)
	records, _ := loadCorpus(t)
	db, err := Compile(patterns, WithBackend(BackendMVS), WithReportLocation(false), WithLogger(silentLogger{}))
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	defer db.Close()
	d := digMVSDB(t, db)
	scr, _ := db.NewScratch()
	sc := scr.(*scratch)

	// 选出 regexp2-recheck pattern (gate 或无 NFA 兜底), 计算其超集 head 上界 (maxHead).
	type gp struct {
		idx     int
		maxHead int // 超集下该 pattern 任一字面量的最大 head; -1 表示无界/不可局部化
		hasLit  bool
	}
	var gates []*gp
	for i := 0; i < d.n; i++ {
		isGate := d.gate[i]
		isFallback := d.nfas[i] == nil
		if !isGate && !isFallback {
			continue
		}
		cp := d.all[i]
		g := &gp{idx: i, maxHead: -1, hasLit: len(cp.literals) > 0}
		if g.hasLit {
			// 从超集 (RE2 可解析) 算 per-literal head, 取 max.
			if super, _, ok := re2SupersetEx(cp.expr); ok {
				heads := computeLitHeads(super, cp.literals)
				mh := int32(-1)
				allBounded := len(heads) > 0
				for _, lit := range cp.literals {
					h, ok := heads[lit]
					if !ok || h < 0 {
						allBounded = false
						break
					}
					if h > mh {
						mh = h
					}
				}
				if allBounded {
					g.maxHead = int(mh)
				}
			}
		}
		gates = append(gates, g)
	}

	t.Logf("=== %d regexp2-recheck patterns (gate/fallback) ===", len(gates))

	type gstat struct {
		trig        int
		matchRecs   int
		sumN        int64 // 触发记录总字节 (整段 regexp2 上界)
		sumWinLo    int64 // 局部化跳过的字节总量 (= 收益上界)
		wholeNanos  int64
		localNanos  int64
		firstHitSum int64
	}
	gs := make(map[int]*gstat)
	for _, g := range gates {
		gs[g.idx] = &gstat{}
	}

	// gateExists 复刻生产存在性门: gate+lit 走 existsInAssertShared/existsIn; always-on 走
	// merged NFA (existsIn). 只有门通过才在生产里跑 regexp2 复核, 故诊断也据此门控.
	gateExists := func(g *gp, data []byte) bool {
		nfa := d.nfas[g.idx]
		if nfa == nil {
			return true // 纯 regexp2 兜底, 每条都跑
		}
		if nfa.hasAssert {
			return nfa.existsInAssertShared(data, computeBoundaries(data, nil))
		}
		return nfa.existsIn(data)
	}

	for _, data := range records {
		n := len(data)
		hits := d.pf.scanHits(data, sc)
		// 每个 gate idx 的首个字面量命中 end.
		firstHit := make(map[int]int)
		for _, h := range hits {
			if int(h.litID) >= len(d.litToPat) {
				continue
			}
			for _, idx32 := range d.litToPat[h.litID] {
				idx := int(idx32)
				if _, ok := gs[idx]; !ok {
					continue
				}
				if cur, ok := firstHit[idx]; !ok || int(h.end) < cur {
					firstHit[idx] = int(h.end)
				}
			}
		}
		for _, g := range gates {
			st := gs[g.idx]
			cp := d.all[g.idx]
			fh, hasTrigger := firstHit[g.idx]
			if g.hasLit && !hasTrigger {
				continue // 有字面量但本记录无命中 -> 生产里不会复核
			}
			if !g.hasLit {
				fh = 0
			}
			// 生产存在性门: 不过则不跑 regexp2.
			if !gateExists(g, data) {
				continue
			}
			st.trig++
			st.sumN += int64(n)
			st.firstHitSum += int64(fh)
			winLo := 0
			if g.maxHead >= 0 {
				if winLo = fh - g.maxHead - 1; winLo < 0 {
					winLo = 0
				}
				winLo = alignRuneStart(data, winLo)
			}
			st.sumWinLo += int64(winLo)
			// 整段 regexp2 计时.
			t0 := nowNanos()
			wholeMatch := len(cp.v.findAll(data)) > 0
			st.wholeNanos += nowNanos() - t0
			if wholeMatch {
				st.matchRecs++
			}
			// 局部 regexp2 计时 (data[winLo:]).
			t1 := nowNanos()
			_ = len(cp.v.findAll(data[winLo:])) > 0
			st.localNanos += nowNanos() - t1
		}
	}

	// 报告 (按整段 regexp2 耗时降序).
	sort.Slice(gates, func(a, b int) bool { return gs[gates[a].idx].wholeNanos > gs[gates[b].idx].wholeNanos })
	var totWhole, totLocal int64
	for _, g := range gates {
		st := gs[g.idx]
		totWhole += st.wholeNanos
		totLocal += st.localNanos
		if st.trig == 0 {
			continue
		}
		avgFirst := float64(st.firstHitSum) / float64(st.trig)
		t.Logf("  %-22s trig=%-5d match=%-5d maxHead=%-6d avgFirstHit=%-9.0f skipBytes=%-12d whole=%-8.2fms local=%-8.2fms (%.1fx)",
			names[d.all[g.idx].id], st.trig, st.matchRecs, g.maxHead, avgFirst, st.sumWinLo,
			float64(st.wholeNanos)/1e6, float64(st.localNanos)/1e6,
			safeRatio(st.wholeNanos, st.localNanos))
	}
	t.Logf("=== TOTAL regexp2: whole=%.2fms  localized=%.2fms  (%.2fx speedup)===",
		float64(totWhole)/1e6, float64(totLocal)/1e6, safeRatio(totWhole, totLocal))
}

func safeRatio(a, b int64) float64 {
	if b == 0 {
		return 0
	}
	return float64(a) / float64(b)
}

// TestMVSCgoCallDiag 量化 cgo 跨界: 统计一遍语料 nfaExists / mergedScan 的调用次数与扫描字节,
// 判定 cgocall 瓶颈是 "跨界开销" (调用多/字节少 -> 批处理) 还是 "扫描工作量" (字节多 -> 收窄窗口).
func TestMVSCgoCallDiag(t *testing.T) {
	requireDiag(t)
	patterns, _ := compilableMITMPatterns(t)
	records, joined := loadCorpus(t)
	db, err := Compile(patterns, WithBackend(BackendMVS), WithReportLocation(false), WithLogger(silentLogger{}))
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	defer db.Close()
	scr, _ := db.NewScratch()

	cgoDiagEnabled = true
	cgoNfaExistsCalls, cgoNfaExistsBytes = 0, 0
	cgoMergedCalls, cgoMergedBytes = 0, 0
	defer func() { cgoDiagEnabled = false }()

	for _, rec := range records {
		db.Scan(rec, scr, func(Match) bool { return true })
	}
	nCalls, nBytes := cgoNfaExistsCalls, cgoNfaExistsBytes
	mCalls, mBytes := cgoMergedCalls, cgoMergedBytes
	t.Logf("corpus: %d records, %d bytes", len(records), len(joined))
	t.Logf("nfaExists: calls=%d bytes=%d avgWin=%.1f", nCalls, nBytes, safeRatio(nBytes, nCalls))
	t.Logf("merged:    calls=%d bytes=%d avgWin=%.1f", mCalls, mBytes, safeRatio(mBytes, mCalls))
	t.Logf("total cgo calls=%d, total cgo scan bytes=%d", nCalls+mCalls, nBytes+mBytes)
}

// TestMVSAssertAlwaysOnDiag 打印断言型 always-on pattern (无可用必需字面量, 每条记录整段
// existsInAssertShared 扫描) 的名称/表达式/nword, 评估能否补充字面量门控或窗口化以省整段扫.
func TestMVSAssertAlwaysOnDiag(t *testing.T) {
	requireDiag(t)
	patterns, names := compilableMITMPatterns(t)
	db, err := Compile(patterns, WithBackend(BackendMVS), WithReportLocation(false), WithLogger(silentLogger{}))
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	defer db.Close()
	d := digMVSDB(t, db)
	t.Logf("=== assertAlwaysOn: %d (整段扫每记录) ===", len(d.assertAlwaysOn))
	for _, idx := range d.assertAlwaysOn {
		cp := d.all[idx]
		nw := 0
		if d.nfas[idx] != nil {
			nw = d.nfas[idx].nword
		}
		t.Logf("  nword=%d lits=%v %s expr=%q", nw, cp.literals, names[cp.id], cp.expr)
	}
	t.Logf("=== otherAlwaysOn (regexp2/RE2 兜底, 逐条 verifier): %d ===", len(d.otherAlwaysOn))
	for _, idx := range d.otherAlwaysOn {
		cp := d.all[idx]
		t.Logf("  lits=%v %s expr=%q", cp.literals, names[cp.id], cp.expr)
	}
	// 评估稀有单字符门控潜力: 统计常见分隔符在多少记录中出现 (出现率越低, 门控收益越大).
	records, _ := loadCorpus(t)
	for _, ch := range []byte{'@', '<', '>', '\\', '$', '#'} {
		nRec := 0
		for _, rec := range records {
			for i := 0; i < len(rec); i++ {
				if rec[i] == ch {
					nRec++
					break
				}
			}
		}
		t.Logf("  byte %q 出现于 %d/%d 记录 (%.1f%%)", string(ch), nRec, len(records), 100*float64(nRec)/float64(len(records)))
	}
}

// TestMVSUnboundedHeadDiag 打印所有 "batch-nfaExists(UNBOUNDED)" pattern (head<0 或 tail<0,
// 当前从 0 整段扫 C 内核), 标注 head/tail 界、是否以 .* / [\s\S]* / (?s) 等 "自由前缀" 开头
// (此类前缀对存在性免费, 可锚定到字面量), 以及触发记录数. 指导"无界头存在性锚定"优化的取舍.
func TestMVSUnboundedHeadDiag(t *testing.T) {
	requireDiag(t)
	patterns, names := compilableMITMPatterns(t)
	records, _ := loadCorpus(t)
	db, err := Compile(patterns, WithBackend(BackendMVS), WithReportLocation(false), WithLogger(silentLogger{}))
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	defer db.Close()
	d := digMVSDB(t, db)
	scr, _ := db.NewScratch()
	sc := scr.(*scratch)

	trig := make([]int, d.n)
	for _, data := range records {
		hits := d.pf.scanHits(data, sc)
		seen := make([]bool, d.n)
		for _, h := range hits {
			if int(h.litID) >= len(d.litToPat) {
				continue
			}
			for _, idx32 := range d.litToPat[h.litID] {
				idx := int(idx32)
				if !seen[idx] {
					seen[idx] = true
					trig[idx]++
				}
			}
		}
	}

	type row struct {
		idx       int
		trig      int
		head      int32
		tail      int32
		freePre   bool
		anchStart bool
	}
	var rows []row
	freeCnt, freeTrig := 0, 0
	for i := 0; i < d.n; i++ {
		if d.nfas[i] == nil || d.nfas[i].hasAssert || d.windowable[i] || d.anchorable[i] || len(d.all[i].literals) == 0 {
			continue
		}
		w := d.win[i]
		if w.head >= 0 && w.tail >= 0 {
			continue // 有界两侧 -> 非 UNBOUNDED
		}
		expr := d.all[i].expr
		// "自由前缀": 去掉行内 flag / 分组开头后, 以 .* 或 [\s\S]* 起头 (存在性免费, 可锚定).
		free := hasFreeAnyPrefix(expr)
		r := row{idx: i, trig: trig[i], head: w.head, tail: w.tail, freePre: free, anchStart: d.nfas[i].anchoredStart}
		rows = append(rows, r)
		if free {
			freeCnt++
			freeTrig += trig[i]
		}
	}
	sort.Slice(rows, func(a, b int) bool { return rows[a].trig > rows[b].trig })
	t.Logf("=== UNBOUNDED batch patterns: %d total, freePrefix(.*|[\\s\\S]*)=%d (trig=%d) ===", len(rows), freeCnt, freeTrig)
	for _, r := range rows {
		cp := d.all[r.idx]
		// 重新算 per-literal head, 看是否能给出有界 (诊断 computeLitWindow 全 pattern 取 max 的退化).
		heads := computeLitHeads(cp.expr, cp.literals)
		t.Logf("  trig=%-6d head=%-6d tail=%-6d free=%-5v anchStart=%-5v lits=%v perLitHead=%v %s",
			r.trig, r.head, r.tail, r.freePre, r.anchStart, cp.literals, heads, names[cp.id])
	}
}

// hasFreeAnyPrefix 粗判 expr 是否以 "对存在性免费的任意前缀" 起头 (.* / .+ / [\s\S]* 等),
// 跳过开头的行内 flag (?ims) 与左括号. 仅诊断用 (保守: 命中即认为可锚定).
func hasFreeAnyPrefix(expr string) bool {
	s := expr
	for {
		if strings.HasPrefix(s, "(?") {
			// 跳过行内 flag 组 (?ims) 或 (?:.
			if k := strings.IndexByte(s, ')'); k > 0 && !strings.Contains(s[:k], ":") {
				s = s[k+1:]
				continue
			}
		}
		s = strings.TrimLeft(s, "(")
		break
	}
	return strings.HasPrefix(s, ".*") || strings.HasPrefix(s, ".+") ||
		strings.HasPrefix(s, "[\\s\\S]*") || strings.HasPrefix(s, "[\\s\\S]+") ||
		strings.HasPrefix(s, "[\\S\\s]*") || strings.HasPrefix(s, "[\\S\\s]+")
}

// nfaMinAcceptWidth 计算 NFA 从任一 first 位置到达接受态消费的最小 rune 数 (BFS 最短路).
// 这是"存在性下界": 任一命中至少消费这么多 rune. 用于评估"存在性最小宽度窗口"的可行性.
func nfaMinAcceptWidth(nfa *mvsNFA) int {
	npos := nfa.npos
	const inf = 1 << 30
	dist := make([]int, npos)
	for i := range dist {
		dist[i] = inf
	}
	// first 位置消费 1 个 rune 即可处于该位置.
	type qe struct{ p, d int }
	var q []qe
	forEachSetBit(nfa.first, func(p int) {
		if dist[p] > 1 {
			dist[p] = 1
			q = append(q, qe{p, 1})
		}
	})
	best := inf
	isAccept := func(p int) bool {
		w := p >> 6
		bit := uint64(1) << uint(p&63)
		return nfa.lastAny[w]&bit != 0 || nfa.lastEnd[w]&bit != 0
	}
	for len(q) > 0 {
		cur := q[0]
		q = q[1:]
		if cur.d > dist[cur.p] {
			continue
		}
		if isAccept(cur.p) && cur.d < best {
			best = cur.d
		}
		forEachSetBit(nfa.follow[cur.p], func(np int) {
			if dist[np] > cur.d+1 {
				dist[np] = cur.d + 1
				q = append(q, qe{np, cur.d + 1})
			}
		})
	}
	if best == inf {
		return -1
	}
	return best
}

// BenchmarkMVSFindAllLocScratch A/B 度量定位热路径 (findAllLoc->findLocFrom) 的缓冲复用净收益:
//   - "nil" 子档: 每次 findLocFrom 调用 make 四个切片 (prevActive/cand/candStart/prevStart),
//     即本轮改动前的行为.
//   - "scratch" 子档: 复用同一 *scratch 的 loc* 缓冲 (本轮改动后的行为).
//
// 两档跑同一组 lean NFA + 同一语料, 仅缓冲来源不同, 故 allocs/op 差值即此改动净收益 (吞吐亦同源对比).
// 关键词: benchmark, findAllLoc, findLocFrom, 定位零分配, scratch 复用
func BenchmarkMVSFindAllLocScratch(b *testing.B) {
	patterns := re2OnlyMITMPatterns(b)
	_, joined := loadCorpusB(b)
	var nfas []*mvsNFA
	for _, p := range patterns {
		nfa := compileExprToNFA(buildExprWithFlags(p))
		if nfa != nil && !nfa.hasAssert {
			nfas = append(nfas, nfa)
		}
	}
	b.Logf("lean NFAs: %d, corpus bytes: %d", len(nfas), len(joined))
	sc := &scratch{}

	run := func(b *testing.B, useSc bool) {
		b.SetBytes(int64(len(joined)) * int64(len(nfas)))
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			for _, nfa := range nfas {
				var s *scratch
				if useSc {
					s = sc
				}
				cnt := 0
				nfa.findAllLoc(joined, s, func(from, to int) bool { cnt++; return true })
				_ = cnt
			}
		}
	}
	b.Run("nil", func(b *testing.B) { run(b, false) })
	b.Run("scratch", func(b *testing.B) { run(b, true) })
}
