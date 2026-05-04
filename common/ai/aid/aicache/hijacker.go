package aicache

import (
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon/aitag"
	"github.com/yaklang/yaklang/common/ai/aispec"
)

// aicache hijacker 把 prompt 中的 high-static 段切出来，包成 role:system
// 单独的 ChatDetail。从 §7.7 起进一步把剩余 user 段按 timeline 的
// "Frozen vs Open" 边界拆成两条 user 消息：
//
//	[
//	  {role: "system", content: "<|AI_CACHE_SYSTEM_high-static|>\n...原文...\n<|AI_CACHE_SYSTEM_END_high-static|>"},
//	  {role: "user",   content: "<semi-dynamic + 已封闭 timeline 块>"},   // 缓存友好段，由上游 cache_rewriter 打 cc
//	  {role: "user",   content: "<最末 open timeline 块 + dynamic + 散文>"}, // 易变段，不打 cc
//	]
//
// 设计意图（参见 common/ai/aid/aicache/TONGYI_CACHE_REPORT.md §7.7）：
//   - dashscope 显式缓存允许"跨消息双 cc 标记"产生两个独立缓存块，
//     最长前缀（system+frozen-timeline）的命中率显著高于"system 单 cc"
//     方案。E14 r3 实测前缀命中率从 32% 提升到 70%。
//   - 拆分锚点是 timeline section 的"最末一个 interval bucket"——
//     按 README_TIMELINE_GROUPS.md §1/§9 的硬约定，末桶=Open（可能继续
//     扩张），其余 interval/reducer block=Frozen（字节级永远不变）。
//   - 只切割已确认存在 high-static 与可分割的 timeline frozen 部分时
//     才走 3 段路径，否则退化到原 2 段路径以保持向后兼容。
//
// 关键词: aicache, hijacker, high-static, AI_CACHE_SYSTEM, role:system,
//        timeline frozen open 切分, 3 段拆分, 双 cc, §7.7

const (
	// aicacheSystemTagName 是包装 high-static 段所用的 AITAG tag name
	aicacheSystemTagName = "AI_CACHE_SYSTEM"
	// aicacheSystemNonce 是包装 high-static 段所用的 AITAG nonce
	aicacheSystemNonce = "high-static"

	// timelineInnerTagName 是 timeline 段内部 GroupByMinutes 渲染时使用
	// 的嵌套 aitag 名（见 aicommon/timeline_groups_render.go 与
	// README_TIMELINE_GROUPS.md §4 输出格式）。
	// 关键词: aicache, timeline 内嵌标签, TIMELINE
	timelineInnerTagName = "TIMELINE"

	// timelineIntervalNoncePrefix 是 timeline interval bucket 的 nonce
	// 前缀（"b{N}t{unixSec}"）；reducer 的 nonce 前缀为 "r"。
	// 关键词: aicache, timeline interval block, b 前缀, Open 桶识别
	timelineIntervalNoncePrefix = "b"
)

// hijackHighStatic 尝试把 msg 切成 [system, user] 或 [system, user1, user2]
//
// 触发条件:
//  1. msg 中存在至少一个 <|AI_CACHE_SYSTEM_high-static|>...<|AI_CACHE_SYSTEM_END_high-static|>
//     或老形态 <|PROMPT_SECTION_high-static|>...<|PROMPT_SECTION_END_high-static|> 块
//  2. 切出 high-static 之后剩余 user 内容不为空
//
// 切分模式:
//   - 3 段（缓存友好优先）: 同时存在 high-static 与可拆分 frozen-timeline 段，
//     且拆分后两条 user 消息都非空。
//   - 2 段（兼容退化）: 只满足触发条件 1+2，但不满足 3 段条件。
//
// 不满足任一触发条件时返回 nil（透传），不会破坏既有路径。
//
// 关键词: aicache, hijackHighStatic, role:system 注入, AI_CACHE_SYSTEM 双标签兼容,
//        3 段拆分, 缓存友好优先
func hijackHighStatic(msg string) *aispec.ChatBaseMirrorResult {
	if strings.TrimSpace(msg) == "" {
		return nil
	}

	res, err := aitag.SplitViaTAG(msg, acceptedTagNames...)
	if err != nil || res == nil {
		return nil
	}

	var (
		staticParts []string
		timelineBlk *aitag.Block
		hasOther    bool
	)
	for _, blk := range res.GetOrderedBlocks() {
		if blk == nil {
			continue
		}
		if isHighStaticBlock(blk) {
			staticParts = append(staticParts, blk.Content)
			continue
		}
		if isTimelineSectionBlock(blk) && timelineBlk == nil {
			timelineBlk = blk
			hasOther = true
			continue
		}
		if blk.IsTagged() {
			hasOther = true
		} else if strings.TrimSpace(blk.Content) != "" {
			hasOther = true
		}
	}

	if len(staticParts) == 0 || !hasOther {
		return nil
	}

	systemContent := wrapAICacheSystem(staticParts)

	// 优先走 3 段拆分；若 timeline 不存在或不可拆分，退化到 2 段。
	if three := build3SegmentMessages(res, systemContent, timelineBlk); three != nil {
		return three
	}

	return build2SegmentMessages(res, systemContent)
}

