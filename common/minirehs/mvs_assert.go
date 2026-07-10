package minirehs

import (
	"math/bits"
	"regexp/syntax"
	"unicode"
	"unicode/utf8"
)

// 本文件把 mvscan 的位置自动机扩展到"零宽断言": 词边界 \b \B、行锚 (?m)^ (?m)$、
// 中缀文本锚 ^ \A $ \z. 这些在 RE2 里是 EmptyOp (不消费输入, 仅在满足"边界条件"时通过).
//
// 设计: 沿用 Glushkov 位置自动机, 但给 first/follow/accept 三类关系附加"边界条件门 (guard)".
// 一个 guard 是若干边界条件的合取 (AND); 多条路径产生多个 guard 即析取 (OR). 运行期在每个
// rune 边界算出"该边界成立的条件集 B", 仅当某 guard 完全被 B 覆盖时对应的注入/后继/接受才生效.
//
// 正确性边界: 本扩展只作为"快速存在性门控". 命中后定位仍交 verifier (re2Verifier, 精确字节偏移),
// 与既有 fallback 行为一致. 因此 guard 构造若有疏漏, 最坏表现是 existsInAssert 的假阴, 会被
// ID 集合差分 (TestMVSExistenceVsOracleMITM / TestConsistency*) 抓到; 假阳无害 (verifier 兜底).
//
// 关键词: zero-width assertion, word boundary, \b, \B, multiline, ^, $, Glushkov, guard, 边界条件

// 边界条件位 (对齐 RE2/Go regexp/syntax 的 6 个 EmptyOp 语义).
const (
	condBeginText      uint8 = 1 << iota // \A 或非多行 ^: 文本开头
	condEndText                          // \z 或非多行 $: 文本结尾
	condBeginLine                        // 多行 ^: 文本开头或前一字符是 \n
	condEndLine                          // 多行 $: 文本结尾或后一字符是 \n
	condWordBoundary                     // \b: 前后字符的"是否单词字符"不同
	condNoWordBoundary                   // \B: 前后字符的"是否单词字符"相同
)

// guard 是若干条件的合取 (置位即"要求成立"); 0 表示恒真 (无条件).
type guard uint8

// guardSet 是 guard 的析取 (任一成立即整体成立); nil/空 表示恒假 (永不); 含 0 表示恒真.
type guardSet []guard

// isWordRune 判定单词字符, 对齐 syntax.IsWordChar (ASCII [0-9A-Za-z_]); r<0 (无字符) 为 false.
func isWordRune(r rune) bool {
	return r == '_' ||
		('0' <= r && r <= '9') ||
		('a' <= r && r <= 'z') ||
		('A' <= r && r <= 'Z')
}

// boundaryConds 计算"前一 rune before 与后一 rune after 之间的边界"成立的条件集. before/after 为 -1 表示无 (文本端).
func boundaryConds(before, after rune) uint8 {
	var b uint8
	if before < 0 {
		b |= condBeginText | condBeginLine
	} else if before == '\n' {
		b |= condBeginLine
	}
	if after < 0 {
		b |= condEndText | condEndLine
	} else if after == '\n' {
		b |= condEndLine
	}
	if isWordRune(before) != isWordRune(after) {
		b |= condWordBoundary
	} else {
		b |= condNoWordBoundary
	}
	return b
}

// guardHolds 报告 guard g 是否在边界条件集 B 下成立 (g 要求的位全部出现在 B 中).
func guardHolds(g guard, B uint8) bool { return uint8(g)&B == uint8(g) }

// ---- guardSet 代数 ----

func gsTrue() guardSet  { return guardSet{0} }
func gsFalse() guardSet { return nil }

func gsHasTrue(gs guardSet) bool {
	for _, g := range gs {
		if g == 0 {
			return true
		}
	}
	return false
}

// gsNorm 去掉被支配项: 若存在 h 是 g 的真子集 (要求更少、更易成立), 则 g 冗余, 删除. 并去重.
func gsNorm(in guardSet) guardSet {
	var out guardSet
	for _, g := range in {
		dominated := false
		for _, h := range in {
			if h != g && uint8(h)&uint8(g) == uint8(h) { // h ⊂ g (真子集)
				dominated = true
				break
			}
		}
		if dominated {
			continue
		}
		dup := false
		for _, o := range out {
			if o == g {
				dup = true
				break
			}
		}
		if !dup {
			out = append(out, g)
		}
	}
	return out
}

