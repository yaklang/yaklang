package aiprojection

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon/aitag"
)

// Outer tag names match the prompt section renderer. The older
// PROMPT_SECTION_high-static and current AI_CACHE_SYSTEM_high-static forms
// produce the same cache chunk category and hash.
const (
	tagPromptSection            = "PROMPT_SECTION"
	tagPromptSectionDynamic     = "PROMPT_SECTION_dynamic"
	tagAICacheSystem            = "AI_CACHE_SYSTEM"
	timelineInnerTagName        = "TIMELINE"
	timelineIntervalNoncePrefix = "b"
)

// acceptedTagNames 是 SplitViaTAG 接受的所有外层标签
// 关键词: aicache, acceptedTagNames
var acceptedTagNames = []string{tagPromptSection, tagPromptSectionDynamic, tagAICacheSystem}

// ParsedPrompt holds the cache observation and projection views produced by
// one pass over the outer AITAG blocks. Legacy timelines may need a nested parse.
type ParsedPrompt struct {
	cacheSplit *PromptSplit
	sections   *ProjectionSections
}

// CacheSplit returns the cache analysis view of the parsed prompt. Split is a
// compatibility shortcut for callers that only need this view.
func (p *ParsedPrompt) CacheSplit() *PromptSplit {
	if p == nil {
		return nil
	}
	return p.cacheSplit
}

// Sections returns the ordered, byte-exact input to projection policies.
func (p *ParsedPrompt) Sections() *ProjectionSections {
	if p == nil {
		return nil
	}
	return p.sections
}

// Parse produces the common input for cache observation and request projection.
// Malformed or untagged prompts retain their original bytes as a raw chunk.
func Parse(prompt string) *ParsedPrompt {
	parsed := &ParsedPrompt{
		sections: &ProjectionSections{Original: prompt, parsed: true},
		cacheSplit: &PromptSplit{
			Original: prompt,
			Bytes:    len(prompt),
		},
	}
	if prompt == "" {
		return parsed
	}

	res, err := aitag.SplitViaTAG(prompt, acceptedTagNames...)
	if err != nil || res == nil {
		parsed.cacheSplit.Chunks = []*Chunk{newRawChunk(prompt)}
		parsed.sections.Items = []ProjectionSection{{Kind: ProjectionSectionRaw, Raw: prompt, content: prompt}}
		return parsed
	}
	var timelineBlk *aitag.Block
	for _, blk := range res.GetOrderedBlocks() {
		if blk == nil {
			continue
		}
		section := ProjectionSection{Kind: ProjectionSectionRaw, Raw: blk.Raw, content: blk.Content}
		if isHighStaticBlock(blk) {
			section.Kind = ProjectionSectionCache
			parsed.sections.cache.staticParts = append(parsed.sections.cache.staticParts, section)
		} else {
			if blk.IsTagged() || strings.TrimSpace(blk.Content) != "" {
				parsed.sections.cache.hasOther = true
			}
			if timelineBlk == nil && isTimelineSectionBlock(blk) {
				timelineBlk = blk
			}
			if isTimelineSectionBlock(blk) {
				section.Kind = ProjectionSectionTimeline
			} else if blk.IsTagged() {
				section.Kind = ProjectionSectionCache
			}
		}
		parsed.sections.Items = append(parsed.sections.Items, section)
		if !blk.IsTagged() {
			continue
		}
		chunkSection, nonce := classifyTagged(blk.TagName, blk.Nonce)
		parsed.cacheSplit.Chunks = append(parsed.cacheSplit.Chunks, &Chunk{
			Section: chunkSection,
			Nonce:   nonce,
			Bytes:   len(blk.Content),
			Hash:    hashSectionContent(chunkSection, blk.Content),
			Content: blk.Content,
		})
	}
	if len(parsed.cacheSplit.Chunks) == 0 {
		parsed.cacheSplit.Chunks = []*Chunk{newRawChunk(prompt)}
	}
	if len(parsed.sections.cache.staticParts) > 0 && parsed.sections.cache.hasOther {
		var userRaw strings.Builder
		for _, blk := range res.GetOrderedBlocks() {
			if blk != nil && !isHighStaticBlock(blk) {
				userRaw.WriteString(blk.Raw)
			}
		}
		parsed.sections.cache.userRaw = userRaw.String()
		parseCacheBoundaries(&parsed.sections.cache)
		if len(parsed.sections.cache.frozen) == 0 && timelineBlk != nil {
			parsed.sections.cache.timeline = parseTimelineFallback(res, timelineBlk)
		}
	}
	return parsed
}

