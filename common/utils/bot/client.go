package bot

import (
	"os"
	"yaklang.io/yaklang/common/utils"
)

type Client struct {
	config       []*Config
	delaySeconds float64

	cooldown *utils.CoolDown
}

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
