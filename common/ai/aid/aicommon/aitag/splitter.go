package aitag

import (
	"fmt"
	"strings"

	"github.com/yaklang/yaklang/common/log"
)

// BlockType describes whether a block is plain text or wrapped by an AITAG.
// 关键词: aitag, BlockType, 切片类型
type BlockType int

const (
	// BlockTypeText 表示标签之外的普通文本片段
	BlockTypeText BlockType = iota
	// BlockTypeTagged 表示被 <|TAG_nonce|>...<|TAG_END_nonce|> 包裹的标签内容
	BlockTypeTagged
)

// String returns the human-readable form of the block type.
func (b BlockType) String() string {
	switch b {
	case BlockTypeText:
		return "text"
	case BlockTypeTagged:
		return "tagged"
	default:
		return "unknown"
	}
}

// Block 表示一次 SplitViaTAG 后的最小切片单元
// 关键词: aitag, Block, 切片块
type Block struct {
	// Type 区分文本块与标签块
	Type BlockType
	// Content 是块的"语义内容"
	// 文本块: 与原文本完全一致
	// 标签块: 标签包裹的内部内容，按 block-text 规则剥离首尾各最多一个换行
	Content string
	// TagName 仅在标签块中有效，表示前缀名（如 AI_CACHE_STATIC）
	TagName string
	// Nonce 仅在标签块中有效，表示标签 nonce（如 ccc1）
	Nonce string
	// Raw 是该块在源串中的原始片段
	// 文本块: 与 Content 一致
	// 标签块: 含起始与结束标签的完整片段 <|TAG_nonce|>...<|TAG_END_nonce|>
	Raw string
	// Index 是该 Block 在 SplitResult.GetOrderedBlocks() 中的下标
	Index int
}

// IsText 判断是否文本块
func (b *Block) IsText() bool {
	return b != nil && b.Type == BlockTypeText
}

// IsTagged 判断是否标签块
func (b *Block) IsTagged() bool {
	return b != nil && b.Type == BlockTypeTagged
}

// Render 返回块的"可拼接形态"
// 文本块: 直接返回 Content
// 标签块: 用 <|TAG_nonce|>\nContent\n<|TAG_END_nonce|> 重新包裹，与现有 block-text 风格保持一致
// 关键词: aitag, Block, Render, 重新包裹
func (b *Block) Render() string {
	if b == nil {
		return ""
	}
	if b.Type == BlockTypeText {
		return b.Content
	}
	return fmt.Sprintf("<|%s_%s|>\n%s\n<|%s_END_%s|>", b.TagName, b.Nonce, b.Content, b.TagName, b.Nonce)
}

// SplitResult 是 SplitViaTAG 的返回结构体
// 关键词: aitag, SplitResult, 切片结果
type SplitResult struct {
	blocks []*Block
}

// GetOrderedBlocks 按源顺序返回所有 block（含文本块与标签块）
func (r *SplitResult) GetOrderedBlocks() []*Block {
	if r == nil {
		return nil
	}
	return r.blocks
}

// GetTextBlocks 仅返回文本块
func (r *SplitResult) GetTextBlocks() []*Block {
	if r == nil {
		return nil
	}
	out := make([]*Block, 0, len(r.blocks))
	for _, b := range r.blocks {
		if b.IsText() {
			out = append(out, b)
		}
	}
	return out
}

// GetTaggedBlocks 仅返回标签块
func (r *SplitResult) GetTaggedBlocks() []*Block {
	if r == nil {
		return nil
	}
	out := make([]*Block, 0, len(r.blocks))
	for _, b := range r.blocks {
		if b.IsTagged() {
			out = append(out, b)
		}
	}
	return out
}

// GetBlocksByTagName 按 TagName 过滤标签块
func (r *SplitResult) GetBlocksByTagName(name string) []*Block {
	if r == nil {
		return nil
	}
	out := make([]*Block, 0)
	for _, b := range r.blocks {
		if b.IsTagged() && b.TagName == name {
			out = append(out, b)
		}
	}
	return out
}

// GetBlocksByNonce 按 Nonce 过滤标签块
func (r *SplitResult) GetBlocksByNonce(nonce string) []*Block {
	if r == nil {
		return nil
	}
	out := make([]*Block, 0)
	for _, b := range r.blocks {
		if b.IsTagged() && b.Nonce == nonce {
			out = append(out, b)
		}
	}
	return out
}

// String 顺序拼接所有 block 的 Raw，用于 round-trip 校验
// 关键词: aitag, SplitResult, String, 还原原文
func (r *SplitResult) String() string {
	if r == nil {
		return ""
	}
	var sb strings.Builder
	for _, b := range r.blocks {
		sb.WriteString(b.Raw)
	}
	return sb.String()
}

// Len 返回 block 总数
func (r *SplitResult) Len() int {
	if r == nil {
		return 0
	}
	return len(r.blocks)
}