// parseCacheBoundaries finds ordered frozen, semi, and semi2 pairs once. Each
// candidate retains the old TrimSpace behavior and exact boundary-tag bytes.
func parseCacheBoundaries(parts *cacheProjectionSections) {
	all := parts.userRaw
	frozenEnd, ok := boundaryEnd(all, 0, frozenBoundaryStartTag, frozenBoundaryEndTag)
	if !ok {
		return
	}
	u1, tail := strings.TrimSpace(all[:frozenEnd]), strings.TrimSpace(all[frozenEnd:])
	if u1 == "" || tail == "" {
		return
	}
	parts.frozen = []ProjectionSection{cacheBoundarySection(u1), cacheBoundarySection(tail)}

	semiEnd, ok := boundaryEnd(all, frozenEnd, semiBoundaryStartTag, semiBoundaryEndTag)
	if !ok {
		return
	}
	u2, tail := strings.TrimSpace(all[frozenEnd:semiEnd]), strings.TrimSpace(all[semiEnd:])
	if u2 == "" || tail == "" {
		return
	}
	parts.semi = []ProjectionSection{cacheBoundarySection(u1), cacheBoundarySection(u2), cacheBoundarySection(tail)}

	semi2End, ok := boundaryEnd(all, semiEnd, semi2BoundaryStartTag, semi2BoundaryEndTag)
	if !ok {
		return
	}
	u3, u4 := strings.TrimSpace(all[semiEnd:semi2End]), strings.TrimSpace(all[semi2End:])
	if u3 != "" && u4 != "" {
		parts.semi2 = []ProjectionSection{cacheBoundarySection(u1), cacheBoundarySection(u2), cacheBoundarySection(u3), cacheBoundarySection(u4)}
	}
}

func cacheBoundarySection(raw string) ProjectionSection {
	return ProjectionSection{Kind: ProjectionSectionCache, Raw: raw}
}

func boundaryEnd(all string, after int, startTag, endTag string) (int, bool) {
	start := strings.Index(all[after:], startTag)
	if start < 0 {
		return 0, false
	}
	start += after + len(startTag)
	end := strings.Index(all[start:], endTag)
	if end < 0 {
		return 0, false
	}
	return start + end + len(endTag), true
}

// parseTimelineFallback handles older prompts that have no frozen boundary.
// It preserves the original block order when splitting a timeline section.
func parseTimelineFallback(res *aitag.SplitResult, timelineBlk *aitag.Block) []ProjectionSection {
	frozenWrapped, openWrapped := splitTimelineFrozenOpen(timelineBlk)
	if frozenWrapped == "" || openWrapped == "" {
		return nil
	}
	var u1, u2 strings.Builder
	seenTimeline := false
	for _, blk := range res.GetOrderedBlocks() {
		if blk == nil || isHighStaticBlock(blk) {
			continue
		}
		if blk == timelineBlk {
			u1.WriteString(frozenWrapped)
			u2.WriteString(openWrapped)
			seenTimeline = true
			continue
		}
		if !seenTimeline {
			u1.WriteString(blk.Raw)
		} else {
			u2.WriteString(blk.Raw)
		}
	}
	first, second := strings.TrimSpace(u1.String()), strings.TrimSpace(u2.String())
	if first == "" || second == "" {
		return nil
	}
	return []ProjectionSection{{Kind: ProjectionSectionTimeline, Raw: first}, {Kind: ProjectionSectionTimeline, Raw: second}}
}