// gsOr 析取合并.
func gsOr(a, b guardSet) guardSet {
	if len(a) == 0 {
		return gsNorm(b)
	}
	if len(b) == 0 {
		return gsNorm(a)
	}
	merged := make(guardSet, 0, len(a)+len(b))
	merged = append(merged, a...)
	merged = append(merged, b...)
	return gsNorm(merged)
}

// gsAnd 合取: 笛卡尔积取每对的并 (要求两边条件都满足).
func gsAnd(a, b guardSet) guardSet {
	if len(a) == 0 || len(b) == 0 {
		return gsFalse()
	}
	out := make(guardSet, 0, len(a)*len(b))
	for _, x := range a {
		for _, y := range b {
			out = append(out, guard(uint8(x)|uint8(y)))
		}
	}
	return gsNorm(out)
}

// posGuard 是"位置 + 进入该位置所需的边界 guard".
type posGuard struct {
	pos int
	g   guard
}

// gsAndPos 把一组带 guard 的位置整体再合取一个前/后缀可空 guardSet (跨可空子表达式累积条件).
func gsAndPos(prefix guardSet, ps []posGuard) []posGuard {
	if len(ps) == 0 || len(prefix) == 0 {
		return nil
	}
	out := make([]posGuard, 0, len(ps)*len(prefix))
	for _, p := range ps {
		for _, pg := range prefix {
			out = append(out, posGuard{pos: p.pos, g: guard(uint8(p.g) | uint8(pg))})
		}
	}
	return out
}

// condOfAssertOp 把 syntax 的零宽断言 op 映射到边界条件位; ok=false 表示非断言 op.
func condOfAssertOp(op syntax.Op) (uint8, bool) {
	switch op {
	case syntax.OpBeginText:
		return condBeginText, true
	case syntax.OpEndText:
		return condEndText, true
	case syntax.OpBeginLine:
		return condBeginLine, true
	case syntax.OpEndLine:
		return condEndLine, true
	case syntax.OpWordBoundary:
		return condWordBoundary, true
	case syntax.OpNoWordBoundary:
		return condNoWordBoundary, true
	}
	return 0, false
}

// synToRuneA 与 synToRune 等价, 但把零宽断言转成 bAssert 节点 (而非交兜底). 用于断言扩展路径.
func synToRuneA(re *syntax.Regexp) (*bnode, bool) {
	if c, ok := condOfAssertOp(re.Op); ok {
		return &bnode{kind: bAssert, acond: c}, true
	}
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
			return nil, false
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
		return synToRuneA(re.Sub[0])

	case syntax.OpConcat:
		var parts []*bnode
		for _, s := range re.Sub {
			n, ok := synToRuneA(s)
			if !ok {
				return nil, false
			}
			parts = append(parts, n)
		}
		return concatNode(parts), true

	case syntax.OpAlternate:
		var parts []*bnode
		for _, s := range re.Sub {
			n, ok := synToRuneA(s)
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
		n, ok := synToRuneA(re.Sub[0])
		if !ok {
			return nil, false
		}
		return &bnode{kind: bStar, sub: []*bnode{n}}, true

	case syntax.OpPlus:
		if len(re.Sub) != 1 {
			return nil, false
		}
		n, ok := synToRuneA(re.Sub[0])
		if !ok {
			return nil, false
		}
		return &bnode{kind: bPlus, sub: []*bnode{n}}, true

	case syntax.OpQuest:
		if len(re.Sub) != 1 {
			return nil, false
		}
		n, ok := synToRuneA(re.Sub[0])
		if !ok {
			return nil, false
		}
		return &bnode{kind: bQuest, sub: []*bnode{n}}, true

	case syntax.OpRepeat:
		if len(re.Sub) != 1 {
			return nil, false
		}
		sub, ok := synToRuneA(re.Sub[0])
		if !ok {
			return nil, false
		}
		return expandRepeatA(sub, re.Min, re.Max)

	default:
		return nil, false
	}
}

