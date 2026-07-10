package feishu

import (
	"fmt"

	"github.com/yaklang/yaklang/common/notify"
)

// Ping 仅校验飞书凭证：获取一次 tenant_access_token，不发送任何消息。
//
// 凭证无效时 token 端点返回非 0 code，据此报错。
func Ping(opts ...notify.SendOption) error {
	cfg := notify.NewSendConfig(opts...)
	tm := newTokenManager(cfg)
	if _, err := tm.getToken(); err != nil {
		return fmt.Errorf("feishu: %w", err)
	}
	return nil
}
