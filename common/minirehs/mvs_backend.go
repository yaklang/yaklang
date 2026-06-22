package minirehs

import (
	"regexp/syntax"
)

// mvsBackend 是自托管 mvscan 引擎的纯 Go 参考实现 (M1): 把每条 RE2 pattern 编译为字节级
// Glushkov 位并行 NFA, 扫描时做存在性判定 (命中 From/To=-1). 无法编入 NFA 的 pattern
// (中缀锚点/词边界/regexp2-only 等) 退回 verifier 兜底, 保证不丢规则、与 oracle 一致.
//
// 它既是 mvscan C 后端的可执行规格 (差分参照), 也用于量化"纯算法 (无 SIMD/无 cgo)"的加速比.
// C 加速档 (SSE/AVX2/NEON) 在后续里程碑以 build tag 接入, 并以本实现做逐位对照.
//
// 关键词: mvscan, BackendMVS, Glushkov, bit-parallel, existence, 纯 Go 参考实现
type mvsBackend struct{}

func (b *mvsBackend) kind() BackendKind { return BackendMVS }
func (b *mvsBackend) tier() int         { return 1 }
func (b *mvsBackend) simd() bool        { return false }

func (b *mvsBackend) compile(patterns []*compiledPattern, cfg *config) (compiledDB, error) {
	db := &mvsDB{
		all:       patterns,
		n:         len(patterns),
		nfas:       make([]*mvsNFA, len(patterns)),
		gate:       make([]bool, len(patterns)),
		re2Loc:     make([]bool, len(patterns)),
		windowable: make([]bool, len(patterns)),
		batchable:  make([]bool, len(patterns)),
		win:        make([]litWindow, len(patterns)),
		reportLoc:  cfg.reportLocation,
	}
	for i := range db.win {
		db.win[i] = litWindow{head: -1, tail: -1} // 默认不收窄 (整段)
	}

	var withLit []*compiledPattern // 有必需字面量 (nfa 或兜底): 进字面量索引, 命中才验证
	var mergeMembers []mergeMember // 无字面量且可编入 lean NFA: 合并为单趟 always-on 自动机
	nfaCount := 0
	assertCount := 0
	gateCount := 0
	for _, cp := range patterns {
		nfa, gate, re2Loc := tryCompileNFA(cp)
		if nfa != nil {
			db.nfas[cp.idx] = nfa
			db.gate[cp.idx] = gate
			db.re2Loc[cp.idx] = re2Loc
			nfaCount++
			if nfa.hasAssert {
				assertCount++
			}
			if gate {
				gateCount++
			}
			// 窗口化存在性快路径资格: 有界宽 (windowed) 且无零宽断言、非超集门的 lean NFA.
			// 仅 !reportLoc 时启用 (见 scan); 定位档仍整段, 保定位语义不变.
			if cp.windowed && cp.winW > 0 && !nfa.hasAssert && !gate {
				db.windowable[cp.idx] = true
			}
			// 批处理资格: 非断言 lean NFA 已序列化进 C blob, 可走 nfaExistsMany 一次 cgo 多条验证.
			if !nfa.hasAssert {
				db.batchable[cp.idx] = true
				// 存在性本地化界: 仅 RE2-exact (NFA 由 cp.expr 直接编译) 才能用 cp.expr 上下文界
				// 安全收窄; gate/re2Loc 的 NFA 源自超集骨架 (宽度可能更大), 保留整段。命中字面量
				// 后, 运行期把整段 existsIn 收到 [h.end-head, h.end+tail] 的 union (见 scan)。
				if !gate && !re2Loc && len(cp.literals) > 0 {
					w := computeLitWindow(cp.expr, cp.literals)
					if nfa.anchoredStart {
						w.head = -1 // ^ \A 锚: 匹配必起于偏移 0, 不可左截
					}
					if nfa.requireEnd {
						w.tail = -1 // $ \z 锚: 匹配必止于偏移 n, 不可右截
					}
					db.win[cp.idx] = w
				}
			}
		}
		if len(cp.literals) > 0 {
			withLit = append(withLit, cp)
			continue
		}
		// 无字面量 (always-on): lean NFA 合并单趟; 断言 NFA 不可合并 (带 guard), 单条门控;
		// 无 NFA (regexp2/RE2 兜底) 逐条验证.
		switch {
		case nfa != nil && nfa.hasAssert:
			db.assertAlwaysOn = append(db.assertAlwaysOn, cp.idx)
		case nfa != nil:
			mergeMembers = append(mergeMembers, mergeMember{idx: cp.idx, nfa: nfa})
		default:
			db.otherAlwaysOn = append(db.otherAlwaysOn, cp.idx)
		}
	}

	if len(withLit) > 0 {
		li := buildLiteralIndex(withLit)
		db.pf = newPrefilter(li)
		db.litToPat = li.litToPat
	}
	db.merged = buildMergedNFA(mergeMembers)
	db.mergedCount = len(mergeMembers)
	db.nfaCount = nfaCount

	// P4 (M2): minirehs_mvs 构建下, 把 per-pattern NFA + 合并 always-on 序列化为平台无关
	// blob, 交纯 C99 运行期内核执行存在性扫描 (与纯 Go 参考执行器逐位一致). 非该构建返回 nil,
	// 自动退化为纯 Go existsIn / scanExist. 定位 (findAllLoc) 始终走已验证的 Go 路径.
	db.kernel = newMVSKernel(db)
	cfg.logger.Infof("minirehs/mvs compiled %d pattern(s): nfa=%d (assert=%d gate=%d) fallback=%d always_on=%d (merged_nfa=%d assert=%d other=%d) c_kernel=%v",
		db.n, nfaCount, assertCount, gateCount, db.n-nfaCount,
		db.mergedCount+len(db.assertAlwaysOn)+len(db.otherAlwaysOn),
		db.mergedCount, len(db.assertAlwaysOn), len(db.otherAlwaysOn), db.kernel != nil)
	return db, nil
}

