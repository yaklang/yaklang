package minirehs

import "regexp/syntax"

// maxByteWidth 估算一条 RE2 正则单次命中可能跨越的最大字节数, 并报告是否"有界".
// 用于邻域验证: 有界宽度的 pattern 在字面量命中点附近的小窗口内验证即可, 无需全量扫描.
//
// 估算保守 (宁可偏大): 任意字符按 UTF-8 最长 4 字节计. 返回 bounded=false 表示存在
// 不定长构造 (*, +, 无上限 {n,}), 无法用固定窗口安全验证.
//
// 关键词: max width, bounded, neighborhood verification, 邻域验证
func maxByteWidth(re *syntax.Regexp) (w int, bounded bool) {
	switch re.Op {
	case syntax.OpLiteral:
		return len(string(re.Rune)), true

	case syntax.OpCharClass, syntax.OpAnyChar, syntax.OpAnyCharNotNL:
		return 4, true // 单个字符 UTF-8 最长 4 字节

	case syntax.OpEmptyMatch, syntax.OpBeginLine, syntax.OpEndLine,
		syntax.OpBeginText, syntax.OpEndText, syntax.OpWordBoundary,
		syntax.OpNoWordBoundary:
		return 0, true

	case syntax.OpCapture:
		if len(re.Sub) == 1 {
			return maxByteWidth(re.Sub[0])
		}
		return 0, false

	case syntax.OpQuest:
		if len(re.Sub) == 1 {
			return maxByteWidth(re.Sub[0])
		}
		return 0, false

	case syntax.OpStar, syntax.OpPlus:
		return 0, false // 不定长

	case syntax.OpRepeat:
		if re.Max < 0 || len(re.Sub) != 1 {
			return 0, false
		}
		sw, ok := maxByteWidth(re.Sub[0])
		if !ok {
			return 0, false
		}
		return sw * re.Max, true

	case syntax.OpConcat:
		total := 0
		for _, sub := range re.Sub {
			sw, ok := maxByteWidth(sub)
			if !ok {
				return 0, false
			}
			total += sw
		}
		return total, true

	case syntax.OpAlternate:
		max := 0
		for _, sub := range re.Sub {
			sw, ok := maxByteWidth(sub)
			if !ok {
				return 0, false
			}
			if sw > max {
				max = sw
			}
		}
		return max, true

	default:
		return 0, false
	}
}

// hasPositionAnchor 报告正则是否含 ^ $ \A \z 行/文本锚点. 含锚点者不能在窗口内安全验证
// (窗口边界会被误当作行首/行尾/文本首尾), 必须全量验证.
func hasPositionAnchor(re *syntax.Regexp) bool {
	switch re.Op {
	case syntax.OpBeginLine, syntax.OpEndLine, syntax.OpBeginText, syntax.OpEndText:
		return true
	}
	for _, sub := range re.Sub {
		if hasPositionAnchor(sub) {
			return true
		}
	}
	return false
}

// windowVerifiable 报告一条正则是否适合邻域窗口验证: 宽度有界、不太大、且无位置锚点.
// maxWindowWidth 限制窗口规模; 超过则退化为全量验证 (收益有限且窗口过大).
const maxWindowWidth = 512

func windowVerifiable(re *syntax.Regexp) (w int, ok bool) {
	if hasPositionAnchor(re) {
		return 0, false
	}
	width, bounded := maxByteWidth(re)
	if !bounded || width == 0 || width > maxWindowWidth {
		return 0, false
	}
	return width, true
}
