package yakgrpc

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"strings"
	"sync"
	"time"

	"github.com/yaklang/yaklang/common/ai"
	"github.com/yaklang/yaklang/common/ai/aispec"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
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
	//if config.APIKey == "" {
	//	return nil, utils.Errorf("list ai failed, config.APIKey is empty")
	//}
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

func (s *Server) TestAIModel(ctx context.Context, req *ypb.TestAIModelRequest) (*ypb.TestAIModelResponse, error) {
	if req == nil {
		return nil, utils.Error("request is nil")
	}
	if req.GetConfig() == nil {
		return nil, utils.Error("config is nil")
	}
	if strings.TrimSpace(req.GetContent()) == "" {
		return nil, utils.Error("content is empty")
	}

	providerType := strings.TrimSpace(req.GetConfig().GetType())
	if providerType == "" {
		return nil, utils.Error("config.type is empty")
	}
	if !ai.HaveAI(providerType) {
		return nil, utils.Errorf("unsupported ai type: %s", providerType)
	}

	resp := &ypb.TestAIModelResponse{}
	start := time.Now()
	var firstByteOnce sync.Once
	markFirstByte := func(reader io.Reader) {
		buffered := bufio.NewReader(reader)
		if _, err := buffered.ReadByte(); err == nil {
			firstByteOnce.Do(func() {
				resp.FirstByteCostMs = time.Since(start).Milliseconds()
			})
			_ = buffered.UnreadByte()
		}
		_, _ = io.Copy(io.Discard, buffered)
	}

	opts := aispec.BuildOptionsFromConfig(&ypb.AIModelConfig{
		Provider: req.GetConfig(),
	})
	opts = append(opts,
		aispec.WithContext(ctx),
		aispec.WithStreamHandler(markFirstByte),
		aispec.WithReasonStreamHandler(markFirstByte),
		aispec.WithRawHTTPRequestResponseCallback(func(requestBytes []byte, responseHeaderBytes []byte, bodyPreview []byte) {
			resp.RawRequest = string(requestBytes)
			resp.ResponseStatusCode = int32(lowhttp.GetStatusCodeFromResponse(responseHeaderBytes))
		}),
	)

	result, err := ai.Chat(req.GetContent(), opts...)
	resp.TotalCostMs = time.Since(start).Milliseconds()
	resp.ResponseContent = result
	if err != nil {
		resp.ErrorMessage = err.Error()
	}

	return resp, nil
}