// tryCompileNFA 尝试把一条 pattern 编入位置自动机, 返回 (nfa, gate, re2Loc):
//   - nfa==nil: 无法编入, 该 pattern 走 verifier 兜底 (理论上 route-B 超集后已极少).
//   - gate==true: nfa 是"严格超集存在性门" (原 regexp2 含 lookaround/backref, 超集放大了语言),
//     命中后必须 regexp2 复核以滤除假阳; 绝不漏报 (R_super ⊇ R_orig).
//   - re2Loc==true: 该 pattern 源自 regexp2 (含与原语言等价的超集), 精确定位交 regexp2 保 PCRE
//     leftmost span 语义, 不与 NFA leftmost-longest 偏差; 纯 RE2 pattern 为 false (NFA 自定位).
//
// 两级 NFA 构造: 先 lean (无零宽断言, 走 C 内核/位并行快路径); lean 失败再试断言扩展
// (compileMVSNFAAssert, hasAssert=true: \b \B / 行锚 / 中缀 ^$\A\z, 仅 Go 侧门控执行).
func tryCompileNFA(cp *compiledPattern) (nfa *mvsNFA, gate bool, re2Loc bool) {
	// RE2-exact pattern: 直接按原 expr 编, NFA 对存在性与定位均权威.
	if cp.v != nil && cp.v.exact() {
		return compileExprToNFA(cp.expr), false, false
	}
	// regexp2-only (lookaround/backref/\uXXXX 等): 改写为语言只增不减的 RE2 超集骨架再编.
	// widened=false (仅 \u / 原子组归一 等语言等价变换): NFA 与原语言相同 -> 权威存在性;
	// widened=true (移除了 lookaround/backref): 严格超集 -> 只作存在性门, regexp2 复核.
	super, widened, ok := re2SupersetEx(cp.expr)
	if !ok {
		return nil, false, false
	}
	nfa = compileExprToNFA(super)
	if nfa == nil {
		return nil, false, false
	}
	return nfa, widened, true
}

// compileExprToNFA 把一条 RE2 可解析的 expr 编为 mvsNFA (先 lean 后断言扩展); 失败返回 nil.
func compileExprToNFA(expr string) *mvsNFA {
	parsed, err := syntax.Parse(expr, syntax.Perl)
	if err != nil {
		return nil
	}
	s := parsed.Simplify()
	if nfa, ok := compileMVSNFA(s); ok {
		return nfa
	}
	if nfa, ok := compileMVSNFAAssert(s); ok {
		return nfa
	}
	return nil
}

