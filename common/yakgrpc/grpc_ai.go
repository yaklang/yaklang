package yakgrpc

import (
	"context"
	"encoding/json"

	"github.com/yaklang/yaklang/common/ai"
	"github.com/yaklang/yaklang/common/ai/aispec"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func (s *Server) ListAiModel(ctx context.Context, req *ypb.ListAiModelRequest) (*ypb.ListAiModelResponse, error) {
	if req == nil {
		return nil, utils.Error("request is nil")
	}
	if req.Config == "" {
		return nil, utils.Errorf("list ai failed, config is empty")
	}
	config := &aispec.AIConfig{}
	err := json.Unmarshal([]byte(req.Config), config)
	if err != nil {
		return nil, err
	}

	// check type is valid ai type
	if config.Type == "" {
		return nil, nil
	}
	_, ok := aispec.Lookup(config.Type)
	if !ok {
		return nil, nil
	}

	// list models
	models, err := ai.ListModels(
		aispec.WithAPIKey(config.APIKey),
		aispec.WithType(config.Type),
		aispec.WithNoHttps(config.NoHttps),
		aispec.WithDomain(config.Domain),
		aispec.WithProxy(config.Proxy),
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
