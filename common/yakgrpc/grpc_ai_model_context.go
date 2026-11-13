package yakgrpc

import (
	"context"
	"errors"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aispec"

	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"google.golang.org/grpc/codes"
	grpcstatus "google.golang.org/grpc/status"
)

func (s *Server) GetAIModelContext(_ context.Context, req *ypb.GetAIModelContextRequest) (*ypb.AIModelContextResponse, error) {
	provider, model := strings.TrimSpace(req.GetProvider()), strings.TrimSpace(req.GetModel())
	key := req.GetContextKey()
	if cfg := req.GetConfig(); cfg != nil {
		provider, model = cfg.GetProvider().GetType(), cfg.GetModelName()
		key = aispec.ModelContextIdentity(cfg)
	}
	if provider == "" || model == "" || aispec.ModelContextOwnerID(key) == "" {
		return nil, grpcstatus.Error(codes.InvalidArgument, "a saved provider configuration and model context identity are required")
	}
	if err := yakit.ValidateAIModelContextOwner(s.GetProfileDatabase(), key); err != nil {
		return nil, grpcstatus.Error(codes.NotFound, err.Error())
	}

	value := yakit.GetAIModelContext(s.GetProfileDatabase(), provider, model, key)
	return &ypb.AIModelContextResponse{
		Provider: provider, Model: model, ContextKey: key,
		ThresholdTokens: value.ThresholdTokens, Source: value.Source,
		MinimumThresholdTokens: yakit.MinimumAIModelContextTokens,
		DisableAutoAdjust:      value.DisableAutoAdjust,
	}, nil
}

func (s *Server) SetAIModelContext(ctx context.Context, req *ypb.SetAIModelContextRequest) (*ypb.AIModelContextResponse, error) {
	identity := &ypb.GetAIModelContextRequest{Provider: req.GetProvider(), Model: req.GetModel(), Config: req.GetConfig(), ContextKey: req.GetContextKey()}
	current, err := s.GetAIModelContext(ctx, identity)
	if err != nil {
		return nil, err
	}
	if err := yakit.SetAIModelContextSettings(s.GetProfileDatabase(), current.Provider, current.Model,
		req.GetThresholdTokens(), req.DisableAutoAdjust, current.ContextKey); err != nil {
		if errors.Is(err, yakit.ErrAIModelContextOwnerDeleted) {
			return nil, grpcstatus.Error(codes.NotFound, err.Error())
		}
		return nil, grpcstatus.Error(codes.InvalidArgument, err.Error())
	}

	return s.GetAIModelContext(ctx, identity)
}
