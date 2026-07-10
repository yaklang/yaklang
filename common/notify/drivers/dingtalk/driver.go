package dingtalk

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
		Platform: notify.PlatformDingTalk,
		Capabilities: notify.Capabilities{
			SendText:          true,
			SendMarkdown:      true,
			Reactions:         true,
			ReceiveEvents:     true,
			DownloadResources: true,
			Onboarding:        true,
		},
		Actions: []notify.Action{
			notify.ActionMessagesSend,
			notify.ActionMessagesReply,
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
		return nil, fmt.Errorf("dingtalk: nil request")
	}
	switch req.Action {
	case notify.ActionMessagesSend:
		msg, err := dingtalkMessageFromRequest(req)
		if err != nil {
			return nil, err
		}
		if msg.TargetID == "" {
			return nil, fmt.Errorf("dingtalk: target id is required for send")
		}
		res, err := d.client.Send(msg, d.cfg)
		if err != nil {
			return nil, err
		}
		return dingtalkResponse(req, res), nil
	case notify.ActionMessagesReply:
		msg, err := dingtalkMessageFromRequest(req)
		if err != nil {
			return nil, err
		}
		if req.Target.ReplyTo == "" {
			return nil, fmt.Errorf("dingtalk: reply context is required")
		}
		res, err := d.client.ReplyMessage(req.Target.ReplyTo, msg, d.cfg)
		if err != nil {
			return nil, err
		}
		return dingtalkResponse(req, res), nil
	case notify.ActionReactionsAdd:
		emojiType, _ := req.Native["emoji_type"].(string)
		if err := d.client.AddReaction(req.Target.ID, emojiType); err != nil {
			return nil, err
		}
		return &notify.Response{
			Platform: notify.PlatformDingTalk,
			Action:   req.Action,
		}, nil
	case notify.ActionResourcesDownload:
		if req.Resource == nil {
			return nil, fmt.Errorf("dingtalk: resource is required for download")
		}
		resourceID := req.Resource.ID
		if resourceID == "" {
			resourceID = req.Resource.Name
		}
		if resourceID == "" {
			return nil, fmt.Errorf("dingtalk: downloadCode is required for download")
		}
		localPath, mimeType, size, err := d.client.DownloadResource(d.cfg, resourceID, req.Resource.Type == "image")
		if err != nil {
			return nil, err
		}
		return &notify.Response{
			Platform: notify.PlatformDingTalk,
			Action:   req.Action,
			Resource: &notify.Resource{
				ID:       resourceID,
				Name:     req.Resource.Name,
				MimeType: mimeType,
				Size:     size,
				Path:     localPath,
			},
		}, nil
	case notify.ActionPing:
		if err := Ping(d.cfg.AsSendOptions()...); err != nil {
			return nil, err
		}
		return &notify.Response{
			Platform: notify.PlatformDingTalk,
			Action:   req.Action,
		}, nil
	default:
		return nil, notify.ErrNotImplemented
	}
}

func (d *Driver) Stream(ctx context.Context, req *notify.Request, emit notify.EventHandler) error {
	if req == nil {
		return fmt.Errorf("dingtalk: nil request")
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
			return fmt.Errorf("dingtalk: stream client is nil")
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
			Platform: notify.PlatformDingTalk,
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
	return onboard(dingtalkIntOption(req.Options, "timeout_seconds"), dingtalkStringOptions(req.Options), func(step *notify.OnboardingStep) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if emit != nil {
			emit(notify.Event{
				Type:       notify.EventOnboarding,
				Platform:   notify.PlatformDingTalk,
				Onboarding: step,
			})
		}
		return nil
	})
}

func dingtalkMessageFromRequest(req *notify.Request) (*platformMessage, error) {
	if req.Message == nil {
		return nil, fmt.Errorf("dingtalk: message is required")
	}
	msg := req.Message
	out := &platformMessage{
		TargetID: req.Target.ID,
		IsGroup:  req.Target.Kind == notify.TargetChat || req.Target.Kind == notify.TargetThread,
	}

	switch {
	case msg.NativeCard != nil || msg.Type == notify.MessageNative:
		return nil, fmt.Errorf("dingtalk: native card is not supported")
	case msg.Type == notify.MessageMarkdown || msg.Markdown != "":
		out.MsgType = notify.MsgMarkdown
		out.Content = msg.Markdown
		out.Card = msg.Card
	case msg.Card != nil || msg.Type == notify.MessageCard:
		out.MsgType = notify.MsgCard
		out.Card = msg.Card
		if msg.Card != nil {
			out.Content = dingtalkFirstNonEmpty(msg.Card.Markdown, msg.Card.Content)
		}
	default:
		out.MsgType = notify.MsgText
		out.Content = msg.Text
	}
	return out, nil
}

func dingtalkResponse(req *notify.Request, res *notify.SendResult) *notify.Response {
	resp := &notify.Response{
		Platform: notify.PlatformDingTalk,
		Action:   req.Action,
	}
	if res != nil {
		resp.MessageID = res.MessageID
		resp.Raw = res.Raw
	}
	return resp
}

func dingtalkFirstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func dingtalkIntOption(values map[string]any, key string) int {
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

func dingtalkStringOptions(values map[string]any) map[string]string {
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
