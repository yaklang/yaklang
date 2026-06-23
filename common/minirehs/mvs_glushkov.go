package minirehs

import (
	"regexp/syntax"
	"sort"
	"unicode"
	"unicode/utf8"
)

// 本文件实现 mvscan 编译期前端核心: 把 RE2 语法树 (regexp/syntax, 即 unify AST 的 RE2 来源)
// 转换为一个 rune 级中间表示 bnode, 再用通用 Glushkov 构造法编译为无 epsilon 的位置自动机
// mvsNFA. 执行 (见 mvs_exec.go) 时按 Go regexp 完全相同的方式解码 rune (utf8.DecodeRune,
// 非法字节 -> RuneError), 配合"字母表压缩 (rune->符号)"保持 active & reach[sym] 的位并行形态.
//
// 为什么 rune 级而非字节级: Go regexp 的语义是逐 rune 的 (非法 UTF-8 解码为 RuneError 单字节).
// 字节级自动机在非法 UTF-8 (真实流量常见) 上对 . / 负类 与 oracle 分歧; rune 级解码与 Go 逐位
// 一致, 同时让 . / 负类 / 字符类各用"一个位置"承载, 既正确又紧凑 (字符类不再展开为字节序列交替).
//
// 设计取舍: 只接受 RE2 可表达且无中缀零宽断言的正则; 词边界 \b / 行锚 (?m) / 反向引用等交
// verifier 兜底. 顶层首 ^/\A 作 anchoredStart, 尾 $/\z 作 requireEnd, 覆盖绝大多数真实规则.
//
// 关键词: mvscan, Glushkov, position automaton, alphabet compression, rune-level, 锚点

// mvsMaxPos 限制单条 pattern 展开后的位置数 (避免 {m,n} 大区间状态爆炸); 超限退化为兜底.
const mvsMaxPos = 4096

type runeRange struct{ lo, hi rune }

type bkind uint8

const (
	bClass  bkind = iota // 叶子: 接受一个 rune 集合 (若干 runeRange)
	bEmpty               // 空 (可空)
	bConcat              // 顺序连接
	bAlt                 // 交替
	bStar                // x*
	bPlus                // x+
	bQuest               // x?
	bAssert              // 零宽断言 (^ $ \A \z \b \B / (?m)^$): 不消费输入, acond 记录条件位
)

// bnode 是 rune 级 AST 节点. 复合节点用 sub; bClass 用 cls; bAssert 用 acond.
type bnode struct {
	kind  bkind
	cls   []runeRange
	sub   []*bnode
	acond uint8 // bAssert: 边界条件位 (condBeginText 等, 见 mvs_assert.go)
}