type mvsDB struct {
	all        []*compiledPattern // 按 idx 索引
	n          int
	nfas       []*mvsNFA // 按 idx; nil 表示该 pattern 走 verifier 兜底
	gate       []bool    // 按 idx; true=该 NFA 为严格超集门, 命中须 regexp2 复核滤假阳
	re2Loc     []bool    // 按 idx; true=精确定位交 regexp2 (regexp2-origin, 保 PCRE span 语义)
	windowable []bool    // 按 idx; true=可在字面量命中点邻域窗口内做存在性验证 (有界宽 lean NFA)
	batchable  []bool    // 按 idx; true=非断言且有 NFA (在 C blob 中), 可走 nfaExistsMany 批处理
	win        []litWindow // 按 idx; 命中字面量结尾两侧上下文界 (head/tail; -1=该侧无界不收窄)
	nfaCount   int

	pf       prefilter // 可能为 nil (无任何字面量 pattern)
	litToPat [][]int32 // litID -> pattern idx

	merged         *mvsMergedNFA // 无字面量且可编入 lean NFA 的 always-on 合并为单趟自动机
	mergedCount    int           // 合并成员数
	assertAlwaysOn []int         // 无字面量的断言 NFA (hasAssert): existsInAssert 门控 + verifier 定位
	otherAlwaysOn  []int         // 无字面量且不可编入 NFA (regexp2/RE2 兜底): 逐条验证

	reportLoc bool // 命中是否上报精确偏移与内容 (见 WithReportLocation)

	// kernel 是纯 C99 运行期内核句柄 (仅 minirehs_mvs 构建非 nil); 非 nil 时存在性扫描
	// (existsIn / merged scanExist) 走 C, 定位仍走 Go. nil 则全程纯 Go.
	kernel *mvsKernel
}

func (d *mvsDB) numAlwaysOn() int {
	return d.mergedCount + len(d.assertAlwaysOn) + len(d.otherAlwaysOn)
}

func (d *mvsDB) close() error {
	if r, ok := d.pf.(prefilterReleaser); ok {
		r.release()
	}
	if d.kernel != nil {
		d.kernel.close()
		d.kernel = nil
	}
	return nil
}

func (d *mvsDB) scan(data []byte, sc *scratch, handler MatchHandler) (bool, error) {
	d.resetScratch(sc)

	// 1) 字面量预过滤: 命中某 pattern 的必需字面量, 才对其做一次存在性验证.
	//    存在性快门档 (!reportLoc) 下, 有界宽 lean NFA 走"邻域窗口"验证: 任一匹配必含必需字面量,
	//    且宽度 <= winW, 故匹配必落在 [h.end-2winW, h.end+2winW]; 在该窗口上 existsIn 即可判定,
	//    把 per-trigger 成本从 O(record) 降到 O(winW). 窗口不命中时不置 fullDone, 留待其它命中点重试.
	if d.pf != nil {
		hits := d.pf.scanHits(data, sc)
		n := len(data)
		batch := sc.batchIdx[:0]
		sc.batchSeen = resetBoolBuf(sc.batchSeen, d.n)
		for _, h := range hits {
			if int(h.litID) >= len(d.litToPat) {
				continue
			}
			for _, idx := range d.litToPat[h.litID] {
				if sc.fullDone[idx] {
					continue
				}
				if !d.reportLoc && d.windowable[idx] {
					if d.windowExists(int(idx), data, h) {
						sc.fullDone[idx] = true
						if !handler(Match{ID: d.all[idx].id, From: -1, To: -1}) {
							return true, nil
						}
					}
					continue
				}
				// 批处理 (有 C 内核且非断言 lean NFA): 累积本 idx 全部命中点的窗口 union, 批后用收窄
				// 子切片做一次 C 存在性门控. 不可界一侧退回报文端 (lo=0/hi=n), 安全. 窗口 union 覆盖
				// 任一含命中字面量的匹配 => 与整段 existsIn 同真伪 (见 mvs_window.go 正确性说明).
				if d.kernel != nil && d.batchable[idx] {
					lo, hi := d.litSpan(int(idx), int(h.end), n)
					if !sc.batchSeen[idx] {
						sc.batchSeen[idx] = true
						sc.winLo[idx] = int32(lo)
						sc.winHi[idx] = int32(hi)
						batch = append(batch, int32(idx))
					} else {
						if int32(lo) < sc.winLo[idx] {
							sc.winLo[idx] = int32(lo)
						}
						if int32(hi) > sc.winHi[idx] {
							sc.winHi[idx] = int32(hi)
						}
					}
					continue
				}
				// 其余 (无内核 / 断言 NFA / 无 NFA 兜底): 立即逐条验证.
				sc.fullDone[idx] = true
				if stop := d.verifyOne(int(idx), data, sc, handler); stop {
					return true, nil
				}
			}
		}
		sc.batchIdx = batch
		for _, idx32 := range batch {
			idx := int(idx32)
			sc.fullDone[idx] = true
			lo, hi := int(sc.winLo[idx]), int(sc.winHi[idx])
			if d.kernel.nfaExists(idx, data[lo:hi]) {
				if stop := d.finalizeHit(idx, data, handler); stop {
					return true, nil
				}
			}
		}
	}

	// 2) 无字面量且可编入 NFA 的 always-on: 合并自动机单趟扫描得命中集合, 命中后逐个定位上报.
	//    有 C 内核时单趟扫描走 C (返回去重后的成员 idx, 再用 fullDone 跨步去重); 否则走纯 Go.
	if d.merged != nil {
		var hits []int
		if d.kernel != nil {
			hits = d.kernel.mergedScan(data, sc)
		} else {
			sc.mergedSeen = resetBoolBuf(sc.mergedSeen, d.n)
			sc.mergedHits = d.merged.scanExist(data, sc.mergedSeen, sc.mergedHits[:0])
			hits = sc.mergedHits
		}
		for _, idx := range hits {
			if sc.fullDone[idx] {
				continue
			}
			sc.fullDone[idx] = true
			if stop := d.reportLocated(idx, data, handler); stop {
				return true, nil
			}
		}
	}

	// 3) 无字面量的断言 NFA always-on: existsInAssertShared 位并行门控 (共享边界), 命中后交 verifier 定位.
	for _, idx := range d.assertAlwaysOn {
		if sc.fullDone[idx] {
			continue
		}
		sc.fullDone[idx] = true
		if stop := d.verifyOne(idx, data, sc, handler); stop {
			return true, nil
		}
	}

	// 4) 无字面量且不可编入 NFA 的 always-on (regexp2/RE2 兜底): 逐条验证.
	for _, idx := range d.otherAlwaysOn {
		if sc.fullDone[idx] {
			continue
		}
		sc.fullDone[idx] = true
		if stop := d.verifyOne(idx, data, sc, handler); stop {
			return true, nil
		}
	}
	return false, nil
}

