package yakgrpc

import (
	"context"
	"sync"
	"time"

	"github.com/yaklang/yaklang/common/imcontrol"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/notify/credential"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

// imEngineMu 保护 Server.imEngine 的并发访问。
var imEngineMu sync.Mutex
var imEngineLifecycleCh = make(chan struct{})
var pendingIMOnboardingWelcomes = map[string]*credential.BotConfig{}

func signalIMEngineLifecycleLocked() {
	close(imEngineLifecycleCh)
	imEngineLifecycleCh = make(chan struct{})
}

func (s *Server) currentIMEngine() (*imcontrol.Engine, <-chan struct{}) {
	imEngineMu.Lock()
	defer imEngineMu.Unlock()
	return s.imEngine, imEngineLifecycleCh
}

// StartIMControl 启动 IM 远程控制引擎。
//
// 启动后，所有已配置且 Enabled 的 IM bot（飞书/钉钉）开始监听入站消息。
// 用户在 IM 端可通过斜杠命令控制会话，或发送普通消息交给 AI agent 执行。
func (s *Server) StartIMControl(ctx context.Context, req *ypb.StartIMControlRequest) (*ypb.StartIMControlResponse, error) {
	imEngineMu.Lock()
	defer imEngineMu.Unlock()

	if s.imEngine != nil {
		// 已启动，先停止旧的
		s.imEngine.Stop()
		s.imEngine = nil
		signalIMEngineLifecycleLocked()
	}

	cfg := imcontrol.Config{
		EngineAddr:                req.GetEngineAddr(),
		Platforms:                 req.GetPlatforms(),
		SessionIdleTimeoutSeconds: int(req.GetSessionIdleTimeoutSeconds()),
		ReplyQuote:                req.GetReplyQuote(),       // 默认 false；grpc 层在 proto 默认值下取 false
		ReplyGranularity:          req.GetReplyGranularity(), // standard / summary / detailed
		GroupTrigger:              req.GetGroupTrigger(),     // must_at / allow_all（allow_slash 兼容旧配置）
		ReviewPolicy:              req.GetReviewPolicy(),     // manual / ai / yolo
		PlatformConfigs:           buildIMRuntimePlatformConfigs(req.GetPlatformConfigs()),
	}
	engine := imcontrol.New(cfg)
	if err := engine.Start(); err != nil {
		log.Errorf("start im engine failed: %v", err)
		signalIMEngineLifecycleLocked()
		return &ypb.StartIMControlResponse{
			Started: false,
			Message: err.Error(),
		}, nil
	}
	s.imEngine = engine
	pendingWelcomes := drainPendingIMOnboardingWelcomesLocked()
	signalIMEngineLifecycleLocked()
	for _, bot := range pendingWelcomes {
		sendIMOnboardingWelcomeAsync(engine, bot)
	}

	return &ypb.StartIMControlResponse{
		Started: true,
		Message: "IM 远程控制已启动",
	}, nil
}

// StopIMControl 停止 IM 远程控制引擎。
func (s *Server) StopIMControl(ctx context.Context, req *ypb.StopIMControlRequest) (*ypb.StopIMControlResponse, error) {
	imEngineMu.Lock()
	defer imEngineMu.Unlock()

	if s.imEngine == nil {
		signalIMEngineLifecycleLocked()
		return &ypb.StopIMControlResponse{
			Stopped: false,
			Message: "IM 远程控制未运行",
		}, nil
	}
	s.imEngine.Stop()
	s.imEngine = nil
	signalIMEngineLifecycleLocked()
	return &ypb.StopIMControlResponse{
		Stopped: true,
		Message: "IM 远程控制已停止",
	}, nil
}

func cloneIMBotConfig(bot *credential.BotConfig) *credential.BotConfig {
	if bot == nil {
		return nil
	}
	cloned := *bot
	return &cloned
}

func drainPendingIMOnboardingWelcomesLocked() []*credential.BotConfig {
	if len(pendingIMOnboardingWelcomes) == 0 {
		return nil
	}
	out := make([]*credential.BotConfig, 0, len(pendingIMOnboardingWelcomes))
	for platform, bot := range pendingIMOnboardingWelcomes {
		out = append(out, cloneIMBotConfig(bot))
		delete(pendingIMOnboardingWelcomes, platform)
	}
	return out
}

func sendIMOnboardingWelcomeAsync(engine *imcontrol.Engine, bot *credential.BotConfig) {
	if engine == nil || bot == nil {
		return
	}
	go func() {
		if err := engine.SendOnboardingWelcome(bot); err != nil {
			log.Warnf("im onboarding: send owner welcome failed for %s/%s: %v", bot.Platform, bot.OwnerID, err)
		}
	}()
}

func (s *Server) scheduleIMOnboardingWelcome(bot *credential.BotConfig) {
	if bot == nil || bot.OwnerID == "" || !bot.Enabled {
		return
	}
	imEngineMu.Lock()
	engine := s.imEngine
	if engine == nil {
		pendingIMOnboardingWelcomes[bot.Platform] = cloneIMBotConfig(bot)
		imEngineMu.Unlock()
		return
	}
	imEngineMu.Unlock()
	sendIMOnboardingWelcomeAsync(engine, cloneIMBotConfig(bot))
}

func clearPendingIMOnboardingWelcome(platform string) {
	if platform == "" {
		return
	}
	imEngineMu.Lock()
	delete(pendingIMOnboardingWelcomes, platform)
	imEngineMu.Unlock()
}

// SubscribeIMControlState 订阅 IM 远程控制引擎运行状态。
func (s *Server) SubscribeIMControlState(req *ypb.SubscribeIMControlStateRequest, stream ypb.Yak_SubscribeIMControlStateServer) error {
	for {
		engine, lifecycleCh := s.currentIMEngine()
		if engine == nil {
			if err := stream.Send(stoppedIMControlStateEvent("engine_not_started")); err != nil {
				return err
			}
			select {
			case <-stream.Context().Done():
				return stream.Context().Err()
			case <-lifecycleCh:
				continue
			}
		}

		watcherID, stateCh := engine.SubscribeState()
		for {
			select {
			case <-stream.Context().Done():
				engine.UnsubscribeState(watcherID)
				return stream.Context().Err()
			case event, ok := <-stateCh:
				if !ok {
					engine.UnsubscribeState(watcherID)
					goto rebind
				}
				if err := stream.Send(event); err != nil {
					engine.UnsubscribeState(watcherID)
					return err
				}
			}
		}
	rebind:
		select {
		case <-stream.Context().Done():
			return stream.Context().Err()
		default:
		}
	}
}

func stoppedIMControlStateEvent(reason string) *ypb.IMControlStateEvent {
	return &ypb.IMControlStateEvent{
		TimestampUnixMs: time.Now().UnixMilli(),
		Reason:          reason,
		State: &ypb.IMControlState{
			Running: false,
		},
	}
}

// UpdateIMControlConfig 热更新 IM 回复配置（转发回复开关 / 回复颗粒度），无需重启 IM Engine。
func (s *Server) UpdateIMControlConfig(ctx context.Context, req *ypb.UpdateIMControlConfigRequest) (*ypb.UpdateIMControlConfigResponse, error) {
	imEngineMu.Lock()
	engine := s.imEngine
	imEngineMu.Unlock()

	if engine == nil {
		return &ypb.UpdateIMControlConfigResponse{
			Updated: false,
			Message: "IM 远程控制未运行",
		}, nil
	}
	platform := req.GetPlatform()
	engine.UpdateConfigForPlatform(platform, req.GetReplyQuote(), req.GetReplyGranularity())
	engine.UpdateGroupTriggerForPlatform(platform, req.GetGroupTrigger())
	engine.UpdateReviewPolicyForPlatform(platform, req.GetReviewPolicy())
	return &ypb.UpdateIMControlConfigResponse{
		Updated: true,
		Message: "配置已更新",
	}, nil
}

func buildIMRuntimePlatformConfigs(configs []*ypb.IMControlRuntimeConfig) map[string]imcontrol.RuntimePlatformConfig {
	if len(configs) == 0 {
		return nil
	}
	out := make(map[string]imcontrol.RuntimePlatformConfig, len(configs))
	for _, cfg := range configs {
		if cfg == nil || cfg.GetPlatform() == "" {
			continue
		}
		out[cfg.GetPlatform()] = imcontrol.RuntimePlatformConfig{
			Platform:         cfg.GetPlatform(),
			ReplyQuote:       cfg.GetReplyQuote(),
			ReplyGranularity: cfg.GetReplyGranularity(),
			GroupTrigger:     cfg.GetGroupTrigger(),
			ReviewPolicy:     cfg.GetReviewPolicy(),
		}
	}
	return out
}

// tryRestartIMPlatform 在 bot 凭证变更（保存/删除）后，如果 IM Engine 在运行，
// 自动用最新凭证重启该平台的监听。失败只记日志，不影响凭证保存操作本身。
func (s *Server) tryRestartIMPlatform(platform string) {
	if platform == "" {
		return
	}
	imEngineMu.Lock()
	engine := s.imEngine
	imEngineMu.Unlock()
	if engine == nil {
		return // IM Engine 未运行，无需刷新
	}
	go func() {
		if err := engine.RestartPlatform(platform); err != nil {
			log.Warnf("auto-restart IM platform %s failed: %v", platform, err)
		}
	}()
}
