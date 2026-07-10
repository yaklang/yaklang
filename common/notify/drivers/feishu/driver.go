package feishu

import (
	"context"
	"fmt"
	"strconv"

	"github.com/yaklang/yaklang/common/notify"
)

type Driver struct {
	client  *Client
	cfg     *notify.SendConfig
	start   func(context.Context, func(*notify.InboundMessage)) error
	onboard onboardingRunner
}

type onboardingRunner func(timeoutSeconds int, opts map[string]string, handler notify.OnboardingHandler) error

func Descriptor() notify.Descriptor {
	return notify.Descriptor{
		Platform: notify.PlatformFeishu,
		Capabilities: notify.Capabilities{
			SendText:          true,
			SendMarkdown:      true,
			SendCard:          true,
			UpdateCard:        true,
			StreamCard:        true,
			CardActions:       true,
			NativeReply:       true,
			Reactions:         true,
			ReceiveEvents:     true,
			DownloadResources: true,
			Onboarding:        true,
			NativeCardSchemas: []string{"feishu.card.v2"},
		},
		Actions: []notify.Action{
			notify.ActionMessagesSend,
			notify.ActionMessagesReply,
			notify.ActionMessagesPatch,
			notify.ActionReactionsAdd,
			notify.ActionResourcesDownload,
			notify.ActionPing,
			notify.ActionEventsReceive,
			notify.ActionOnboardingStart,
		},
		New: func(cfg notify.DriverConfig) (notify.Driver, error) {
			sendCfg := cfg.SendConfig
			if sendCfg == nil {
				sendCfg = notify.NewSendConfig()
			}
			client := New(sendCfg.AsSendOptions()...)
			return &Driver{
				client:  client,
				cfg:     sendCfg,
				start:   client.Start,
				onboard: RunOnboarding,
			}, nil
		},
	}
}

func (d *Driver) Do(ctx context.Context, req *notify.Request) (*notify.Response, error) {
	if req == nil {
		return nil, fmt.Errorf("feishu: nil request")
	}
	switch req.Action {
	case notify.ActionMessagesSend:
		msg, err := feishuMessageFromRequest(req)
		if err != nil {
			return nil, err
		}
		if msg.TargetID == "" {
			return nil, fmt.Errorf("feishu: target id is required for send")
		}
		res, err := d.client.Send(msg, d.cfg)
		if err != nil {
			return nil, err
		}
		return feishuResponse(req, res), nil
	case notify.ActionMessagesReply:
		msg, err := feishuMessageFromRequest(req)
		if err != nil {
			return nil, err
		}
		if req.Target.ReplyTo == "" {
			return nil, fmt.Errorf("feishu: reply target message id is required")
		}
		res, err := d.client.ReplyMessage(req.Target.ReplyTo, msg, d.cfg)
		if err != nil {
			return nil, err
		}
		return feishuResponse(req, res), nil
	case notify.ActionMessagesPatch:
		msg, err := feishuMessageFromRequest(req)
		if err != nil {
			return nil, err
		}
		messageID := firstNonEmpty(req.Target.ID, req.Target.ReplyTo)
		if messageID == "" {
			return nil, fmt.Errorf("feishu: message id is required for patch")
		}
		res, err := d.client.PatchCard(messageID, msg, d.cfg)
		if err != nil {
			return nil, err
		}
		return feishuResponse(req, res), nil
	case notify.ActionReactionsAdd:
		messageID := firstNonEmpty(req.Target.ID, req.Target.ReplyTo)
		emojiType := stringOption(req.Native, "emoji_type")
		if emojiType == "" {
			emojiType = stringOption(req.Options, "emoji_type")
		}
		if messageID == "" || emojiType == "" {
			return nil, fmt.Errorf("feishu: message id and emoji_type are required for reaction")
		}
		if err := d.client.AddReaction(messageID, emojiType); err != nil {
			return nil, err
		}
		return &notify.Response{
			Platform: notify.PlatformFeishu,
			Action:   req.Action,
		}, nil
	case notify.ActionPing:
		if err := Ping(d.cfg.AsSendOptions()...); err != nil {
			return nil, err
		}
		return &notify.Response{
			Platform: notify.PlatformFeishu,
			Action:   req.Action,
		}, nil
	case notify.ActionResourcesDownload:
		if req.Resource == nil {
			return nil, fmt.Errorf("feishu: resource is required for download")
		}
		resourceID := firstNonEmpty(req.Resource.ID, req.Resource.Name)
		if resourceID == "" || req.Resource.MessageID == "" {
			return nil, fmt.Errorf("feishu: message_id and resource id are required for download")
		}
		localPath, mimeType, size, err := d.client.DownloadResource(d.cfg, req.Resource.MessageID, resourceID, req.Resource.Type == "image")
		if err != nil {
			return nil, err
		}
		return &notify.Response{
			Platform: notify.PlatformFeishu,
			Action:   req.Action,
			Resource: &notify.Resource{
				ID:       resourceID,
				Name:     req.Resource.Name,
				MimeType: mimeType,
				Size:     size,
				Path:     localPath,
			},
		}, nil
	default:
		return nil, notify.ErrNotImplemented
	}
}

