package minirehs

import (
	"regexp/syntax"
	"sort"
	"strings"
	"testing"
)

// 本诊断量化"前向锚定 ∪ 反向锚定"(Rose-lite 双向锚定) 对当前整段扫 pattern 的可救比例.
//
// 现状: head_L<0 (per-literal 回看上限无界) 的 lean pattern 落到 batch 整段 C 扫 (litSpan lo=0).
// 双向锚定的零假阴前提: 对触发字面量 L 的 *每个* AST 出现处, head 有界 *或* tail 有界. 则
//   - head 有界的出现处由前向锚定 (existsInAnchored, 注入区间 [h.end-H_f, h.end]) 覆盖;
//   - tail 有界的出现处由反向锚定 (反向 NFA, 注入区间 [h.end, h.end+T_r]) 覆盖;
//   - 二者并集 = 全部匹配 (任一匹配必属某出现处, 该处至少一侧有界 => 至少一向命中). 绝不漏报.
// 若某出现处两侧全无界 (如 .*L.* / [^x]+?L[^x]+?), 双向锚定都救不了 => 仍需整段, 不纳入.
//
// 关键词: Rose-lite, 双向锚定, 反向锚定, 零假阴, per-occurrence 边界分析, 可救性测量

type occCov struct{ headB, tailB bool }

type rosecovAcc struct {
	set map[string]struct{}
	occ map[string][]occCov
}

// walk 以"边界有界性"继承属性遍历 AST (镜像 litWindowAcc, 但按出现处分别记录 head/tail 是否有界,
// 而非全 pattern 取 max). OpStar/OpPlus/OpRepeat 一律置两侧无界 (与 computeLitHeads 同口径, 安全保守).
func (a *rosecovAcc) walk(re *syntax.Regexp, preB, sufB bool) {
	switch re.Op {
	case syntax.OpLiteral:
		s := strings.ToLower(string(re.Rune))
		if _, ok := a.set[s]; ok {
			a.occ[s] = append(a.occ[s], occCov{headB: preB, tailB: sufB})
		}
	case syntax.OpCapture, syntax.OpQuest:
		if len(re.Sub) == 1 {
			a.walk(re.Sub[0], preB, sufB)
		}
	case syntax.OpConcat:
		k := len(re.Sub)
		for i, sub := range re.Sub {
			_, lb := sumWidthRange(re.Sub, 0, i)
			_, rb := sumWidthRange(re.Sub, i+1, k)
			a.walk(sub, preB && lb, sufB && rb)
		}
	case syntax.OpAlternate:
		for _, sub := range re.Sub {
			a.walk(sub, preB, sufB)
		}
	case syntax.OpStar, syntax.OpPlus, syntax.OpRepeat:
		if len(re.Sub) == 1 {
			a.walk(re.Sub[0], false, false)
		}
	}
}

// litCoverClass 返回字面量 L 在 expr 中的可救分类:
//
//	"fwd"   每个出现处 head 有界 (当前前向锚定即可, 已覆盖)
//	"rev"   每个出现处 tail 有界 (但非全 head 有界): 纯反向锚定可救
//	"both"  每个出现处 head 或 tail 有界 (但既非全 head 也非全 tail): 需前向 ∪ 反向
//	"none"  存在某出现处两侧全无界: 双向锚定救不了, 仍需整段
//	"absent" 该字面量未在 AST 命中 (异常)
func litCoverClass(occ []occCov) string {
	if len(occ) == 0 {
		return "absent"
	}
	allHead, allTail, everyOr := true, true, true
	for _, o := range occ {
		if !o.headB {
			allHead = false
		}
		if !o.tailB {
			allTail = false
		}
		if !o.headB && !o.tailB {
			everyOr = false
		}
	}
	switch {
	case allHead:
		return "fwd"
	case allTail:
		return "rev"
	case everyOr:
		return "both"
	default:
		return "none"
	}
}