// reportLocated 对一条已由合并自动机确认命中的成员产出最终上报 (合并自动机的存在性判定与
// 单条 existsIn 同源; 差分测试为护栏). 实际上报逻辑收敛到 finalizeHit (含 gate 复核 / re2Loc 定位).
func (d *mvsDB) reportLocated(idx int, data []byte, handler MatchHandler) bool {
	return d.finalizeHit(idx, data, handler)
}

// windowExists 在字面量命中点 h 的邻域窗口内做一条 (有界宽 lean NFA) pattern 的存在性判定.
// 窗口取 [h.end-2winW, h.end+2winW] (2 倍余量, 完整包含任一跨命中点的匹配); existsIn 在该子切片
// 上判定: 子切片命中 <=> 原串在该处命中 (子串关系), 故无假阳; 任一含此字面量的匹配必落窗口内,
// 故对此命中点无假阴 (其它命中点由调用方循环覆盖). 绝不漏报、绝不误报.
func (d *mvsDB) windowExists(idx int, data []byte, h litHit) bool {
	cp := d.all[idx]
	ws := int(h.end) - 2*cp.winW
	we := int(h.end) + 2*cp.winW
	if ws < 0 {
		ws = 0
	}
	if we > len(data) {
		we = len(data)
	}
	return d.nfas[idx].existsIn(data[ws:we])
}

// finalizeHit 在 "NFA 存在性门已命中" 后产出最终上报, 统一三种出口:
//   - gate: 该 NFA 为严格超集门, 必须 regexp2 复核 (确认真匹配并按其语义上报), 滤除超集假阳.
//   - 仅存在性 (reportLoc=false): 命中即上报 (-1,-1), 不做定位扫描 (80x 快门档).
//   - 需定位: regexp2-origin (re2Loc) 或断言 NFA 交 verifier 定位 (保 PCRE/既有 span 语义);
//     纯 RE2 lean NFA 用自身 findAllLoc 给精确非重叠区间.
func (d *mvsDB) finalizeHit(idx int, data []byte, handler MatchHandler) bool {
	cp := d.all[idx]
	if d.gate[idx] {
		// 超集门: regexp2 复核 (不命中则不上报, 命中按 verifier 语义上报).
		return d.reportViaVerifier(cp, data, handler)
	}
	if !d.reportLoc {
		return !handler(Match{ID: cp.id, From: -1, To: -1})
	}
	nfa := d.nfas[idx]
	if d.re2Loc[idx] || nfa.hasAssert {
		// regexp2-origin (语言等价) 或断言 NFA: 精确定位交 verifier.
		return d.reportViaVerifier(cp, data, handler)
	}
	stopped := false
	nfa.findAllLoc(data, func(from, to int) bool {
		if !handler(Match{ID: cp.id, From: from, To: to}) {
			stopped = true
			return false
		}
		return true
	})
	return stopped
}

