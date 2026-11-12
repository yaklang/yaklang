package yakgrpc

import (
	"context"
	"strings"

	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"google.golang.org/grpc/codes"
	grpcstatus "google.golang.org/grpc/status"
)

func (s *Server) GetAIModelContext(_ context.Context, req *ypb.GetAIModelContextRequest) (*ypb.AIModelContextResponse, error) {
	provider, model := strings.TrimSpace(req.GetProvider()), strings.TrimSpace(req.GetModel())
	if provider == "" || model == "" {
		return nil, grpcstatus.Error(codes.InvalidArgument, "provider and model are required")
	}
	value := yakit.GetAIModelContext(s.GetProfileDatabase(), provider, model)
	return &ypb.AIModelContextResponse{
		Provider: provider, Model: model,
		ContextTokens: value.ContextTokens, ThresholdTokens: value.ThresholdTokens, Source: value.Source,
		MinimumThresholdTokens: yakit.MinimumAIModelContextTokens,
		TooSmall:               value.ContextTokens < yakit.MinimumAIModelContextTokens,
		DisableAutoAdjust:      value.DisableAutoAdjust,
	}, nil
}

func (s *Server) SetAIModelContext(ctx context.Context, req *ypb.SetAIModelContextRequest) (*ypb.AIModelContextResponse, error) {
	identity := &ypb.GetAIModelContextRequest{Provider: req.GetProvider(), Model: req.GetModel()}
	current, err := s.GetAIModelContext(ctx, identity)
	if err != nil {
		return nil, err
	}
	if err := yakit.SetAIModelContextSettings(s.GetProfileDatabase(), current.Provider, current.Model,
		req.GetThresholdTokens(), req.DisableAutoAdjust); err != nil {
		return nil, grpcstatus.Error(codes.InvalidArgument, err.Error())
	}

	return s.GetAIModelContext(ctx, identity)
}
