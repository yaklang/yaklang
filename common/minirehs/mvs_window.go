package minirehs

import (
	"regexp/syntax"
	"strings"
)

// 本文件实现"存在性验证本地化"的编译期分析: 为每条 (RE2-exact, 无位置锚) 且有必需字面量的
// pattern 计算"命中字面量结尾"的两侧上下文界 (headMax/tailMax), 使运行期可把 per-pattern 的
// 整段 existsIn 收窄到字面量命中点邻域的子切片 (union 覆盖本报文全部命中点), 把 O(record) 降到
// O(window). 这是逼近 Hyperscan "literal -> 局部验证 (Rose)" 的关键一步 (我们仍是朴素 union 窗口,
// 非 Rose 子串图, 但已能消除"无界后缀类"之外的大量整段重扫).
//
// 正确性 (绝不假阴, 安全核心):
//   - 设必需字面量 s 在 data 命中, 结束于偏移 h.end. 任一包含该命中的匹配 M 满足
//     M.start >= h.end - headMax 且 M.end <= h.end + tailMax, 其中
//       headMax = max(在 AST 中能产生 s 的每个字面量结点 N) (M.start 到 N 结尾的最大字节宽),
//       tailMax = max(同上) (N 结尾到 M.end 的最大字节宽)。
//   - 任一匹配 M 必含某必需字面量 (必需字面量集语义), 其命中点对应窗口必覆盖 M; 故对该命中点
//     existsIn(窗口) 真伪与整段一致 (子串关系 => 无假阳; 窗口含 M => 该命中点无假阴; 其它命中点
//     由 union 一并覆盖)。
//   - 不可界一侧 (无界重复后仍有必需内容 / 含位置锚 ^$ / 超集门 NFA) 一律标 -1 (该侧不收窄), 退回
//     整段, 保安全。
//
// 关键词: existence localization, Rose-lite, window, headMax, tailMax, 局部验证, 字面量上下文界

// litWindow 是一条 pattern 的"命中点两侧界". -1 表示该侧无界 (运行期不收窄, 退回该侧到报文端)。
type litWindow struct {
	head int32 // 命中字面量结尾回看上限 (>= 该界即覆盖 match-start); -1 无界
	tail int32 // 命中字面量结尾前看上限 (>= 该界即覆盖 match-end); -1 无界
}

// litWindowCap 限制窗口界规模: 超过则视为无界 (收益有限且窗口接近整段)。
const litWindowCap = 1 << 20

// computeLitWindow 为 RE2-exact pattern (NFA 直接由 cp.expr 编译) 计算命中点两侧上下文界。
// lits 为该 pattern 的必需字面量集 (已 ASCII 小写, 即运行期触发集)。无法解析或集为空时返回
// 两侧无界 (不收窄)。anchoredStart/requireEnd 由调用方据 NFA 另行禁用对应侧。
func computeLitWindow(expr string, lits []string) litWindow {
	w := litWindow{head: -1, tail: -1}
	if len(lits) == 0 {
		return w
	}
	re, err := syntax.Parse(expr, syntax.Perl)
	if err != nil {
		return w
	}
	re = re.Simplify()
	set := make(map[string]struct{}, len(lits))
	for _, l := range lits {
		set[l] = struct{}{}
	}
	acc := &litWindowAcc{set: set, head: 0, tail: 0, headBounded: true, tailBounded: true, found: false}
	// 顶层: match-start 到本结点前缀宽=0(有界), 本结点后缀宽=0(有界)。
	acc.walk(re, 0, true, 0, true)
	if !acc.found {
		return w // 集里的字面量未在 AST 命中 (异常), 保守不收窄。
	}
	if acc.headBounded && acc.head <= litWindowCap {
		w.head = int32(acc.head)
	}
	if acc.tailBounded && acc.tail <= litWindowCap {
		w.tail = int32(acc.tail)
	}
	return w
}

type litWindowAcc struct {
	set         map[string]struct{}
	head        int  // 已发现命中字面量结点中, match-start 到字面量结尾的最大宽
	tail        int  // 已发现命中字面量结点中, 字面量结尾到 match-end 的最大宽
	headBounded bool // 是否所有命中结点的 head 侧都有界
	tailBounded bool // 是否所有命中结点的 tail 侧都有界
	found       bool
}

// record 累计一个命中字面量结点的两侧界 (pre/suf 为该结点 [起,止) 之外的上下文宽与有界性)。
func (a *litWindowAcc) record(litLen, pre int, preB bool, suf int, sufB bool) {
	a.found = true
	// head = pre (match-start 到字面量起点) + litLen (字面量本身)。
	h := pre + litLen
	if !preB {
		a.headBounded = false
	} else if h > a.head {
		a.head = h
	}
	if !sufB {
		a.tailBounded = false
	} else if suf > a.tail {
		a.tail = suf
	}
}

