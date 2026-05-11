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

	// 切分优先级 (高到低):
	//   1. 5 段 (frozen + semi-1 + semi-2 三对边界都齐): build5SegmentMessages
	//   2. 4 段 (frozen + semi 两条边界都齐): build4SegmentMessages
	//   3. 3 段 (仅 frozen 边界, 或 timeline 内部解析): build3SegmentMessages
	//   4. 2 段 (兼容退化): build2SegmentMessages
	// 关键词: hijack 切分优先级, 5 段优先, 4 段 SEMI 单 cc, 3 段 frozen-only, 2 段兼容
	if five := build5SegmentMessages(res, systemContent); five != nil {
		return five
	}
	if four := build4SegmentMessages(res, systemContent); four != nil {
		return four
	}
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

	// P2.1 短 prompt 阈值合并: user1 (frozen 段) < 1KB 时单独打 cc 没意义
	// (上游 dashscope 1024 token 阈值不会建块), 直接返回 nil 让上游退化到
	// build2SegmentMessages 路径 (system 单 cc + 全 user 不打 cc), 避免浪费
	// 一个 cc slot 元数据.
	// 关键词: P2.1, build3 阈值合并, 短 frozen 段旁路
	if len(user1) < minCachableUserSegmentBytes {
		return nil
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
	frozenBoundaryTagName  = "AI_CACHE_FROZEN"
	frozenBoundaryNonce    = "semi-dynamic"
	frozenBoundaryStartTag = "<|" + frozenBoundaryTagName + "_" + frozenBoundaryNonce + "|>"
	frozenBoundaryEndTag   = "<|" + frozenBoundaryTagName + "_END_" + frozenBoundaryNonce + "|>"
)

// semiBoundaryTagName / semiBoundaryNonce / semiBoundaryStartTag /
// semiBoundaryEndTag 是 aicache hijacker 用来识别 aireact 主路径在
// "semi-dynamic 段外层"插入的二级 cache 边界字面量, 形成
// <|AI_CACHE_SEMI_semi|>...<|AI_CACHE_SEMI_END_semi|>.
//
// 与 aicommon.SemiDynamicCacheBoundaryTagName / SemiDynamicCacheBoundaryNonce
// 严格一致 (本地副本, 避免反向 import cycle).
//
// 命中时 hijacker 切成 4 段:
//   - system (high-static, ephemeral cc)
//   - user1 (frozen 段, ephemeral cc)
//   - user2 (semi 段, ephemeral cc)
//   - user3 (open + dynamic, 无 cc)
// frozen 边界缺失时只能切 3 段 (走 build3SegmentMessages 老路径), 不影响
// semi 边界存在的退化能力.
//
// 关键词: aicache, semi boundary 字面量, AI_CACHE_SEMI, 4 段切分, 双 cc
const (
	semiBoundaryTagName  = "AI_CACHE_SEMI"
	semiBoundaryNonce    = "semi"
	semiBoundaryStartTag = "<|" + semiBoundaryTagName + "_" + semiBoundaryNonce + "|>"
	semiBoundaryEndTag   = "<|" + semiBoundaryTagName + "_END_" + semiBoundaryNonce + "|>"
)

// semi2BoundaryTagName / semi2BoundaryNonce / semi2BoundaryStartTag /
// semi2BoundaryEndTag 是 aicache hijacker 用来识别 aireact 主路径 P1.1 把
// "semi-dynamic 段第二块"再单独包一层 cache 边界的字面量, 形成
// <|AI_CACHE_SEMI2_semi|>...<|AI_CACHE_SEMI2_END_semi|>.
//
// 与 aicommon.SemiDynamicPart2CacheBoundaryTagName / SemiDynamicPart2CacheBoundaryNonce
// 严格一致 (本地副本, 避免反向 import cycle).
//
// 命中时 hijacker 切成 5 段:
//   - system (high-static, ephemeral cc)
//   - user1 (frozen 段, ephemeral cc)
//   - user2 (semi-1 段, 无 cc)
//   - user3 (semi-2 段, ephemeral cc)
//   - user4 (open + dynamic, 无 cc)
// SEMI2 边界缺失时回退到 4 段 (走 build4SegmentMessages 老路径).
//
// 关键词: aicache, semi2 boundary 字面量, AI_CACHE_SEMI2, 5 段切分,
//        semi-1 无 cc, semi-2 ephemeral cc, P1.1
const (
	semi2BoundaryTagName  = "AI_CACHE_SEMI2"
	semi2BoundaryNonce    = "semi"
	semi2BoundaryStartTag = "<|" + semi2BoundaryTagName + "_" + semi2BoundaryNonce + "|>"
	semi2BoundaryEndTag   = "<|" + semi2BoundaryTagName + "_END_" + semi2BoundaryNonce + "|>"
)

