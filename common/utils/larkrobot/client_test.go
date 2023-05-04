package larkrobot

import "testing"

func TestNewClient(t *testing.T) {
	client := NewClient("https://open.feishu.cn/open-apis/bot/v2/hook/ee730f6d-63bf-4154-9a2c-805540fce3b0")
	client.SendMessage(NewTextMessage("hello", false))
}
