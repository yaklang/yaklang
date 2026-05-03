package aicache

import (
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon/aitag"
	"github.com/yaklang/yaklang/common/ai/aispec"
)

// aicache hijacker 把 prompt 中的 high-static 段切出来，包成 role:system
// 单独的 ChatDetail，剩余内容继续作为 user 消息发出。语义上等价于：
//
//	[
//	  {role: "system", content: "<|AI_CACHE_SYSTEM_high-static|>\n...原文...\n<|AI_CACHE_SYSTEM_END_high-static|>"},
//	  {role: "user",   content: "<原 prompt 去掉 high-static 段后剩余的 semi-dynamic + timeline + dynamic>"}
//	]
//
// 设计意图：把"跨调用稳定的内容"用 role 边界与上游 LLM 对齐，让上游能更
// 容易识别为系统级缓存内容；同时保留原 PROMPT_SECTION 标签结构在 user
// 消息中，aibalance flatten 后字节序仍然稳定。
//
// 关键词: aicache, hijacker, high-static, AI_CACHE_SYSTEM, role:system

const (
	// aicacheSystemTagName 是包装 high-static 段所用的 AITAG tag name
	aicacheSystemTagName = "AI_CACHE_SYSTEM"
	// aicacheSystemNonce 是包装 high-static 段所用的 AITAG nonce
	aicacheSystemNonce = "high-static"
)

// hijackHighStatic 尝试把 msg 中的 high-static 段切出来构造 [system, user] 消息对
//
// 触发条件：
//  1. msg 中存在至少一个 <|AI_CACHE_SYSTEM_high-static|>...<|AI_CACHE_SYSTEM_END_high-static|>
//     或老形态 <|PROMPT_SECTION_high-static|>...<|PROMPT_SECTION_END_high-static|> 块
//  2. 切出 high-static 之后剩余 user 内容不为空
//
// 不满足任一触发条件时返回 nil（透传），不会破坏既有路径。
//
// 关键词: aicache, hijackHighStatic, role:system 注入, AI_CACHE_SYSTEM 双标签兼容
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
		userBuf     strings.Builder
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
		// text block 与其他 tagged block 全部按 Raw 拼回 user buffer，
		// 保持原 PROMPT_SECTION 标签结构（让上游缓存与 aicache 自身的
		// 后续 Split 都能继续识别）
		userBuf.WriteString(blk.Raw)
		if blk.IsTagged() {
			hasOther = true
		} else if strings.TrimSpace(blk.Content) != "" {
			hasOther = true
		}
	}

	if len(staticParts) == 0 {
		return nil
	}
	userContent := strings.TrimSpace(userBuf.String())
	// 没有任何"非 high-static"内容时，发单条 system 没意义，透传
	if userContent == "" || !hasOther {
		return nil
	}

	systemContent := wrapAICacheSystem(staticParts)
	return &aispec.ChatBaseMirrorResult{
		IsHijacked: true,
		Messages: []aispec.ChatDetail{
			aispec.NewSystemChatDetail(systemContent),
			aispec.NewUserChatDetail(userContent),
		},
	}
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
