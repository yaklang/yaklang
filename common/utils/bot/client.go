package bot

import (
	"github.com/yaklang/yaklang/common/utils"
	"os"
)

type Client struct {
	config       []*Config
	delaySeconds float64

	cooldown *utils.CoolDown
}

// New 创建一个机器人客户端，用于向钉钉、企业微信、飞书等推送消息
// 通过 bot.webhook / bot.ding / bot.workwx / bot.webhookWithSecret 等可选项配置目标 webhook
// 参数:
//   - opts: 机器人配置可选项
//
// 返回值:
//   - 机器人客户端对象，可调用 SendText / SendMarkdown 等方法推送消息
//
// Example:
// ```
// // 示意性示例，需替换为真实 webhook 地址
// client = bot.New(bot.webhook("https://oapi.dingtalk.com/robot/send?access_token=xxx"))
// client.SendText("hello from yak")
// ```
func New(opts ...ConfigOpt) *Client {
	client := &Client{
		config:       nil,
		delaySeconds: 1,
	}
	for _, p := range opts {
		p(client)
	}

	if client.delaySeconds <= 0 {
		client.delaySeconds = 1
	}
	client.cooldown = utils.NewCoolDown(utils.FloatSecondDuration(client.delaySeconds))
	return client
}

// FromEnv 从环境变量读取 webhook 配置创建机器人客户端
// 读取 YAKIT_DINGTALK_WEBHOOK/SECRET、YAKIT_WORKWX_WEBHOOK/SECRET、YAKIT_FEISHU_WEBHOOK/SECRET
// 返回值:
//   - 机器人客户端对象
//
// Example:
// ```
// // 示意性示例，需要预先设置相关环境变量
// client = bot.FromEnv()
// client.SendText("hello from env")
// ```
func FromEnv() *Client {
	var opts []ConfigOpt
	if os.Getenv("YAKIT_DINGTALK_WEBHOOK") != "" {
		opts = append(opts, WithWebhookWithSecret(os.Getenv("YAKIT_DINGTALK_WEBHOOK"), os.Getenv("YAKIT_DINGTALK_SECRET")))
	}
	if os.Getenv("YAKIT_WORKWX_WEBHOOK") != "" {
		opts = append(opts, WithWebhookWithSecret(os.Getenv("YAKIT_WORKWX_WEBHOOK"), os.Getenv("YAKIT_WORKWX_SECRET")))
	}
	if os.Getenv("YAKIT_FEISHU_WEBHOOK") != "" {
		opts = append(opts, WithWebhookWithSecret(os.Getenv("YAKIT_FEISHU_WEBHOOK"), os.Getenv("YAKIT_FEISHU_SECRET")))
	}
	return New(opts...)
}

func (s *Client) Configs() []*Config {
	return s.config
}

func (c *Client) SendText(text string, items ...interface{}) {
	if c == nil || len(c.config) <= 0 {
		return
	}
	c.cooldown.Do(func() {
		for _, i := range c.config {
			i.SendText(text, items...)
		}
	})
}

func (c *Client) SendMarkdown(text string) {
	if c == nil || len(c.config) <= 0 {
		return
	}
	c.cooldown.Do(func() {
		for _, i := range c.config {
			i.SendMarkdown(text)
		}
	})
}
