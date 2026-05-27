package aibalance

import (
	"bytes"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/stretchr/testify/assert"
)

// TestRuneByteBoundaries_ASCII 验证纯 ASCII 输入下边界与字节下标一致。
// 关键词: runeByteBoundaries ASCII
func TestRuneByteBoundaries_ASCII(t *testing.T) {
	p := []byte("hello")
	bds := runeByteBoundaries(p)
	assert.Equal(t, []int{0, 1, 2, 3, 4, 5}, bds)
}

// TestRuneByteBoundaries_CJK 验证中文（每个字符 3 字节）正确分段。
// 关键词: runeByteBoundaries 中文边界
func TestRuneByteBoundaries_CJK(t *testing.T) {
	p := []byte("你好世界")
	bds := runeByteBoundaries(p)
	// 4 个汉字 × 3 字节 = 12 字节
	assert.Equal(t, []int{0, 3, 6, 9, 12}, bds)
	// 验证按边界切出来的每段都是合法的 rune
	for i := 0; i < len(bds)-1; i++ {
		seg := p[bds[i]:bds[i+1]]
		assert.True(t, utf8.Valid(seg), "segment %d should be valid utf8", i)
	}
}

// TestRuneByteBoundaries_Mixed 验证 ASCII + emoji + 中文混合输入下，
// 切分边界一定落在合法 rune 后面，绝不切碎多字节字符。
// 关键词: runeByteBoundaries 混合 emoji 中文
func TestRuneByteBoundaries_Mixed(t *testing.T) {
	p := []byte("ab中文ef")
	bds := runeByteBoundaries(p)
	// a(1) b(1) 中(3) 文(3) e(1) f(1) = 10 字节, 6 rune
	assert.Equal(t, []int{0, 1, 2, 5, 8, 9, 10}, bds)
}

// TestSplitByRuneSegments_NoSplit 单段或空输入直接返回。
// 关键词: splitByRuneSegments 边界条件
func TestSplitByRuneSegments_NoSplit(t *testing.T) {
	assert.Nil(t, splitByRuneSegments(nil, 5))
	assert.Nil(t, splitByRuneSegments([]byte{}, 5))

	segs := splitByRuneSegments([]byte("abc"), 1)
	assert.Equal(t, [][]byte{[]byte("abc")}, segs)
}

// TestSplitByRuneSegments_NeverBreakUTF8 这是本次任务的核心断言：
// 不论 segments 怎么设置，切出来的每一段都必须是合法 UTF-8。
// 关键词: splitByRuneSegments UTF-8 完整性, 严禁切碎多字节字符
func TestSplitByRuneSegments_NeverBreakUTF8(t *testing.T) {
	cases := []string{
		"你好世界",
		"hello 世界",
		"a你b好c世d界e",
		"🚀 火箭 emoji 测试",
		strings.Repeat("中", 20),
	}
	for _, text := range cases {
		t.Run(text, func(t *testing.T) {
			p := []byte(text)
			for segments := 1; segments <= 30; segments++ {
				out := splitByRuneSegments(p, segments)
				// 重新拼起来必须 = 原始 p
				joined := bytes.Join(out, nil)
				assert.Equal(t, p, joined, "joined segments must equal original (segments=%d)", segments)
				// 每段必须 valid utf8
				for i, seg := range out {
					assert.True(t, utf8.Valid(seg),
						"segment %d (%q) must be valid utf8 (segments=%d)", i, string(seg), segments)
				}
			}
		})
	}
}

// TestSplitByRuneSegments_SegmentCountClamp 验证 segments 超过字符数时
// 段数自动收敛到字符数（不会出现空段或重复段）。
// 关键词: splitByRuneSegments segments 收敛
func TestSplitByRuneSegments_SegmentCountClamp(t *testing.T) {
	p := []byte("中文")
	out := splitByRuneSegments(p, 100)
	assert.Equal(t, 2, len(out))
	assert.Equal(t, []byte("中"), out[0])
	assert.Equal(t, []byte("文"), out[1])
}

// TestSplitByRuneSegments_InvalidUTF8 验证遇到不完整 UTF-8 序列时
// 不会切碎已经合法的 rune，且拼回来 = 原始字节。
// 关键词: splitByRuneSegments 无效 UTF-8 鲁棒性
func TestSplitByRuneSegments_InvalidUTF8(t *testing.T) {
	// "中" 的 UTF-8 编码是 0xE4 0xB8 0xAD；只给前 2 字节模拟跨 chunk 切断
	p := append([]byte("abc"), 0xE4, 0xB8)
	for segments := 1; segments <= 6; segments++ {
		out := splitByRuneSegments(p, segments)
		joined := bytes.Join(out, nil)
		// 核心保证：拼回来 = 原始字节，不丢任何字节、不切碎任何已合法的 rune。
		assert.Equal(t, p, joined, "joined must equal original (segments=%d)", segments)

		// 进一步：不完整 UTF-8 字节（0xE4, 0xB8）必须始终连在一起，
		// 不会被切成 [0xE4] + [0xB8] 两段。等价于：任何含 0xE4 的段
		// 必须同时含 0xB8（且顺序保持）。
		// 关键词: 不切碎 invalid utf8 续字节
		for _, seg := range out {
			if bytes.Contains(seg, []byte{0xE4}) {
				assert.True(t, bytes.Contains(seg, []byte{0xB8}),
					"0xE4 and 0xB8 must stay in the same segment (seg=%v)", seg)
			}
		}
	}
}