// synToRune 把 RE2 语法树转换为 rune 级 bnode. 第二返回值 false 表示遇到本核无法处理的构造
// (中缀锚点 / 词边界 / 行锚 / 反向引用 / 空匹配类 等), 应交 verifier 兜底.
func synToRune(re *syntax.Regexp) (*bnode, bool) {
	switch re.Op {
	case syntax.OpEmptyMatch:
		return &bnode{kind: bEmpty}, true

	case syntax.OpLiteral:
		fold := re.Flags&syntax.FoldCase != 0
		var parts []*bnode
		for _, r := range re.Rune {
			parts = append(parts, &bnode{kind: bClass, cls: runeClass(r, fold)})
		}
		return concatNode(parts), true

	case syntax.OpCharClass:
		cls := pairsToRanges(re.Rune)
		if len(cls) == 0 {
			return nil, false // 空类 (永不匹配)
		}
		return &bnode{kind: bClass, cls: cls}, true

	case syntax.OpAnyCharNotNL:
		return &bnode{kind: bClass, cls: []runeRange{{0, '\n' - 1}, {'\n' + 1, unicode.MaxRune}}}, true

	case syntax.OpAnyChar:
		return &bnode{kind: bClass, cls: []runeRange{{0, unicode.MaxRune}}}, true

	case syntax.OpCapture:
		if len(re.Sub) != 1 {
			return nil, false
		}
		return synToRune(re.Sub[0])

	case syntax.OpConcat:
		var parts []*bnode
		for _, s := range re.Sub {
			n, ok := synToRune(s)
			if !ok {
				return nil, false
			}
			parts = append(parts, n)
		}
		return concatNode(parts), true

	case syntax.OpAlternate:
		var parts []*bnode
		for _, s := range re.Sub {
			n, ok := synToRune(s)
			if !ok {
				return nil, false
			}
			parts = append(parts, n)
		}
		if len(parts) == 1 {
			return parts[0], true
		}
		return &bnode{kind: bAlt, sub: parts}, true

	case syntax.OpStar:
		if len(re.Sub) != 1 {
			return nil, false
		}
		n, ok := synToRune(re.Sub[0])
		if !ok {
			return nil, false
		}
		return &bnode{kind: bStar, sub: []*bnode{n}}, true

	case syntax.OpPlus:
		if len(re.Sub) != 1 {
			return nil, false
		}
		n, ok := synToRune(re.Sub[0])
		if !ok {
			return nil, false
		}
		return &bnode{kind: bPlus, sub: []*bnode{n}}, true

	case syntax.OpQuest:
		if len(re.Sub) != 1 {
			return nil, false
		}
		n, ok := synToRune(re.Sub[0])
		if !ok {
			return nil, false
		}
		return &bnode{kind: bQuest, sub: []*bnode{n}}, true

	case syntax.OpRepeat:
		return repeatNode(re)

	default:
		// OpBeginText/OpEndText (中缀位置) / OpBeginLine/OpEndLine / OpWordBoundary /
		// OpNoWordBoundary / OpNoMatch 等: 本核不处理, 交兜底.
		return nil, false
	}
}

// repeatNode 展开 x{min,max} (min 必经 + 余下 quest, 或无上界用 star). 多次引用同一子 bnode 安全:
// Glushkov 在每次 visit 时为叶子分配独立位置.
func repeatNode(re *syntax.Regexp) (*bnode, bool) {
	if len(re.Sub) != 1 {
		return nil, false
	}
	sub, ok := synToRune(re.Sub[0])
	if !ok {
		return nil, false
	}
	cnt := countPos(sub)
	if cnt == 0 {
		cnt = 1
	}
	min, max := re.Min, re.Max
	var total int
	if max < 0 {
		total = (min + 1) * cnt
	} else {
		total = max * cnt
	}
	if total > mvsMaxPos {
		return nil, false
	}
	var parts []*bnode
	for i := 0; i < min; i++ {
		parts = append(parts, sub)
	}
	if max < 0 {
		parts = append(parts, &bnode{kind: bStar, sub: []*bnode{sub}})
	} else {
		for i := 0; i < max-min; i++ {
			parts = append(parts, &bnode{kind: bQuest, sub: []*bnode{sub}})
		}
	}
	if len(parts) == 0 {
		return &bnode{kind: bEmpty}, true
	}
	return concatNode(parts), true
}

func concatNode(parts []*bnode) *bnode {
	if len(parts) == 0 {
		return &bnode{kind: bEmpty}
	}
	if len(parts) == 1 {
		return parts[0]
	}
	return &bnode{kind: bConcat, sub: parts}
}

// runeClass 构造匹配单个码点 (可选大小写折叠) 的 rune 集合.
func runeClass(r rune, fold bool) []runeRange {
	if !fold {
		return []runeRange{{r, r}}
	}
	var out []runeRange
	for _, x := range foldRunes(r) {
		out = append(out, runeRange{x, x})
	}
	return out
}

// foldRunes 返回 r 的简单大小写折叠等价类.
func foldRunes(r rune) []rune {
	out := []rune{r}
	for f := unicode.SimpleFold(r); f != r; f = unicode.SimpleFold(f) {
		out = append(out, f)
	}
	return out
}

// pairsToRanges 把 syntax 字符类的 []rune 对转为 runeRange 列表.
func pairsToRanges(runes []rune) []runeRange {
	out := make([]runeRange, 0, len(runes)/2)
	for i := 0; i+1 < len(runes); i += 2 {
		out = append(out, runeRange{runes[i], runes[i+1]})
	}
	return out
}

func countPos(n *bnode) int {
	if n == nil {
		return 0
	}
	if n.kind == bClass {
		return 1
	}
	c := 0
	for _, s := range n.sub {
		c += countPos(s)
	}
	return c
}

