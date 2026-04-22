package aicommon

import (
	"context"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"io"
	"strings"

	"github.com/yaklang/yaklang/common/ai"
	"github.com/yaklang/yaklang/common/ai/aid/aitool/buildinaitools"
	"github.com/yaklang/yaklang/common/ai/aispec"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

type AICallbackType func(i AICallerConfigIf, req *AIRequest) (*AIResponse, error)

type aiGenerationConfigProvider interface {
	GetTemperatureConfig() *float64
	GetTopPConfig() *float64
	GetTopKConfig() *int64
	GetMaxTokensConfig() *int64
	GetPresencePenaltyConfig() *float64
	GetFrequencyPenaltyConfig() *float64
}

func appendGenerationOptionsFromConfig(opts []aispec.AIConfigOption, config any) []aispec.AIConfigOption {
	provider, ok := config.(aiGenerationConfigProvider)
	if !ok {
		return opts
	}
	if temperature := provider.GetTemperatureConfig(); temperature != nil {
		opts = append(opts, aispec.WithTemperature(*temperature))
	}
	if topP := provider.GetTopPConfig(); topP != nil {
		opts = append(opts, aispec.WithTopP(*topP))
	}
	if topK := provider.GetTopKConfig(); topK != nil {
		opts = append(opts, aispec.WithTopK(*topK))
	}
	if maxTokens := provider.GetMaxTokensConfig(); maxTokens != nil {
		opts = append(opts, aispec.WithMaxTokens(*maxTokens))
	}
	if presencePenalty := provider.GetPresencePenaltyConfig(); presencePenalty != nil {
		opts = append(opts, aispec.WithPresencePenalty(*presencePenalty))
	}
	if frequencyPenalty := provider.GetFrequencyPenaltyConfig(); frequencyPenalty != nil {
		opts = append(opts, aispec.WithFrequencyPenalty(*frequencyPenalty))
	}
	return opts
}

type AICallerConfigIf interface {
	AICaller
	KeyValueConfigIf

	// Interactivable
	Interactivable

	// Checkpointable
	CheckpointableStorage

	AcquireId() int64
	GetRuntimeId() string
	IsCtxDone() bool
	GetContext() context.Context
	CallAIResponseConsumptionCallback(int)
	GetAITransactionAutoRetryCount() int64
	GetToolComposeConcurrency() int
	GetTimelineContentSizeLimit() int64
	GetUserInteractiveLimitedTimes() int64
	GetMaxIterationCount() int64
	GetAllowUserInteraction() bool
	RetryPromptBuilder(string, error) string
	GetEmitter() *Emitter
	NewAIResponse() *AIResponse
	CallAIResponseOutputFinishedCallback(string)
	GetAiToolManager() *buildinaitools.AiToolManager
	OriginOptions() []ConfigOption
	GetOrCreateWorkDir() string
	GetContextProviderManager() *ContextProviderManager
	GetSessionEvidenceRendered() string
	ApplySessionEvidenceOps(ops []EvidenceOperation)
}

func AIChatToAICallbackType(cb func(prompt string, opts ...aispec.AIConfigOption) (string, error)) AICallbackType {
	return func(aicf AICallerConfigIf, req *AIRequest) (*AIResponse, error) {
		resp := NewAIResponse(aicf)
		go func() {
			defer resp.Close()
			isStream := false
			optList := []aispec.AIConfigOption{
				aispec.WithStreamHandler(func(reader io.Reader) {
					isStream = true
					resp.EmitOutputStream(reader)
				}),
				aispec.WithReasonStreamHandler(func(reader io.Reader) {
					isStream = true
					resp.EmitReasonStream(reader)
				}),
				aispec.WithModelInfoCallback(func(provider, model string) {
					resp.SetModelInfo(provider, model)
					if cfg, ok := aicf.(interface{ UpdateAIModelInfo(string, string) }); ok {
						cfg.UpdateAIModelInfo(provider, model)
					}
					log.Infof("ai request %v:%v is sent with request body: %v bytes",
						provider, model, len(req.GetPrompt()))
				}),
				aispec.WithModelInfoConfirmCallback(func(provider, model string) {
					resp.SetModelInfo(provider, model)
				}),
				aispec.WithRawHTTPResponseHeaderCallback(func(headerBytes []byte) {
					resp.SetRawHTTPResponseHeader(headerBytes)
				}),
				aispec.WithRawHTTPResponseCallback(func(headerBytes []byte, bodyPreview []byte) {
					resp.SetRawHTTPResponseData(headerBytes, bodyPreview)
				}),
			}
			for _, data := range req.GetImageList() {
				if data.IsBase64 {
					optList = append(optList, aispec.WithImageBase64(string(data.Data)))
				} else {
					optList = append(optList, aispec.WithImageRaw(data.Data))
				}
			}
			optList = appendGenerationOptionsFromConfig(optList, aicf)
			output, err := cb(
				req.GetPrompt(),
				optList...,
			)
			if err != nil {
				log.Errorf("chat error: %v", err)
			}
			if !isStream {
				resp.EmitOutputStream(strings.NewReader(output))
			}
		}()
		return resp, nil
	}
}

type ProxyAICaller struct {
	proxyFunc func(request *AIRequest) *AIRequest
	callFunc  func(request *AIRequest) (*AIResponse, error)
}

type AICaller interface {
	CallAI(request *AIRequest) (*AIResponse, error)
	CallSpeedPriorityAI(request *AIRequest) (*AIResponse, error)
	CallQualityPriorityAI(request *AIRequest) (*AIResponse, error)
}

func CreateProxyAICaller(
	caller AICaller,
	proxyFunc func(request *AIRequest) *AIRequest,
) *ProxyAICaller {
	return &ProxyAICaller{
		callFunc:  caller.CallAI,
		proxyFunc: proxyFunc,
	}
}

func (p ProxyAICaller) CallAI(request *AIRequest) (*AIResponse, error) {
	if p.proxyFunc != nil {
		request = p.proxyFunc(request)
		if request == nil {
			return nil, utils.Error("proxy function returned nil request")
		}
	}
	if p.callFunc != nil {
		return p.callFunc(request)
	}
	return nil, utils.Error("proxy function returned nil request")
}

func (p ProxyAICaller) CallSpeedPriorityAI(request *AIRequest) (*AIResponse, error) {
	return p.CallAI(request)
}

func (p ProxyAICaller) CallQualityPriorityAI(request *AIRequest) (*AIResponse, error) {
	return p.CallAI(request)
}

func LoadAIService(typeName string, opts ...aispec.AIConfigOption) (AICallbackType, error) {
	chatter, err := ai.LoadChater(typeName, opts...)
	if err != nil {
		return nil, err
	}
	return AIChatToAICallbackType(chatter), nil
}

// CreateCallbackFromConfig creates an AICallbackType from an AIModelConfig.
func CreateCallbackFromConfig(config *ypb.AIModelConfig) (AICallbackType, error) {
	if config == nil {
		return nil, utils.Error("config is nil")
	}
	if config.GetProvider() == nil {
		return nil, utils.Error("provider config is nil")
	}

	opts := aispec.BuildOptionsFromConfig(config)
	return LoadAIService(config.GetProvider().GetType(), opts...)
}
