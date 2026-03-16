package yakgrpc

import (
	"context"

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
	return &ypb.ListAIProvidersResponse{Providers: providers}, nil
}

func (s *Server) QueryAIProvider(ctx context.Context, req *ypb.QueryAIProvidersRequest) (*ypb.QueryAIProvidersResponse, error) {
	if req == nil {
		req = &ypb.QueryAIProvidersRequest{}
	}

	paging := req.GetPagination()
	if paging == nil {
		paging = &ypb.Paging{Page: 1, Limit: 10, OrderBy: "id", Order: "asc"}
	}
	if paging.GetPage() <= 0 {
		paging.Page = 1
	}
	if paging.GetLimit() == 0 {
		paging.Limit = 10
	}
	if paging.GetRawOrder() == "" && paging.GetOrderBy() == "" {
		paging.OrderBy = "id"
	}
	if paging.GetRawOrder() == "" && paging.GetOrder() == "" {
		paging.Order = "asc"
	}

	pag, providers, err := yakit.QueryAIProviders(s.GetProfileDatabase(), req.GetFilter(), paging)
	if err != nil {
		return nil, err
	}

	resp := &ypb.QueryAIProvidersResponse{
		Pagination: &ypb.Paging{
			Page:    int64(pag.Page),
			Limit:   int64(pag.Limit),
			OrderBy: paging.GetOrderBy(),
			Order:   paging.GetOrder(),
		},
		Total: int64(pag.TotalRecord),
	}

	resp.Providers = providers

	return resp, nil
}

func (s *Server) UpsertAIProvider(ctx context.Context, req *ypb.UpsertAIProviderRequest) (*ypb.UpsertAIProviderResponse, error) {
	if req == nil || req.Provider == nil || req.Provider.Config == nil {
		return nil, utils.Error("provider config is required")
	}
	provider, err := yakit.UpsertAIProvider(s.GetProfileDatabase(), req.Provider)
	if err != nil {
		return nil, err
	}
	return &ypb.UpsertAIProviderResponse{Provider: provider}, nil
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

func (s *Server) GetAIThirdPartyAppConfigTemplate(ctx context.Context, _ *ypb.Empty) (*ypb.GetThirdPartyAppConfigTemplateResponse, error) {
	templates, err := buildAIGatewayTemplates()
	if err != nil {
		return nil, err
	}
	return &ypb.GetThirdPartyAppConfigTemplateResponse{Templates: templates}, nil
}
