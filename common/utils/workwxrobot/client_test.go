package workwxrobot

import "testing"

func TestNewRobot(t *testing.T) {
	NewRobot(`https://qyapi.weixin.qq.com/cgi-bin/webhook/send?key=4ccf3c46-a1f4-433e-95b5-ca88da9ef2f4`).Send(&WxBotMessage{
		MsgType:  "markdown",
		BotText:  BotText{},
		MarkDown: BotMarkDown{
			Content: "Test Message",
		},
		Image:    BotImage{},
		News:     News{},
		File:     Media{},
	})
}
