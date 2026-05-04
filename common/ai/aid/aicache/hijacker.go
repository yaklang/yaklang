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
//	  {role: "system", content: []*ChatContent{{...high-static..., CacheControl: ephemeral}}},  // 主动打 cc
//	  {role: "user",   content: []*ChatContent{{...semi-dynamic + frozen timeline..., CacheControl: ephemeral}}},  // 主动打 cc
//	  {role: "user",   content: "<最末 open timeline 块 + dynamic + 散文>"}, // 易变段, string content, 不打 cc
//	]
//
// 设计意图（参见 common/ai/aid/aicache/TONGYI_CACHE_REPORT.md §7.7 + §7.7.7）：
//   - dashscope 显式缓存允许"跨消息双 cc 标记"产生两个独立缓存块，
//     最长前缀（system+frozen-timeline）的命中率显著高于"system 单 cc"
//     方案。E14 r3 实测前缀命中率从 32% 提升到 70%。
//   - 拆分锚点是 timeline section 的"最末一个 interval bucket"——
//     按 README_TIMELINE_GROUPS.md §1/§9 的硬约定，末桶=Open（可能继续
//     扩张），其余 interval/reducer block=Frozen（字节级永远不变）。
//   - 只切割已确认存在 high-static 与可分割的 timeline frozen 部分时
//     才走 3 段路径，否则退化到原 2 段路径以保持向后兼容。
//
// §7.7.7 职责重排：双 cc 由 hijacker 自管，不再依赖 aibalance 注入。
//   - 3 段路径下 hijacker 直接给 system + user1 包成 []*aispec.ChatContent
//     形态并挂 cache_control:{"type":"ephemeral"}; user2 保持 string 不打 cc。
//   - aibalance.RewriteMessagesForExplicitCache 检测到客户端自带 cc 后会
//     完全 pass-through，不再做任何重叠注入，避免双注入风险。
//   - 2 段退化路径（原始 [system, user] 形态）仍由 hijacker 输出 string content,
//     让 aibalance 走"baseline 单 cc 兜底"路径给最末 system 注入 cc。
//   - cc 字段对非 dashscope provider 无副作用（aispec.ChatContent.CacheControl
//     文档承诺"上游若不识别该字段会原样忽略"），因此 hijacker 不需要知道
//     model 是不是 dashscope 显式缓存模型，一律打 cc 即可。
//
// 关键词: aicache, hijacker, high-static, AI_CACHE_SYSTEM, role:system,
//        timeline frozen open 切分, 3 段拆分, 双 cc, §7.7, §7.7.7,
//        hijacker 自管 cc, ephemeral cache_control

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
//   - system = high-static 包装后内容, 主动打 ephemeral cc (缓存 system 前缀)
//   - user1 = "稳定前缀段" (含开头到 frozen 边界 END 标签自身), 主动打
//     ephemeral cc (缓存 system + frozen 长前缀)
//   - user2 = "易变尾段" (frozen 边界 END 之后到末尾), string content 不打 cc
//
// 切割锚点 (优先级从高到低):
//
//  1. **frozen boundary 标签** (主路径, §7.7.8):
//     由 aicommon.TimelineRenderableBlocks.RenderWithFrozenBoundary 在
//     prompt 中插入的 <|AI_CACHE_FROZEN_semi-dynamic|>...
//     <|AI_CACHE_FROZEN_END_semi-dynamic|> 标签对。一旦发现这对标签,
//     直接用字符串 IndexOf 切割, 无需深入解析 timeline 内部结构。这是
//     最精准、最安全的切割方式 (字节边界由上游 Render 一次性写入)。
//
//  2. **timeline 内部 frozen/open 解析** (退化路径, §7.7):
//     当 prompt 中没有 frozen boundary 标签 (例如老版本 caller 直接走
//     裸 Render 没插边界), 退化到原 splitTimelineFrozenOpen 路径, 通过
//     解析 timeline section 内嵌的 <|TIMELINE_xxx|> 子标签按 last-b-is-open
//     约定切分。
//
//  3. **均失败** -> 返回 nil 让上游退化到 2 段。
//
// §7.7.7 职责重排: hijacker 自管双 cc, 不再依赖 aibalance rewriter 注入;
// aibalance 检测到自带 cc 后 pass-through, 不做重叠注入。
//
// 关键词: aicache, hijacker, 3 段拆分, build3SegmentMessages, 双 cc 自管,
//        frozen boundary 优先, timeline 内部解析退化, §7.7.7, §7.7.8
func build3SegmentMessages(res *aitag.SplitResult, systemContent string, timelineBlk *aitag.Block) *aispec.ChatBaseMirrorResult {
	// 主路径: 先把所有非 high-static block.Raw 顺序拼接, 再用 frozen
	// boundary 标签切割。这样不依赖 timeline 是否被识别为 PROMPT_SECTION,
	// 任何 caller 通过插入边界标签都能触发缓存友好切分。
	user1, user2, ok := splitByFrozenBoundary(res)
	if !ok {
		// 退化路径: timeline 内部解析
		if timelineBlk == nil {
			return nil
		}
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
		user1 = strings.TrimSpace(u1.String())
		user2 = strings.TrimSpace(u2.String())
		if user1 == "" || user2 == "" {
			return nil
		}
	}

	return &aispec.ChatBaseMirrorResult{
		IsHijacked: true,
		Messages: []aispec.ChatDetail{
			{Role: "system", Content: wrapTextWithEphemeralCC(systemContent)},
			{Role: "user", Content: wrapTextWithEphemeralCC(user1)},
			aispec.NewUserChatDetail(user2),
		},
	}
}