// minCachableUserSegmentBytes 是 user 段单独打 ephemeral cc 的最小字节阈值.
// 低于该阈值的 user 段无论 cc 与否都不会被 dashscope 建块 (TONGYI_CACHE_REPORT.md
// §4.4 E4 实测: 733 token ~3KB 即低于上游 1024 token 阈值, cache_creation=0,
// 客户端 prefix_hit_ratio 接近 100% 但上游 cached_tokens=0, 是典型的"白打 cc").
//
// P2.1 短 prompt 旁路策略 (按用户给定的 1KB 保守阈值):
//   - 4 段路径下 user1 (frozen 段) < 1KB → 合并到 user2, 退化为 3 段 (sys + u12 + u3)
//   - 合并后 user1+user2 < 1KB → 全合并到 user3, 退化为 2 段 (sys + all_user, 仅 system 打 cc)
//   - 3 段路径下 user1 < 1KB → 退化到 build2SegmentMessages (返回 nil 让上游降级)
//
// 1024 byte ~ 256 token 远低于上游 1024 token 实际阈值, 用 byte 而非 token 估算
// 是为了无依赖 / 低开销 / 字节边界精确; 命中 < 1KB 阈值的 user 段一定无法触发
// 上游建块, 合并旁路至少避免浪费一个 cache slot 元数据.
//
// 注意: 这是包级 var (而非 const), 仅为了让单测可以临时覆盖默认值 (设为 0
// 关闭阈值合并以测试 happy path), 生产代码绝对不能改写该值. 不导出到包外.
//
// 关键词: aicache, P2.1, 短 prompt 阈值, 合并旁路, 1024 byte, dashscope 1024 token
var minCachableUserSegmentBytes = 1024

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

// splitBySemiBoundary 在所有非 high-static block.Raw 顺序拼接后的字符串中,
// 同时找到 frozen 边界 + semi 边界两对完整标签, 切成 3 段:
//
//   - user1: 开头 ~ frozen END 标签 (含)               -> 给 build4 的 user1
//   - user2: frozen END 后 ~ semi END 标签 (含)        -> 给 build4 的 user2
//   - user3: semi END 之后到末尾                       -> 给 build4 的 user3
//
// 必要条件:
//   - frozen START / END 都存在, frozen END 在 frozen START 之后
//   - semi START / END 都存在, semi START 在 frozen END 之后, semi END 在
//     semi START 之后
//   - user1 / user2 / user3 三段都非空 (TrimSpace 后)
// 任一条件不满足 -> ok=false 让上游退化到 3 段或 2 段.
//
// 用户给出的典型 prompt 形态 (CACHE_BOUNDARY_GUIDE.md §4.5 双 cache 边界场景):
//
//	A-system  -> system + cc
//	<|AI_CACHE_FROZEN_semi-dynamic|>
//	<Tool/Forge/Timeline-Frozen>
//	<|AI_CACHE_FROZEN_END_semi-dynamic|>
//	<|AI_CACHE_SEMI_semi|>
//	<Skills + Schema + CacheToolCall>
//	<|AI_CACHE_SEMI_END_semi|>
//	<Timeline-Open>
//	<Dynamic>
//
// 切完:
//
//	user1 = "<|AI_CACHE_FROZEN_semi-dynamic|>\n<frozen body>
//	         \n<|AI_CACHE_FROZEN_END_semi-dynamic|>"             + cc
//	user2 = "<|AI_CACHE_SEMI_semi|>\n<semi body>
//	         \n<|AI_CACHE_SEMI_END_semi|>"                       + cc
//	user3 = "<Timeline-Open>\n<Dynamic>"
//
// 关键词: aicache, splitBySemiBoundary, AI_CACHE_SEMI, 4 段 prefix 字节边界
func splitBySemiBoundary(res *aitag.SplitResult) (user1, user2, user3 string, ok bool) {
	if res == nil {
		return "", "", "", false
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
		return "", "", "", false
	}

	frozenStart := strings.Index(all, frozenBoundaryStartTag)
	if frozenStart < 0 {
		return "", "", "", false
	}
	frozenEndRel := strings.Index(all[frozenStart+len(frozenBoundaryStartTag):], frozenBoundaryEndTag)
	if frozenEndRel < 0 {
		return "", "", "", false
	}
	frozenEnd := frozenStart + len(frozenBoundaryStartTag) + frozenEndRel + len(frozenBoundaryEndTag)

	semiStart := strings.Index(all[frozenEnd:], semiBoundaryStartTag)
	if semiStart < 0 {
		return "", "", "", false
	}
	semiStart += frozenEnd
	semiEndRel := strings.Index(all[semiStart+len(semiBoundaryStartTag):], semiBoundaryEndTag)
	if semiEndRel < 0 {
		return "", "", "", false
	}
	semiEnd := semiStart + len(semiBoundaryStartTag) + semiEndRel + len(semiBoundaryEndTag)

	user1 = strings.TrimSpace(all[:frozenEnd])
	user2 = strings.TrimSpace(all[frozenEnd:semiEnd])
	user3 = strings.TrimSpace(all[semiEnd:])
	if user1 == "" || user2 == "" || user3 == "" {
		return "", "", "", false
	}
	return user1, user2, user3, true
}

