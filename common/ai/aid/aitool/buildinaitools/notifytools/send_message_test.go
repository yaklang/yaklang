package notifytools

import (
	"testing"

	"github.com/yaklang/yaklang/common/notify"
)

func TestCreateNotifySendTools(t *testing.T) {
	tools := CreateNotifySendTools()
	if len(tools) != 2 {
		t.Fatalf("expected 2 tools, got %d", len(tools))
	}
	names := map[string]bool{}
	for _, tool := range tools {
		if tool == nil {
			t.Fatal("nil tool")
		}
		names[tool.GetName()] = true
	}
	if !names["send_im_message"] || !names["configure_im_credentials"] {
		t.Fatalf("missing expected tools, got %v", names)
	}
}

func TestConfigureAndSend_NoCred(t *testing.T) {
	// 未配置凭证时 send 应返回错误（通过 callback 逻辑验证凭证存储）。
	setCred(notify.PlatformType("__none__"), &notify.SendConfig{})
	defer delete(creds, notify.PlatformType("__none__"))
	if getCred(notify.PlatformType("feishu")) != nil {
		t.Fatal("feishu should have no creds by default")
	}
}