// SplitViaTAG 把 input 字符串切分为有序 Block 列表
// tagNames 是被识别为"标签块"的标签前缀名集合（不含 nonce）
// 同一个 (tagName, nonce) 组合可在 input 中多次出现，每次都切成独立 block
// 关键词: aitag, SplitViaTAG, prompt 切片, 标签切分
func SplitViaTAG(input string, tagNames ...string) (*SplitResult, error) {
	if len(tagNames) == 0 {
		return nil, fmt.Errorf("aitag split: at least one tag name is required")
	}

	accepted := make(map[string]bool, len(tagNames))
	for _, name := range tagNames {
		if name == "" {
			continue
		}
		accepted[name] = true
	}
	if len(accepted) == 0 {
		return nil, fmt.Errorf("aitag split: at least one non-empty tag name is required")
	}

	result := &SplitResult{}
	pos := 0

	appendText := func(s string) {
		if s == "" {
			return
		}
		result.blocks = append(result.blocks, &Block{
			Type:    BlockTypeText,
			Content: s,
			Raw:     s,
			Index:   len(result.blocks),
		})
	}

	appendTagged := func(tagName, nonce, content, raw string) {
		result.blocks = append(result.blocks, &Block{
			Type:    BlockTypeTagged,
			Content: content,
			TagName: tagName,
			Nonce:   nonce,
			Raw:     raw,
			Index:   len(result.blocks),
		})
	}

	for pos < len(input) {
		matchedAt, tagName, nonce, afterStart := findNextAcceptedStartTag(input, pos, accepted)

		if matchedAt < 0 {
			appendText(input[pos:])
			break
		}

		if matchedAt > pos {
			appendText(input[pos:matchedAt])
		}

		// 精确匹配的结束标签字面量
		// 关键词: aitag, SplitViaTAG, 结束标签匹配
		endTagLiteral := "<|" + tagName + "_END_" + nonce + "|>"
		rel := strings.Index(input[afterStart:], endTagLiteral)
		if rel < 0 {
			return nil, fmt.Errorf("aitag split: missing end tag %s for opener at offset %d", endTagLiteral, matchedAt)
		}
		endTagAt := afterStart + rel

		rawInner := input[afterStart:endTagAt]
		content := stripBlockFormattingNewlines(rawInner)
		raw := input[matchedAt : endTagAt+len(endTagLiteral)]

		appendTagged(tagName, nonce, content, raw)
		log.Debugf("[AITAG] split tagged block <%s_%s> length=%d", tagName, nonce, len(content))

		pos = endTagAt + len(endTagLiteral)
	}

	return result, nil
}

// findNextAcceptedStartTag 从 from 起向后扫描 input，寻找下一个 tagName 命中 accepted 集合的起始标签
// 返回值:
//
//	matchedAt:  起始标签 '<' 的位置；找不到时返回 -1
//	tagName:    解析出的标签前缀名
//	nonce:      解析出的 nonce
//	afterStart: 起始标签结束 '>' 的下一个字节位置
//
// 关键词: aitag, findNextAcceptedStartTag, 起始标签扫描
func findNextAcceptedStartTag(input string, from int, accepted map[string]bool) (int, string, string, int) {
	cursor := from
	for cursor < len(input) {
		idx := strings.Index(input[cursor:], "<|")
		if idx < 0 {
			return -1, "", "", -1
		}
		idx += cursor

		// 在同一行 + 200 字节范围内寻找 |>
		// 与现有 parser 长度限制一致，避免误匹配跨段的 |>
		// 关键词: aitag, findNextAcceptedStartTag, 闭合扫描
		closeAt := findTagCloseInLine(input, idx+2, 200)
		if closeAt < 0 {
			cursor = idx + 2
			continue
		}

		tagStr := input[idx : closeAt+2]
		tagName, nonce := parseStartTagLiteral(tagStr)

		// parseStartTagLiteral 在最后一个下划线处切分，会把 <|FOO_END_xyz|> 解析成 tagName="FOO_END" / nonce="xyz"
		// accepted 集合自然过滤掉这种情况；这里无需额外排除 _END
		if tagName != "" && nonce != "" && accepted[tagName] {
			return idx, tagName, nonce, closeAt + 2
		}

		cursor = idx + 2
	}
	return -1, "", "", -1
}

// findTagCloseInLine 在 input[start:] 范围内寻找 '|>' 的位置（返回 '|' 的下标）
// 限制条件:
//   - 不允许跨越 '\n'（标签必须在单行内）
//   - 最多扫描 maxLen 字节
//
// 找不到返回 -1
// 关键词: aitag, findTagCloseInLine, 单行闭合限制
func findTagCloseInLine(input string, start, maxLen int) int {
	end := start + maxLen
	if end > len(input) {
		end = len(input)
	}
	for i := start; i < end-1; i++ {
		ch := input[i]
		if ch == '\n' {
			return -1
		}
		if ch == '|' && input[i+1] == '>' {
			return i
		}
	}
	return -1
}

// stripBlockFormattingNewlines 剥离至多一个首换行与至多一个末换行
// 与现有 extractor 的 block-text 行为对齐，使
//
//	"\nstatic block\n"     -> "static block"
//	"\n\ncontent\n\n"      -> "\ncontent\n"
//
// 关键词: aitag, stripBlockFormattingNewlines, 块文本格式化
func stripBlockFormattingNewlines(s string) string {
	if strings.HasPrefix(s, "\n") {
		s = s[1:]
	}
	if strings.HasSuffix(s, "\n") {
		s = s[:len(s)-1]
	}
	return s
}
