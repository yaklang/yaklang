package aid

import (
	"context"
	"github.com/yaklang/yaklang/common/ai/aid/aiddb"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"sync"
	"time"

	"github.com/segmentio/ksuid"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/utils"
)

type endpointManager struct {
	config  *Config
	ctx     context.Context
	cancel  context.CancelFunc
	results *sync.Map
}

func newEndpointManagerContext(ctx context.Context) *endpointManager {
	if utils.IsNil(ctx) {
		ctx = context.Background()
	}
	ctx, cancel := context.WithCancel(ctx)
	epm := &endpointManager{
		ctx:     ctx,
		cancel:  cancel,
		results: &sync.Map{},
	}
	go func() {
		<-ctx.Done()
		epm.results.Range(func(key, value interface{}) bool {
			if raw, ok := value.(*Endpoint); ok {
				raw.ActiveWithParams(ctx, make(aitool.InvokeParams))
			}
			return true
		})
	}()
	return epm
}

func newEndpointManager() *endpointManager {
	return newEndpointManagerContext(context.Background())
}

func (e *endpointManager) feed(id string, params aitool.InvokeParams) {
	if ep, ok := e.loadEndpoint(id); ok {
		ep.ActiveWithParams(e.ctx, params)
	}
}

func (e *endpointManager) createEndpointWithEventType(typeName EventType) *Endpoint {
	id := ksuid.New().String()
	endpoint := &Endpoint{
		id:              id,
		sig:             newSignal(), // sync.NewCond(&sync.Mutex{}), // 正确初始化 Cond
		activeParams:    make(aitool.InvokeParams),
		reviewMaterials: make(aitool.InvokeParams),
	}
	e.results.Store(id, endpoint)
	if c := e.config; c != nil {
		endpoint.seq = c.AcquireId()
		if ret, ok := aiddb.GetReviewCheckpoint(c.GetDB(), c.id, endpoint.seq); ok {
			endpoint.SetParams(aiddb.AiCheckPointGetResponseParams(ret))
			endpoint.checkpoint = ret
		} else {
			endpoint.checkpoint = e.config.createReviewCheckpoint(endpoint.seq)
		}
	}
	return endpoint
}

func (e *endpointManager) createEndpoint() *Endpoint {
	return e.createEndpointWithEventType("")
}

func (e *endpointManager) loadEndpoint(id string) (*Endpoint, bool) {
	raw, ok := e.results.Load(id)
	if !ok {
		return nil, false
	}
	ep, typeOk := raw.(*Endpoint)
	if !typeOk {
		return nil, false
	}
	return ep, true
}

type Endpoint struct {
	id              string
	sig             *signal
	reviewType      EventType
	activeParams    aitool.InvokeParams
	reviewMaterials aitool.InvokeParams

	// seq and checkpoint for recovering
	seq        int64
	checkpoint *schema.AiCheckpoint
}

func (e *Endpoint) SetReviewMaterials(
	params aitool.InvokeParams) {
	if !utils.IsNil(params) {
		e.reviewMaterials = params
	}
}

func (e *Endpoint) GetReviewMaterials() aitool.InvokeParams {
	params := make(aitool.InvokeParams)
	for k, v := range e.reviewMaterials {
		params[k] = v
	}
	return params
}

func (e *Endpoint) WaitContext(ctx context.Context) {
	err := e.sig.WaitContext(ctx)
	if err != nil {
		log.Errorf("Failed to wait for endpoint %s: %v", e.id, err)
		return
	}
}

func (e Endpoint) ReleaseContext(ctx context.Context) {
	e.sig.ActiveContext(ctx)
}

func (e *Endpoint) WaitTimeoutSeconds(i float64) bool {
	return e.WaitTimeout(time.Duration(i * float64(time.Second)))
}

// 新增的 WaitTimeout 方法
func (e *Endpoint) WaitTimeout(timeout time.Duration) bool {
	return e.sig.WaitTimeout(timeout) == nil
}

func (e *Endpoint) Wait() {
	e.sig.Wait()
}

// 修改后的 GetParams 方法，添加锁保护
func (e *Endpoint) GetParams() aitool.InvokeParams {
	params := make(aitool.InvokeParams)
	for k, v := range e.activeParams {
		params[k] = v
	}
	return params
}

func (e *Endpoint) SetParams(params aitool.InvokeParams) {
	if !utils.IsNil(params) {
		e.activeParams = params
	}
}

func (e *Endpoint) SetDefaultSuggestion(suggestion string) {
	e.activeParams["suggestion"] = suggestion
}

func (e *Endpoint) SetDefaultSuggestionContinue() {
	e.SetDefaultSuggestion("continue")
}

func (e *Endpoint) SetDefaultSuggestionEnd() {
	e.SetDefaultSuggestion("end")
}

func (e *Endpoint) SetDefaultSuggestionYes() {
	e.SetDefaultSuggestion("yes")
}

func (e *Endpoint) SetDefaultSuggestionNo() {
	e.SetDefaultSuggestion("no")
}

func (e *Endpoint) ActiveWithParams(ctx context.Context, params aitool.InvokeParams) {
	if !utils.IsNil(params) {
		e.activeParams = params
	}
	e.sig.ActiveAsyncContext(ctx)
}

func (e *Endpoint) Release() {
	e.sig.ActiveAsyncContext(context.Background())
}