// frozenBoundaryTagName / frozenBoundaryNonce / frozenBoundaryStartTag /
// frozenBoundaryEndTag 是 aicache hijacker 用来识别上游 aicommon
// (TimelineRenderableBlocks.RenderWithFrozenBoundary) 注入的"frozen 段"
// 边界标签所需的字面量。
//
// 注意: 这里没有直接 import aicommon 包是为了避免反向 import cycle
// (aicommon 已经依赖 aicache 做缓存观察集成, 见 aicommon/aicache_init.go)。
// 因此用本地常量副本, 字面量必须与 aicommon.TimelineFrozenBoundaryTagName /
// TimelineFrozenBoundaryNonce 严格一致。如果 aicommon 那边改名, 此处也必须
// 同步改, 否则两边对不上 hijacker 找不到边界会退化到 timeline 内部解析
// (功能仍可用但失去 §7.7.8 主路径的精度)。
//
// 关键词: aicache, frozen boundary 字面量, AI_CACHE_FROZEN, semi-dynamic,
//        与 aicommon.TimelineFrozenBoundaryTagName 同步
const (
	frozenBoundaryTagName = "AI_CACHE_FROZEN"
	frozenBoundaryNonce   = "semi-dynamic"
	frozenBoundaryStartTag = "<|" + frozenBoundaryTagName + "_" + frozenBoundaryNonce + "|>"
	frozenBoundaryEndTag   = "<|" + frozenBoundaryTagName + "_END_" + frozenBoundaryNonce + "|>"
)

