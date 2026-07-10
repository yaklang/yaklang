package yakgrpc

import (
	"context"
	"encoding/base64"
	"fmt"

	yaklog "github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/notify"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

// isTerminal 判断 onboarding 状态是否为终态（推送后应结束流）。
func isTerminal(state string) bool {
	switch state {
	case "success", "expired", "error":
		return true
	}
	return false
}

// StartIMOnboarding 是 IM 扫码注册的平台无关单入口（对标 RequestYakURL 的设计）。
//
// 按 req.Platform 构造 notify://<platform>/onboarding:start 事件流，
// 流式推送状态给前端：qr(二维码URL+PNG) / pending / success(含凭证) / expired / error。
// 新增平台只需要提供 notify driver descriptor，本方法与 proto 保持平台无关。
func (s *Server) StartIMOnboarding(req *ypb.StartIMOnboardingRequest, stream ypb.Yak_StartIMOnboardingServer) error {
	ctx := stream.Context()
	platform := notify.PlatformType(req.GetPlatform())
	if platform == "" {
		return sendIMOnboardingError(stream, "缺少平台标识 Platform")
	}
	desc, err := imNotifyDescriptor(platform)
	if err != nil {
		return sendIMOnboardingError(stream, fmt.Sprintf("平台 %q 暂不支持扫码注册", platform))
	}

	reg := notify.NewRegistry()
	reg.Register(desc)
	client := notify.NewClient(notify.WithRegistry(reg))
	streamCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	var sendErr error
	err = client.Stream(streamCtx, &notify.Request{
		Platform: notify.Platform(platform),
		Action:   notify.ActionOnboardingStart,
		Options:  onboardingRequestOptions(req),
	}, func(ev notify.Event) {
		if ev.Onboarding == nil || sendErr != nil {
			return
		}
		pb := onboardingStepToPB(ev.Onboarding)
		if err := stream.Send(pb); err != nil {
			sendErr = err
			cancel()
			return
		}
		if isTerminal(ev.Onboarding.State) {
			cancel()
		}
	})
	if sendErr != nil {
		return sendErr
	}
	if err != nil && err != context.Canceled {
		yaklog.Warnf("im onboarding (%s) failed: %v", platform, err)
		_ = sendIMOnboardingError(stream, err.Error())
	}
	return nil
}

func onboardingRequestOptions(req *ypb.StartIMOnboardingRequest) notify.Options {
	opts := notify.Options{}
	for key, value := range req.GetOptions() {
		opts[key] = value
	}
	if req.GetTimeoutSeconds() > 0 {
		opts["timeout_seconds"] = int(req.GetTimeoutSeconds())
	}
	return opts
}

func onboardingStepToPB(step *notify.OnboardingStep) *ypb.IMOnboardingEvent {
	if step == nil {
		return &ypb.IMOnboardingEvent{}
	}
	ev := &ypb.IMOnboardingEvent{
		State:   step.State,
		QrUrl:   step.QrURL,
		Message: step.Message,
	}
	if len(step.QrPNG) > 0 {
		ev.QrImageBase64 = base64.StdEncoding.EncodeToString(step.QrPNG)
	}
	if step.Result != nil {
		ev.Bot = &ypb.IMBotConfig{
			Platform:  string(step.Result.Platform),
			AppId:     step.Result.AppID,
			AppSecret: step.Result.AppSecret,
			Enabled:   true,
			OwnerId:   step.Result.OwnerID,
		}
	}
	return ev
}

// sendIMOnboardingError 推送一个 error 终态事件后返回 nil（流正常结束）。
func sendIMOnboardingError(stream ypb.Yak_StartIMOnboardingServer, msg string) error {
	_ = stream.Send(&ypb.IMOnboardingEvent{State: "error", Message: msg})
	return nil
}
