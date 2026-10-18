package minirehs

import (
	"regexp"
	"sync"

	regexp_utils "github.com/yaklang/yaklang/common/utils/regexp-utils"
)

// verifier 负责对候选数据做"真实命中"判定与取偏移. 它是命中语义的唯一权威:
// 引擎与 stdlib oracle 共享同一份 verifier 对象, 因此两者对同一 pattern 的判定恒等,
// 预过滤只影响"验证哪些位置", 不影响"判定结果".
type verifier interface {
	// findAll 返回非重叠 [from,to) 字节区间, 语义对齐 regexp.FindAllIndex.
	// 对 PCRE2 兜底 pattern，命中时返回单个 {-1,-1} 表示"存在命中"。
	findAll(data []byte) [][2]int
	// exact 报告该 verifier 是否能给出精确字节偏移 (re2Verifier 为 true).
	exact() bool
}

// re2Verifier 基于标准库 regexp (RE2), 给出精确的非重叠字节区间.
type re2Verifier struct {
	re      *regexp.Regexp
	once    sync.Once
	expr    string
	longest bool
}

func (v *re2Verifier) load() *regexp.Regexp {
	// Compiled databases already own their verifier. Images initialize it once,
	// only if a scan actually needs verification rather than NFA existence.
	v.once.Do(func() {
		if v.re == nil {
			v.re, _ = regexp.Compile(v.expr)
			if v.re != nil && v.longest {
				v.re.Longest()
			}
		}
	})
	return v.re
}

func (v *re2Verifier) exact() bool { return true }

func (v *re2Verifier) findAll(data []byte) [][2]int {
	re := v.load()
	if re == nil {
		return nil
	}
	locs := re.FindAllIndex(data, -1)
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
	re := v.load()
	if re == nil {
		return nil
	}
	locs := re.FindAllIndex(sub, -1)
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

// pcre2Verifier 通过现有 regexp-utils 使用 VillanCh/go-pcre2-lite，
// 承载 backref / lookaround 等 RE2 自动机无法表达的 pattern。
// 这类 pattern 报告存在命中 (偏移 -1)，不引入其他正则引擎或无 cgo 替代库。
type pcre2Verifier struct {
	yak  *regexp_utils.YakRegexpUtils
	once sync.Once
	expr string
}

func (v *pcre2Verifier) exact() bool { return false }

func (v *pcre2Verifier) findAll(data []byte) [][2]int {
	return v.findAllString(string(data))
}

func (v *pcre2Verifier) findAllString(data string) [][2]int {
	v.once.Do(func() {
		if v.yak == nil {
			v.yak = regexp_utils.NewYakRegexpUtils(v.expr)
		}
	})
	ok, err := v.yak.MatchString(data)
	if err != nil || !ok {
		return nil
	}
	return [][2]int{{-1, -1}}
}

// findAllInRecord shares subject copies between PCRE2 confirmations of the
// same record. Copy the first requested window only; geometric growth bounds
// all later copies by O(record size), without copying a large irrelevant prefix
// for a single localized match. RE2 continues to consume the original bytes.
// A scratch belongs to one scan/worker; clearVerifierText ends that lifetime.
func findAllInRecord(v verifier, data []byte, lo, hi int, sc *scratch) [][2]int {
	pcre, ok := v.(*pcre2Verifier)
	if !ok || sc == nil {
		return v.findAll(data[lo:hi])
	}
	if lo == hi {
		return pcre.findAllString("")
	}
	base := sc.verifierTextLo
	end := base + len(sc.verifierText)
	if sc.verifierText == "" || lo < base || hi > end {
		if sc.verifierText == "" {
			base, end = lo, hi
		} else {
			if lo < base {
				base = lo
			}
			if hi > end {
				end = hi
			}
			size := len(sc.verifierText)
			if size > len(data)/2 {
				size = len(data)
			} else {
				size *= 2
			}
			if size < end-base {
				size = end - base
			}
			base = end - size
			if base < 0 {
				base = 0
			}
			end = base + size
		}
		sc.verifierText = string(data[base:end])
		sc.verifierTextLo = base
	}
	return pcre.findAllString(sc.verifierText[lo-base : hi-base])
}

func (s *scratch) clearVerifierText() {
	s.verifierText, s.verifierTextLo = "", 0
}
