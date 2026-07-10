package yakgrpc

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/yaklang/yaklang/common/notify"
	"github.com/yaklang/yaklang/common/notify/credential"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

// toPB 把持久化 BotConfig 映射成 proto IMBotConfig。
func botToPB(b *credential.BotConfig) *ypb.IMBotConfig {
	if b == nil {
		return nil
	}
	return &ypb.IMBotConfig{
		Platform:           b.Platform,
		AppId:              b.AppID,
		AppSecret:          b.AppSecret,
		RobotSecret:        b.RobotSecret,
		BaseUrl:            b.BaseURL,
		Enabled:            b.Enabled,
		OwnerId:            b.OwnerID,
		AllowedUsers:       parseJSONStringSlice(b.AllowedUsers),
		AllowedChats:       parseJSONStringSlice(b.AllowedChats),
		GroupAccessControl: b.GroupAccessControl,
	}
}

// pbToBot 把 proto IMBotConfig 映射成持久化 BotConfig。
func pbToBot(p *ypb.IMBotConfig) *credential.BotConfig {
	if p == nil {
		return nil
	}
	return &credential.BotConfig{
		Platform:           p.GetPlatform(),
		AppID:              p.GetAppId(),
		AppSecret:          p.GetAppSecret(),
		RobotSecret:        p.GetRobotSecret(),
		BaseURL:            p.GetBaseUrl(),
		Enabled:            p.GetEnabled(),
		OwnerID:            p.GetOwnerId(),
		AllowedUsers:       marshalStringSlice(p.GetAllowedUsers()),
		AllowedChats:       marshalStringSlice(p.GetAllowedChats()),
		GroupAccessControl: p.GetGroupAccessControl(),
	}
}

// parseJSONStringSlice 把 JSON 字符串数组解析成 []string；空/非法返回 nil。
func parseJSONStringSlice(raw string) []string {
	if raw == "" {
		return nil
	}
	var out []string
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return nil
	}
	return out
}

// marshalStringSlice 把 []string 序列化成 JSON 字符串（BotConfig.AllowedUsers/AllowedChats 存 JSON 字符串）。
func marshalStringSlice(s []string) string {
	if len(s) == 0 {
		return ""
	}
	b, err := json.Marshal(s)
	if err != nil {
		return ""
	}
	return string(b)
}

// SaveIMBot 保存（按平台 upsert）一条 IM bot 凭证。
func (s *Server) SaveIMBot(ctx context.Context, req *ypb.SaveIMBotRequest) (*ypb.SaveIMBotResponse, error) {
	if req == nil || req.GetBot() == nil {
		return nil, fmt.Errorf("bot config is required")
	}
	bot := pbToBot(req.GetBot())
	bot.ClearOwnerID = req.GetClearOwnerId()
	saved, err := credential.SaveBotConfig(bot)
	if err != nil {
		return nil, err
	}
	// 凭证变更后，如果 IM Engine 在运行，自动刷新该平台的监听（用新凭证重建长连接）
	s.tryRestartIMPlatform(saved.Platform)
	if saved.OwnerID != "" {
		s.scheduleIMOnboardingWelcome(saved)
	} else {
		clearPendingIMOnboardingWelcome(saved.Platform)
	}
	return &ypb.SaveIMBotResponse{Bot: botToPB(saved)}, nil
}

// ListIMBots 列出所有已配置的 IM bot。
func (s *Server) ListIMBots(ctx context.Context, req *ypb.ListIMBotRequest) (*ypb.ListIMBotResponse, error) {
	bots, err := credential.ListBotConfigs()
	if err != nil {
		return nil, err
	}
	resp := &ypb.ListIMBotResponse{}
	for _, b := range bots {
		resp.Bots = append(resp.Bots, botToPB(b))
	}
	return resp, nil
}

// DeleteIMBot 按平台删除一条 IM bot 配置。
func (s *Server) DeleteIMBot(ctx context.Context, req *ypb.DeleteIMBotRequest) (*ypb.DeleteIMBotResponse, error) {
	if req == nil || req.GetPlatform() == "" {
		return nil, fmt.Errorf("platform is required")
	}
	if err := credential.DeleteBotConfig(req.GetPlatform()); err != nil {
		return nil, err
	}
	clearPendingIMOnboardingWelcome(req.GetPlatform())
	// bot 删除后，如果 IM Engine 在运行，停止该平台的监听
	s.tryRestartIMPlatform(req.GetPlatform())
	return &ypb.DeleteIMBotResponse{}, nil
}

// TestIMBot 用传入凭证做一次连接自检：向指定目标发送一条测试消息。
//
// 当 TargetId 为空时，自检退化为「尝试获取平台 access_token」是否成功，
// 这能在不实际发消息的情况下验证凭证正确性（更安全，避免误发）。
func (s *Server) TestIMBot(ctx context.Context, req *ypb.TestIMBotRequest) (*ypb.TestIMBotResponse, error) {
	if req == nil || req.GetBot() == nil {
		return nil, fmt.Errorf("bot config is required")
	}
	bot := req.GetBot()
	platform := notify.PlatformType(bot.GetPlatform())

	// 1) 优先：直接发送一条测试消息到指定目标。
	if target := req.GetTargetId(); target != "" {
		err := sendIMTestMessage(platform, target, "[yakit] IM bot 连接测试成功 ✓", pbToBot(bot).ToSendConfig())
		if err != nil {
			return &ypb.TestIMBotResponse{Ok: false, Message: err.Error()}, nil
		}
		return &ypb.TestIMBotResponse{Ok: true, Message: "测试消息已发送"}, nil
	}

	// 2) 退化：无目标时，验证凭证能换取 token（各平台的 token 自检）。
	if err := verifyIMCredentials(platform, pbToBot(bot).ToSendConfig()); err != nil {
		return &ypb.TestIMBotResponse{Ok: false, Message: err.Error()}, nil
	}
	return &ypb.TestIMBotResponse{Ok: true, Message: "凭证校验通过"}, nil
}