// walk 以继承属性遍历 AST: pre/preB = 本结点之前 (match-start 方向) 的最大上下文宽与有界性;
// suf/sufB = 本结点之后 (match-end 方向) 同义。命中目标字面量结点即 record。
func (a *litWindowAcc) walk(re *syntax.Regexp, pre int, preB bool, suf int, sufB bool) {
	switch re.Op {
	case syntax.OpLiteral:
		s := strings.ToLower(string(re.Rune))
		if _, ok := a.set[s]; ok {
			a.record(len(string(re.Rune)), pre, preB, suf, sufB)
		}

	case syntax.OpCapture:
		if len(re.Sub) == 1 {
			a.walk(re.Sub[0], pre, preB, suf, sufB)
		}

	case syntax.OpConcat:
		k := len(re.Sub)
		for i, sub := range re.Sub {
			lw, lb := sumWidthRange(re.Sub, 0, i)      // 左兄弟总宽 (0..i-1)
			rw, rb := sumWidthRange(re.Sub, i+1, k)    // 右兄弟总宽 (i+1..k-1)
			a.walk(sub, addSat(pre, lw), preB && lb, addSat(suf, rw), sufB && rb)
		}

	case syntax.OpAlternate:
		// 各分支共享同一外层上下文 (alternation 不增宽)。
		for _, sub := range re.Sub {
			a.walk(sub, pre, preB, suf, sufB)
		}

	case syntax.OpQuest:
		// x?: 可选, 不引入无界; 内部字面量的外层上下文不变。
		if len(re.Sub) == 1 {
			a.walk(re.Sub[0], pre, preB, suf, sufB)
		}

	case syntax.OpStar, syntax.OpPlus:
		// x* / x+: 字面量可被任意多次重复包裹 -> 两侧上下文无界。
		if len(re.Sub) == 1 {
			a.walk(re.Sub[0], pre, false, suf, false)
		}

	case syntax.OpRepeat:
		// {n,m}: 保守按无界处理 (即便有上界, 多副本环绕宽度难精确, 退回不收窄, 安全)。
		if len(re.Sub) == 1 {
			a.walk(re.Sub[0], pre, false, suf, false)
		}

		// 其它 (OpCharClass/OpAnyChar/锚点/Empty 等) 不含字面量子, 无需下行。
	}
}

// computeLitHeads 为 expr 的每个必需字面量分别计算"命中点回看上限 head" (match-start 到字面量
// 结尾的最大字节宽; -1 表示该字面量任一出现处 head 侧无界). 与 computeLitWindow 同源、同样的安全性
// 论证 (见本文件顶部), 区别在于 *按字面量分别给界* 而非全 pattern 取 max —— 这样"同一 pattern 内
// 多分支、仅个别分支含无界前缀"(如 \b(pass|...|\[...\]Password=.*extension:ica)\b) 时, 有界分支的
// 字面量 (pass/key/...) 仍可走锚定式单趟, 只有真正无界的分支字面量退化为整段.
//
// 锚定式只需 head (注入位置), 不需 tail (前向扫描 + 提前消亡自然收尾), 故此处只算 head.
// 返回 map[字面量(已小写)] -> head; 未在 AST 命中的字面量不入表 (调用方按 -1 即整段处理, 安全)。
func computeLitHeads(expr string, lits []string) map[string]int32 {
	out := make(map[string]int32, len(lits))
	if len(lits) == 0 {
		return out
	}
	re, err := syntax.Parse(expr, syntax.Perl)
	if err != nil {
		return out
	}
	re = re.Simplify()
	set := make(map[string]struct{}, len(lits))
	for _, l := range lits {
		set[l] = struct{}{}
	}
	acc := &litHeadAcc{set: set, heads: out}
	acc.walk(re, 0, true)
	return out
}

type litHeadAcc struct {
	set   map[string]struct{}
	heads map[string]int32
}

func (a *litHeadAcc) record(lit string, litLen, pre int, preB bool) {
	h := int32(-1)
	if preB {
		if hh := pre + litLen; hh <= litWindowCap {
			h = int32(hh)
		}
	}
	cur, ok := a.heads[lit]
	if !ok {
		a.heads[lit] = h
		return
	}
	// 同一字面量多处出现: 任一处无界则该字面量无界; 否则取 max (保守覆盖所有出现)。
	if cur < 0 || h < 0 {
		a.heads[lit] = -1
	} else if h > cur {
		a.heads[lit] = h
	}
}

func (a *litHeadAcc) walk(re *syntax.Regexp, pre int, preB bool) {
	switch re.Op {
	case syntax.OpLiteral:
		s := strings.ToLower(string(re.Rune))
		if _, ok := a.set[s]; ok {
			a.record(s, len(string(re.Rune)), pre, preB)
		}
	case syntax.OpCapture:
		if len(re.Sub) == 1 {
			a.walk(re.Sub[0], pre, preB)
		}
	case syntax.OpConcat:
		for i, sub := range re.Sub {
			lw, lb := sumWidthRange(re.Sub, 0, i)
			a.walk(sub, addSat(pre, lw), preB && lb)
		}
	case syntax.OpAlternate:
		for _, sub := range re.Sub {
			a.walk(sub, pre, preB)
		}
	case syntax.OpQuest:
		if len(re.Sub) == 1 {
			a.walk(re.Sub[0], pre, preB)
		}
	case syntax.OpStar, syntax.OpPlus, syntax.OpRepeat:
		if len(re.Sub) == 1 {
			a.walk(re.Sub[0], pre, false)
		}
	}
}

// sumWidthRange 求 subs[lo:hi) 的 maxByteWidth 之和与整体有界性。
func sumWidthRange(subs []*syntax.Regexp, lo, hi int) (int, bool) {
	total := 0
	for i := lo; i < hi; i++ {
		w, b := maxByteWidth(subs[i])
		if !b {
			return 0, false
		}
		total = addSat(total, w)
	}
	return total, true
}

func addSat(a, b int) int {
	s := a + b
	if s < a { // 溢出兜底
		return litWindowCap + 1
	}
	return s
}