// splitBySemi2Boundary 在所有非 high-static block.Raw 顺序拼接后的字符串中,
// 同时找到 frozen 边界 + semi 边界 + semi2 边界三对完整标签, 切成 4 段:
//
//   - user1: 开头 ~ frozen END 标签 (含)               -> 给 build5 的 user1
//   - user2: frozen END 后 ~ semi END 标签 (含)        -> 给 build5 的 user2 (semi-1)
//   - user3: semi END 后 ~ semi2 END 标签 (含)         -> 给 build5 的 user3 (semi-2)
//   - user4: semi2 END 之后到末尾                       -> 给 build5 的 user4
//
// 必要条件:
//   - frozen START / END 都存在, frozen END 在 frozen START 之后
//   - semi START / END 都存在, semi START 在 frozen END 之后, semi END 在 semi START 之后
//   - semi2 START / END 都存在, semi2 START 在 semi END 之后, semi2 END 在 semi2 START 之后
//   - user1 / user2 / user3 / user4 四段都非空 (TrimSpace 后)
// 任一条件不满足 -> ok=false 让上游退化到 4 段或更低.
//
// 用户给出的典型 prompt 形态 (CACHE_BOUNDARY_GUIDE.md §4.5+ 三 cache 边界场景):
//
//	A-system  -> system + cc
//	<|AI_CACHE_FROZEN_semi-dynamic|>
//	<Tool/Forge/Timeline-Frozen>
//	<|AI_CACHE_FROZEN_END_semi-dynamic|>
//	<|AI_CACHE_SEMI_semi|>
//	<Skills + RecentToolsCache>
//	<|AI_CACHE_SEMI_END_semi|>
//	<|AI_CACHE_SEMI2_semi|>
//	<Persistent + Schema + OutputExample>
//	<|AI_CACHE_SEMI2_END_semi|>
//	<Timeline-Open>
//	<Dynamic>
//
// 关键词: aicache, splitBySemi2Boundary, AI_CACHE_SEMI2, 5 段 prefix 字节边界, P1.1
func splitBySemi2Boundary(res *aitag.SplitResult) (user1, user2, user3, user4 string, ok bool) {
	if res == nil {
		return "", "", "", "", false
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
		return "", "", "", "", false
	}

	frozenStart := strings.Index(all, frozenBoundaryStartTag)
	if frozenStart < 0 {
		return "", "", "", "", false
	}
	frozenEndRel := strings.Index(all[frozenStart+len(frozenBoundaryStartTag):], frozenBoundaryEndTag)
	if frozenEndRel < 0 {
		return "", "", "", "", false
	}
	frozenEnd := frozenStart + len(frozenBoundaryStartTag) + frozenEndRel + len(frozenBoundaryEndTag)

	semiStart := strings.Index(all[frozenEnd:], semiBoundaryStartTag)
	if semiStart < 0 {
		return "", "", "", "", false
	}
	semiStart += frozenEnd
	semiEndRel := strings.Index(all[semiStart+len(semiBoundaryStartTag):], semiBoundaryEndTag)
	if semiEndRel < 0 {
		return "", "", "", "", false
	}
	semiEnd := semiStart + len(semiBoundaryStartTag) + semiEndRel + len(semiBoundaryEndTag)

	semi2Start := strings.Index(all[semiEnd:], semi2BoundaryStartTag)
	if semi2Start < 0 {
		return "", "", "", "", false
	}
	semi2Start += semiEnd
	semi2EndRel := strings.Index(all[semi2Start+len(semi2BoundaryStartTag):], semi2BoundaryEndTag)
	if semi2EndRel < 0 {
		return "", "", "", "", false
	}
	semi2End := semi2Start + len(semi2BoundaryStartTag) + semi2EndRel + len(semi2BoundaryEndTag)

	user1 = strings.TrimSpace(all[:frozenEnd])
	user2 = strings.TrimSpace(all[frozenEnd:semiEnd])
	user3 = strings.TrimSpace(all[semiEnd:semi2End])
	user4 = strings.TrimSpace(all[semi2End:])
	if user1 == "" || user2 == "" || user3 == "" || user4 == "" {
		return "", "", "", "", false
	}
	return user1, user2, user3, user4, true
}

