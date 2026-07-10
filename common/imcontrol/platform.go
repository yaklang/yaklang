package imcontrol

import (
	"context"
	"fmt"

	"github.com/yaklang/yaklang/common/notify"
	dingtalkdriver "github.com/yaklang/yaklang/common/notify/drivers/dingtalk"
	feishudriver "github.com/yaklang/yaklang/common/notify/drivers/feishu"
)

func isPlatformReceivable(platform notify.PlatformType) bool {
	desc, err := descriptorForPlatform(notify.Platform(platform))
	if err != nil {
		return false
	}
	for _, action := range desc.Actions {
		if action == notify.ActionEventsReceive {
			return true
		}
	}
	return false
}

func addReaction(platform notify.PlatformType, messageID, emojiType string, cfg *notify.SendConfig) error {
	req, err := buildReactionRequest(platform, messageID, emojiType)
	if err != nil {
		return err
	}
	_, err = executeNotifyRequest(req, cfg)
	return err
}

func sendText(platform notify.PlatformType, chatID, messageID, text string, cfg *notify.SendConfig, replyQuote bool) error {
	req, err := buildTextRequest(platform, chatID, messageID, text, replyQuote)
	if err != nil {
		return err
	}
	_, err = executeNotifyRequest(req, cfg)
	return err
}

func buildTextRequest(platform notify.PlatformType, chatID, messageID, text string, replyQuote bool) (*notify.Request, error) {
	action := notify.ActionMessagesSend
	target := notify.Target{
		ID:     chatID,
		Kind:   targetKindForPlatformID(platform, chatID),
		Native: map[string]any{},
	}
	if platform == notify.PlatformFeishu {
		if receiveIDType := inferFeishuReceiveIDType(chatID); receiveIDType != "" {
			target.Native["receive_id_type"] = receiveIDType
		}
	}
	if messageID != "" && (replyQuote || platform == notify.PlatformDingTalk) {
		action = notify.ActionMessagesReply
		target.ReplyTo = messageID
	}
	return &notify.Request{
		Platform: notify.Platform(platform),
		Action:   action,
		Target:   target,
		Message: &notify.Message{
			Type: notify.MessageText,
			Text: text,
		},
	}, nil
}

func buildReactionRequest(platform notify.PlatformType, messageID, emojiType string) (*notify.Request, error) {
	return &notify.Request{
		Platform: notify.Platform(platform),
		Action:   notify.ActionReactionsAdd,
		Target: notify.Target{
			ID: messageID,
		},
		Native: notify.NativeOptions{
			"emoji_type": emojiType,
		},
	}, nil
}

func buildMessageRequest(platform notify.PlatformType, action notify.Action, targetID, messageID, receiveIDType string, msg *notify.Message) (*notify.Request, error) {
	if msg == nil {
		return nil, fmt.Errorf("message is nil")
	}
	target := notify.Target{
		ID:     targetID,
		Kind:   targetKindForPlatformID(platform, targetID),
		Native: map[string]any{},
	}
	if platform == notify.PlatformFeishu {
		if receiveIDType == "" {
			receiveIDType = inferFeishuReceiveIDType(targetID)
		}
		if receiveIDType != "" {
			target.Native["receive_id_type"] = receiveIDType
		}
	}
	if action == notify.ActionMessagesReply {
		target.ReplyTo = messageID
	}
	if action == notify.ActionMessagesPatch {
		target.ID = messageID
	}
	return &notify.Request{
		Platform: notify.Platform(platform),
		Action:   action,
		Target:   target,
		Message:  msg,
	}, nil
}

func sendCardMessage(platform notify.PlatformType, targetID string, msg *notify.Message, cfg *notify.SendConfig, inbound *notify.InboundMessage, replyQuote bool) (*notify.SendResult, error) {
	action := notify.ActionMessagesSend
	messageID := ""
	if inbound != nil {
		messageID = msgIDToString(inbound.ReplyContext)
	}
	if inbound != nil && inbound.IsCardAction {
		messageID = ""
	}
	if messageID != "" && (replyQuote || platform == notify.PlatformDingTalk) {
		action = notify.ActionMessagesReply
	}
	req, err := buildMessageRequest(platform, action, targetID, messageID, inferFeishuReceiveIDType(targetID), msg)
	if err != nil {
		return nil, err
	}
	resp, err := executeNotifyRequest(req, cfg)
	if err != nil {
		return nil, err
	}
	return &notify.SendResult{MessageID: resp.MessageID, Raw: resp.Raw, Platform: resp.Platform}, nil
}

func patchCardMessage(platform notify.PlatformType, messageID string, msg *notify.Message, cfg *notify.SendConfig) (*notify.SendResult, error) {
	req, err := buildMessageRequest(platform, notify.ActionMessagesPatch, "", messageID, "", msg)
	if err != nil {
		return nil, err
	}
	resp, err := executeNotifyRequest(req, cfg)
	if err != nil {
		return nil, err
	}
	return &notify.SendResult{MessageID: resp.MessageID, Raw: resp.Raw, Platform: resp.Platform}, nil
}

func buildOnboardingWelcomeRequest(platform notify.PlatformType, ownerID string, card *notify.Card) (*notify.Request, error) {
	msg := &notify.Message{
		Type: notify.MessageCard,
		Card: card,
	}
	if !platformCapabilities(platform).SendCard {
		msg.Type = notify.MessageMarkdown
		if card != nil {
			msg.Markdown = firstNonEmptyString(card.Markdown, card.Content)
		}
	}
	return buildMessageRequest(platform, notify.ActionMessagesSend, ownerID, "", inferFeishuReceiveIDType(ownerID), msg)
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func executeNotifyRequest(req *notify.Request, cfg *notify.SendConfig) (*notify.Response, error) {
	if req == nil {
		return nil, fmt.Errorf("nil notify request")
	}
	reg := notify.NewRegistry()
	desc, err := descriptorForPlatform(req.Platform)
	if err != nil {
		return nil, err
	}
	reg.Register(desc)
	client := notify.NewClient(notify.WithRegistry(reg), notify.WithSendConfig(cfg))
	return client.Do(context.Background(), req)
}

func descriptorForPlatform(platform notify.Platform) (notify.Descriptor, error) {
	switch platform {
	case notify.PlatformFeishu:
		return feishudriver.Descriptor(), nil
	case notify.PlatformDingTalk:
		return dingtalkdriver.Descriptor(), nil
	default:
		return notify.Descriptor{}, fmt.Errorf("unknown platform %q", platform)
	}
}

func targetKindForPlatformID(platform notify.PlatformType, id string) notify.TargetKind {
	if platform == notify.PlatformFeishu && inferFeishuReceiveIDType(id) != "chat_id" {
		return notify.TargetUser
	}
	return notify.TargetChat
}

func platformCapabilities(platform notify.PlatformType) notify.PlatformCapabilities {
	switch platform {
	case notify.PlatformFeishu:
		return toLegacyCapabilities(feishudriver.Descriptor().Capabilities)
	case notify.PlatformDingTalk:
		return toLegacyCapabilities(dingtalkdriver.Descriptor().Capabilities)
	default:
		return notify.PlatformCapabilities{}
	}
}

func toLegacyCapabilities(c notify.Capabilities) notify.PlatformCapabilities {
	return notify.PlatformCapabilities{
		NativeReply: c.NativeReply,
		Reactions:   c.Reactions,
		SendCard:    c.SendCard,
		UpdateCard:  c.UpdateCard,
		CardActions: c.CardActions,
	}
}
