package yakgrpc

import (
	"context"
	"github.com/yaklang/yaklang/common/ai"
	"github.com/yaklang/yaklang/common/ai/aispec"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func (s *Server) ListAiModel(ctx context.Context, req *ypb.ListAiModelRequest) (*ypb.ListAiModelResponse, error) {
	if req == nil || req.Config == nil {
		return nil, utils.Error("request is nil")
	}
	config := req.Config
	models, err := ai.ListModels(
		aispec.WithAPIKey(config.GetApiKey()),
		aispec.WithType(config.GetModelType()),
		aispec.WithNoHttps(config.GetNoHTTPS()),
		aispec.WithDomain(config.GetDomain()),
		aispec.WithProxy(config.GetProxy()),
	)
	if err != nil {
		return nil, err
	}
	rsp := &ypb.ListAiModelResponse{}
	for _, model := range models {
		rsp.ModelName = append(rsp.ModelName, model.Id)
	}
	return rsp, nil
}
