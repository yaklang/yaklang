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
}

// BaseInteractiveHandler 基础交互处理器
type BaseInteractiveHandler struct {
	Interactivable

	endpointManager *EndpointManager
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