// verifyOne 对一条 pattern 做判定与上报. 有 NFA 时: 先用 existsIn 做廉价存在性门控 (绝大多数
// pattern/记录不命中, 走纯位运算快路径); 命中后再用 findAllLoc 由 NFA 自身算出每个非重叠匹配的
// 精确字节区间 [from,to), 以 Match{ID, From, To} 上报 (匹配内容即 data[from:to]). 无 NFA 的
// 兜底 pattern (regexp2-only: lookaround/backref) 无法可靠给出字节偏移, 仍以存在性 (-1,-1) 上报.
// 返回 true 表示 handler 要求停止.
func (d *mvsDB) verifyOne(idx int, data []byte, sc *scratch, handler MatchHandler) bool {
	cp := d.all[idx]
	if nfa := d.nfas[idx]; nfa != nil {
		// 存在性门控: 断言 NFA 走 Go existsInAssertShared (不进 C 内核, 复用每报文共享边界);
		// lean NFA 有 C 内核走 C (与 Go existsIn 逐位一致), 否则纯 Go.
		var hit bool
		switch {
		case nfa.hasAssert:
			hit = nfa.existsInAssertShared(data, d.sharedBound(data, sc))
		case d.kernel != nil:
			hit = d.kernel.nfaExists(idx, data)
		default:
			hit = nfa.existsIn(data)
		}
		if !hit {
			return false
		}
		// 命中后统一收敛到 finalizeHit (含 gate 超集复核 / re2Loc / 断言定位 / 纯 NFA 定位).
		return d.finalizeHit(idx, data, handler)
	}
	// 无 NFA 的兜底: verifier 命中即存在; regexp2-only 无精确字节偏移, 以 -1/-1 表示存在性命中.
	return d.reportViaVerifier(cp, data, handler)
}

// reportViaVerifier 用 verifier 给出的非重叠区间上报命中 (regexp2-only 为 -1/-1 存在性).
func (d *mvsDB) reportViaVerifier(cp *compiledPattern, data []byte, handler MatchHandler) bool {
	for _, loc := range cp.v.findAll(data) {
		if !handler(Match{ID: cp.id, From: loc[0], To: loc[1]}) {
			return true
		}
	}
	return false
}

func (d *mvsDB) resetScratch(sc *scratch) {
	sc.fullDone = resetBoolBuf(sc.fullDone, d.n)
	sc.assertBoundReady = false
	sc.winLo = ensureInt32(sc.winLo, d.n)
	sc.winHi = ensureInt32(sc.winHi, d.n)
	// batchSeen 在使用前由 scan 用 resetBoolBuf 清零 (仅 pf != nil 时需要)。
}

// litSpan 计算 idx 的 pattern 在字面量命中点 (结束于 hitEnd) 的收窄子切片 [lo,hi]:
// 据该 pattern 的两侧上下文界 (d.win[idx]) 回看/前看; 不可界一侧退回报文端 (0 / n)。
// 任一含该命中字面量的匹配必落 [lo,hi] 内 (见 mvs_window.go 正确性), 故窗口 existsIn 与整段同真伪。
func (d *mvsDB) litSpan(idx, hitEnd, n int) (int, int) {
	w := d.win[idx]
	lo := 0
	if w.head >= 0 {
		if lo = hitEnd - int(w.head); lo < 0 {
			lo = 0
		}
	}
	hi := n
	if w.tail >= 0 {
		if hi = hitEnd + int(w.tail); hi > n {
			hi = n
		}
	}
	return lo, hi
}

// ensureInt32 返回长度为 n 的 []int32 (尽量复用底层数组; 内容无需清零, 调用方按 batchSeen 门控读)。
func ensureInt32(buf []int32, n int) []int32 {
	if cap(buf) < n {
		return make([]int32, n)
	}
	return buf[:n]
}

// sharedBound 惰性计算本报文的零宽断言边界条件 (每报文至多一次), 供多条断言 NFA 共享.
// 仅在首条断言 NFA 需要门控时触发; 无断言命中的报文完全不计算 (零开销).
func (d *mvsDB) sharedBound(data []byte, sc *scratch) []uint8 {
	if !sc.assertBoundReady {
		sc.assertBound = computeBoundaries(data, sc.assertBound)
		sc.assertBoundReady = true
	}
	return sc.assertBound
}

// resetBoolBuf 返回一个长度为 n、全部清零的 []bool, 尽量复用入参底层数组.
func resetBoolBuf(buf []bool, n int) []bool {
	if cap(buf) < n {
		return make([]bool, n)
	}
	buf = buf[:n]
	for i := range buf {
		buf[i] = false
	}
	return buf
}
