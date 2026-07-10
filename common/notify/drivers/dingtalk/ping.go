package dingtalk

import (
	"fmt"

	"github.com/yaklang/yaklang/common/notify"
)

// Ping 仅校验钉钉凭证：获取一次 access_token，不发送任何消息。
//
// 凭证无效时 oauth2/accessToken 端点返回错误，据此报错。
func Ping(opts ...notify.SendOption) error {
	cfg := notify.NewSendConfig(opts...)
	tm := newTokenManager(cfg)
	if _, err := tm.getToken(); err != nil {
		return fmt.Errorf("dingtalk: %w", err)
	}
	return nil
}