// ---- Glushkov 构造 ----

// mvsNFA 是一条 pattern 的 rune 级位置自动机 (位并行可执行).
type mvsNFA struct {
	npos  int
	nword int

	first   []uint64
	lastAny []uint64   // 可作命中终点的位置集 (无尾锚)
	lastEnd []uint64   // 必须在输入末尾才命中的终点位置集 ($ / \z)
	follow  [][]uint64 // 每位置的后继集

	// 字母表压缩: rune -> 符号 id -> reach[符号] 位置集. 符号是按所有位置类边界切出的码点区间.
	nsym     int
	asciiSym [128]int32 // r<128 的符号 id (快表)
	cuts     []rune     // 升序切点 (含 0 与 MaxRune+1 哨兵); 符号 i 覆盖 [cuts[i],cuts[i+1])
	reach    [][]uint64 // reach[符号] = 接受该符号的位置集 (nsym 行)

	anchoredStart bool
	requireEnd    bool

	// nword==1 (位置数 <=64, 绝大多数真实 pattern) 的零分配快路径预摊平表.
	single   bool
	first1   uint64
	lastAny1 uint64
	lastEnd1 uint64
	follow1  []uint64 // follow1[p] = follow[p][0]
	reach1   []uint64 // reach1[符号] = reach[符号][0]

	// ---- 零宽断言扩展 (见 mvs_assert.go) ----
	// hasAssert 为真表示本 NFA 含零宽断言 (\b \B / 行锚 (?m)^$ / 中缀 ^$\A\z), 走 existsInAssert
	// 执行器 (带边界条件门控的位并行递推), 不走 lean existsIn; 且不进 C 内核 (Go 侧执行).
	// 此时 anchoredStart/requireEnd/single 不使用; 锚点全部编码为 condFirst/condAccept 的 guard.
	hasAssert  bool
	condFirst  []guardedBits   // 起点位置的条件注入: guard 在"消费当前 rune 前的边界"成立才注入 bits
	condFollow [][]guardedBits // 每位置的条件后继: 同一边界成立才把 bits 并入候选 (与 follow[p] 互补)
	condAccept []guardedBits   // 接受位置的条件: guard 在"消费当前 rune 后的边界"成立且 active 命中即接受
}

// guardedBits 是"条件位集": 当 guard (若干边界条件的合取) 在某边界成立时, bits 对应的位置生效.
type guardedBits struct {
	g    guard
	bits []uint64
}

type glushkovBuilder struct {
	posClass [][]runeRange
	follow   [][]int
	over     bool
}

func (g *glushkovBuilder) newPos(cls []runeRange) int {
	if len(g.posClass) >= mvsMaxPos {
		g.over = true
		return 0
	}
	p := len(g.posClass)
	g.posClass = append(g.posClass, cls)
	g.follow = append(g.follow, nil)
	return p
}

func (g *glushkovBuilder) addFollow(from int, to []int) {
	if len(to) == 0 {
		return
	}
	g.follow[from] = append(g.follow[from], to...)
}

// visit 返回 (nullable, first 位置集, tail 位置集).
func (g *glushkovBuilder) visit(n *bnode) (bool, []int, []int) {
	if g.over {
		return true, nil, nil
	}
	switch n.kind {
	case bClass:
		p := g.newPos(n.cls)
		return false, []int{p}, []int{p}

	case bEmpty:
		return true, nil, nil

	case bConcat:
		var first, tail []int
		nullablePrefix := true
		allNullable := true
		for _, s := range n.sub {
			sn, sf, sl := g.visit(s)
			for _, p := range tail {
				g.addFollow(p, sf)
			}
			if nullablePrefix {
				first = append(first, sf...)
				if !sn {
					nullablePrefix = false
				}
			}
			if sn {
				tail = append(tail, sl...)
			} else {
				tail = sl
			}
			allNullable = allNullable && sn
		}
		return allNullable, first, tail

	case bAlt:
		var first, last []int
		nullable := false
		for _, s := range n.sub {
			sn, sf, sl := g.visit(s)
			first = append(first, sf...)
			last = append(last, sl...)
			nullable = nullable || sn
		}
		return nullable, first, last

	case bStar:
		_, sf, sl := g.visit(n.sub[0])
		for _, p := range sl {
			g.addFollow(p, sf)
		}
		return true, sf, sl

	case bPlus:
		sn, sf, sl := g.visit(n.sub[0])
		for _, p := range sl {
			g.addFollow(p, sf)
		}
		return sn, sf, sl

	case bQuest:
		_, sf, sl := g.visit(n.sub[0])
		return true, sf, sl
	}
	return true, nil, nil
}

