package minirehs

import (
	"regexp"

	regexp_utils "github.com/yaklang/yaklang/common/utils/regexp-utils"
)

// verifier 负责对候选数据做"真实命中"判定与取偏移. 它是命中语义的唯一权威:
// 引擎与 stdlib oracle 共享同一份 verifier 对象, 因此两者对同一 pattern 的判定恒等,
// 预过滤只影响"验证哪些位置", 不影响"判定结果".
type verifier interface {
	// findAll 返回非重叠 [from,to) 字节区间, 语义对齐 regexp.FindAllIndex.
	// 对 regexp2-only pattern (无法给出精确字节偏移), 命中时返回单个 {-1,-1} 表示"存在命中".
	findAll(data []byte) [][2]int
	// exact 报告该 verifier 是否能给出精确字节偏移 (re2Verifier 为 true).
	exact() bool
}

// re2Verifier 基于标准库 regexp (RE2), 给出精确的非重叠字节区间.
type re2Verifier struct {
	re *regexp.Regexp
}

func (v *re2Verifier) exact() bool { return true }

func (v *re2Verifier) findAll(data []byte) [][2]int {
	locs := v.re.FindAllIndex(data, -1)
	if len(locs) == 0 {
		return nil
	}
	out := make([][2]int, 0, len(locs))
	for _, l := range locs {
		out = append(out, [2]int{l[0], l[1]})
	}
	return out
}

// findAllInWindow 在 data[winStart:winEnd] 子区间上验证, 返回的偏移已换算回 data 的绝对坐标.
// 仅对"窗口可验证" (有界宽度、无位置锚点) 的 pattern 使用.
//
// 正确性要点: 在子切片上跑正则时, 切片首尾会被当作文本首尾, 导致 \b \B 等位置断言在
// 人为边界处误判. 但调用方对每个字面量命中以 2 倍最大宽度居中开窗, 可证明: 任何真实命中
// 在其"自身字面量命中点的窗口"里必然严格位于窗口内部 (两端各留有余量), 此时 \b 的相邻
// 字节都是真实上下文, 判定正确. 因此这里丢弃"贴着人为窗口边界"的命中 (它们会在自身
// 字面量的窗口中被正确找到), 既不漏报也不误报, 与全量 FindAllIndex 结果一致.
func (v *re2Verifier) findAllInWindow(data []byte, winStart, winEnd int) [][2]int {
	if winStart < 0 {
		winStart = 0
	}
	if winEnd > len(data) {
		winEnd = len(data)
	}
	if winStart >= winEnd {
		return nil
	}
	sub := data[winStart:winEnd]
	locs := v.re.FindAllIndex(sub, -1)
	if len(locs) == 0 {
		return nil
	}
	out := make([][2]int, 0, len(locs))
	for _, l := range locs {
		// 贴着人为左边界 (winStart>0) 或人为右边界 (winEnd<len) 的命中丢弃.
		if l[0] == 0 && winStart > 0 {
			continue
		}
		if l[1] == len(sub) && winEnd < len(data) {
			continue
		}
		out = append(out, [2]int{l[0] + winStart, l[1] + winStart})
	}
	return out
}

// regexp2Verifier 基于 yaklang 的 regexp-utils 包 (优先 RE2, 失败回退 regexp2),
// 用于承载 backref / lookaround 等自动机无法表达、但 regexp2 可处理的 pattern.
// 这类 pattern 报告"存在命中" (偏移 -1), 满足 MITM 打标等以命中为准的场景.
//
// 注: regexp-utils 的 regexp2 后端已全局切换为 go-pcre2-lite (PCRE2, 线性时间, 远低回溯成本),
// 故本 verifier 经 yak.Match 自动享有该加速, 无需 minirehs 内单独旁路.
type regexp2Verifier struct {
	yak *regexp_utils.YakRegexpUtils
}

func (v *regexp2Verifier) exact() bool { return false }

func (v *regexp2Verifier) findAll(data []byte) [][2]int {
	ok, err := v.yak.Match(data)
	if err != nil || !ok {
		return nil
	}
	return [][2]int{{-1, -1}}
}
