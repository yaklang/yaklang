package aicommon

import (
	"context"

	"github.com/yaklang/yaklang/common/ai/aid/aitool"
)

type Interactivable interface {
	// wait and review
	Feed(endpointId string, params aitool.InvokeParams)
	GetEndpointManager() *EndpointManager
	DoWaitAgree(ctx context.Context, endpoint *Endpoint)
	CallAfterInteractiveEventReleased(string, aitool.InvokeParams)
	CallAfterReview(seq int64, reviewQuestion string, userInput aitool.InvokeParams)
	// SubmitReviewValueFeedbackFromEndpoint 通用审批价值评估监控入口 (plan/task/aiforge
	// 等通路), 由各 review 通路在 CallAfterReview 之后调用; 未注册 aive 实现时安全 no-op.
	SubmitReviewValueFeedbackFromEndpoint(ep *Endpoint, focusMode string, reviewQuestion string)
}

// BaseInteractiveHandler 基础交互处理器
type BaseInteractiveHandler struct {
	Interactivable

	endpointManager *EndpointManager
}

func NewBaseInteractiveHandler() *BaseInteractiveHandler {
	return &BaseInteractiveHandler{
		endpointManager: NewEndpointManager(),
	}
}

func (h *BaseInteractiveHandler) Feed(endpointId string, params aitool.InvokeParams) {
	h.endpointManager.Feed(endpointId, params)
}

func (h *BaseInteractiveHandler) GetEndpointManager() *EndpointManager {
	return h.endpointManager
}

func (h *BaseInteractiveHandler) DoWaitAgree(ctx context.Context, endpoint *Endpoint) {
	endpoint.Wait()
}

func (h *BaseInteractiveHandler) CallAfterInteractiveEventReleased(eventID string, invoke aitool.InvokeParams) {
	// save user interactive params
}

func (h *BaseInteractiveHandler) CallAfterReview(seq int64, reviewQuestion string, userInput aitool.InvokeParams) {
	// save user review input
}

func (h *BaseInteractiveHandler) SubmitReviewValueFeedbackFromEndpoint(ep *Endpoint, focusMode string, reviewQuestion string) {
	// no-op: 价值评估监控仅在具备完整 Config 的实现 (aicommon.Config) 上生效.
}
