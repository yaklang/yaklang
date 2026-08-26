package yakgrpc

import (
	"context"
	"strings"

	"github.com/yaklang/yaklang/common/ai"
	"github.com/yaklang/yaklang/common/ai/aispec"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
	"github.com/yaklang/yaklang/common/yak/yaklib"
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

func (s *Server) GetApiKeyByOnline(ctx context.Context, req *ypb.GetApiKeyByOnlineRequest) (*ypb.GetApiKeyByOnlineResponse, error) {
	cancelCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	client := yaklib.NewOnlineClient(consts.GetOnlineBaseUrl())
	apiKey, err := client.GetAIApiKeyByOnline(cancelCtx, req.Token)
	if err != nil {
		return nil, err
	}

	cfg, err := s.GetAIGlobalConfig(ctx, &ypb.Empty{})
	if err != nil {
		return nil, err
	}
	if cfg != nil {
		for _, models := range [][]*ypb.AIModelConfig{
			cfg.IntelligentModels,
			cfg.LightweightModels,
			cfg.VisionModels,
		} {
			for _, model := range models {
				if yakit.IsBuiltinAIProvider(model) {
					model.Provider.APIKey = apiKey
				}
			}
		}
		// SetAIGlobalConfig 会写入并手动 Apply 到运行时缓存
		if _, err := s.SetAIGlobalConfig(ctx, cfg); err != nil {
			return nil, utils.Errorf("update AIGlobalConfig failed: %v", err)
		}
	}

	return &ypb.GetApiKeyByOnlineResponse{ApiKey: apiKey}, nil
}

func (s *Server) UpdateApiKey(ctx context.Context, req *ypb.UpdateApiKeyRequest) (*ypb.Empty, error) {
	if req.ApiKey == "" {
		return nil, utils.Errorf("params is empty")
	}
	cfg, err := s.GetAIGlobalConfig(ctx, &ypb.Empty{})
	if err != nil {
		return nil, err
	}
	if cfg != nil {
		for _, models := range [][]*ypb.AIModelConfig{
			cfg.IntelligentModels,
			cfg.LightweightModels,
			cfg.VisionModels,
		} {
			for _, model := range models {
				if model.Provider == nil {
					continue
				}
				model.Provider.APIKey = req.ApiKey
			}
		}
		if _, err := s.SetAIGlobalConfig(ctx, cfg); err != nil {
			return nil, utils.Errorf("UpdateApiKey failed: %v", err)
		}
	}

	return &ypb.Empty{}, nil
}

func (s *Server) ProbeReasoningEffort(ctx context.Context, req *ypb.ProbeReasoningEffortRequest) (*ypb.ProbeReasoningEffortResponse, error) {
	if req == nil || req.Config == nil {
		return nil, utils.Error("config is nil")
	}
	if strings.TrimSpace(req.Config.GetType()) == "" {
		return nil, utils.Error("config.type is empty")
	}
	if !ai.HaveAI(req.Config.GetType()) {
		return nil, utils.Errorf("unsupported ai type: %s", req.Config.GetType())
	}

	model := strings.TrimSpace(req.GetModel())
	resp := &ypb.ProbeReasoningEffortResponse{}

	probeOne := func(effort string) (bool, string) {
		config := cloneThirdPartyApplicationConfig(req.Config)
		config.ReasoningEffort = &effort

		var statusCode int32
		var errMsg string

		opts := aispec.BuildOptionsFromConfig(&ypb.AIModelConfig{
			Provider:  config,
			ModelName: model,
		})
		opts = append(opts,
			aispec.WithContext(ctx),
			aispec.WithDisableProviderFallback(true),
			aispec.WithDisableStream(true),
			aispec.WithRawHTTPRequestResponseCallback(func(_ []byte, headerBytes []byte, bodyPreview []byte, _ *aispec.ChatUsage) {
				statusCode = int32(lowhttp.GetStatusCodeFromResponse(headerBytes))
				if statusCode >= 400 {
					errMsg = string(bodyPreview)
				}
			}),
		)

		result, err := ai.Chat("hi", opts...)
		if err != nil {
			return false, err.Error()
		}
		if statusCode == 0 {
			return false, "no response received"
		}
		if statusCode >= 400 {
			return false, errMsg
		}
		if isLikelyErrorResponse(result) {
			return false, result
		}
		return true, ""
	}

	xhighOK, xhighErr := probeOne("xhigh")
	resp.XhighSupported = xhighOK
	if !xhighOK {
		resp.XhighErrorMessage = xhighErr
	}

	maxOK, maxErr := probeOne("max")
	resp.MaxSupported = maxOK
	if !maxOK {
		resp.MaxErrorMessage = maxErr
	}

	return resp, nil
}

func isLikelyErrorResponse(result string) bool {
	result = strings.TrimSpace(result)
	if result == "" {
		return true
	}
	lower := strings.ToLower(result)
	if strings.Contains(lower, "\"error\"") && strings.Contains(lower, "\"message\"") {
		return true
	}
	return false
}
