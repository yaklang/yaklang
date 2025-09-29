package aicommon

import (
	"context"
	"io"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aispec"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

type AICallbackType func(i AICallerConfigIf, req *AIRequest) (*AIResponse, error)

type AICallerConfigIf interface {
	AcquireId() int64
	GetRuntimeId() string
	IsCtxDone() bool
	GetContext() context.Context
	CallAIResponseConsumptionCallback(int)
	GetAITransactionAutoRetryCount() int64
	GetTimelineContentSizeLimit() int64
	RetryPromptBuilder(string, error) string
	GetEmitter() *Emitter
	NewAIResponse() *AIResponse
	CallAIResponseOutputFinishedCallback(string)

	// Interactivable
	Interactivable

	// Checkpointable
	CheckpointableStorage
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
				})}
			for _, data := range req.GetImageList() {
				if data.IsBase64 {
					optList = append(optList, aispec.WithImageBase64(string(data.Data)))
				} else {
					optList = append(optList, aispec.WithImageRaw(data.Data))
				}
			}
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
