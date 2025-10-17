package aicommon

import (
	"context"
	"github.com/segmentio/ksuid"
	"github.com/yaklang/yaklang/common/ai/aid/aiddb"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"sync"
)

type EndpointManager struct {
	config  AICallerConfigIf
	ctx     context.Context
	cancel  context.CancelFunc
	results *sync.Map
}

func (e *EndpointManager) SetConfig(config AICallerConfigIf) {
	if e.config != nil {
		return
	}
	e.config = config
}

func (e *EndpointManager) GetConfig() AICallerConfigIf {
	if e.config == nil {
		return nil
	}
	return e.config
}

func (e *EndpointManager) GetContext() context.Context {
	if e.ctx == nil {
		return context.Background()
	}
	return e.ctx
}

func NewEndpointManagerContext(ctx context.Context) *EndpointManager {
	if utils.IsNil(ctx) {
		ctx = context.Background()
	}
	ctx, cancel := context.WithCancel(ctx)
	epm := &EndpointManager{
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

func NewEndpointManager() *EndpointManager {
	return NewEndpointManagerContext(context.Background())
}

func (e *EndpointManager) Feed(id string, params aitool.InvokeParams) {
	if ep, ok := e.LoadEndpoint(id); ok {
		ep.ActiveWithParams(e.ctx, params)
	}
}

func (e *EndpointManager) CreateEndpointWithEventType(typeName schema.EventType) *Endpoint {
	id := ksuid.New().String()
	endpoint := &Endpoint{
		id:              id,
		sig:             NewEndpointSignal(), // sync.NewCond(&sync.Mutex{}), // 正确初始化 Cond
		activeParams:    make(aitool.InvokeParams),
		reviewMaterials: make(aitool.InvokeParams),
	}
	e.results.Store(id, endpoint)
	if c := e.config; c != nil {
		endpoint.seq = c.AcquireId()
		if ret, ok := yakit.GetReviewCheckpoint(c.GetDB(), c.GetRuntimeId(), endpoint.seq); ok {
			endpoint.SetParams(aiddb.AiCheckPointGetResponseParams(ret))
			endpoint.checkpoint = ret
		} else {
			endpoint.checkpoint = e.config.CreateReviewCheckpoint(endpoint.seq)
		}
	}
	return endpoint
}

func (e *EndpointManager) CreateEndpoint() *Endpoint {
	return e.CreateEndpointWithEventType("")
}

func (e *EndpointManager) LoadEndpoint(id string) (*Endpoint, bool) {
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