// build5SegmentMessages 试图走"5 段缓存友好拆分路径" (P1.1):
//
//   - system = high-static 包装后内容, 主动打 ephemeral cc
//   - user1 = 开头 ~ frozen END (含), 主动打 ephemeral cc (frozen prefix 缓存)
//   - user2 = frozen END 后 ~ semi END (含), string content 不打 cc (semi-1 段)
//   - user3 = semi END 后 ~ semi2 END (含), 主动打 ephemeral cc (semi-1+semi-2 合并 prefix 缓存)
//   - user4 = semi2 END 之后到末尾, string content 不打 cc (易变尾段)
//
// 切割锚点: frozen + semi + semi2 三对完整边界都存在. 任一缺失 -> 返回 nil
// 让上游退化到 build4SegmentMessages (老 SEMI 单 cc 路径).
//
// 设计意图: dashscope cc 是 prefix-cache 命中点; semi-1 不打 cc, prefix 仍跨过其
// 字节序列直达 semi-2 cc 处, 等价于把 semi 当作合并 prefix 计算缓存; 物理上仍
// 是两条 user message, 让上游 UI 字节统计 / caller 端观测树各自展示一组语义分块.
//
// 短 prompt 阈值合并: user1 (frozen) < 1KB 时退化到 4 段路径让 build4 处理 SEMI
// 单 cc; semi-1 (user2) 不打 cc 所以无需阈值检查; semi-2 (user3) < 1KB 时也退化
// 到 4 段, 因为单独打 cc 触不到上游 1024 token 建块阈值.
//
// 关键词: aicache, hijacker, 5 段拆分, build5SegmentMessages, P1.1, semi 拆两条 message
func build5SegmentMessages(res *aitag.SplitResult, systemContent string) *aispec.ChatBaseMirrorResult {
	user1, user2, user3, user4, ok := splitBySemi2Boundary(res)
	if !ok {
		return nil
	}

	// 阈值检查: user1 (frozen) 与 user3 (semi-2) 都需要打 cc, 任一段太短都让
	// 上游退化到 build4SegmentMessages (SEMI 单 cc 合并形态), 避免浪费 cc 元数据.
	// user2 (semi-1) 与 user4 (open+dynamic) 不打 cc, 不参与阈值判定.
	if len(user1) < minCachableUserSegmentBytes || len(user3) < minCachableUserSegmentBytes {
		return nil
	}

	return &aispec.ChatBaseMirrorResult{
		IsHijacked: true,
		Messages: []aispec.ChatDetail{
			{Role: "system", Content: wrapTextWithEphemeralCC(systemContent)},
			{Role: "user", Content: wrapTextWithEphemeralCC(user1)},
			aispec.NewUserChatDetail(user2),
			{Role: "user", Content: wrapTextWithEphemeralCC(user3)},
			aispec.NewUserChatDetail(user4),
		},
	}
}