// compileMVSNFA 把一条 RE2 语法树编译成 mvsNFA. ok=false 表示应交 verifier 兜底.
func compileMVSNFA(re *syntax.Regexp) (*mvsNFA, bool) {
	anchoredStart, requireEnd, core := stripEndAnchors(re)
	root, ok := synToRune(core)
	if !ok {
		return nil, false
	}
	return glushkovNFA(root, anchoredStart, requireEnd)
}

// glushkovNFA 由 rune 级 bnode 根 (经 synToRune/anchor 剥离后) 与文本锚标志构造位置自动机.
// 正向 (compileMVSNFA) 与反向 (compileReverseExprToNFA, 反转 bnode 后调用) 共用此构造器, 保证
// 二者位并行结构、字母表压缩口径完全一致 —— 反向 NFA 即"反转语言的同构 Glushkov 自动机",
// 配合反向扫描 (DecodeLastRune 自尾向头) 与正向 existsIn 对同一 data 判定同真伪 (见差分护栏).
func glushkovNFA(root *bnode, anchoredStart, requireEnd bool) (*mvsNFA, bool) {
	g := &glushkovBuilder{}
	rootNullable, first, last := g.visit(root)
	if g.over {
		return nil, false
	}
	// 可空根 (能匹配空串) 与空 first (纯锚点等) 交兜底, 避免存在性边界歧义.
	if rootNullable || len(first) == 0 {
		return nil, false
	}

	npos := len(g.posClass)
	nword := (npos + 63) / 64
	if nword == 0 {
		return nil, false
	}

	nfa := &mvsNFA{
		npos:          npos,
		nword:         nword,
		first:         bsFromPositions(first, nword),
		follow:        make([][]uint64, npos),
		anchoredStart: anchoredStart,
		requireEnd:    requireEnd,
	}
	lastBits := bsFromPositions(last, nword)
	if requireEnd {
		nfa.lastEnd = lastBits
		nfa.lastAny = bsNew(nword)
	} else {
		nfa.lastAny = lastBits
		nfa.lastEnd = bsNew(nword)
	}
	for p := 0; p < npos; p++ {
		nfa.follow[p] = bsFromPositions(dedupInts(g.follow[p]), nword)
	}

	nfa.buildAlphabet(g.posClass)

	nfa.initScalar()
	return nfa, true
}

// initScalar 在 nword==1 (位置数<=64) 时填充标量快路径字段, 供 existsIn1 / existsInAnchored1 /
// existsInAssertShared1 / existsInAssertAnchored1 全程寄存器位运算且零分配地执行 (须在 buildAlphabet
// 建好 reach 之后调用)。lean 与断言 NFA 共用: 断言额外的 condFirst/condFollow/condAccept 在标量执行器里
// 直接取 gb.bits[0] (nword==1 时其长度即 1), 无需此处单独镜像。nword>1 时不置 single, 走多字通用路径。
func (nfa *mvsNFA) initScalar() {
	if nfa.nword != 1 {
		return
	}
	nfa.single = true
	nfa.first1 = nfa.first[0]
	nfa.lastAny1 = nfa.lastAny[0]
	nfa.lastEnd1 = nfa.lastEnd[0]
	nfa.follow1 = make([]uint64, nfa.npos)
	for p := 0; p < nfa.npos; p++ {
		nfa.follow1[p] = nfa.follow[p][0]
	}
	nfa.reach1 = make([]uint64, nfa.nsym)
	for s := 0; s < nfa.nsym; s++ {
		nfa.reach1[s] = nfa.reach[s][0]
	}
}

