package yakgrpc

import (
	"context"
	"fmt"
	"strings"

	"github.com/yaklang/yaklang/common/notify"
	dingtalkdriver "github.com/yaklang/yaklang/common/notify/drivers/dingtalk"
	feishudriver "github.com/yaklang/yaklang/common/notify/drivers/feishu"
)

func sendIMTestMessage(platform notify.PlatformType, targetID, content string, cfg *notify.SendConfig) error {
	reg := notify.NewRegistry()
	desc, err := imNotifyDescriptor(platform)
	if err != nil {
		return err
	}
	reg.Register(desc)
	client := notify.NewClient(notify.WithRegistry(reg), notify.WithSendConfig(cfg))
	_, err = client.Do(context.Background(), &notify.Request{
		Platform: notify.Platform(platform),
		Action:   notify.ActionMessagesSend,
		Target: notify.Target{
			ID:     targetID,
			Kind:   notify.TargetUser,
			Native: imNotifyTargetNative(platform, targetID),
		},
		Message: &notify.Message{
			Type: notify.MessageText,
			Text: content,
		},
	})
	return err
}

func imNotifyDescriptor(platform notify.PlatformType) (notify.Descriptor, error) {
	switch platform {
	case notify.PlatformFeishu:
		return feishudriver.Descriptor(), nil
	case notify.PlatformDingTalk:
		return dingtalkdriver.Descriptor(), nil
	default:
		return notify.Descriptor{}, fmt.Errorf("unknown platform %q", platform)
	}
}

func imNotifyTargetNative(platform notify.PlatformType, targetID string) notify.NativeOptions {
	if platform != notify.PlatformFeishu {
		return nil
	}
	switch {
	case strings.HasPrefix(targetID, "oc_"):
		return notify.NativeOptions{"receive_id_type": "chat_id"}
	case strings.HasPrefix(targetID, "ou_"):
		return notify.NativeOptions{"receive_id_type": "open_id"}
	case strings.HasPrefix(targetID, "on_"):
		return notify.NativeOptions{"receive_id_type": "union_id"}
	case strings.HasPrefix(targetID, "u_"):
		return notify.NativeOptions{"receive_id_type": "user_id"}
	}
	return nil
}

func verifyIMCredentials(platform notify.PlatformType, cfg *notify.SendConfig) error {
	reg := notify.NewRegistry()
	desc, err := imNotifyDescriptor(platform)
	if err != nil {
		return err
	}
	reg.Register(desc)
	client := notify.NewClient(notify.WithRegistry(reg), notify.WithSendConfig(cfg))
	_, err = client.Do(context.Background(), &notify.Request{
		Platform: notify.Platform(platform),
		Action:   notify.ActionPing,
	})
	return err
}
