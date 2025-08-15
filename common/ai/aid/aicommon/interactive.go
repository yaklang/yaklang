package aicommon

import (
	"context"
	"sync"
	"time"

	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils/chanx"
)

// InteractiveEventType 交互事件类型
type InteractiveEventType string

const (
	InteractiveEventTypeInput   InteractiveEventType = "input"
	InteractiveEventTypeRelease InteractiveEventType = "release"
	InteractiveEventTypeTimeout InteractiveEventType = "timeout"
	InteractiveEventTypeCancel  InteractiveEventType = "cancel"
)

// InteractiveEvent 交互事件
type InteractiveEvent struct {
	Type         InteractiveEventType `json:"type"`
	EventID      string               `json:"event_id"`
	EndpointID   string               `json:"endpoint_id"`
	InvokeParams aitool.InvokeParams  `json:"invoke_params"`
	Timestamp    int64                `json:"timestamp"`
}

// InteractiveFeeder 交互输入接口
type InteractiveFeeder interface {
	// Feed 向指定的端点发送交互事件
	Feed(endpointID string, params aitool.InvokeParams) error
	// FeedWithEventID 向指定的事件ID发送交互事件
	FeedWithEventID(eventID string, params aitool.InvokeParams) error
	// Cancel 取消指定端点的等待
	Cancel(endpointID string) error
	// Close 关闭交互器
	Close() error
}

// BaseInteractiveHandler 基础交互处理器
type BaseInteractiveHandler struct {
	ctx    context.Context
	cancel context.CancelFunc
	mutex  *sync.RWMutex

	// 事件通道
	eventChan *chanx.UnlimitedChan[InteractiveEvent]

	// 端点管理器引用
	endpointManager *EndpointManager

	// 事件处理回调
	onEventReceived func(event InteractiveEvent)

	// 超时配置
	defaultTimeout time.Duration
}

// NewBaseInteractiveHandler 创建新的基础交互处理器
func NewBaseInteractiveHandler(ctx context.Context, epm *EndpointManager) *BaseInteractiveHandler {
	if ctx == nil {
		ctx = context.Background()
	}
	ctx, cancel := context.WithCancel(ctx)

	handler := &BaseInteractiveHandler{
		ctx:             ctx,
		cancel:          cancel,
		mutex:           &sync.RWMutex{},
		eventChan:       chanx.NewUnlimitedChan[InteractiveEvent](ctx, 100),
		endpointManager: epm,
		defaultTimeout:  30 * time.Second, // 默认30秒超时
	}

	// 启动事件处理协程
	go handler.processEvents()

	return handler
}

// SetEventHandler 设置事件处理回调
func (h *BaseInteractiveHandler) SetEventHandler(callback func(event InteractiveEvent)) {
	h.mutex.Lock()
	defer h.mutex.Unlock()
	h.onEventReceived = callback
}

// SetDefaultTimeout 设置默认超时时间
func (h *BaseInteractiveHandler) SetDefaultTimeout(timeout time.Duration) {
	h.mutex.Lock()
	defer h.mutex.Unlock()
	h.defaultTimeout = timeout
}

// GetDefaultTimeout 获取默认超时时间
func (h *BaseInteractiveHandler) GetDefaultTimeout() time.Duration {
	h.mutex.RLock()
	defer h.mutex.RUnlock()
	return h.defaultTimeout
}

// Feed 向指定的端点发送交互事件
func (h *BaseInteractiveHandler) Feed(endpointID string, params aitool.InvokeParams) error {
	event := InteractiveEvent{
		Type:         InteractiveEventTypeInput,
		EndpointID:   endpointID,
		InvokeParams: params,
		Timestamp:    time.Now().Unix(),
	}

	h.eventChan.SafeFeed(event)
	log.Debugf("fed interactive event to endpoint %s with params: %+v", endpointID, params)
	return nil
}

// FeedWithEventID 向指定的事件ID发送交互事件
func (h *BaseInteractiveHandler) FeedWithEventID(eventID string, params aitool.InvokeParams) error {
	event := InteractiveEvent{
		Type:         InteractiveEventTypeInput,
		EventID:      eventID,
		InvokeParams: params,
		Timestamp:    time.Now().Unix(),
	}

	h.eventChan.SafeFeed(event)
	log.Debugf("fed interactive event with eventID %s and params: %+v", eventID, params)
	return nil
}

// Cancel 取消指定端点的等待
func (h *BaseInteractiveHandler) Cancel(endpointID string) error {
	event := InteractiveEvent{
		Type:       InteractiveEventTypeCancel,
		EndpointID: endpointID,
		Timestamp:  time.Now().Unix(),
	}

	h.eventChan.SafeFeed(event)
	log.Debugf("cancelled interactive event for endpoint %s", endpointID)
	return nil
}