func (d *Driver) Stream(ctx context.Context, req *notify.Request, emit notify.EventHandler) error {
	if req == nil {
		return fmt.Errorf("feishu: nil request")
	}
	switch req.Action {
	case notify.ActionEventsReceive:
		return d.streamEvents(ctx, emit)
	case notify.ActionOnboardingStart:
		return d.streamOnboarding(ctx, req, emit)
	default:
		return notify.ErrNotImplemented
	}
}

func (d *Driver) streamEvents(ctx context.Context, emit notify.EventHandler) error {
	start := d.start
	if start == nil {
		if d.client == nil {
			return fmt.Errorf("feishu: stream client is nil")
		}
		start = d.client.Start
	}
	if d.client != nil {
		d.client.SetEventHandler(emit)
		defer d.client.SetEventHandler(nil)
	}
	return start(ctx, func(msg *notify.InboundMessage) {
		if emit == nil || msg == nil {
			return
		}
		eventType := notify.EventMessage
		if msg.IsCardAction {
			eventType = notify.EventCardAction
		}
		emit(notify.Event{
			Type:     eventType,
			Platform: notify.PlatformFeishu,
			Message:  msg,
			Raw:      msg.Raw,
		})
	})
}

func (d *Driver) streamOnboarding(ctx context.Context, req *notify.Request, emit notify.EventHandler) error {
	onboard := d.onboard
	if onboard == nil {
		onboard = RunOnboarding
	}
	return onboard(intOption(req.Options, "timeout_seconds"), stringOptions(req.Options), func(step *notify.OnboardingStep) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if emit != nil {
			emit(notify.Event{
				Type:       notify.EventOnboarding,
				Platform:   notify.PlatformFeishu,
				Onboarding: step,
			})
		}
		return nil
	})
}

func feishuMessageFromRequest(req *notify.Request) (*platformMessage, error) {
	if req.Message == nil {
		return nil, fmt.Errorf("feishu: message is required")
	}
	msg := req.Message
	out := &platformMessage{
		TargetID:      req.Target.ID,
		ReceiveIDType: feishuReceiveIDType(req.Target),
		IsGroup:       req.Target.Kind == notify.TargetChat,
	}

	switch {
	case msg.NativeCard != nil || msg.Type == notify.MessageNative:
		if msg.NativeCard == nil {
			return nil, fmt.Errorf("feishu: native card is required")
		}
		out.MsgType = notify.MsgCard
		out.NativeCard = msg.NativeCard
	case msg.Card != nil || msg.Type == notify.MessageCard:
		out.MsgType = notify.MsgCard
		out.Card = msg.Card
		if msg.Card != nil {
			out.Content = firstNonEmpty(msg.Card.Markdown, msg.Card.Content)
		}
	case msg.Type == notify.MessageMarkdown || msg.Markdown != "":
		out.MsgType = notify.MsgMarkdown
		out.Content = msg.Markdown
	default:
		out.MsgType = notify.MsgText
		out.Content = msg.Text
	}
	return out, nil
}

func feishuReceiveIDType(target notify.Target) string {
	if receiveIDType := stringOption(target.Native, "receive_id_type"); receiveIDType != "" {
		return receiveIDType
	}
	switch target.Kind {
	case notify.TargetChat, notify.TargetThread:
		return "chat_id"
	default:
		return "open_id"
	}
}

func feishuResponse(req *notify.Request, res *notify.SendResult) *notify.Response {
	resp := &notify.Response{
		Platform: notify.PlatformFeishu,
		Action:   req.Action,
	}
	if res != nil {
		resp.MessageID = res.MessageID
		resp.Raw = res.Raw
	}
	return resp
}

func stringOption(values map[string]any, key string) string {
	if len(values) == 0 {
		return ""
	}
	if v, ok := values[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

func intOption(values map[string]any, key string) int {
	if len(values) == 0 {
		return 0
	}
	switch v := values[key].(type) {
	case int:
		return v
	case int32:
		return int(v)
	case int64:
		return int(v)
	case float64:
		return int(v)
	case string:
		n, _ := strconv.Atoi(v)
		return n
	default:
		return 0
	}
}

func stringOptions(values map[string]any) map[string]string {
	if len(values) == 0 {
		return nil
	}
	out := make(map[string]string, len(values))
	for key, value := range values {
		if s, ok := value.(string); ok {
			out[key] = s
		}
	}
	return out
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
