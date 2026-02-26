package yakgrpc

import (
	"context"

	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func (s *Server) GetAIGlobalConfig(ctx context.Context, _ *ypb.Empty) (*ypb.AIGlobalConfig, error) {
	cfg, err := yakit.GetAIGlobalConfig(s.GetProfileDatabase())
	if err != nil {
		return nil, err
	}
	if cfg == nil {
		return &ypb.AIGlobalConfig{}, nil
	}
	return cfg, nil
}

func (s *Server) SetAIGlobalConfig(ctx context.Context, cfg *ypb.AIGlobalConfig) (*ypb.Empty, error) {
	if cfg == nil {
		return nil, utils.Error("config is nil")
	}
	normalized, err := yakit.SetAIGlobalConfig(s.GetProfileDatabase(), cfg)
	if err != nil {
		return nil, err
	}
	if err := yakit.ApplyAIGlobalConfig(s.GetProfileDatabase(), normalized); err != nil {
		return nil, err
	}
	return &ypb.Empty{}, nil
}

func (s *Server) ListAIProviders(ctx context.Context, _ *ypb.Empty) (*ypb.ListAIProvidersResponse, error) {
	providers, err := yakit.ListAIProviders(s.GetProfileDatabase())
	if err != nil {
		return nil, err
	}
	resp := &ypb.ListAIProvidersResponse{}
	for _, provider := range providers {
		if provider == nil {
			continue
		}
		resp.Providers = append(resp.Providers, provider.ToAIProvider())
	}
	return resp, nil
}

func (s *Server) UpsertAIProvider(ctx context.Context, req *ypb.UpsertAIProviderRequest) (*ypb.UpsertAIProviderResponse, error) {
	if req == nil || req.Provider == nil || req.Provider.Config == nil {
		return nil, utils.Error("provider config is required")
	}
	model := schema.AIThirdPartyConfigFromGRPC(req.Provider.Config)
	if req.Provider.Id > 0 {
		model.ID = uint(req.Provider.Id)
	}
	provider, err := yakit.UpsertAIProvider(s.GetProfileDatabase(), model)
	if err != nil {
		return nil, err
	}
	return &ypb.UpsertAIProviderResponse{Provider: provider.ToAIProvider()}, nil
}

func (s *Server) DeleteAIProvider(ctx context.Context, req *ypb.DeleteAIProviderRequest) (*ypb.Empty, error) {
	if req == nil || req.Id == 0 {
		return nil, utils.Error("provider id is required")
	}
	if err := yakit.DeleteAIProvider(s.GetProfileDatabase(), req.Id); err != nil {
		return nil, err
	}
	return &ypb.Empty{}, nil
}
