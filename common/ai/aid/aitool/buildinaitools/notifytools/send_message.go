package notifytools

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"sync"

	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/notify"
	dingtalkdriver "github.com/yaklang/yaklang/common/notify/drivers/dingtalk"
	feishudriver "github.com/yaklang/yaklang/common/notify/drivers/feishu"
)

// 凭证存储：IM 平台 AppID/AppSecret 在运行期由 ConfigureIMCredentials 工具注入，
// send_im_message 工具读取此处凭证发送。进程内单例。
var (
	credMu sync.RWMutex
	creds  = map[notify.PlatformType]*notify.SendConfig{}
)

func setCred(platform notify.PlatformType, cfg *notify.SendConfig) {
	credMu.Lock()
	defer credMu.Unlock()
	creds[platform] = cfg
}

func getCred(platform notify.PlatformType) *notify.SendConfig {
	credMu.RLock()
	defer credMu.RUnlock()
	return creds[platform]
}

// CreateNotifySendTools 构造 IM 远程通知相关的 AI 工具：
//   - send_im_message: 向飞书/钉钉等平台发送一条消息
//   - configure_im_credentials: 注入某平台的 AppID/AppSecret（凭证）
func CreateNotifySendTools() []*aitool.Tool {
	sendTool, err := aitool.New(
		"send_im_message",
		aitool.WithDescription(`Send a message to an IM platform (Feishu/Lark or DingTalk). Use this to proactively push results, alerts, or reports to a user or group chat.
Credentials (app_id/app_secret) must be configured first via the 'configure_im_credentials' tool.`),
		aitool.WithStringParam("platform",
			aitool.WithParam_Required(true),
			aitool.WithParam_Description("Target IM platform. Currently supported: 'feishu' (Feishu/Lark), 'dingtalk' (DingTalk)."),
			aitool.WithParam_Enum("feishu", "dingtalk"),
		),
		aitool.WithStringParam("target_id",
			aitool.WithParam_Required(true),
			aitool.WithParam_Description("Message recipient identifier. For Feishu: an open_id/chat_id/user_id (set receive_id_type accordingly). For DingTalk: a staffId/outerUserId (single chat) or conversationId (group)."),
		),
		aitool.WithStringParam("content",
			aitool.WithParam_Required(true),
			aitool.WithParam_Description("Message content. Plain text by default; when msg_type is 'markdown' this is the markdown body."),
		),
		aitool.WithStringParam("msg_type",
			aitool.WithParam_Required(false),
			aitool.WithParam_Description("Message type. Default 'text'. Options: 'text', 'markdown', 'card'."),
			aitool.WithParam_Enum("text", "markdown", "card"),
			aitool.WithParam_Default("text"),
		),
		aitool.WithStringParam("receive_id_type",
			aitool.WithParam_Required(false),
			aitool.WithParam_Description("Feishu only. The type of target_id: 'open_id', 'chat_id', 'user_id', 'union_id', or 'email'. Default 'open_id'. Ignored by DingTalk."),
			aitool.WithParam_Default("open_id"),
		),
		aitool.WithBoolParam("is_group",
			aitool.WithParam_Required(false),
			aitool.WithParam_Description("DingTalk only. Set true to send to a group (target_id becomes the conversationId). Default false (single chat)."),
		),
		aitool.WithStringParam("card_title",
			aitool.WithParam_Required(false),
			aitool.WithParam_Description("Optional title used when msg_type is 'card'."),
		),
		aitool.WithSimpleCallback(func(params aitool.InvokeParams, stdout io.Writer, stderr io.Writer) (any, error) {
			platform := notify.PlatformType(params.GetString("platform"))
			if platform == "" {
				return nil, stderrWrite(stderr, "platform is required")
			}
			cfg := getCred(platform)
			if cfg == nil {
				return nil, stderrWrite(stderr, "no credentials configured for platform "+platform.String()+"; call configure_im_credentials first")
			}
			targetID := params.GetString("target_id")
			msgType := notify.MsgType(params.GetString("msg_type", "text"))
			msg := notifyToolMessage(msgType, params.GetString("content"), params.GetString("card_title"))
			reg := notify.NewRegistry()
			desc, err := descriptorForPlatform(platform)
			if err != nil {
				return nil, stderrWrite(stderr, err.Error())
			}
			reg.Register(desc)
			client := notify.NewClient(notify.WithRegistry(reg), notify.WithSendConfig(cfg))
			resp, err := client.Do(context.Background(), &notify.Request{
				Platform: notify.Platform(platform),
				Action:   notify.ActionMessagesSend,
				Target: notify.Target{
					ID:     targetID,
					Kind:   notifyToolTargetKind(params.GetBool("is_group")),
					Native: notifyToolTargetNative(platform, params.GetString("receive_id_type")),
				},
				Message: msg,
			})
			if err != nil {
				return nil, stderrWrite(stderr, err.Error())
			}
			out := map[string]any{
				"platform":  platform.String(),
				"messageId": resp.MessageID,
			}
			b, _ := json.Marshal(out)
			log.Infof("send_im_message: %s -> %s (messageId=%s)", platform, targetID, resp.MessageID)
			return string(b), nil
		}),
	)
	if err != nil {
		log.Errorf("create send_im_message tool: %v", err)
	}

	credTool, err := aitool.New(
		"configure_im_credentials",
		aitool.WithDescription(`Configure the credentials (app_id/app_secret) for an IM platform so that 'send_im_message' can use them. Credentials are kept in memory for the lifetime of this process.`),
		aitool.WithStringParam("platform",
			aitool.WithParam_Required(true),
			aitool.WithParam_Description("Platform to configure: 'feishu' or 'dingtalk'."),
			aitool.WithParam_Enum("feishu", "dingtalk"),
		),
		aitool.WithStringParam("app_id",
			aitool.WithParam_Required(true),
			aitool.WithParam_Description("AppID. Feishu app_id, or DingTalk appKey."),
		),
		aitool.WithStringParam("app_secret",
			aitool.WithParam_Required(true),
			aitool.WithParam_Description("AppSecret. Feishu app_secret, or DingTalk appSecret."),
		),
		aitool.WithStringParam("robot_secret",
			aitool.WithParam_Required(false),
			aitool.WithParam_Description("DingTalk custom group robot signing secret (optional)."),
		),
		aitool.WithStringParam("base_url",
			aitool.WithParam_Required(false),
			aitool.WithParam_Description("Override the default platform API base URL (e.g. for Lark international: https://open.larksuite.com). Optional."),
		),
		aitool.WithSimpleCallback(func(params aitool.InvokeParams, stdout io.Writer, stderr io.Writer) (any, error) {
			platform := notify.PlatformType(params.GetString("platform"))
			if platform == "" {
				return nil, stderrWrite(stderr, "platform is required")
			}
			cfg := &notify.SendConfig{
				AppID:       params.GetString("app_id"),
				AppSecret:   params.GetString("app_secret"),
				RobotSecret: params.GetString("robot_secret"),
				BaseURL:     params.GetString("base_url"),
			}
			setCred(platform, cfg)
			return "ok: credentials configured for " + platform.String(), nil
		}),
	)
	if err != nil {
		log.Errorf("create configure_im_credentials tool: %v", err)
	}

	var tools []*aitool.Tool
	for _, t := range []*aitool.Tool{sendTool, credTool} {
		if t != nil {
			tools = append(tools, t)
		}
	}
	return tools
}

