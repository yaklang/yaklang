package aiprojection

import (
	"strings"

	"github.com/yaklang/yaklang/common/ai/aispec"
)

// Cache projection turns parsed high-static content into a system message.
// Complete frozen, semi, and semi2 boundaries can add stable user messages with
// ephemeral cache_control; otherwise the remaining prompt stays one user message.

const (
	// cacheSystemTagName 是包装 high-static 段所用的 AITAG tag name
	cacheSystemTagName = "AI_CACHE_SYSTEM"
	// cacheSystemNonce 是包装 high-static 段所用的 AITAG nonce
	cacheSystemNonce = "high-static"
)

// projectCache chooses the widest valid cache layout from projection sections.
// It does not parse tags or modify the caller's tool selection.
func projectCache(sections *ProjectionSections) *aispec.ChatBaseHijackResult {
	if sections == nil || strings.TrimSpace(sections.Original) == "" {
		return nil
	}
	parts := &sections.cache
	if len(parts.staticParts) == 0 || !parts.hasOther {
		return nil
	}

	staticContent := make([]string, 0, len(parts.staticParts))
	for _, section := range parts.staticParts {
		staticContent = append(staticContent, section.content)
	}
	systemContent := wrapAICacheSystem(staticContent)

	// 切分优先级 (高到低):
	//   1. 5 段 (frozen + semi-1 + semi-2 三对边界都齐): build5SegmentMessages
	//   2. 4 段 (frozen + semi 两条边界都齐): build4SegmentMessages
	//   3. 3 段 (仅 frozen 边界, 或 timeline 内部解析): build3SegmentMessages
	//   4. 2 段 (兼容退化): build2SegmentMessages
	// 关键词: hijack 切分优先级, 5 段优先, 4 段 SEMI 单 cc, 3 段 frozen-only, 2 段兼容
	if five := build5SegmentMessages(parts, systemContent); five != nil {
		return expandReasoningReplayMessages(five)
	}
	if four := build4SegmentMessages(parts, systemContent); four != nil {
		return expandReasoningReplayMessages(four)
	}
	if three := build3SegmentMessages(parts, systemContent); three != nil {
		return expandReasoningReplayMessages(three)
	}

	return expandReasoningReplayMessages(build2SegmentMessages(parts, systemContent))
}

// build2SegmentMessages preserves all nonstatic prompt bytes in one user message.
func build2SegmentMessages(parts *cacheProjectionSections, systemContent string) *aispec.ChatBaseHijackResult {
	userContent := strings.TrimSpace(parts.userRaw)
	if userContent == "" {
		return nil
	}
	return &aispec.ChatBaseHijackResult{
		IsHijacked: true,
		Messages: []aispec.ChatDetail{
			aispec.NewSystemChatDetail(systemContent),
			aispec.NewUserChatDetail(userContent),
		},
	}
}

// build3SegmentMessages uses a frozen boundary or a parsed legacy timeline.
// The stable user prefix receives cache_control when it meets the size threshold.
func build3SegmentMessages(parts *cacheProjectionSections, systemContent string) *aispec.ChatBaseHijackResult {
	segments := parts.frozen
	if len(segments) == 0 {
		segments = parts.timeline
	}
	if len(segments) != 2 {
		return nil
	}
	user1, user2 := segments[0].Raw, segments[1].Raw

	// P2.1 短 prompt 阈值合并: user1 (frozen 段) < 1KB 时单独打 cc 没意义
	// (上游 dashscope 1024 token 阈值不会建块), 直接返回 nil 让上游退化到
	// build2SegmentMessages 路径 (system 单 cc + 全 user 不打 cc), 避免浪费
	// 一个 cc slot 元数据.
	// 关键词: P2.1, build3 阈值合并, 短 frozen 段旁路
	if len(user1) < minCachableUserSegmentBytes {
		return nil
	}

	return &aispec.ChatBaseHijackResult{
		IsHijacked: true,
		Messages: []aispec.ChatDetail{
			{Role: "system", Content: wrapTextWithEphemeralCC(systemContent)},
			{Role: "user", Content: wrapTextWithEphemeralCC(user1)},
			aispec.NewUserChatDetail(user2),
		},
	}
}

// minCachableUserSegmentBytes avoids adding cache_control to short user parts.
// It is a variable so tests can exercise each layout at small fixture sizes.
var minCachableUserSegmentBytes = 1024

// build5SegmentMessages separates frozen, semi1, semi2, and the open tail.
// Cache markers go on system, frozen, and semi2; semi1 stays a separate message.
func build5SegmentMessages(parts *cacheProjectionSections, systemContent string) *aispec.ChatBaseHijackResult {
	if len(parts.semi2) != 4 {
		return nil
	}
	user1, user2, user3, user4 := parts.semi2[0].Raw, parts.semi2[1].Raw, parts.semi2[2].Raw, parts.semi2[3].Raw

	// 阈值检查: user1 (frozen) 与 user3 (semi-2) 都需要打 cc, 任一段太短都让
	// 上游退化到 build4SegmentMessages (SEMI 单 cc 合并形态), 避免浪费 cc 元数据.
	// user2 (semi-1) 与 user4 (open+dynamic) 不打 cc, 不参与阈值判定.
	if len(user1) < minCachableUserSegmentBytes || len(user3) < minCachableUserSegmentBytes {
		return nil
	}

	return &aispec.ChatBaseHijackResult{
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

// build4SegmentMessages separates frozen, semi, and the open tail.
// Short frozen content is merged into semi before applying cache_control.
func build4SegmentMessages(parts *cacheProjectionSections, systemContent string) *aispec.ChatBaseHijackResult {
	if len(parts.semi) != 3 {
		return nil
	}
	user1, user2, user3 := parts.semi[0].Raw, parts.semi[1].Raw, parts.semi[2].Raw

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
			return &aispec.ChatBaseHijackResult{
				IsHijacked: true,
				Messages: []aispec.ChatDetail{
					{Role: "system", Content: wrapTextWithEphemeralCC(systemContent)},
					aispec.NewUserChatDetail(allUser),
				},
			}
		}

		// 3 段降级: sys cc + u12 cc + u3
		return &aispec.ChatBaseHijackResult{
			IsHijacked: true,
			Messages: []aispec.ChatDetail{
				{Role: "system", Content: wrapTextWithEphemeralCC(systemContent)},
				{Role: "user", Content: wrapTextWithEphemeralCC(merged)},
				aispec.NewUserChatDetail(user3),
			},
		}
	}

	return &aispec.ChatBaseHijackResult{
		IsHijacked: true,
		Messages: []aispec.ChatDetail{
			{Role: "system", Content: wrapTextWithEphemeralCC(systemContent)},
			{Role: "user", Content: wrapTextWithEphemeralCC(user1)},
			{Role: "user", Content: wrapTextWithEphemeralCC(user2)},
			aispec.NewUserChatDetail(user3),
		},
	}
}

// wrapTextWithEphemeralCC marks a stable prefix for explicit provider caching.
func wrapTextWithEphemeralCC(text string) []*aispec.ChatContent {
	return []*aispec.ChatContent{
		{
			Type:         "text",
			Text:         text,
			CacheControl: map[string]any{"type": "ephemeral"},
		},
	}
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
	sb.WriteString(cacheSystemTagName)
	sb.WriteString("_")
	sb.WriteString(cacheSystemNonce)
	sb.WriteString("|>\n")
	sb.WriteString(body)
	sb.WriteString("\n<|")
	sb.WriteString(cacheSystemTagName)
	sb.WriteString("_END_")
	sb.WriteString(cacheSystemNonce)
	sb.WriteString("|>")
	return sb.String()
}