// build4SegmentMessages 试图走"4 段缓存友好拆分路径":
//
//   - system = high-static 包装后内容, 主动打 ephemeral cc
//   - user1 = 开头 ~ frozen END (含), 主动打 ephemeral cc (frozen prefix 缓存)
//   - user2 = frozen END 后 ~ semi END (含), 主动打 ephemeral cc (semi 段缓存)
//   - user3 = semi END 之后到末尾, string content 不打 cc (易变尾段)
//
// 切割锚点: frozen 边界 + semi 边界两对完整标签都存在. 任一缺失 -> 返回 nil
// 让上游退化到 build3SegmentMessages.
//
// P2.1 短 prompt 阈值合并 (minCachableUserSegmentBytes = 1024):
//   - user1 < 1KB: 把 user1 内容合并到 user2 → 退化到 3 段 (sys cc + u12 cc + u3)
//   - user1+user2 < 1KB: 再合并 user3 → 退化到 2 段 (sys cc + all_user 无 cc, 仅 system 缓存)
//   - 阈值检查在 splitBySemiBoundary 成功后做, 用 len(byte) 而非 token, 见
//     minCachableUserSegmentBytes 注释.
//
// 关键词: aicache, hijacker, 4 段拆分, build4SegmentMessages, 双 cc 自管,
//        frozen + semi 双边界, P1 双 cache 边界, P2.1 阈值合并
func build4SegmentMessages(res *aitag.SplitResult, systemContent string) *aispec.ChatBaseMirrorResult {
	user1, user2, user3, ok := splitBySemiBoundary(res)
	if !ok {
		return nil
	}

	// P2.1 阶段 1: user1 (frozen 段) < 阈值 → 合并到 user2 退化 3 段
	// frozen 段太短单独打 cc 触不到上游 1024 token 建块阈值, 合并让 semi 段独占 cc,
	// 至少保住 semi 段的 prefix cache 命中.
	if len(user1) < minCachableUserSegmentBytes {
		merged := strings.TrimSpace(user1 + "\n" + user2)
		if merged == "" {
			merged = strings.TrimSpace(user1 + user2)
		}

		// P2.1 阶段 2: 合并后 user1+user2 仍 < 阈值 → 全合并 user3 退化 2 段
		// 全 user 段太短打 cc 也不会触发建块, 让 user 段透传, 仅 system 打 cc.
		if len(merged) < minCachableUserSegmentBytes {
			allUser := strings.TrimSpace(merged + "\n" + user3)
			if allUser == "" {
				return nil
			}
			return &aispec.ChatBaseMirrorResult{
				IsHijacked: true,
				Messages: []aispec.ChatDetail{
					{Role: "system", Content: wrapTextWithEphemeralCC(systemContent)},
					aispec.NewUserChatDetail(allUser),
				},
			}
		}

		// 3 段降级: sys cc + u12 cc + u3
		return &aispec.ChatBaseMirrorResult{
			IsHijacked: true,
			Messages: []aispec.ChatDetail{
				{Role: "system", Content: wrapTextWithEphemeralCC(systemContent)},
				{Role: "user", Content: wrapTextWithEphemeralCC(merged)},
				aispec.NewUserChatDetail(user3),
			},
		}
	}

	return &aispec.ChatBaseMirrorResult{
		IsHijacked: true,
		Messages: []aispec.ChatDetail{
			{Role: "system", Content: wrapTextWithEphemeralCC(systemContent)},
			{Role: "user", Content: wrapTextWithEphemeralCC(user1)},
			{Role: "user", Content: wrapTextWithEphemeralCC(user2)},
			aispec.NewUserChatDetail(user3),
		},
	}
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