// build2SegmentMessages 走"原始 2 段"兼容路径: 把所有非 high-static block
// 的 Raw 顺序拼成单条 user 消息。
// 关键词: aicache, hijacker, 2 段兼容路径
func build2SegmentMessages(res *aitag.SplitResult, systemContent string) *aispec.ChatBaseMirrorResult {
	var userBuf strings.Builder
	for _, blk := range res.GetOrderedBlocks() {
		if blk == nil || isHighStaticBlock(blk) {
			continue
		}
		userBuf.WriteString(blk.Raw)
	}
	userContent := strings.TrimSpace(userBuf.String())
	if userContent == "" {
		return nil
	}
	return &aispec.ChatBaseMirrorResult{
		IsHijacked: true,
		Messages: []aispec.ChatDetail{
			aispec.NewSystemChatDetail(systemContent),
			aispec.NewUserChatDetail(userContent),
		},
	}
}

// build3SegmentMessages 试图走"3 段"缓存友好拆分路径:
//
//   - user1 = (high-static 之后, timelineBlk 之前的所有 block.Raw) +
//     timelineBlk 内部的 frozen 部分（外面重新包 PROMPT_SECTION_timeline）
//   - user2 = timelineBlk 内部的 open 部分（外面重新包 PROMPT_SECTION_timeline） +
//     (timelineBlk 之后的所有 block.Raw)
//
// 不可拆分（无 timelineBlk / timeline 全 frozen / timeline 全 open / 拆分
// 后任一 user 段为空）时返回 nil 让上游退化到 2 段。
//
// 关键词: aicache, hijacker, 3 段拆分, build3SegmentMessages
func build3SegmentMessages(res *aitag.SplitResult, systemContent string, timelineBlk *aitag.Block) *aispec.ChatBaseMirrorResult {
	if timelineBlk == nil {
		return nil
	}
	frozenWrapped, openWrapped := splitTimelineFrozenOpen(timelineBlk)
	if frozenWrapped == "" || openWrapped == "" {
		return nil
	}

	var (
		u1, u2       strings.Builder
		seenTimeline bool
	)
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

	user1 := strings.TrimSpace(u1.String())
	user2 := strings.TrimSpace(u2.String())
	if user1 == "" || user2 == "" {
		return nil
	}

	return &aispec.ChatBaseMirrorResult{
		IsHijacked: true,
		Messages: []aispec.ChatDetail{
			aispec.NewSystemChatDetail(systemContent),
			aispec.NewUserChatDetail(user1),
			aispec.NewUserChatDetail(user2),
		},
	}
}

// splitTimelineFrozenOpen 把一个 timeline section block 内嵌的若干
// <|TIMELINE_xxx|>...<|TIMELINE_END_xxx|> 子块按 frozen / open 边界
// 切成两半，再分别用 PROMPT_SECTION_timeline 重新包裹，方便分别送到
// user1 / user2 消息中保持上下游对 PROMPT_SECTION 标签的识别能力。
//
// 切分规则（来自 README_TIMELINE_GROUPS.md §1 / §6 / §9）:
//   - reducer block: nonce 形如 "r{key}t{unixSec}"，恒为 Frozen
//   - interval block: nonce 形如 "b{N}t{unixSec}"，仅最末 interval 为 Open
//   - 没有 b* block（全是 reducer）: 没有 Open 段，返回 ("","")
//   - 只有 1 个 b* block 且它是首个 timeline 子块: 没有 Frozen 段，
//     返回 ("","") 让上游走 2 段路径
//
// 关键词: aicache, splitTimelineFrozenOpen, frozen open 边界识别,
//        TIMELINE 内嵌标签, last-b-is-open 约定
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

// isTimelineSectionBlock 判断一个顶层 aitag block 是否是 timeline section
// (PROMPT_SECTION_timeline 包裹整段 timeline 渲染输出)
// 关键词: aicache, isTimelineSectionBlock, PROMPT_SECTION_timeline 识别
func isTimelineSectionBlock(blk *aitag.Block) bool {
	if blk == nil || !blk.IsTagged() {
		return false
	}
	return blk.TagName == tagPromptSection && blk.Nonce == SectionTimeline
}

// wrapAICacheSystem 把多段 high-static 原文按出现顺序拼接，再用
// <|AI_CACHE_SYSTEM_high-static|>...<|AI_CACHE_SYSTEM_END_high-static|>
// 包装。多段之间用一个空行分隔，保持可读性与字节稳定性。
//
// 关键词: aicache, wrapAICacheSystem, AI_CACHE_SYSTEM 包装
func wrapAICacheSystem(parts []string) string {
	body := strings.Join(parts, "\n\n")
	var sb strings.Builder
	sb.WriteString("<|")
	sb.WriteString(aicacheSystemTagName)
	sb.WriteString("_")
	sb.WriteString(aicacheSystemNonce)
	sb.WriteString("|>\n")
	sb.WriteString(body)
	sb.WriteString("\n<|")
	sb.WriteString(aicacheSystemTagName)
	sb.WriteString("_END_")
	sb.WriteString(aicacheSystemNonce)
	sb.WriteString("|>")
	return sb.String()
}