// TestMVSRoseCovPotential 在真实 MITM 规则 + 真实流量上量化双向锚定可救的整段扫成本占比.
// 仅诊断, 不改生产逻辑. 运行: go test -run TestMVSRoseCovPotential -v
func TestMVSRoseCovPotential(t *testing.T) {
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

	// 每条触发记录的整段扫 (batch full) 成本权重: 用 litSpan 窗口字节 (与 OptDiag 同口径).
	// 先按 idx 统计触发记录数, 再乘以"该 pattern 整段扫每记录的平均窗口字节"过于复杂, 这里
	// 直接以"触发记录数"为成本代理 (每触发一次 = 一次整段 existsIn), 并单列窗口字节累计.
	type pcost struct {
		idx       int
		trig      int
		winBytes  int64
		coverable bool   // 触发字面量全部 fwd/rev/both => 双向锚定可整体替换整段扫
		newWin    bool   // 是否当前就走 batch 整段 (head<0 或 tail<0 的 lean, 非 window/anchor)
		class     string // 该 pattern 触发字面量的汇总分类
		litClass  map[string]string
	}
	costs := make([]*pcost, d.n)
	for i := range costs {
		costs[i] = &pcost{idx: i, litClass: map[string]string{}}
	}

	// 预计算每条 lean pattern 的 per-literal 可救分类.
	for i := 0; i < d.n; i++ {
		cp := d.all[i]
		nfa := d.nfas[i]
		if nfa == nil || nfa.hasAssert || d.gate[i] || len(cp.literals) == 0 {
			continue
		}
		// analysisExpr 同 compile: re2Loc 用超集骨架, 否则原 expr.
		expr := cp.expr
		if d.re2Loc[i] {
			if super, _, ok := re2SupersetEx(cp.expr); ok {
				expr = super
			} else {
				expr = ""
			}
		}
		if expr == "" {
			continue
		}
		re, perr := syntax.Parse(expr, syntax.Perl)
		if perr != nil {
			continue
		}
		re = re.Simplify()
		set := make(map[string]struct{}, len(cp.literals))
		for _, l := range cp.literals {
			set[l] = struct{}{}
		}
		acc := &rosecovAcc{set: set, occ: map[string][]occCov{}}
		acc.walk(re, true, true)
		for _, l := range cp.literals {
			costs[i].litClass[l] = litCoverClass(acc.occ[l])
		}
	}

	// 逐记录跑预过滤, 累计触发数 + batch 窗口字节 (复刻 scan 的 batch 选择: 非 window/anchor 的 lean).
	triggered := make([]bool, d.n)
	for _, data := range records {
		n := len(data)
		if d.pf == nil {
			break
		}
		hits := d.pf.scanHits(data, sc)
		for i := range triggered {
			triggered[i] = false
		}
		winLo := map[int]int{}
		winHi := map[int]int{}
		for _, h := range hits {
			if int(h.litID) >= len(d.litToPat) {
				continue
			}
			for _, idx32 := range d.litToPat[h.litID] {
				idx := int(idx32)
				if !triggered[idx] {
					triggered[idx] = true
					costs[idx].trig++
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
			costs[idx].winBytes += int64(winHi[idx] - lo)
		}
	}

	// 标注当前走 batch 整段的 lean pattern (非 windowable/anchorable, 有内核 batch 资格).
	for i := 0; i < d.n; i++ {
		nfa := d.nfas[i]
		if nfa == nil || nfa.hasAssert || d.gate[i] || len(d.all[i].literals) == 0 {
			continue
		}
		if d.windowable[i] || d.anchorable[i] {
			continue
		}
		costs[i].newWin = true
		// 汇总 pattern 分类: 取触发字面量里"最坏"类 (none > both > rev > fwd).
		rank := map[string]int{"fwd": 0, "rev": 1, "both": 2, "none": 3, "absent": 3, "": 0}
		worst := "fwd"
		for _, c := range costs[i].litClass {
			if rank[c] > rank[worst] {
				worst = c
			}
		}
		costs[i].class = worst
		costs[i].coverable = worst == "rev" || worst == "both"
	}

	// 汇总报告.
	var totBatchTrig, covBatchTrig int64
	var totBatchWin, covBatchWin int64
	clsTrig := map[string]int64{}
	clsWin := map[string]int64{}
	clsCount := map[string]int{}
	for _, c := range costs {
		if !c.newWin {
			continue
		}
		totBatchTrig += int64(c.trig)
		totBatchWin += c.winBytes
		clsTrig[c.class] += int64(c.trig)
		clsWin[c.class] += c.winBytes
		clsCount[c.class]++
		if c.coverable {
			covBatchTrig += int64(c.trig)
			covBatchWin += c.winBytes
		}
	}
	t.Logf("=== batch 整段扫 lean pattern 双向锚定可救性 (真实 MITM %d 记录) ===", len(records))
	t.Logf("总 batch 整段: trig=%d  winBytes=%d", totBatchTrig, totBatchWin)
	t.Logf("反向/双向可救 (rev|both): trig=%d (%.1f%%)  winBytes=%d (%.1f%%)",
		covBatchTrig, pct(covBatchTrig, totBatchTrig), covBatchWin, pct(covBatchWin, totBatchWin))
	for _, cls := range []string{"fwd", "rev", "both", "none"} {
		t.Logf("  class=%-5s patterns=%-3d trig=%-7d winBytes=%d", cls, clsCount[cls], clsTrig[cls], clsWin[cls])
	}

	// Top batch pattern 明细 (按窗口字节).
	sort.Slice(costs, func(i, j int) bool { return costs[i].winBytes > costs[j].winBytes })
	t.Logf("=== top batch-整段 pattern (按窗口字节) ===")
	shown := 0
	for _, c := range costs {
		if !c.newWin || c.winBytes == 0 {
			continue
		}
		nm := names[d.all[c.idx].id]
		t.Logf("  [%-4s cov=%-5v] win=%-10d trig=%-5d lits=%v %.45s", c.class, c.coverable, c.winBytes, c.trig, c.litClass, nm)
		shown++
		if shown >= 20 {
			break
		}
	}
}

func pct(a, b int64) float64 {
	if b == 0 {
		return 0
	}
	return 100 * float64(a) / float64(b)
}