// buildAlphabet 用所有位置类的区间边界切分码点空间为若干"符号", 建立 rune->符号 映射与
// reach[符号] 位置集. 这样运行期对每个 rune 仅一次查表 + 一次 active & reach[sym] 位并行推进.
func (nfa *mvsNFA) buildAlphabet(posClass [][]runeRange) {
	// 收集切点.
	cutSet := map[rune]struct{}{0: {}, unicode.MaxRune + 1: {}}
	for _, cls := range posClass {
		for _, r := range cls {
			cutSet[r.lo] = struct{}{}
			cutSet[r.hi+1] = struct{}{}
		}
	}
	cuts := make([]rune, 0, len(cutSet))
	for c := range cutSet {
		if c >= 0 && c <= unicode.MaxRune+1 {
			cuts = append(cuts, c)
		}
	}
	sort.Slice(cuts, func(i, j int) bool { return cuts[i] < cuts[j] })
	nfa.cuts = cuts
	nfa.nsym = len(cuts) - 1 // 符号 i: [cuts[i], cuts[i+1])

	nfa.reach = make([][]uint64, nfa.nsym)
	for s := 0; s < nfa.nsym; s++ {
		nfa.reach[s] = bsNew(nfa.nword)
	}
	for p := 0; p < len(posClass); p++ {
		for _, r := range posClass[p] {
			s0 := symIndex(cuts, r.lo)
			s1 := symIndex(cuts, r.hi)
			for s := s0; s <= s1; s++ {
				bsSet(nfa.reach[s], p)
			}
		}
	}
	for r := rune(0); r < 128; r++ {
		nfa.asciiSym[r] = int32(symIndex(cuts, r))
	}
}

// symIndex 返回 r 所在符号区间下标 (cuts[i] <= r < cuts[i+1]).
func symIndex(cuts []rune, r rune) int {
	// 最后一个 cuts[i] <= r 的 i.
	i := sort.Search(len(cuts), func(k int) bool { return cuts[k] > r }) - 1
	if i < 0 {
		i = 0
	}
	if i >= len(cuts)-1 {
		i = len(cuts) - 2
	}
	return i
}

// symbolOf 把一个 rune 映射到本 NFA 的符号 id.
func (nfa *mvsNFA) symbolOf(r rune) int {
	if r >= 0 && r < 128 {
		return int(nfa.asciiSym[r])
	}
	if r > unicode.MaxRune {
		r = utf8.RuneError
	}
	return symIndex(nfa.cuts, r)
}

// stripEndAnchors 剥离顶层 concat 的首 ^/\A 与尾 $/\z, 返回 (anchoredStart, requireEnd, 核心树).
// 仅处理顶层两端的文本锚 (覆盖 ^foo / foo$ / ^foo$); 其它位置的锚点保留, 由 synToRune 探测后兜底.
func stripEndAnchors(re *syntax.Regexp) (bool, bool, *syntax.Regexp) {
	if re.Op != syntax.OpConcat {
		return false, false, re
	}
	subs := re.Sub
	anchoredStart := false
	requireEnd := false
	if len(subs) > 0 && subs[0].Op == syntax.OpBeginText {
		anchoredStart = true
		subs = subs[1:]
	}
	if len(subs) > 0 && subs[len(subs)-1].Op == syntax.OpEndText {
		requireEnd = true
		subs = subs[:len(subs)-1]
	}
	if !anchoredStart && !requireEnd {
		return false, false, re
	}
	if len(subs) == 1 {
		return anchoredStart, requireEnd, subs[0]
	}
	return anchoredStart, requireEnd, &syntax.Regexp{Op: syntax.OpConcat, Sub: subs, Flags: re.Flags}
}

// ---- bitset 工具 (每字 64 位) ----

func bsNew(nword int) []uint64 { return make([]uint64, nword) }

func bsSet(bs []uint64, i int) { bs[i>>6] |= 1 << uint(i&63) }

func bsFromPositions(pos []int, nword int) []uint64 {
	bs := make([]uint64, nword)
	for _, p := range pos {
		bs[p>>6] |= 1 << uint(p&63)
	}
	return bs
}

func dedupInts(in []int) []int {
	if len(in) <= 1 {
		return in
	}
	seen := make(map[int]struct{}, len(in))
	out := in[:0]
	for _, v := range in {
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}
	return out
}
