package bot

import (
	"net/url"
	"yaklang/common/log"
	"yaklang/common/utils"
	"yaklang/common/utils/dingrobot"
	"yaklang/common/utils/larkrobot"
	"yaklang/common/utils/workwxrobot"
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

func WithWebhook(webhook string) ConfigOpt {
	return WithWebhookWithSecret(webhook, "")
}

func WithDelaySeconds(i float64) ConfigOpt {
	return func(client *Client) {
		client.delaySeconds = i
	}
}
