package minirehs

import (
	"regexp/syntax"
	"strings"
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
		all:          patterns,
		n:            len(patterns),
		nfas:         make([]*mvsNFA, len(patterns)),
		gate:         make([]bool, len(patterns)),
		re2Loc:       make([]bool, len(patterns)),
		windowable:   make([]bool, len(patterns)),
		batchable:    make([]bool, len(patterns)),
		anchorable:   make([]bool, len(patterns)),
		win:          make([]litWindow, len(patterns)),
		gateHead:     make([]int32, len(patterns)),
		biAnchorable: make([]bool, len(patterns)),
		revNFAs:      make([]*mvsNFA, len(patterns)),
		reportLoc:    cfg.reportLocation,
	}
	perPatHeads := make(map[int]map[string]int32, len(patterns))        // 锚定式 pattern 的 per-literal head
	perPatBiCover := make(map[int]map[string]litBiCover, len(patterns)) // 双向锚定 pattern 的 per-literal headF/tailR
	for i := range db.win {
		db.win[i] = litWindow{head: -1, tail: -1} // 默认不收窄 (整段)
		db.gateHead[i] = -1                       // 默认不局部化门复核
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
				// 超集门局部化: 命中字面量后, 把 regexp2 复核 + 断言超集预检收窄到 data[winLo:]
				// (winLo=firstHitEnd-gateHead-1). gateHead 取超集骨架下全部字面量的最大回看上限。
				if len(cp.literals) > 0 {
					db.gateHead[cp.idx] = computeGateHead(cp)
				}
			}
			// 窗口化存在性快路径资格: 有界宽 (windowed) 且无零宽断言、非超集门的 lean NFA.
			// 仅 !reportLoc 时启用 (见 scan); 定位档仍整段, 保定位语义不变.
			if cp.windowed && cp.winW > 0 && !nfa.hasAssert && !gate {
				db.windowable[cp.idx] = true
			}
			// 批处理资格: 非断言 lean NFA 已序列化进 C blob, 可走 nfaExistsMany 一次 cgo 多条验证.
			if !nfa.hasAssert {
				db.batchable[cp.idx] = true
			}
			// 存在性本地化界 (lean 与断言通用): 窗口/锚定分析用一条 RE2 可解析的表达式 analysisExpr:
			//   - RE2-exact (!gate && !re2Loc): 直接用 cp.expr (NFA 即由它编译, 界精确)。
			//   - re2Loc (语言等价超集, widened=false): 用超集骨架 —— cp.expr 可能含 \u / 原子组等
			//     RE2 不可解析构造 (syntax.Parse 会失败, 退化整段), 而超集已归一为 \x{} 且与原语言等价,
			//     故界精确、可安全窗口化/锚定 (修复 \u 类 pattern 如 "Url信息" 被迫整段扫的退化)。
			//   - gate (widened 超集 ⊋ 原): 不在此锚定 (NFA 命中仍需 regexp2 复核); 其局部化由 gateHead 覆盖。
			if !gate && len(cp.literals) > 0 {
				analysisExpr := cp.expr
				if re2Loc {
					if super, _, ok := re2SupersetEx(cp.expr); ok {
						analysisExpr = super
					} else {
						analysisExpr = "" // 超集都不可解析: 保守整段
					}
				}
				if analysisExpr != "" {
					w := computeLitWindow(analysisExpr, cp.literals)
					if nfa.anchoredStart {
						w.head = -1 // ^ \A 锚: 匹配必起于偏移 0, 不可左截
					}
					if nfa.requireEnd {
						w.tail = -1 // $ \z 锚: 匹配必止于偏移 n, 不可右截
					}
					db.win[cp.idx] = w

					// 锚定式单趟资格: 命中字面量后只在邻域 [h.end-head_L, h.end] 注入起点, 靠 NFA 提前
					// 消亡省去整段扫 (整段 existsIn/existsInAssertShared 每步重注入 first, 活跃集永不消亡,
					// 非匹配报文被迫扫满窗口; 锚定式注入区间外不注入, 失配后即消亡退出):
					//   - 断言 NFA: 本就 Go 侧整段扫, 锚定后大幅省尾; 位置锚 (^ $ \b) 编码为 guard 按真实
					//     bound 门控, 多注入位置自动滤除, 无害。
					//   - lean NFA: 仅当尾部无界 (tail<0)、头部有界、非 ^ 锚 (anchoredStart) 且未走廉价小窗
					//     (windowable) 时锚定 —— 这类当前在 C 内核被迫扫 [hit,n] 整段, 锚定式提前消亡可显著
					//     省去非匹配报文的尾部扫描。尾有界者已被 C 小窗廉价覆盖, 不替换。
					// per-literal head 见 perPatHeads (单条 pattern 内有界分支字面量仍可锚定, 无界分支退化整段)。
					eligible := false
					if nfa.hasAssert {
						eligible = true
					} else if !nfa.anchoredStart && w.tail < 0 && !db.windowable[cp.idx] {
						eligible = true
					}
					if eligible {
						heads := computeLitHeads(analysisExpr, cp.literals)
						anyBoundedHead := false
						for _, h := range heads {
							if h >= 0 {
								anyBoundedHead = true
								break
							}
						}
						if anyBoundedHead {
							db.anchorable[cp.idx] = true
							perPatHeads[cp.idx] = heads
							if nfa.nword > db.maxAnchorNword {
								db.maxAnchorNword = nfa.nword
							}
						}
					}
					// 双向锚定 (Rose-lite 完全体): 仅对当前会落整段 batch (非 window/anchor) 的 lean、
					// 无文本锚 pattern. 若每个必需字面量"每个出现处 head 或 tail 至少一侧有界"且至少一条
					// 字面量需反向 (tailR>=0), 则编反向 NFA, 走前向锚定 ∪ 反向锚定替换整段扫. 头有界出现处
					// 由前向覆盖、尾有界出现处由反向覆盖, 二者并集 = 全部匹配 (零假阴, 见 mvs_bicover.go).
					if biAnchorEnabled && !nfa.hasAssert && !nfa.anchoredStart && !nfa.requireEnd &&
						!db.windowable[cp.idx] && !db.anchorable[cp.idx] {
						bc := computeLitBiCover(analysisExpr, cp.literals)
						allOK, needRev := true, false
						for _, l := range cp.literals {
							c, ok := bc[l]
							if !ok || !c.ok {
								allOK = false
								break
							}
							if c.tailR >= 0 {
								needRev = true
							}
						}
						if allOK && needRev {
							if rev := compileReverseExprToNFA(analysisExpr); rev != nil && !rev.hasAssert {
								db.biAnchorable[cp.idx] = true
								db.revNFAs[cp.idx] = rev
								db.hasBiAnchor = true
								perPatBiCover[cp.idx] = bc
								if nfa.nword > db.maxAnchorNword {
									db.maxAnchorNword = nfa.nword
								}
								if rev.nword > db.maxAnchorNword {
									db.maxAnchorNword = rev.nword
								}
							}
						}
					}
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
		// litHead 与 litToPat 同形: 为锚定式 pattern 填该 (litID,pattern) 的 per-literal head; 其余 -1.
		// litBiHead/litBiTail 同形: 为双向锚定 pattern 填该字面量的前向 headF / 反向 tailR; 其余 -1.
		db.litHead = make([][]int32, len(li.litToPat))
		db.litBiHead = make([][]int32, len(li.litToPat))
		db.litBiTail = make([][]int32, len(li.litToPat))
		for litID, pats := range li.litToPat {
			heads := make([]int32, len(pats))
			biH := make([]int32, len(pats))
			biT := make([]int32, len(pats))
			litStr := li.literals[litID]
			for k, idx := range pats {
				heads[k] = -1
				biH[k], biT[k] = -1, -1
				if db.anchorable[int(idx)] {
					if h, ok := perPatHeads[int(idx)][litStr]; ok {
						heads[k] = h
					}
				}
				if db.biAnchorable[int(idx)] {
					if c, ok := perPatBiCover[int(idx)][litStr]; ok {
						biH[k], biT[k] = c.headF, c.tailR
					}
				}
			}
			db.litHead[litID] = heads
			db.litBiHead[litID] = biH
			db.litBiTail[litID] = biT
		}
	}
	db.merged = buildMergedNFA(mergeMembers)
	db.mergedCount = len(mergeMembers)
	// 必要条件预过滤: 对 always-on NFA (merged + assert) 从 RE2 表达式提取必要条件,
	// 运行期先做廉价字节检查, 不满足则跳过整段 NFA 扫描 (绝不假阴).
	if len(mergeMembers) > 0 {
		factors := make([]necFactor, len(mergeMembers))
		for mi, mem := range mergeMembers {
			cp := patterns[mem.idx]
			anchoredStart := false
			requireEnd := false
			if nfa := db.nfas[mem.idx]; nfa != nil {
				anchoredStart = nfa.anchoredStart
				requireEnd = nfa.requireEnd
			}
			factors[mi] = extractNecFactor(analysisExprFor(cp), anchoredStart, requireEnd)
		}
		db.mergedNecFactor = mergeNecFactorsDisj(factors)
	}
	if len(db.assertAlwaysOn) > 0 {
		db.assertNecFactor = make([]necFactor, db.n)
		for _, idx := range db.assertAlwaysOn {
			cp := patterns[idx]
			nfa := db.nfas[idx]
			anchoredStart := false
			requireEnd := false
			if nfa != nil {
				anchoredStart = nfa.anchoredStart
				requireEnd = nfa.requireEnd
			}
			db.assertNecFactor[idx] = extractNecFactor(analysisExprFor(cp), anchoredStart, requireEnd)
		}
	}
	// R2: 把无字面量的断言 always-on NFA 合并为单趟扫描 (共享边界条件),
	// 替代逐条 existsInAssertShared 整段扫 (省 K-1 趟全量扫描 + K-1 次 make).
	// 注: 实测合并后 nword 1→2 净回归 (见 assertMergedEnabled), 仅在 A/B 开关开启时构建.
	if assertMergedEnabled && len(db.assertAlwaysOn) >= 2 {
		var assertMembers []mergeMember
		for _, idx := range db.assertAlwaysOn {
			if nfa := db.nfas[idx]; nfa != nil && nfa.hasAssert {
				assertMembers = append(assertMembers, mergeMember{idx: idx, nfa: nfa})
			}
		}
		if len(assertMembers) >= 2 {
			db.assertMerged = buildAssertMergedNFA(assertMembers)
		}
	}
	// R1：把可 forward-anchor 的 lean NFA 编成一个 span-injected 不相交并集。
	// 只在 A/B 开关开启时构建，避免尚未验收的全局位集给默认数据库增加内存与编译成本。
	var anchorMembers []mergeMember
	for idx, nfa := range db.nfas {
		if nfa != nil && db.anchorable[idx] && !nfa.hasAssert {
			anchorMembers = append(anchorMembers, mergeMember{idx: idx, nfa: nfa})
		}
	}
	if anchorMergedEnabled && len(anchorMembers) >= 2 {
		db.anchoredMerged = buildMergedAnchoredNFA(anchorMembers)
		db.anchorMergedSlot = make([]int, db.n)
		for i := range db.anchorMergedSlot {
			db.anchorMergedSlot[i] = -1
		}
		for mi, idx := range db.anchoredMerged.memIdx {
			db.anchorMergedSlot[idx] = mi
		}
	}
	db.nfaCount = nfaCount

	// P4 (M2): minirehs_mvs 构建下, 把 per-pattern NFA + 合并 always-on 序列化为平台无关
	// blob, 交纯 C99 运行期内核执行存在性扫描 (与纯 Go 参考执行器逐位一致). 非该构建返回 nil,
	// 自动退化为纯 Go existsIn / scanExist. 定位 (findAllLoc) 始终走已验证的 Go 路径.
	db.kernel = newMVSKernel(db)
	anchorCount, biAnchorCount := 0, 0
	for i := 0; i < db.n; i++ {
		if db.anchorable[i] {
			anchorCount++
		}
		if db.biAnchorable[i] {
			biAnchorCount++
		}
	}
	cfg.logger.Infof("minirehs/mvs compiled %d pattern(s): nfa=%d (assert=%d gate=%d) fallback=%d anchor=%d bi_anchor=%d always_on=%d (merged_nfa=%d assert=%d other=%d) c_kernel=%v",
		db.n, nfaCount, assertCount, gateCount, db.n-nfaCount, anchorCount, biAnchorCount,
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

// computeGateHead 计算超集门 gate 的局部化回看上限: 超集骨架下全部必需字面量的最大 head
// (match-start 到字面量结尾的最大字节宽). 任一字面量无界 / 含 lookbehind / verifier 可给精确偏移
// (非 regexp2-only) 时返回 -1 (不局部化, 保守整段).
//
// 正确性: gate 语言只增不减 (R_super ⊇ R_orig), 故超集 head >= 原 head; winLo=firstHitEnd-head-1
// 必 <= 任一原匹配起点 (绝不漏报), 且 winLo+head < firstHitEnd <= 任一字面量结尾 (winLo 前无该门
// 字面量可达, 子切片首位起不了匹配 -> 绝不误报). lookbehind 会读 match-start 左侧可能越过 winLo, 故排除。
func computeGateHead(cp *compiledPattern) int32 {
	if cp.v == nil || cp.v.exact() {
		return -1 // 仅对 regexp2-only (命中报 -1/-1) 局部化, 避免精确 verifier 偏移错位
	}
	if strings.Contains(cp.expr, "(?<=") || strings.Contains(cp.expr, "(?<!") {
		return -1 // lookbehind: 读 match-start 左侧上下文, 可能越过 winLo
	}
	super, _, ok := re2SupersetEx(cp.expr)
	if !ok {
		return -1
	}
	heads := computeLitHeads(super, cp.literals)
	if len(heads) == 0 {
		return -1
	}
	maxHead := int32(-1)
	for _, lit := range cp.literals {
		h, ok := heads[lit]
		if !ok || h < 0 {
			return -1 // 任一字面量无界 -> 不能安全收窄
		}
		if h > maxHead {
			maxHead = h
		}
	}
	return maxHead
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
	nfas       []*mvsNFA   // 按 idx; nil 表示该 pattern 走 verifier 兜底
	gate       []bool      // 按 idx; true=该 NFA 为严格超集门, 命中须 regexp2 复核滤假阳
	re2Loc     []bool      // 按 idx; true=精确定位交 regexp2 (regexp2-origin, 保 PCRE span 语义)
	windowable []bool      // 按 idx; true=可在字面量命中点邻域窗口内做存在性验证 (有界宽 lean NFA)
	batchable  []bool      // 按 idx; true=非断言且有 NFA (在 C blob 中), 可走 nfaExistsMany 批处理
	anchorable []bool      // 按 idx; true=可走锚定式单趟存在性 (有界头/无界尾, 命中字面量后只在邻域注入起点)
	win        []litWindow // 按 idx; 命中字面量结尾两侧上下文界 (head/tail; -1=该侧无界不收窄)
	nfaCount   int

	// 双向锚定 (Rose-lite 完全体): biAnchorable[idx]=true 表示该 lean pattern 当前会落整段 batch,
	// 但每个必需字面量"每个出现处 head 或 tail 至少一侧有界" (computeLitBiCover.ok), 可用前向锚定
	// (头有界出现处) ∪ 反向锚定 (尾有界出现处) 替换整段扫. revNFAs[idx] 为该 pattern 的反向 NFA
	// (反转语言的同构 Glushkov 自动机, 见 mvs_reverse.go), nil 表示非 biAnchorable.
	biAnchorable []bool
	revNFAs      []*mvsNFA
	hasBiAnchor  bool
	// litBiHead/litBiTail 与 litToPat 同形: 该字面量在该 biAnchorable pattern 的 per-literal
	// 前向回看上限 headF (>=0 才注入前向区间) 与反向前看上限 tailR (>=0 才注入反向区间); 非
	// biAnchorable 项为 -1. 见 computeLitBiCover / scan 双向锚定批处理.
	litBiHead [][]int32
	litBiTail [][]int32

	// litHead 与 litToPat 同形 (litHead[litID][k] 对应 litToPat[litID][k] 那条 pattern): 该字面量在
	// 该 pattern 的 per-literal 回看上限 head (-1=无界, 锚定式退化为 [0,hitEnd] 整段注入). 仅锚定式
	// pattern 的项有意义, 其余为 -1. 见 computeLitHeads / scan 锚定批处理。
	litHead [][]int32
	// maxAnchorNword 是全部锚定式 pattern 的 nfa.nword 最大值, 用于一次性分配锚定执行器工作缓冲。
	maxAnchorNword int

	// gateHead 按 idx; 仅对"可局部化的超集门 gate" >=0, 表示该 pattern 全部字面量在超集骨架下的
	// 最大回看上限 (match-start 到字面量结尾的字节宽). gate 命中字面量后, 把 regexp2 复核与断言 NFA
	// 超集预检都收窄到 data[winLo:] (winLo=firstHitEnd-gateHead-1, rune 对齐), 省去字面量前的整段扫.
	// -1 表示不局部化 (无字面量 / 任一字面量无界 / 含 lookbehind / verifier 可给精确偏移). 见 verifyGateLocalized.
	gateHead []int32

	pf       prefilter // 可能为 nil (无任何字面量 pattern)
	litToPat [][]int32 // litID -> pattern idx

	merged           *mvsMergedNFA // 无字面量且可编入 lean NFA 的 always-on 合并为单趟自动机
	mergedCount      int           // 合并成员数

	// assertMerged 是无字面量的断言 NFA always-on (hasAssert) 的单趟合并自动机 (R2),
	// 共享 computeBoundaries 预算的边界条件, 替代逐条 existsInAssertShared 整段扫.
	// 仅在有 >=2 条断言 always-on 时构建 (单条直接走 verifyOne 逐条). 不影响命中语义.
	assertMerged *mvsAssertMergedNFA

	// mergedNecFactor 是合并 always-on NFA 的必要条件预过滤 (所有成员必要条件的"最宽松交集").
	// scan() 中, merged_scan 前先 check; check==false 则跳过 cgo 调用 (绝不假阴).
	mergedNecFactor necFactor
	// assertNecFactor 按 idx: 断言 always-on NFA 的 per-pattern 必要条件预过滤.
	// scan() 中, 对每条 assert always-on 先 check; check==false 则跳过 existsInAssertShared.
	assertNecFactor []necFactor
	anchoredMerged   *mvsMergedNFA // R1 span-injected lean anchored 合并自动机（A/B 开关控制使用）
	anchorMergedSlot []int         // pattern idx -> anchoredMerged 成员槽位；-1 表示非成员
	assertAlwaysOn   []int         // 无字面量的断言 NFA (hasAssert): existsInAssert 门控 + verifier 定位
	otherAlwaysOn    []int         // 无字面量且不可编入 NFA (regexp2/RE2 兜底): 逐条验证

	reportLoc bool // 命中是否上报精确偏移与内容 (见 WithReportLocation)

	// kernel 是纯 C99 运行期内核句柄 (仅 minirehs_mvs 构建非 nil); 非 nil 时存在性扫描
	// (existsIn / merged scanExist) 走 C, 定位仍走 Go. nil 则全程纯 Go.
	kernel *mvsKernel
}

func (d *mvsDB) numAlwaysOn() int {
	return d.mergedCount + len(d.assertAlwaysOn) + len(d.otherAlwaysOn)
}

// gateSupersetPrecheck 控制可局部化超集门复核前是否先跑超集 NFA 存在性预检 (见 verifyGateLocalized).
// PCRE2 (go-pcre2-lite) 线性复核后该预检对 gate 几乎不过滤、反成净开销, 故默认关闭; 仅 A/B 基准临时开。
var gateSupersetPrecheck = false

// anchorMergedEnabled 控制 R1 span-injected merged verifier 的运行期接线。保守起见默认
// 关闭，直到真实语料的 oracle 与基准都证明其全局位集成本低于逐条 gap-jump 路径。
var anchorMergedEnabled = false

// assertMergedEnabled 控制 R2 断言 NFA 合并单趟的运行期接线。实测合并后 nword 1→2,
// 多字循环开销 > 省的趟数 (与 lean 合并的 A/B 结论一致), 默认关闭保留作差分护栏.
var assertMergedEnabled = false

// necPrefilterEnabled 控制必要条件字节预过滤的运行期接线. A/B 实测: micro-benchmark 下
// 预检 ~267 MB/s vs NFA ~149 MB/s, 但端到端基准仍净回归 (-2~3%): LimEx NFA (excCount=0)
// 每字节仅 shift+AND (3 操作), 与 digit-run 检查 (4 操作/字节) 相当; 高 skip rate 省下的
// NFA 扫描抵不过每记录额外的预检扫描 + 分支开销. 需 C 内核 SIMD 预检才净赚. 默认关闭.
var necPrefilterEnabled = false

// anchorCBatchEnabled 控制 C anchored-many 接线。它与 Go gap-jump 路径语义等价，
// 但真实规则集上 C 对大量单字 NFA 的逐条重扫目前慢于 Go 标量快路径；默认关闭，保留作
// C 侧 trigger/event 融合完成前的差分护栏和 A/B 基线。
var anchorCBatchEnabled = false

// cgo 调用诊断计数器 (仅 cgoDiagEnabled=true 时累加, 默认关闭, 近零开销). 用于量化
// "cgo 跨界次数 vs 实际扫描字节" 以判定瓶颈是跨界开销还是扫描工作量 (见 TestMVSCgoCallDiag).
// 声明置于无构建标签文件, 使 cgo / 非 cgo 构建与测试均可引用 (增量仅在 mvs_cgo.go 的真实调用处).
var (
	cgoDiagEnabled    bool
	cgoNfaExistsCalls int64
	cgoNfaExistsBytes int64
	cgoMergedCalls    int64
	cgoMergedBytes    int64
)

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
		anchorBatch := sc.anchorBatch[:0]
		sc.anchorSeen = resetBoolBuf(sc.anchorSeen, d.n)
		biBatch := sc.biBatch[:0]
		if d.hasBiAnchor {
			sc.biSeen = resetBoolBuf(sc.biSeen, d.n)
		}
		for _, h := range hits {
			if int(h.litID) >= len(d.litToPat) {
				continue
			}
			for k, idx := range d.litToPat[h.litID] {
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
				// 锚定式单趟 (有界头/无界尾的 lean 或断言 NFA): 累积本 idx 全部命中点的注入区间
				// [h.end-head_L, h.end] (head_L<0 退化为 [0,h.end]), 批后做一次锚定式存在性. 只在
				// 这些区间注入起点 => NFA 可提前消亡, 省去无界尾整段扫. 见 mvs_anchored.go 正确性说明.
				if d.anchorable[idx] {
					head := d.litHead[h.litID][k]
					lo := 0
					if head >= 0 {
						if lo = int(h.end) - int(head); lo < 0 {
							lo = 0
						}
					}
					if !sc.anchorSeen[idx] {
						sc.anchorSeen[idx] = true
						sc.anchorRanges[idx] = sc.anchorRanges[idx][:0]
						anchorBatch = append(anchorBatch, idx)
					}
					sc.anchorRanges[idx] = append(sc.anchorRanges[idx], anchorSpan{int32(lo), h.end})
					continue
				}
				// 双向锚定: 据该字面量 headF/tailR 分别累积前向区间 [h.end-headF, h.end] (头有界) 与
				// 反向区间 [h.end, h.end+tailR] (尾有界), 批后做前向锚定 ∪ 反向锚定单趟替换整段扫.
				if d.biAnchorable[idx] {
					if !sc.biSeen[idx] {
						sc.biSeen[idx] = true
						sc.biFwdRanges[idx] = sc.biFwdRanges[idx][:0]
						sc.biRevRanges[idx] = sc.biRevRanges[idx][:0]
						biBatch = append(biBatch, idx)
					}
					if hf := d.litBiHead[h.litID][k]; hf >= 0 {
						lo := int(h.end) - int(hf)
						if lo < 0 {
							lo = 0
						}
						sc.biFwdRanges[idx] = append(sc.biFwdRanges[idx], anchorSpan{int32(lo), h.end})
					}
					if tr := d.litBiTail[h.litID][k]; tr >= 0 {
						hi := int(h.end) + int(tr)
						if hi > n {
							hi = n
						}
						sc.biRevRanges[idx] = append(sc.biRevRanges[idx], anchorSpan{h.end, int32(hi)})
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
				// 可局部化超集门: 命中字面量后只在 data[winLo:] 复核 (winLo=h.end-gateHead-1, rune
				// 对齐). 本 idx 首个命中 => h.end 最小, 取超集 max head 必覆盖任一原匹配起点 (见
				// computeGateHead / verifyGateLocalized 正确性), 省去字面量前的整段 regexp2/断言扫。
				sc.fullDone[idx] = true
				if gh := d.gateHead[idx]; gh >= 0 {
					winLo := int(h.end) - int(gh) - 1
					if winLo < 0 {
						winLo = 0
					}
					winLo = alignRuneStart(data, winLo)
					if stop := d.verifyGateLocalized(int(idx), data, winLo, sc, handler); stop {
						return true, nil
					}
					continue
				}
				// 其余 (无内核 / 断言 NFA / 无 NFA 兜底): 立即逐条验证.
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
				if stop := d.finalizeHit(idx, data, sc, handler); stop {
					return true, nil
				}
			}
		}
		// 锚定式批处理: 对本报文每条触发的锚定式 pattern, 合并其注入区间后做一次锚定式单趟存在性.
		sc.anchorBatch = anchorBatch
		// lean anchored NFA 已在 C blob 中。把本报文所有 pattern 的 spans 平铺后一次跨界；
		// C 内仍按 pattern 独立执行相同的提前消亡递推，但避免 Go 热循环和 O(pattern) cgo。
		// 含零宽断言的 NFA 不入 blob，必须留在下方 Go 路径以复用真实边界 guard。
		if anchorCBatchEnabled && d.kernel != nil && !anchorMergedEnabled && len(anchorBatch) > 0 {
			sc.anchorCIdx = sc.anchorCIdx[:0]
			sc.anchorSpanOff = sc.anchorSpanOff[:0]
			sc.anchorSpansLo = sc.anchorSpansLo[:0]
			sc.anchorSpansHi = sc.anchorSpansHi[:0]
			for _, idx32 := range anchorBatch {
				idx := int(idx32)
				nfa := d.nfas[idx]
				if nfa == nil || nfa.hasAssert {
					continue
				}
				spans := mergeAnchorSpans(sc.anchorRanges[idx])
				if len(spans) == 0 {
					continue
				}
				sc.anchorCIdx = append(sc.anchorCIdx, idx32)
				sc.anchorSpanOff = append(sc.anchorSpanOff, int32(len(sc.anchorSpansLo)))
				for _, span := range spans {
					sc.anchorSpansLo = append(sc.anchorSpansLo, span.lo)
					sc.anchorSpansHi = append(sc.anchorSpansHi, span.hi)
				}
			}
			if len(sc.anchorCIdx) > 0 {
				sc.anchorSpanOff = append(sc.anchorSpanOff, int32(len(sc.anchorSpansLo)))
				out := d.kernel.nfaExistsAnchoredMany(sc.anchorCIdx, data, sc.anchorSpanOff,
					sc.anchorSpansLo, sc.anchorSpansHi, sc)
				for i, hit := range out {
					idx := int(sc.anchorCIdx[i])
					sc.fullDone[idx] = true
					if hit != 0 && d.finalizeHit(idx, data, sc, handler) {
						return true, nil
					}
				}
			}
		}
		if anchorMergedEnabled && d.anchoredMerged != nil {
			if cap(sc.anchorMergedSpans) < d.anchoredMerged.nmem {
				sc.anchorMergedSpans = make([][]anchorSpan, d.anchoredMerged.nmem)
			} else {
				sc.anchorMergedSpans = sc.anchorMergedSpans[:d.anchoredMerged.nmem]
				for i := range sc.anchorMergedSpans {
					sc.anchorMergedSpans[i] = nil
				}
			}
			mergedAny := false
			for _, idx32 := range anchorBatch {
				idx := int(idx32)
				mi := d.anchorMergedSlot[idx]
				if mi < 0 {
					continue // 断言 NFA 仍由下方单条 guarded verifier 执行
				}
				spans := mergeAnchorSpans(sc.anchorRanges[idx])
				sc.anchorMergedSpans[mi] = spans
				sc.fullDone[idx] = true
				mergedAny = true
			}
			if mergedAny {
				sc.mergedSeen = resetBoolBuf(sc.mergedSeen, d.n)
				sc.mergedHits = d.anchoredMerged.scanExistAnchored(data, sc.anchorMergedSpans, sc.mergedSeen, sc.mergedHits[:0])
				for _, idx := range sc.mergedHits {
					if stop := d.finalizeHit(idx, data, sc, handler); stop {
						return true, nil
					}
				}
			}
		}
		for _, idx32 := range anchorBatch {
			idx := int(idx32)
			if anchorCBatchEnabled && d.kernel != nil && !anchorMergedEnabled && d.nfas[idx] != nil && !d.nfas[idx].hasAssert {
				continue // 已由上方 C anchored-many 精确判定
			}
			if anchorMergedEnabled && d.anchoredMerged != nil && d.anchorMergedSlot[idx] >= 0 {
				continue // 已由上方 merged verifier 精确判定
			}
			sc.fullDone[idx] = true
			spans := mergeAnchorSpans(sc.anchorRanges[idx])
			nfa := d.nfas[idx]
			var hit bool
			// nword==1 (绝大多数真实 pattern) 走标量零分配快路径, 否则走多字通用版.
			switch {
			case nfa.hasAssert && nfa.single:
				hit = nfa.existsInAssertAnchored1(data, d.sharedBound(data, sc), spans)
			case nfa.hasAssert:
				hit = nfa.existsInAssertAnchored(data, d.sharedBound(data, sc), spans, sc.anchorPrev, sc.anchorCand)
			case nfa.single:
				hit = nfa.existsInAnchored1(data, spans)
			case nfa.nword == 2:
				hit = nfa.existsInAnchored2(data, spans)
			default:
				hit = nfa.existsInAnchored(data, spans, sc.anchorPrev, sc.anchorCand, sc.anchorActive)
			}
			if hit {
				if stop := d.finalizeHit(idx, data, sc, handler); stop {
					return true, nil
				}
			}
		}
		// 双向锚定批处理: 前向锚定 (头有界出现处) ∪ 反向锚定 (尾有界出现处) 替换整段 batch. 任一向命中
		// 即真匹配 (子串/邻域关系无假阳); 二者并集覆盖全部匹配 (每出现处至少一侧有界, 无假阴).
		sc.biBatch = biBatch
		for _, idx32 := range biBatch {
			idx := int(idx32)
			sc.fullDone[idx] = true
			var hit bool
			if fwdSpans := mergeAnchorSpans(sc.biFwdRanges[idx]); len(fwdSpans) > 0 {
				fwd := d.nfas[idx]
				if fwd.single {
					hit = fwd.existsInAnchored1(data, fwdSpans)
				} else if fwd.nword == 2 {
					hit = fwd.existsInAnchored2(data, fwdSpans)
				} else {
					hit = fwd.existsInAnchored(data, fwdSpans, sc.anchorPrev, sc.anchorCand, sc.anchorActive)
				}
			}
			if !hit {
				if revSpans := mergeAnchorSpans(sc.biRevRanges[idx]); len(revSpans) > 0 {
					rev := d.revNFAs[idx]
					if rev.single {
						hit = rev.existsInReverseAnchored1(data, revSpans)
					} else if rev.nword == 2 {
						hit = rev.existsInReverseAnchored2(data, revSpans)
					} else {
						hit = rev.existsInReverseAnchored(data, revSpans, sc.anchorPrev, sc.anchorCand, sc.anchorActive)
					}
				}
			}
			if hit {
				if stop := d.finalizeHit(idx, data, sc, handler); stop {
					return true, nil
				}
			}
		}
	}

	// 2) 无字面量且可编入 NFA 的 always-on: 合并自动机单趟扫描得命中集合, 命中后逐个定位上报.
	//    有 C 内核时单趟扫描走 C (返回去重后的成员 idx, 再用 fullDone 跨步去重); 否则走纯 Go.
	//    注: 必要条件预过滤仅在纯 Go 路径 (无 C 内核) 时启用 —— C 内核 merged_scan 已是 SIMD 加速,
	//    Go 字节预检 O(n) 不比 C 快, 反成净开销. 纯 Go 路径则 NFA 也是 Go, 预检能省整段位递推.
	if d.merged != nil {
		var hits []int
		if d.kernel != nil {
			hits = d.kernel.mergedScan(data, sc)
		} else {
			if !d.mergedNecFactor.hasFactor || d.mergedNecFactor.check(data) {
				sc.mergedSeen = resetBoolBuf(sc.mergedSeen, d.n)
				sc.mergedHits = d.merged.scanExist(data, sc.mergedSeen, sc.mergedHits[:0])
				hits = sc.mergedHits
			}
		}
		for _, idx := range hits {
			if sc.fullDone[idx] {
				continue
			}
			sc.fullDone[idx] = true
			if stop := d.reportLocated(idx, data, sc, handler); stop {
				return true, nil
			}
		}
	}

	// 3) 无字面量的断言 NFA always-on: 有合并自动机时走单趟 scanExistAssert (共享边界条件),
	//    命中后交 verifier 定位; 否则逐条 existsInAssertShared (共享边界) 门控.
	// 注: 合并经 A/B 实测为净回归 (nword 1→2, 多字循环开销 > 省的趟数), 默认不启用 (见 assertMergedEnabled).
	if assertMergedEnabled && d.assertMerged != nil {
		bound := d.sharedBound(data, sc)
		sc.mergedSeen = resetBoolBuf(sc.mergedSeen, d.n)
		hits := d.assertMerged.scanExistAssert(data, bound, sc.mergedSeen, sc.mergedHits[:0])
		sc.mergedHits = hits
		for _, idx := range hits {
			if sc.fullDone[idx] {
				continue
			}
			sc.fullDone[idx] = true
			if stop := d.finalizeHit(idx, data, sc, handler); stop {
				return true, nil
			}
		}
	} else {
		for _, idx := range d.assertAlwaysOn {
			if sc.fullDone[idx] {
				continue
			}
			sc.fullDone[idx] = true
			if stop := d.verifyOne(idx, data, sc, handler); stop {
				return true, nil
			}
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
func (d *mvsDB) reportLocated(idx int, data []byte, sc *scratch, handler MatchHandler) bool {
	return d.finalizeHit(idx, data, sc, handler)
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
func (d *mvsDB) finalizeHit(idx int, data []byte, sc *scratch, handler MatchHandler) bool {
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
	nfa.findAllLoc(data, sc, func(from, to int) bool {
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
		// 注: 此处 gate (gateHead<0, 整段 regexp2 复核) 仍保留超集预检 —— 整段 PCRE2 复核较贵 (cgo),
		// 超集 NFA 预检能滤掉"字面量在但无完整结构"的报文, 实测移除为净亏 (与 verifyGateLocalized 不同:
		// 后者复核已收窄到 data[winLo:] 廉价, 故移除预检为净赚)。真正的杠杆是把该 gate 也局部化 (gateHead>=0)。
		var hit bool
		switch {
		case nfa.hasAssert && nfa.single && d.kernel != nil:
			// 断言 NFA 单字: 有 C 内核时走 C nfa_run_assert_1 (C 侧 computeBoundaries + guard 门控).
			// 与 Go existsInAssertShared1 逐位一致 (差分护栏).
			bound := d.sharedBound(data, sc)
			hit = d.kernel.nfaExistsAssert(idx, data, bound)
		case nfa.hasAssert && nfa.single:
			hit = nfa.existsInAssertShared1(data, d.sharedBound(data, sc))
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
		return d.finalizeHit(idx, data, sc, handler)
	}
	// 无 NFA 的兜底: verifier 命中即存在; regexp2-only 无精确字节偏移, 以 -1/-1 表示存在性命中.
	return d.reportViaVerifier(cp, data, handler)
}

// verifyGateLocalized 对可局部化超集门做"收窄到 data[winLo:]"的存在性门 + regexp2 复核.
// winLo 已 rune 对齐且满足 winLo < 任一原匹配起点、winLo 前无该门字面量可达 (见 computeGateHead),
// 故 data[winLo:] 上的判定与整段一致:
//   - 无假阴: regexp2 真匹配整体落在 [winLo, n) 内 (其字面量结尾 >= firstHitEnd > winLo+head);
//   - 无假阳: 子切片首位 (winLo) 起不了匹配 (前方 head 距内无该门字面量), 内部命中左上下文真实。
//
// gate 的 verifier 命中报 -1/-1, 偏移无需换算; 与整段 reportViaVerifier 同上报。
func (d *mvsDB) verifyGateLocalized(idx int, data []byte, winLo int, sc *scratch, handler MatchHandler) bool {
	sub := data
	if winLo > 0 {
		sub = data[winLo:]
	}
	// 超集 NFA 预检 (gateSupersetPrecheck): 历史上用来在 dlclark/regexp2 (回溯, 极贵) 之前廉价过滤,
	// 避免对每个字面量命中都跑昂贵复核。regexp2 后端切到 go-pcre2-lite (PCRE2, 线性) 后, 复核本身已
	// 很廉价, 而预检 (尤其多字断言超集 existsInAssertShared + computeBoundaries 整段 data[winLo:]) 对
	// gate 几乎不过滤 (字面量命中 => 超集结构通常已成立), 反成净开销。故默认关 (直接复核), 仅 A/B 保留。
	if gateSupersetPrecheck {
		nfa := d.nfas[idx]
		var hit bool
		switch {
		case nfa.hasAssert && nfa.single:
			sc.gateBound = computeBoundaries(sub, sc.gateBound)
			hit = nfa.existsInAssertShared1(sub, sc.gateBound)
		case nfa.hasAssert:
			sc.gateBound = computeBoundaries(sub, sc.gateBound)
			hit = nfa.existsInAssertShared(sub, sc.gateBound)
		case d.kernel != nil:
			hit = d.kernel.nfaExists(idx, sub)
		default:
			hit = nfa.existsIn(sub)
		}
		if !hit {
			return false
		}
	}
	// regexp2 复核 (收窄到 data[winLo:]) 即权威判定 (与 oracle 同 verifier 对象): gate 命中报 -1/-1。
	return d.reportViaVerifier(d.all[idx], sub, handler)
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
	// batchSeen / anchorSeen 在使用前由 scan 用 resetBoolBuf 清零 (仅 pf != nil 时需要)。
	if d.maxAnchorNword > 0 {
		// 锚定执行器工作缓冲: 一次分配到最大 nword, 各 pattern 内部按自身 nword 切片复用。
		sc.anchorPrev = ensureU64(sc.anchorPrev, d.maxAnchorNword)
		sc.anchorCand = ensureU64(sc.anchorCand, d.maxAnchorNword)
		sc.anchorActive = ensureU64(sc.anchorActive, d.maxAnchorNword)
		if cap(sc.anchorRanges) < d.n {
			sc.anchorRanges = make([][]anchorSpan, d.n)
		} else {
			sc.anchorRanges = sc.anchorRanges[:d.n]
		}
	}
	if d.hasBiAnchor {
		if cap(sc.biFwdRanges) < d.n {
			sc.biFwdRanges = make([][]anchorSpan, d.n)
		} else {
			sc.biFwdRanges = sc.biFwdRanges[:d.n]
		}
		if cap(sc.biRevRanges) < d.n {
			sc.biRevRanges = make([][]anchorSpan, d.n)
		} else {
			sc.biRevRanges = sc.biRevRanges[:d.n]
		}
	}
}

// ensureU64 返回长度为 n 的 []uint64 (尽量复用底层数组; 内容由调用方按需清零)。
func ensureU64(buf []uint64, n int) []uint64 {
	if cap(buf) < n {
		return make([]uint64, n)
	}
	return buf[:n]
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
