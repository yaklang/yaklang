package aicache

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon/aitag"
)

// 与 prompt_loop_materials.go 里的 wrapPromptMessageSection 对齐
// - high-static 段 tagName 为 AI_CACHE_SYSTEM（升级后形态，let 上游识别 system 边界）
// - 其它三段 tagName 为 PROMPT_SECTION
// - dynamic 段 tagName 为 PROMPT_SECTION_dynamic（带 nonce 防 prompt injection）
// 兼容性：旧 PROMPT_SECTION_high-static 仍然识别（向后兼容过渡期），
// 切片归类时与 AI_CACHE_SYSTEM_high-static 等价，使老服务器产生的 prompt
// 与新服务器产生的 prompt 在 aicache 缓存表里能复用同一份 chunk hash 序列。
// （aitag 解析器在最后一个下划线处分 tagName/nonce，所以三个 tag name 都要声明）
// 关键词: aicache, AI_CACHE_SYSTEM, PROMPT_SECTION, 外层标签, 双标签兼容
const (
	tagPromptSection        = "PROMPT_SECTION"
	tagPromptSectionDynamic = "PROMPT_SECTION_dynamic"
	tagAICacheSystem        = "AI_CACHE_SYSTEM"
)

// acceptedTagNames 是 SplitViaTAG 接受的所有外层标签
// 关键词: aicache, acceptedTagNames
var acceptedTagNames = []string{tagPromptSection, tagPromptSectionDynamic, tagAICacheSystem}

// Split 把 prompt 按外层 PROMPT_SECTION 系列标签切成有序 Chunk 列表
// 切片规则参考 plan 第 4 节：
//  1. 调 aitag.SplitViaTAG 抽出 PROMPT_SECTION / PROMPT_SECTION_dynamic 块
//  2. 文本块（段间散文）不计入 chunk
//  3. tagName == PROMPT_SECTION 时，nonce 为 high-static / semi-dynamic / timeline
//     tagName == PROMPT_SECTION_dynamic 时，归到 Section="dynamic"，Nonce="dynamic_<inner>"
//  4. 不带任何外层标签时，整段视作单个 raw chunk
//  5. 哈希源为 Section + "|" + Content，dynamic 的 inner-nonce 不进哈希源
//
// 关键词: aicache, Split, prompt 切片
func Split(prompt string) *PromptSplit {
	out := &PromptSplit{
		Original: prompt,
		Bytes:    len(prompt),
	}
	if prompt == "" {
		return out
	}

	res, err := aitag.SplitViaTAG(prompt, acceptedTagNames...)
	if err != nil || res == nil {
		// 解析失败时退化成单个 raw chunk
		out.Chunks = []*Chunk{newRawChunk(prompt)}
		return out
	}

	taggedFound := false
	for _, blk := range res.GetOrderedBlocks() {
		if blk == nil {
			continue
		}
		if !blk.IsTagged() {
			continue
		}
		section, nonce := classifyTagged(blk.TagName, blk.Nonce)
		taggedFound = true
		out.Chunks = append(out.Chunks, &Chunk{
			Section: section,
			Nonce:   nonce,
			Bytes:   len(blk.Content),
			Hash:    hashSectionContent(section, blk.Content),
			Content: blk.Content,
		})
	}

	if !taggedFound {
		out.Chunks = []*Chunk{newRawChunk(prompt)}
	}
	return out
}

// classifyTagged 根据原始 tagName/nonce 推断 (Section, Nonce)
// AI_CACHE_SYSTEM 与 PROMPT_SECTION 在归类上等价（仅 section 含义来自 nonce），
// 这样新老两种 tagName 写出来的 high-static 段可以归到同一个 chunk hash 序列。
// 关键词: aicache, classifyTagged, section 识别, 双标签兼容
func classifyTagged(tagName, rawNonce string) (string, string) {
	tagName = strings.TrimSpace(tagName)
	rawNonce = strings.TrimSpace(rawNonce)

	switch tagName {
	case tagPromptSectionDynamic:
		nonce := SectionDynamic
		if rawNonce != "" {
			nonce = SectionDynamic + "_" + rawNonce
		}
		return SectionDynamic, nonce
	case tagPromptSection, tagAICacheSystem:
		// nonce 即 section 名（high-static / semi-dynamic / timeline / 其它扩展）
		if rawNonce == "" {
			return "unknown", "unknown"
		}
		return rawNonce, rawNonce
	}
	// 未知 tag，按原样返回
	if rawNonce == "" {
		return tagName, tagName
	}
	return tagName, rawNonce
}

// newRawChunk 把整段 prompt 包成一个 raw chunk
// 关键词: aicache, raw chunk, 无标签 prompt
func newRawChunk(prompt string) *Chunk {
	return &Chunk{
		Section: SectionRaw,
		Nonce:   SectionRaw,
		Bytes:   len(prompt),
		Hash:    hashSectionContent(SectionRaw, prompt),
		Content: prompt,
	}
}

// hashSectionContent 计算 sha256(Section + "|" + Content) 的 hex 字符串
// 关键词: aicache, hashSectionContent, 稳定哈希
func hashSectionContent(section, content string) string {
	h := sha256.New()
	h.Write([]byte(section))
	h.Write([]byte("|"))
	h.Write([]byte(content))
	return hex.EncodeToString(h.Sum(nil))
}
