package imcontrol

import (
	"fmt"
	"strings"

	"github.com/yaklang/yaklang/common/notify"
	"github.com/yaklang/yaklang/common/notify/credential"
)

func (e *Engine) BuildOnboardingWelcomeCard(platform notify.PlatformType, ownerID string) *notify.Card {
	return e.buildOnboardingWelcomeCard(platform, ownerID, nil, false)
}

func (e *Engine) BuildOnboardingWelcomeCardForMessage(platform notify.PlatformType, ownerID string, inbound *notify.InboundMessage) *notify.Card {
	return e.buildOnboardingWelcomeCard(platform, ownerID, inbound, true)
}

func (e *Engine) buildOnboardingWelcomeCard(platform notify.PlatformType, ownerID string, inbound *notify.InboundMessage, includeActions bool) *notify.Card {
	ownerID = strings.TrimSpace(ownerID)
	msg := &notify.InboundMessage{
		Platform: platform,
		ChatID:   ownerID,
		SenderID: ownerID,
		ChatType: "private",
	}
	if inbound != nil {
		copied := *inbound
		if copied.Platform == "" {
			copied.Platform = platform
		}
		if copied.SenderID == "" {
			copied.SenderID = ownerID
		}
		if copied.ChatType == "" {
			copied.ChatType = "private"
		}
		msg = &copied
	}
	s := e.statusSnapshot(msg)
	platformLabel := platformDisplayLabel(string(platform))
	ownerLine := ""
	if ownerID != "" {
		ownerLine = "\n所有者：" + shortIDForConfig(ownerID)
	}
	summary := fmt.Sprintf(`**%s 已连接**
模型：%s
回复：%s · 审批：%s`,
		platformLabel,
		s.Model,
		replyGranularityLabel(s.Granularity),
		reviewPolicyLabel(s.ReviewPolicy),
	)
	permission := "私聊管理仅所有者可用；群聊默认允许成员使用，默认需要 @ 提及。"
	hint := "直接发送消息即可开始。如需限制群成员，可在 Yakit 机器人配置面板开启群聊访问控制。"
	text := fmt.Sprintf("Yak Agent 已连接\n\n%s\n\n权限：%s%s\n%s", summary, permission, ownerLine, hint)
	elements := []map[string]any{
		configHintElement(summary),
		configHintElement("权限：" + permission + ownerLine),
		configHintElement(hint),
	}
	if includeActions {
		elements = append(elements, actionRowElement(
			e.controlButtonElement("会话", "primary", msg, string(ActionSessionInfo), nil),
			e.controlButtonElement("配置", "default", msg, string(ActionConfig), nil),
			e.controlButtonElement("审批", "default", msg, string(ActionReview), nil),
			e.controlButtonElement("状态", "default", msg, string(ActionStatus), nil),
		))
	}
	return &notify.Card{
		Title:    "Yak Agent 已连接",
		Content:  text,
		Markdown: text,
		Config: map[string]any{
			"wide_screen_mode": true,
		},
		Elements: elements,
	}
}

func (e *Engine) SendOnboardingWelcome(bot *credential.BotConfig) error {
	if bot == nil || strings.TrimSpace(bot.OwnerID) == "" || !bot.Enabled {
		return nil
	}
	platform := notify.PlatformType(bot.Platform)
	card := e.BuildOnboardingWelcomeCard(platform, bot.OwnerID)
	req, err := buildOnboardingWelcomeRequest(platform, bot.OwnerID, card)
	if err != nil {
		return err
	}
	_, err = executeNotifyRequest(req, bot.ToSendConfig())
	return err
}

func (e *Engine) SendOnboardingWelcomeReply(bot *credential.BotConfig, inbound *notify.InboundMessage) error {
	if bot == nil || inbound == nil || strings.TrimSpace(bot.OwnerID) == "" || !bot.Enabled {
		return nil
	}
	platform := notify.PlatformType(bot.Platform)
	card := e.BuildOnboardingWelcomeCardForMessage(platform, bot.OwnerID, inbound)
	_, err := sendCardMessage(platform, inbound.ChatID, cardMessage(card), bot.ToSendConfig(), inbound, true)
	return err
}