// Split returns the cache chunk view of a complete prompt.
// Ordinary text between recognized outer tags remains in the projection input
// but does not become a cache chunk. Untagged prompts yield one raw chunk.
func Split(prompt string) *PromptSplit {
	return Parse(prompt).CacheSplit()
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

// splitTimelineFrozenOpen parses legacy nested TIMELINE tags. The last interval
// bucket is open; earlier tagged blocks form the frozen prefix. Both halves
// keep PROMPT_SECTION_timeline wrappers for downstream recognition.
func splitTimelineFrozenOpen(timelineBlk *aitag.Block) (frozenWrapped, openWrapped string) {
	if timelineBlk == nil || !timelineBlk.IsTagged() {
		return "", ""
	}
	inner, err := aitag.SplitViaTAG(timelineBlk.Content, timelineInnerTagName)
	if err != nil || inner == nil {
		return "", ""
	}
	ordered := inner.GetOrderedBlocks()

	// 找最末一个 nonce 以 "b" 开头的 TIMELINE block（=最末 interval bucket = Open）
	lastIntervalIdx := -1
	for i := len(ordered) - 1; i >= 0; i-- {
		blk := ordered[i]
		if blk == nil || !blk.IsTagged() {
			continue
		}
		if blk.TagName != timelineInnerTagName {
			continue
		}
		if strings.HasPrefix(blk.Nonce, timelineIntervalNoncePrefix) {
			lastIntervalIdx = i
			break
		}
	}
	if lastIntervalIdx < 0 {
		return "", ""
	}

	// 至少要有 1 个 frozen tagged block（reducer 或 frozen interval）在最末
	// interval 之前，才有"双 cc"分段的价值。
	hasFrozenTagged := false
	for i := 0; i < lastIntervalIdx; i++ {
		blk := ordered[i]
		if blk != nil && blk.IsTagged() && blk.TagName == timelineInnerTagName {
			hasFrozenTagged = true
			break
		}
	}
	if !hasFrozenTagged {
		return "", ""
	}

	var frozenBuf, openBuf strings.Builder
	for i, blk := range ordered {
		if blk == nil {
			continue
		}
		if i < lastIntervalIdx {
			frozenBuf.WriteString(blk.Raw)
		} else {
			openBuf.WriteString(blk.Raw)
		}
	}

	frozenWrapped = wrapPromptSectionTimeline(frozenBuf.String())
	openWrapped = wrapPromptSectionTimeline(openBuf.String())
	return frozenWrapped, openWrapped
}

// wrapPromptSectionTimeline 把一段 inner timeline 内容重新用
// PROMPT_SECTION_timeline 标签包裹，让 user 消息再次被 Split 时仍然能被
// 识别为 timeline section（与 splitter classifyTagged 对齐）。
// 关键词: aicache, wrapPromptSectionTimeline, 重包 PROMPT_SECTION_timeline
func wrapPromptSectionTimeline(inner string) string {
	trimmed := strings.Trim(inner, "\n")
	if trimmed == "" {
		return ""
	}
	var sb strings.Builder
	sb.WriteString("<|")
	sb.WriteString(tagPromptSection)
	sb.WriteString("_")
	sb.WriteString(SectionTimeline)
	sb.WriteString("|>\n")
	sb.WriteString(trimmed)
	sb.WriteString("\n<|")
	sb.WriteString(tagPromptSection)
	sb.WriteString("_END_")
	sb.WriteString(SectionTimeline)
	sb.WriteString("|>")
	return sb.String()
}

// isHighStaticBlock 判断一个 aitag block 是否是 high-static 段
// （两种 tagName 等价：新形态 AI_CACHE_SYSTEM_high-static、老形态 PROMPT_SECTION_high-static）
// 关键词: aicache, isHighStaticBlock, AI_CACHE_SYSTEM 双标签兼容
func isHighStaticBlock(blk *aitag.Block) bool {
	if blk == nil || !blk.IsTagged() {
		return false
	}
	if blk.Nonce != SectionHighStatic {
		return false
	}
	return blk.TagName == tagAICacheSystem || blk.TagName == tagPromptSection
}

// isTimelineSectionBlock recognizes either timeline wrapper for legacy fallback.
func isTimelineSectionBlock(blk *aitag.Block) bool {
	if blk == nil || !blk.IsTagged() {
		return false
	}
	if blk.TagName != tagPromptSection {
		return false
	}
	return blk.Nonce == SectionTimeline || blk.Nonce == SectionTimelineOpen
}

// These boundary literals match the aicommon renderer. They stay local to
// avoid an import cycle because aicommon uses aiprojection for observation.
const (
	frozenBoundaryTagName  = "AI_CACHE_FROZEN"
	frozenBoundaryNonce    = "semi-dynamic"
	frozenBoundaryStartTag = "<|" + frozenBoundaryTagName + "_" + frozenBoundaryNonce + "|>"
	frozenBoundaryEndTag   = "<|" + frozenBoundaryTagName + "_END_" + frozenBoundaryNonce + "|>"
)

// The semi boundary follows frozen and precedes the open tail.
const (
	semiBoundaryTagName  = "AI_CACHE_SEMI"
	semiBoundaryNonce    = "semi"
	semiBoundaryStartTag = "<|" + semiBoundaryTagName + "_" + semiBoundaryNonce + "|>"
	semiBoundaryEndTag   = "<|" + semiBoundaryTagName + "_END_" + semiBoundaryNonce + "|>"
)

// The semi2 boundary separates two semi-dynamic user messages.
const (
	semi2BoundaryTagName  = "AI_CACHE_SEMI2"
	semi2BoundaryNonce    = "semi"
	semi2BoundaryStartTag = "<|" + semi2BoundaryTagName + "_" + semi2BoundaryNonce + "|>"
	semi2BoundaryEndTag   = "<|" + semi2BoundaryTagName + "_END_" + semi2BoundaryNonce + "|>"
)