// Close 关闭交互处理器
func (h *BaseInteractiveHandler) Close() error {
	if h.cancel != nil {
		h.cancel()
	}
	if h.eventChan != nil {
		h.eventChan.Close()
	}
	return nil
}

// processEvents 处理交互事件
func (h *BaseInteractiveHandler) processEvents() {
	defer log.Debug("interactive handler event processor stopped")

	for {
		select {
		case <-h.ctx.Done():
			return
		case event, ok := <-h.eventChan.OutputChannel():
			if !ok {
				return
			}
			h.handleEvent(event)
		}
	}
}

// handleEvent 处理单个交互事件
func (h *BaseInteractiveHandler) handleEvent(event InteractiveEvent) {
	// 调用事件处理回调
	h.mutex.RLock()
	callback := h.onEventReceived
	h.mutex.RUnlock()

	if callback != nil {
		callback(event)
	}

	// 处理不同类型的事件
	switch event.Type {
	case InteractiveEventTypeInput:
		h.handleInputEvent(event)
	case InteractiveEventTypeCancel:
		h.handleCancelEvent(event)
	case InteractiveEventTypeTimeout:
		h.handleTimeoutEvent(event)
	default:
		log.Warnf("unknown interactive event type: %s", event.Type)
	}
}

// handleInputEvent 处理输入事件
func (h *BaseInteractiveHandler) handleInputEvent(event InteractiveEvent) {
	if h.endpointManager == nil {
		log.Warn("endpoint manager is nil, cannot handle input event")
		return
	}

	// 根据端点ID或事件ID查找端点
	var endpoint *Endpoint
	var found bool

	if event.EndpointID != "" {
		endpoint, found = h.endpointManager.LoadEndpoint(event.EndpointID)
	} else if event.EventID != "" {
		// 通过事件ID查找端点
		endpoint, found = h.endpointManager.LoadEndpoint(event.EventID)
	}

	if !found || endpoint == nil {
		log.Warnf("endpoint not found for event: %+v", event)
		return
	}

	// 设置参数并释放端点
	if event.InvokeParams != nil {
		endpoint.SetParams(event.InvokeParams)
	}
	endpoint.Release()

	log.Debugf("released endpoint %s with params: %+v", endpoint.GetId(), event.InvokeParams)
}

// handleCancelEvent 处理取消事件
func (h *BaseInteractiveHandler) handleCancelEvent(event InteractiveEvent) {
	if h.endpointManager == nil {
		log.Warn("endpoint manager is nil, cannot handle cancel event")
		return
	}

	endpoint, found := h.endpointManager.LoadEndpoint(event.EndpointID)
	if !found || endpoint == nil {
		log.Warnf("endpoint not found for cancel event: %s", event.EndpointID)
		return
	}

	// 设置取消标记并释放端点
	endpoint.SetParams(aitool.InvokeParams{"cancelled": true})
	endpoint.Release()

	log.Debugf("cancelled endpoint %s", event.EndpointID)
}

// handleTimeoutEvent 处理超时事件
func (h *BaseInteractiveHandler) handleTimeoutEvent(event InteractiveEvent) {
	if h.endpointManager == nil {
		log.Warn("endpoint manager is nil, cannot handle timeout event")
		return
	}

	endpoint, found := h.endpointManager.LoadEndpoint(event.EndpointID)
	if !found || endpoint == nil {
		log.Warnf("endpoint not found for timeout event: %s", event.EndpointID)
		return
	}

	// 设置超时标记并释放端点
	endpoint.SetParams(aitool.InvokeParams{"timeout": true})
	endpoint.Release()

	log.Debugf("timeout endpoint %s", event.EndpointID)
}

// CreateTimeoutTrigger 创建超时触发器
func (h *BaseInteractiveHandler) CreateTimeoutTrigger(endpointID string, timeout time.Duration) {
	go func() {
		timer := time.NewTimer(timeout)
		defer timer.Stop()

		select {
		case <-h.ctx.Done():
			return
		case <-timer.C:
			event := InteractiveEvent{
				Type:       InteractiveEventTypeTimeout,
				EndpointID: endpointID,
				Timestamp:  time.Now().Unix(),
			}
			h.eventChan.SafeFeed(event)
		}
	}()
}

// GetEventChannel 获取事件通道（只读）
func (h *BaseInteractiveHandler) GetEventChannel() <-chan InteractiveEvent {
	return h.eventChan.OutputChannel()
}

// IsActive 检查交互处理器是否活跃
func (h *BaseInteractiveHandler) IsActive() bool {
	select {
	case <-h.ctx.Done():
		return false
	default:
		return true
	}
}
