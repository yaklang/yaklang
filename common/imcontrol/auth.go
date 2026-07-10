package imcontrol

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/yaklang/yaklang/common/notify"
	"github.com/yaklang/yaklang/common/notify/credential"
)

// checkPermission 校验一个用户是否有权使用某平台的 bot。
// 返回 (allowed, reason)；allowed=true 时 reason 为空。
//
// 校验顺序（任一不通过即拒绝）：
//  1. AllowedChats 非空且 chatID 不在列表 → 拒绝
//  2. 群聊/话题默认允许所有群成员使用；仅 GroupAccessControl=true 时要求 OwnerID 或 AllowedUsers 命中
//  3. 私聊保留 owner-only 行为：OwnerID 非空且 senderID != OwnerID 且 AllowedUsers 为空 → 拒绝
//  4. 全空 = 放行（向后兼容默认值）
//
// 普通消息和卡片回调共用本校验（文档明确"交互卡片 callback 同样必须校验权限"）。
func (e *Engine) checkPermission(platform, chatID, senderID string) (bool, string) {
	bot, err := credential.GetBotConfig(platform)
	if err != nil || bot == nil {
		// 无 bot 配置：放行（可能是测试环境或未配置权限）
		return true, ""
	}

	// 1. AllowedChats
	if bot.AllowedChats != "" {
		chats, _ := parseJSONStringList(bot.AllowedChats)
		if !containsString(chats, chatID) {
			return false, "当前会话未在允许列表中"
		}
	}

	if isGroupChatType(e.chatTypeForPermission(platform, chatID)) {
		if !bot.GroupAccessControl {
			return true, ""
		}
		if senderID != "" && senderID == strings.TrimSpace(bot.OwnerID) {
			return true, ""
		}
		users, _ := parseJSONStringList(bot.AllowedUsers)
		if containsString(users, senderID) {
			return true, ""
		}
		return false, "未在群聊访问白名单中"
	}

	// 2. AllowedUsers
	if bot.AllowedUsers != "" {
		users, _ := parseJSONStringList(bot.AllowedUsers)
		if !containsString(users, senderID) {
			return false, "未在允许的用户列表中"
		}
		// AllowedUsers 命中即放行（不再看 OwnerID）
		return true, ""
	}

	// 3. OwnerID（owner-only 模式）
	if bot.OwnerID != "" && senderID != bot.OwnerID {
		return false, "仅 bot 所有者可操作"
	}

	return true, ""
}

func (e *Engine) chatTypeForPermission(platform, chatID string) string {
	e.mu.Lock()
	defer e.mu.Unlock()
	for _, sess := range e.sessions {
		if sess == nil {
			continue
		}
		if sess.platform == platform && sess.chatID == chatID {
			return strings.TrimSpace(sess.chatType)
		}
	}
	return ""
}

func isGroupChatType(chatType string) bool {
	return chatType == "group" || chatType == "topic"
}

func (e *Engine) checkOwnerPermission(platform, senderID string) (bool, string) {
	bot, err := credential.GetBotConfig(platform)
	if err != nil || bot == nil {
		return true, ""
	}
	ownerID := strings.TrimSpace(bot.OwnerID)
	if ownerID == "" {
		return false, "仅 bot 所有者可操作；当前 bot 配置没有 OwnerID"
	}
	if senderID != ownerID {
		return false, "仅 bot 所有者可操作"
	}
	return true, ""
}

// parseJSONStringList 解析 JSON 字符串数组。空/非法返回 nil。
func parseJSONStringList(raw string) ([]string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	var list []string
	if err := json.Unmarshal([]byte(raw), &list); err != nil {
		return nil, fmt.Errorf("parse json string list: %w", err)
	}
	return list, nil
}

func containsString(list []string, target string) bool {
	for _, s := range list {
		if s == target {
			return true
		}
	}
	return false
}

// canBrowseAllAISessions 判断当前 IM 请求是否可浏览/恢复全局 Yakit AI Session。
// 扫码所有者在私聊里是最高权限控制面板；共享用户和群聊只暴露当前 IM 会话，避免企业内分享后泄露历史。
func (e *Engine) canBrowseAllAISessions(msg *notify.InboundMessage) bool {
	ok, _ := e.browseAllAISessionsAccess(msg)
	return ok
}

func (e *Engine) browseAllAISessionsAccess(msg *notify.InboundMessage) (bool, string) {
	if msg == nil || strings.TrimSpace(msg.SenderID) == "" {
		return false, "全局历史仅 bot 所有者私聊可见；当前请求没有识别到发送者 ID。"
	}
	chatType := e.chatTypeForAccess(msg)
	if chatType == "" {
		return false, "全局历史仅 bot 所有者私聊可见；当前请求没有识别到会话类型，请在私聊里发送 /session 重新打开。"
	}
	if chatType != "private" {
		return false, fmt.Sprintf("全局历史仅 bot 所有者私聊可见；当前场景是 %s，不是私聊。", chatType)
	}
	bot, err := credential.GetBotConfig(string(msg.Platform))
	if err != nil || bot == nil {
		return false, "全局历史仅 bot 所有者私聊可见；当前平台没有读取到 bot 配置。"
	}
	ownerID := strings.TrimSpace(bot.OwnerID)
	if ownerID == "" {
		return false, fmt.Sprintf("全局历史仅 bot 所有者私聊可见；但当前 bot 配置没有 OwnerID。当前 sender: %s。", shortIDForConfig(msg.SenderID))
	}
	if msg.SenderID != ownerID {
		return false, fmt.Sprintf("全局历史仅 bot 所有者私聊可见；当前 sender=%s，bot owner=%s，二者不一致。",
			shortIDForConfig(msg.SenderID), shortIDForConfig(ownerID))
	}
	return true, "所有者私聊控制台：可浏览最近 Yakit AI Session，并把当前 IM 会话切换到指定历史。"
}

func (e *Engine) chatTypeForAccess(msg *notify.InboundMessage) string {
	if msg == nil {
		return ""
	}
	if chatType := strings.TrimSpace(msg.ChatType); chatType != "" {
		return chatType
	}
	sessionKey := imSessionKey(msg)
	e.mu.Lock()
	sess := e.sessions[sessionKey]
	e.mu.Unlock()
	if sess == nil {
		return ""
	}
	return strings.TrimSpace(sess.chatType)
}