func stderrWrite(stderr io.Writer, msg string) error {
	_, err := stderr.Write([]byte(msg))
	return err
}

func descriptorForPlatform(platform notify.PlatformType) (notify.Descriptor, error) {
	switch platform {
	case notify.PlatformFeishu:
		return feishudriver.Descriptor(), nil
	case notify.PlatformDingTalk:
		return dingtalkdriver.Descriptor(), nil
	default:
		return notify.Descriptor{}, fmt.Errorf("unknown platform %s", platform)
	}
}

func notifyToolMessage(msgType notify.MsgType, content, title string) *notify.Message {
	switch msgType {
	case notify.MsgMarkdown:
		return &notify.Message{Type: notify.MessageMarkdown, Markdown: content}
	case notify.MsgCard:
		return &notify.Message{Type: notify.MessageCard, Text: content, Card: &notify.Card{Title: title, Content: content}}
	default:
		return &notify.Message{Type: notify.MessageText, Text: content}
	}
}

func notifyToolTargetKind(isGroup bool) notify.TargetKind {
	if isGroup {
		return notify.TargetChat
	}
	return notify.TargetUser
}

func notifyToolTargetNative(platform notify.PlatformType, receiveIDType string) notify.NativeOptions {
	if platform != notify.PlatformFeishu || receiveIDType == "" {
		return nil
	}
	return notify.NativeOptions{"receive_id_type": receiveIDType}
}
