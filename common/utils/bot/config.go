package bot

import (
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/dingrobot"
	"github.com/yaklang/yaklang/common/utils/larkrobot"
	"github.com/yaklang/yaklang/common/utils/workwxrobot"
	"net/url"
)

const (
	BotType_DingTalk   = "dingtalk"
	BotType_WorkWechat = "workwechat"
	BotType_Feishu     = "lark"
	BotType_Lark       = "lark"
)

// Config 这个 Bot 主要针对钉钉 / 企业微信 / 飞书lark
// 企业微信的推送是最简单的，其次是飞书，最后是钉钉
// 配置一般来说分两个字段，Webhook 和 Secret
type Config struct {
	Webhook string
	Secret  string
	BotType string

	_dingtalkCache dingrobot.Roboter
	_wxCache       workwxrobot.Roboter
	_larkCache     *larkrobot.Client
}

type ConfigOpt func(*Client)

// WithWebhookWithSecret 配置带签名密钥的 webhook（导出名为 bot.webhookWithSecret / bot.ding）
// 会根据 webhook 域名自动识别钉钉、飞书或企业微信类型
// 参数:
//   - webhook: 机器人 webhook 地址
//   - key: 加签密钥（secret）
//
// 返回值:
//   - 机器人配置可选项
//
// Example:
// ```
// // 示意性示例，需替换为真实 webhook 与密钥
// client = bot.New(bot.ding("https://oapi.dingtalk.com/robot/send?access_token=xxx", "SECxxx"))
// client.SendText("hello with secret")
// ```
func WithWebhookWithSecret(webhook string, key string) ConfigOpt {
	return func(c *Client) {
		u, err := url.Parse(webhook)
		if err != nil {
			log.Errorf("parse webhook url[%v] failed: %s", webhook, err)
			return
		}
		item := &Config{}
		switch true {
		case utils.MatchAllOfGlob(u.Host, "*.dingtalk.*"):
			item.BotType = BotType_DingTalk
		case utils.MatchAnyOfGlob(u.Host, "*.feishu.*", "*.lark.*"):
			item.BotType = BotType_Feishu
		case utils.MatchAnyOfGlob(u.Host, "*.weixin.*", "*.qq.*"):
			item.BotType = BotType_WorkWechat
		default:
			if u.Host != "" {
				log.Errorf("webhook host: %s, cannot identify botType", u.Host)
			}
			return
		}

		item.Webhook = webhook
		item.Secret = key
		c.config = append(c.config, item)
	}
}

// WithWebhook 配置无密钥的 webhook（导出名为 bot.webhook / bot.workwx）
// 会根据 webhook 域名自动识别钉钉、飞书或企业微信类型
// 参数:
//   - webhook: 机器人 webhook 地址
//
// 返回值:
//   - 机器人配置可选项
//
// Example:
// ```
// // 示意性示例，需替换为真实 webhook 地址
// client = bot.New(bot.webhook("https://qyapi.weixin.qq.com/cgi-bin/webhook/send?key=xxx"))
// client.SendText("hello workwx")
// ```
func WithWebhook(webhook string) ConfigOpt {
	return WithWebhookWithSecret(webhook, "")
}

func WithDelaySeconds(i float64) ConfigOpt {
	return func(client *Client) {
		client.delaySeconds = i
	}
}
