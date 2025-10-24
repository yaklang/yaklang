package bot

import (
	"fmt"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils/dingrobot"
	"github.com/yaklang/yaklang/common/utils/larkrobot"
	"github.com/yaklang/yaklang/common/utils/workwxrobot"
)

func (c *Config) SendText(text string, items ...interface{}) {
	if c == nil {
		return
	}

	var finalText = text
	if len(items) > 0 {
		finalText = fmt.Sprintf(text, items...)
	}

	switch c.BotType {
	case BotType_Feishu:
		if c._larkCache == nil {
			robot := larkrobot.NewClient(c.Webhook)
			if c.Secret != "" {
				robot.Secret = c.Secret
			}
			c._larkCache = robot
		}
		if c._larkCache == nil {
			log.Error("lark failed")
			return
		}
		// 使用 TextMessage 而不是 SendMessageStr
		textMsg := larkrobot.NewTextMessage(finalText, false)
		rsp, err := c._larkCache.SendMessage(textMsg)
		if err != nil {
			log.Errorf("lark send message failed: %s", err)
			return
		}
		if !rsp.IsSuccess() {
			log.Errorf("lark bot[%v]: %v", rsp.Code, rsp.Msg)
		}
	case BotType_WorkWechat:
		if c._wxCache == nil {
			robot := workwxrobot.NewRobot(c.Webhook)
			c._wxCache = robot
		}
		if c._wxCache == nil {
			log.Error("work weixin failed")
			return
		}

		err := c._wxCache.Send(&workwxrobot.WxBotMessage{
			MsgType: "text",
			BotText: workwxrobot.BotText{
				Content: finalText,
			},
		})
		if err != nil {
			log.Errorf("wxbot failed: %s", err.Error())
		}
	case BotType_DingTalk:
		if c._dingtalkCache == nil {
			r := dingrobot.NewRobot(c.Webhook)
			if c.Secret != "" {
				r.SetSecret(c.Secret)
			}
			c._dingtalkCache = r
		}

		if c._dingtalkCache == nil {
			log.Error("dingtalk cannot found")
			return
		}
		err := c._dingtalkCache.SendText(
			finalText, nil, false,
		)
		if err != nil {
			log.Errorf("dingtalk notify failed: %s", err)
		}

	}
}

func (c *Config) SendMarkdown(text string) {
	if c == nil {
		return
	}

	var finalText = text
	switch c.BotType {
	case BotType_Feishu:
		if c._larkCache == nil {
			robot := larkrobot.NewClient(c.Webhook)
			if c.Secret != "" {
				robot.Secret = c.Secret
			}
			c._larkCache = robot
		}
		if c._larkCache == nil {
			log.Error("lark failed")
			return
		}
		// 使用 PostMessage 发送富文本
		postTags := larkrobot.NewPostTags(&larkrobot.TextTag{Text: finalText, UnEscape: false})
		postItems := larkrobot.NewPostItems("通知", postTags)
		langItem := larkrobot.NewLangPostItem("zh_cn", postItems)
		postMsg := larkrobot.NewPostMessage(langItem)
		rsp, err := c._larkCache.SendMessage(postMsg)
		if err != nil {
			log.Errorf("lark send message failed: %s", err)
			return
		}
		if !rsp.IsSuccess() {
			log.Errorf("lark bot[%v]: %v", rsp.Code, rsp.Msg)
		}
	case BotType_WorkWechat:
		if c._wxCache == nil {
			robot := workwxrobot.NewRobot(c.Webhook)
			c._wxCache = robot
		}
		if c._wxCache == nil {
			log.Error("work weixin failed")
			return
		}

		log.Info("start to send markdown message")
		err := c._wxCache.Send(&workwxrobot.WxBotMessage{
			MsgType:  "markdown",
			MarkDown: workwxrobot.BotMarkDown{Content: finalText},
		})
		if err != nil {
			log.Errorf("wxbot failed: %s", err.Error())
		}
	case BotType_DingTalk:
		if c._dingtalkCache == nil {
			r := dingrobot.NewRobot(c.Webhook)
			if c.Secret != "" {
				r.SetSecret(c.Secret)
			}
			c._dingtalkCache = r
		}

		if c._dingtalkCache == nil {
			log.Error("dingtalk cannot found")
			return
		}
		err := c._dingtalkCache.SendMarkdown(
			"",
			finalText, nil, false,
		)
		if err != nil {
			log.Errorf("dingtalk notify failed: %s", err)
		}
	}
}
