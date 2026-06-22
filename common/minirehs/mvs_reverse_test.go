package minirehs

import (
	"math/rand"
	"regexp"
	"regexp/syntax"
	"testing"
)

// 本文件是 Rose-lite 双向锚定"反向"半边的零假阴护栏: 反向 NFA + 反向锚定执行器必须与正向 existsIn
// 对同一 data 判定同真伪. 这是后续"前向 ∪ 反向替换整段扫"的正确性地基, 先于任何分发改造落地.
//
// 关键词: Rose-lite, 反向锚定, 零假阴护栏, 差分, 非法 UTF-8 切分一致性

// fullRevSpans 覆盖 [0,n] 的单一注入区间: 反向锚定在每个 rune 终点都注入起点, 使
// existsInReverseAnchored 等价于"反向 NFA 在 data 任意位置是否命中" = 正向 existsIn. 用于隔离
// 校验反向 NFA + 反向执行器 + UTF-8 切分, 不掺入 tail 界逻辑.
func fullRevSpans(n int) []anchorSpan { return []anchorSpan{{0, int32(n)}} }

// genRevInput 生成一段随机输入, 三档混合: pattern 常见字母 / ASCII / 任意字节 (制造大量非法 UTF-8).
func genRevInput(r *rand.Rand, maxLen int) []byte {
	n := r.Intn(maxLen + 1)
	in := make([]byte, n)
	for i := range in {
		switch r.Intn(3) {
		case 0:
			in[i] = "abcd0123.z"[r.Intn(10)]
		case 1:
			in[i] = byte(r.Intn(128))
		default:
			in[i] = byte(r.Intn(256)) // 高位字节 -> 非法 UTF-8 序列, 压测反向切分一致性
		}
	}
	return in
}

// TestMVSReverseNFADifferential: 反向 NFA (全区间注入) 必须与正向 existsIn 同真伪. 大量随机
// lean 正则 x 随机输入 (含非法 UTF-8). 若 DecodeLastRune 与 DecodeRune 在非法字节上切分分歧,
// 此测试必失败 -> 提示改走"正向切分边界反向遍历". 同时校验 nword==1 标量快路径与通用版一致.
func TestMVSReverseNFADifferential(t *testing.T) {
	r := rand.New(rand.NewSource(0x5EED9))
	iters := diffIters(t, defaultDiffIters)
	tested, skipped := 0, 0
	for it := 0; it < iters; it++ {
		expr := genRE(r, 2)
		if _, err := regexp.Compile(expr); err != nil {
			continue
		}
		parsed, err := syntax.Parse(expr, syntax.Perl)
		if err != nil {
			continue
		}
		fwd, ok := compileMVSNFA(parsed.Simplify())
		if !ok || fwd.hasAssert || fwd.anchoredStart || fwd.requireEnd {
			skipped++
			continue
		}
		rev := compileReverseExprToNFA(expr)
		if rev == nil {
			skipped++
			continue
		}
		tested++
		nword := rev.nword
		prev := make([]uint64, nword)
		cand := make([]uint64, nword)
		active := make([]uint64, nword)
		for s := 0; s < 6; s++ {
			in := genRevInput(r, 22)
			want := fwd.existsIn(in)
			got := rev.existsInReverseAnchored(in, fullRevSpans(len(in)), prev, cand, active)
			if got != want {
				t.Fatalf("REVERSE mismatch expr=%q input=%q: rev=%v fwd=%v", expr, in, got, want)
			}
			if rev.single {
				got1 := rev.existsInReverseAnchored1(in, fullRevSpans(len(in)))
				if got1 != want {
					t.Fatalf("REVERSE1 mismatch expr=%q input=%q: rev1=%v fwd=%v", expr, in, got1, want)
				}
			}
		}
	}
	t.Logf("reverse NFA differential: tested=%d skipped=%d", tested, skipped)
	if tested < 1000 {
		t.Fatalf("too few reverse-eligible patterns exercised: %d", tested)
	}
}

// TestMVSReverseRealMITMHotPatterns: 对真实 MITM 三大热点 (双无界 keyword 类) + 全语料, 反向 NFA
// (全区间注入) 必须与正向 existsIn 逐记录同真伪. 这是双向锚定将要替换整段扫的那批 pattern, 直接
// 在真实负载上证伪反向半边.
func TestMVSReverseRealMITMHotPatterns(t *testing.T) {
	patterns, names := compilableMITMPatterns(t)
	records, _ := loadCorpus(t)

	checked := 0
	for _, p := range patterns {
		fwd := compileExprToNFA(p.Expr)
		if fwd == nil || fwd.hasAssert || fwd.anchoredStart || fwd.requireEnd {
			continue
		}
		rev := compileReverseExprToNFA(p.Expr)
		if rev == nil {
			continue
		}
		checked++
		nword := rev.nword
		prev := make([]uint64, nword)
		cand := make([]uint64, nword)
		active := make([]uint64, nword)
		for _, data := range records {
			want := fwd.existsIn(data)
			got := rev.existsInReverseAnchored(data, fullRevSpans(len(data)), prev, cand, active)
			if got != want {
				t.Fatalf("REVERSE real mismatch rule=%q recLen=%d: rev=%v fwd=%v", names[p.ID], len(data), got, want)
			}
		}
	}
	t.Logf("reverse real-MITM differential: lean patterns checked=%d over %d records", checked, len(records))
	if checked < 10 {
		t.Fatalf("too few lean reverse patterns checked: %d", checked)
	}
}
