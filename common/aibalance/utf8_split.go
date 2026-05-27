package aibalance

import (
	"unicode/utf8"
)

// runeByteBoundaries 把 p 按 UTF-8 rune 边界拆分，返回所有"段边界"的字节下标
// （含起点 0 和终点 len(p)）。相邻两个下标 boundaries[i] 与 boundaries[i+1]
// 截取出来的 p[start:end] 一定是一个完整的 rune 或一段连续的"无效 UTF-8 字节"。
//
// 设计要点：
//   - 对 valid rune：boundaries 推进 size 个字节（size 即 utf8.DecodeRune 返回值）。
//   - 对 invalid rune（DecodeRune 返回 RuneError 且 size==1）：把后续连续的无效字节
//     合并为一个"无效 span"，作为单个 boundary 段输出。这样上游若送来不完整的多字节
//     序列（极少见，但理论上可能跨 chunk），我们既不切碎合法 rune，也不把无效字节切成
//     更碎的 1-字节段。
//
// 返回切片长度 = "字符段数 + 1"，长度 1 表示 p 为空。
// 关键词: runeByteBoundaries, UTF-8 安全切分, 流式分段
func runeByteBoundaries(p []byte) []int {
	boundaries := make([]int, 0, len(p)/2+2)
	boundaries = append(boundaries, 0)
	if len(p) == 0 {
		return boundaries
	}
	i := 0
	for i < len(p) {
		r, size := utf8.DecodeRune(p[i:])
		if r == utf8.RuneError && size == 1 {
			// 把连续的无效字节合并为一个段，避免切碎
			j := i + 1
			for j < len(p) {
				r2, sz2 := utf8.DecodeRune(p[j:])
				if r2 == utf8.RuneError && sz2 == 1 {
					j++
					continue
				}
				break
			}
			i = j
		} else {
			i += size
		}
		boundaries = append(boundaries, i)
	}
	return boundaries
}

// splitByRuneSegments 把 p 在 UTF-8 rune 边界上拆分成 ~= segments 段。
// 真实段数 = min(segments, 字符总数)；当 p 为空或 segments<=1 时返回单段 p。
//
// 切分规则：把字符总数 n 按比例分到 segments 段（s 段覆盖 [s*n/segments,
// (s+1)*n/segments) 范围内的字符），保证：
//   - 每段都对齐 rune 边界，绝不会把多字节字符切碎。
//   - 段与段之间的字节范围合并起来正好覆盖整段 p（无重叠、无遗漏）。
//
// 关键词: splitByRuneSegments, UTF-8 安全切分, 按字符数比例切分
func splitByRuneSegments(p []byte, segments int) [][]byte {
	if len(p) == 0 {
		return nil
	}
	if segments <= 1 {
		return [][]byte{p}
	}
	boundaries := runeByteBoundaries(p)
	n := len(boundaries) - 1 // 字符段数
	if n <= 1 {
		return [][]byte{p}
	}
	if segments > n {
		segments = n
	}
	out := make([][]byte, 0, segments)
	for s := 0; s < segments; s++ {
		startIdx := s * n / segments
		endIdx := (s + 1) * n / segments
		if startIdx >= endIdx {
			continue
		}
		startByte := boundaries[startIdx]
		endByte := boundaries[endIdx]
		if startByte >= endByte {
			continue
		}
		out = append(out, p[startByte:endByte])
	}
	return out
}