// splitByFrozenBoundary 在所有非 high-static block.Raw 顺序拼接后的字符串中,
// 用 <|AI_CACHE_FROZEN_semi-dynamic|>...<|AI_CACHE_FROZEN_END_semi-dynamic|>
// 标签对做切割:
//
//   - user1 = startIdx==0 时整个 frozen 段 (含 START + 内容 + END 标签自身),
//     startIdx>0 时还包含从开头到 START 标签之前的所有内容 (如 semi-dynamic)
//   - user2 = END 标签之后的所有内容 (open + dynamic + 散文)
//
// 必要条件:
//   - 同时找到 START 与 END 标签 (一对完整)
//   - END 在 START 之后
//   - user1 与 user2 都非空 (TrimSpace 后)
// 任一条件不满足 -> ok=false 让上游退化路径。
//
// 用户给出的典型场景 (CACHE_BOUNDARY_GUIDE.md §1):
//
//	A-system  -> system + cc
//	B-semi-static
//	<|AI_CACHE_FROZEN_semi-dynamic|>
//	<Timeline-Reducer>
//	<Timeline-ITEM1>
//	<Timeline-ITEM2>
//	<|AI_CACHE_FROZEN_END_semi-dynamic|>
//	<Timeline-ITEM3-Open>
//	D
//	E
//	F
//
// 切完:
//
//	user1 = "B-semi-static\n<|AI_CACHE_FROZEN_semi-dynamic|>\n<Timeline-Reducer>
//	         \n<Timeline-ITEM1>\n<Timeline-ITEM2>\n<|AI_CACHE_FROZEN_END_semi-dynamic|>"  + cc
//	user2 = "<Timeline-ITEM3-Open>\nD\nE\nF"
//
// user1 包含 END 标签自身是关键: 它让 user1 拥有干净的字节边界 (END 标签
// 字面量恒定), 任何 user2 内容变化都不会影响 user1 字节序列, 从而 dashscope
// 能以 user1 整体作为 prefix 命中缓存。
//
// 关键词: aicache, splitByFrozenBoundary, AI_CACHE_FROZEN 字符串 IndexOf 切割,
//        prefix 字节边界, hijacker §7.7.8 主路径
func splitByFrozenBoundary(res *aitag.SplitResult) (user1, user2 string, ok bool) {
	if res == nil {
		return "", "", false
	}

	var allBuf strings.Builder
	for _, blk := range res.GetOrderedBlocks() {
		if blk == nil || isHighStaticBlock(blk) {
			continue
		}
		allBuf.WriteString(blk.Raw)
	}
	all := allBuf.String()
	if all == "" {
		return "", "", false
	}

	startIdx := strings.Index(all, frozenBoundaryStartTag)
	if startIdx < 0 {
		return "", "", false
	}
	endRel := strings.Index(all[startIdx+len(frozenBoundaryStartTag):], frozenBoundaryEndTag)
	if endRel < 0 {
		return "", "", false
	}
	endIdx := startIdx + len(frozenBoundaryStartTag) + endRel + len(frozenBoundaryEndTag)

	user1 = strings.TrimSpace(all[:endIdx])
	user2 = strings.TrimSpace(all[endIdx:])
	if user1 == "" || user2 == "" {
		return "", "", false
	}
	return user1, user2, true
}

// wrapTextWithEphemeralCC 把一段文本包成单元素 []*aispec.ChatContent,
// 并挂上 cache_control:{"type":"ephemeral"} 标记。这是 §7.7.7 hijacker
// 自管双 cc 的标准包装方式: system / user1 都用这个 helper 输出 content,
// 由此触发 dashscope 显式缓存命名缓存块创建 (5 分钟 TTL)。
//
// 字面量字段顺序与 aibalance.ephemeralCacheControl() 保持一致, 让
// aibalance.messagesAlreadyHaveCacheControl 能稳定识别为"客户端自带 cc"
// 并完整退让。
//
// 关键词: aicache, wrapTextWithEphemeralCC, ephemeral cache_control,
//        hijacker 自管 cc, ChatContent 包装
func wrapTextWithEphemeralCC(text string) []*aispec.ChatContent {
	return []*aispec.ChatContent{
		{
			Type:         "text",
			Text:         text,
			CacheControl: map[string]any{"type": "ephemeral"},
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

// isTimelineSectionBlock 判断一个顶层 aitag block 是否是 timeline section。
// 同时识别两种 section 包装:
//   - <|PROMPT_SECTION_timeline|>...      老路径合并 timeline 段
//   - <|PROMPT_SECTION_timeline-open|>... 新路径仅含 open 尾段 (frozen 部分被
//     单独迁到 <|AI_CACHE_FROZEN_semi-dynamic|> 块, 不再走 timeline section)
//
// 两种 nonce 都纳入 timeline 识别, 是为了 build3SegmentMessages 退化路径
// (splitByFrozenBoundary 没找到 frozen 边界时) 能够回到 splitTimelineFrozenOpen
// 通过解析 inner <|TIMELINE_xxx|> 子块按 last-b-is-open 切分。新路径下若出现
// 这种退化 (frozen 块为空), 实际 timeline-open 内只有一个 b 桶 + workspace,
// 切分会自然退化到 2 段, 安全无副作用。
//
// 关键词: aicache, isTimelineSectionBlock, PROMPT_SECTION_timeline 识别,
//        PROMPT_SECTION_timeline-open 识别
func isTimelineSectionBlock(blk *aitag.Block) bool {
	if blk == nil || !blk.IsTagged() {
		return false
	}
	if blk.TagName != tagPromptSection {
		return false
	}
	return blk.Nonce == SectionTimeline || blk.Nonce == SectionTimelineOpen
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