// expandRepeatA 展开 x{min,max} (沿用 repeatNode 策略), 子节点可含断言.
func expandRepeatA(sub *bnode, min, max int) (*bnode, bool) {
	cnt := countPos(sub)
	if cnt == 0 {
		cnt = 1
	}
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

// ---- 断言版 Glushkov 构造 ----

type assertBuilder struct {
	posClass [][]runeRange
	followG  []map[int]guardSet // followG[from][to] = 进入该后继所需 guardSet
	over     bool
}

func (b *assertBuilder) newPos(cls []runeRange) int {
	if len(b.posClass) >= mvsMaxPos {
		b.over = true
		return 0
	}
	p := len(b.posClass)
	b.posClass = append(b.posClass, cls)
	b.followG = append(b.followG, nil)
	return p
}

func (b *assertBuilder) addFollow(from, to int, g guard) {
	if b.followG[from] == nil {
		b.followG[from] = make(map[int]guardSet)
	}
	b.followG[from][to] = gsOr(b.followG[from][to], guardSet{g})
}

// visit 返回 (nullable guardSet, first 位置集, last 位置集), 并把内部 follow 边写入 b.followG.
func (b *assertBuilder) visit(n *bnode) (guardSet, []posGuard, []posGuard) {
	if b.over {
		return gsFalse(), nil, nil
	}
	switch n.kind {
	case bClass:
		p := b.newPos(n.cls)
		return gsFalse(), []posGuard{{pos: p, g: 0}}, []posGuard{{pos: p, g: 0}}

	case bEmpty:
		return gsTrue(), nil, nil

	case bAssert:
		return guardSet{guard(n.acond)}, nil, nil

	case bConcat:
		k := len(n.sub)
		ns := make([]guardSet, k)
		fs := make([][]posGuard, k)
		ls := make([][]posGuard, k)
		for i, s := range n.sub {
			ns[i], fs[i], ls[i] = b.visit(s)
		}
		// nullable = AND 所有子.
		null := gsTrue()
		for i := 0; i < k; i++ {
			null = gsAnd(null, ns[i])
		}
		// first: 前缀可空累积.
		var first []posGuard
		prefix := gsTrue()
		for i := 0; i < k; i++ {
			first = append(first, gsAndPos(prefix, fs[i])...)
			prefix = gsAnd(prefix, ns[i])
			if len(prefix) == 0 {
				break
			}
		}
		// last: 后缀可空累积.
		var last []posGuard
		suffix := gsTrue()
		for i := k - 1; i >= 0; i-- {
			last = append(last, gsAndPos(suffix, ls[i])...)
			suffix = gsAnd(suffix, ns[i])
			if len(suffix) == 0 {
				break
			}
		}
		// follow: last_i -> first_j (中间子全可空时跨越, guard 累积中间可空条件).
		for i := 0; i < k; i++ {
			between := gsTrue()
			for j := i + 1; j < k; j++ {
				for _, lp := range ls[i] {
					for _, fp := range fs[j] {
						for _, bg := range between {
							b.addFollow(lp.pos, fp.pos, guard(uint8(lp.g)|uint8(fp.g)|uint8(bg)))
						}
					}
				}
				between = gsAnd(between, ns[j])
				if len(between) == 0 {
					break
				}
			}
		}
		return null, first, last

	case bAlt:
		null := gsFalse()
		var first, last []posGuard
		for _, s := range n.sub {
			sn, sf, sl := b.visit(s)
			null = gsOr(null, sn)
			first = append(first, sf...)
			last = append(last, sl...)
		}
		return null, first, last

	case bStar:
		sn, sf, sl := b.visit(n.sub[0])
		if len(sn) > 0 { // 可空子的星号会产生空循环歧义, 交兜底.
			b.over = true
		}
		b.addLoop(sl, sf)
		return gsTrue(), sf, sl

	case bPlus:
		sn, sf, sl := b.visit(n.sub[0])
		if len(sn) > 0 {
			b.over = true
		}
		b.addLoop(sl, sf)
		return sn, sf, sl

	case bQuest:
		_, sf, sl := b.visit(n.sub[0])
		return gsTrue(), sf, sl
	}
	return gsTrue(), nil, nil
}

// addLoop 为 star/plus 添加 last -> first 的回边 (本次迭代尾接下次迭代头).
func (b *assertBuilder) addLoop(last, first []posGuard) {
	for _, lp := range last {
		for _, fp := range first {
			b.addFollow(lp.pos, fp.pos, guard(uint8(lp.g)|uint8(fp.g)))
		}
	}
}

// compileMVSNFAAssert 编译含零宽断言的 RE2 树为带 guard 的 mvsNFA. ok=false 交兜底.
func compileMVSNFAAssert(re *syntax.Regexp) (*mvsNFA, bool) {
	root, ok := synToRuneA(re)
	if !ok {
		return nil, false
	}
	b := &assertBuilder{}
	null, first, last := b.visit(root)
	if b.over {
		return nil, false
	}
	if len(null) > 0 { // 可空根 (能匹配空串): 存在性边界歧义, 交兜底.
		return nil, false
	}
	if len(first) == 0 {
		return nil, false
	}
	npos := len(b.posClass)
	if npos == 0 {
		return nil, false
	}
	nword := (npos + 63) / 64

	nfa := &mvsNFA{
		npos:      npos,
		nword:     nword,
		hasAssert: true,
		follow:    make([][]uint64, npos),
	}

	// first -> 无条件 first 位集 + condFirst.
	nfa.first, nfa.condFirst = buildCondBits(first, npos, nword)
	// last -> 无条件 lastAny 位集 + condAccept.
	nfa.lastAny, nfa.condAccept = buildCondBits(last, npos, nword)
	nfa.lastEnd = bsNew(nword)

	// follow: 无条件部分入 follow[p], 有条件部分入 condFollow[p].
	nfa.condFollow = make([][]guardedBits, npos)
	for p := 0; p < npos; p++ {
		nfa.follow[p] = bsNew(nword)
		if b.followG[p] == nil {
			continue
		}
		byGuard := map[guard][]int{}
		for to, gs := range b.followG[p] {
			gs = gsNorm(gs)
			if gsHasTrue(gs) {
				bsSet(nfa.follow[p], to)
				continue
			}
			for _, g := range gs {
				byGuard[g] = append(byGuard[g], to)
			}
		}
		for g, tos := range byGuard {
			nfa.condFollow[p] = append(nfa.condFollow[p], guardedBits{g: g, bits: bsFromPositions(tos, nword)})
		}
	}

	nfa.buildAlphabet(b.posClass)
	// nword==1 的断言 NFA 也启用标量快路径 (existsInAssertShared1 / existsInAssertAnchored1):
	// 此前 compileMVSNFAAssert 漏置 single, 致所有断言 NFA (含 always-on 身份证/MAC 整段扫) 恒走
	// 多字 existsInAssertShared, 标量孪生形同虚设。dispatch 处均 hasAssert 优先, 故 single 只会路由到
	// 断言标量版, 不会误入 lean existsIn1。
	nfa.initScalar()
	return nfa, true
}

// buildCondBits 把一组带 guard 的位置拆成"无条件位集"与"按 guard 分组的条件位集".
// 若某位置存在 guard==0 的路径, 则该位置归入无条件位集 (恒真支配其它 guard).
func buildCondBits(ps []posGuard, npos, nword int) (uncond []uint64, cond []guardedBits) {
	perPos := make([]guardSet, npos)
	for _, p := range ps {
		perPos[p.pos] = gsOr(perPos[p.pos], guardSet{p.g})
	}
	uncond = bsNew(nword)
	byGuard := map[guard][]int{}
	for pos, gs := range perPos {
		if len(gs) == 0 {
			continue
		}
		if gsHasTrue(gs) {
			bsSet(uncond, pos)
			continue
		}
		for _, g := range gs {
			byGuard[g] = append(byGuard[g], pos)
		}
	}
	for g, positions := range byGuard {
		cond = append(cond, guardedBits{g: g, bits: bsFromPositions(positions, nword)})
	}
	return uncond, cond
}

// computeBoundaries 预计算 data 每个字节边界位置的零宽条件集 (与具体 pattern 无关, 仅取决于输入):
// bound[j] = 第 j 字节处 (前一 rune 结尾 / 后一 rune 开头之间) 的边界条件. 仅在 rune 起始偏移与
// computeBoundaries 预算每个 rune 起始处的边界条件集 (bound[i]) 与文本末尾 (bound[n]),
// 供多条断言 NFA 共享 (existsInAssertShared 只读这些位置). 复用入参 buf 底层数组。
// 多条断言 NFA 共享同一份 bound, 省去 boundaryConds / isWordRune 的逐 pattern 重复计算。
//
// ASCII 快路径: 真实流量绝大多数为 ASCII, 对 c<0x80 的字节 rune==rune(c)、size==1 (与
// utf8.DecodeRune 逐位一致), 直接用字节判定 isWordRune/\n, 跳过 DecodeRune 的分支与 rune 宽化开销。
// 遇到非 ASCII 字节才回退到逐 rune 解码路径。
func computeBoundaries(data []byte, buf []uint8) []uint8 {
	n := len(data)
	if cap(buf) < n+1 {
		buf = make([]uint8, n+1)
	} else {
		buf = buf[:n+1]
	}
	// wordPrev/prevIsNewline 追踪"前一 rune 是否单词字符 / 是否 \n", prevIsTextStart 追踪是否文本始.
	// 用字节级状态避免 rune 比较 (ASCII 快路径内零 rune 操作).
	prevWord := false       // isWordRune(prev); prev==-1 时 isWordRune(-1)=false
	prevIsNewline := false  // prev == '\n'
	prevIsStart := true     // prev < 0 (文本始)
	i := 0
	for i < n {
		c := data[i]
		var b uint8
		// before 侧边界.
		if prevIsStart {
			b |= condBeginText | condBeginLine
		} else if prevIsNewline {
			b |= condBeginLine
		}
		if c < utf8.RuneSelf {
			// ASCII 快路径: rune == rune(c), size == 1.
			curWord := isWordByte(c)
			curIsNewline := c == '\n'
			// after 侧边界 (after = rune(c), 非 -1).
			if curIsNewline {
				b |= condEndLine
			}
			if prevWord != curWord {
				b |= condWordBoundary
			} else {
				b |= condNoWordBoundary
			}
			buf[i] = b
			prevWord = curWord
			prevIsNewline = curIsNewline
			prevIsStart = false
			i++
			continue
		}
		// 非 ASCII: 回退逐 rune 解码.
		r, size := utf8.DecodeRune(data[i:])
		curWord := isWordRune(r)
		curIsNewline := r == '\n'
		if curIsNewline {
			b |= condEndLine
		}
		if prevWord != curWord {
			b |= condWordBoundary
		} else {
			b |= condNoWordBoundary
		}
		buf[i] = b
		prevWord = curWord
		prevIsNewline = curIsNewline
		prevIsStart = false
		i += size
	}
	// 末尾: after = -1 (文本末).
	var b uint8
	if prevIsStart {
		b |= condBeginText | condBeginLine
	} else if prevIsNewline {
		b |= condBeginLine
	}
	b |= condEndText | condEndLine
	if prevWord { // isWordRune(prev) != isWordRune(-1)=false
		b |= condWordBoundary
	} else {
		b |= condNoWordBoundary
	}
	buf[n] = b
	return buf
}

// isWordByte 是 isWordRune 的 ASCII 字节版 (c < 0x80 时与 isWordRune(rune(c)) 同真伪).
func isWordByte(c byte) bool {
	return c == '_' ||
		'0' <= c && c <= '9' ||
		'a' <= c && c <= 'z' ||
		'A' <= c && c <= 'Z'
}

// existsInAssert 是带边界条件门控的位并行存在性判定 (与 existsIn 同语义, 额外处理零宽断言).
// 自带边界计算的便捷封装 (供测试 / 非热路径); 热路径用 existsInAssertShared 复用共享边界。
func (nfa *mvsNFA) existsInAssert(data []byte) bool {
	return nfa.existsInAssertShared(data, computeBoundaries(data, nil))
}

// existsInAssertShared 同 existsInAssert, 但边界条件由调用方预计算并跨多条断言 NFA 共享 (bound 长度
// 必须为 len(data)+1, 由 computeBoundaries 产出)。作为快速门控: 命中后定位仍交 verifier。
func (nfa *mvsNFA) existsInAssertShared(data []byte, bound []uint8) bool {
	nword := nfa.nword
	n := len(data)
	prev := make([]uint64, nword)
	cand := make([]uint64, nword)

	i := 0
	for i < n {
		c := data[i]
		var sym, ni int
		if c < utf8.RuneSelf {
			sym = int(nfa.asciiSym[c])
			ni = i + 1
		} else {
			r, size := utf8.DecodeRune(data[i:])
			sym = nfa.symbolOf(r)
			ni = i + size
		}
		bpre := bound[i]
		bpost := bound[ni]

		// 候选 = 无条件 first + 条件 first (按 bpre 门控) + 活跃位置的后继.
		copy(cand, nfa.first)
		for _, gb := range nfa.condFirst {
			if guardHolds(gb.g, bpre) {
				for w := 0; w < nword; w++ {
					cand[w] |= gb.bits[w]
				}
			}
		}
		for w := 0; w < nword; w++ {
			pw := prev[w]
			for pw != 0 {
				p := w*64 + bits.TrailingZeros64(pw)
				pw &= pw - 1
				fp := nfa.follow[p]
				for k := 0; k < nword; k++ {
					cand[k] |= fp[k]
				}
				for _, gb := range nfa.condFollow[p] {
					if guardHolds(gb.g, bpre) {
						for k := 0; k < nword; k++ {
							cand[k] |= gb.bits[k]
						}
					}
				}
			}
		}

		// active = cand & reach[sym]; 同时检查无条件接受.
		rc := nfa.reach[sym]
		for w := 0; w < nword; w++ {
			v := cand[w] & rc[w]
			prev[w] = v
			if v&nfa.lastAny[w] != 0 {
				return true
			}
		}
		// 条件接受 (按消费当前 rune 后的边界 bpost 门控).
		for _, gb := range nfa.condAccept {
			if guardHolds(gb.g, bpost) {
				for w := 0; w < nword; w++ {
					if prev[w]&gb.bits[w] != 0 {
						return true
					}
				}
			}
		}

		i = ni
	}
	return false
}

// existsInAssertShared1 是 existsInAssertShared 的 nword==1 标量快路径: 活跃集为单个 uint64,
// 全程寄存器位运算且零分配 (多字版每调用 make 两个 []uint64; 本版用本地标量, 显著省分配 + 位运算).
// 语义与 existsInAssertShared 完全一致, 仅用于 nfa.single 的断言 NFA. guard 位集取 gb.bits[0].
// 含 ASCII 快路径 (省 utf8.DecodeRune + symbolOf 调用) + LimEx 链/异常拆分 (链边左移批量推进).
// bound 为整段共享边界 (computeBoundaries).
func (nfa *mvsNFA) existsInAssertShared1(data []byte, bound []uint8) bool {
	first := nfa.first1
	lastAny := nfa.lastAny1
	reach := nfa.reach1
	chainTarget := nfa.chainTarget1
	excMask := nfa.excMask1
	excFollow := nfa.excFollow1
	n := len(data)

	var prev uint64
	i := 0
	for i < n {
		c := data[i]
		var sym, ni int
		if c < utf8.RuneSelf {
			sym = int(nfa.asciiSym[c])
			ni = i + 1
		} else {
			r, size := utf8.DecodeRune(data[i:])
			sym = nfa.symbolOf(r)
			ni = i + size
		}
		bpre := bound[i]

		// LimEx: 链边用左移批量推进; 异常边逐个 OR.
		shifted := (prev << 1) & chainTarget
		cand := first | shifted
		for _, gb := range nfa.condFirst {
			if guardHolds(gb.g, bpre) {
				cand |= gb.bits[0]
			}
		}
		// 异常边: 仅对活跃的异常位置展开无条件异常后继.
		if exc := prev & excMask; exc != 0 {
			for exc != 0 {
				p := bits.TrailingZeros64(exc)
				exc &= exc - 1
				cand |= excFollow[p]
			}
		}
		// condFollow: 条件后继对所有"有 condFollow 条目的活跃位置"展开.
		if cfm := prev & nfa.condFollowMask1; cfm != 0 {
			for cfm != 0 {
				p := bits.TrailingZeros64(cfm)
				cfm &= cfm - 1
				for _, gb := range nfa.condFollow[p] {
					if guardHolds(gb.g, bpre) {
						cand |= gb.bits[0]
					}
				}
			}
		}

		active := cand & reach[sym]
		if active&lastAny != 0 {
			return true
		}
		if len(nfa.condAccept) > 0 {
			bpost := bound[ni]
			for _, gb := range nfa.condAccept {
				if guardHolds(gb.g, bpost) && active&gb.bits[0] != 0 {
					return true
				}
			}
		}
		prev = active
		i = ni
	}
	return false
}
