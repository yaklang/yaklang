package aid

import (
	"context"
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
				raw.ActiveWithParams(make(aitool.InvokeParams))
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
		ep.ActiveWithParams(params)
	}
}

func (e *endpointManager) createEndpoint() *Endpoint {
	id := ksuid.New().String()
	endpoint := &Endpoint{
		id:           id,
		sig:          sync.NewCond(&sync.Mutex{}), // 正确初始化 Cond
		activeParams: make(aitool.InvokeParams),
	}
	e.results.Store(id, endpoint)
	if e.config != nil {
		seq := e.config.AcquireId()
		endpoint.seq = seq
		ck := e.config.createReviewCheckpoint(seq)
		endpoint.checkpoint = ck
	}
	return endpoint
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
	id           string
	sig          *sync.Cond
	activeParams aitool.InvokeParams

	// seq and checkpoint for recovering
	seq        int64
	checkpoint *schema.AiCheckpoint
}

func (e *Endpoint) Wait() {
	e.sig.L.Lock()
	defer e.sig.L.Unlock()
	e.sig.Wait()
}

func (e Endpoint) Release() {
	e.sig.L.Lock()
	defer e.sig.L.Unlock()
	e.sig.Broadcast()
}

func (e *Endpoint) WaitTimeoutSeconds(i float64) bool {
	return e.WaitTimeout(time.Duration(i * float64(time.Second)))
}

// 新增的 WaitTimeout 方法
func (e *Endpoint) WaitTimeout(timeout time.Duration) bool {
	timer := time.NewTimer(timeout)
	defer timer.Stop()

	// 创建一个通道用于通知完成
	done := make(chan struct{})

	go func() {
		e.sig.L.Lock()
		e.sig.Wait()
		e.sig.L.Unlock()
		close(done)
	}()

	// 等待信号或超时
	select {
	case <-done:
		return true // 成功接收到信号
	case <-timer.C:
		return false // 超时
	}
}

// 修改后的 GetParams 方法，添加锁保护
func (e *Endpoint) GetParams() aitool.InvokeParams {
	e.sig.L.Lock()
	defer e.sig.L.Unlock()
	// 创建一个副本以避免外部修改
	params := make(aitool.InvokeParams)
	for k, v := range e.activeParams {
		params[k] = v
	}
	return params
}

func (e *Endpoint) SetParams(params aitool.InvokeParams) {
	e.sig.L.Lock()
	defer e.sig.L.Unlock()
	if !utils.IsNil(params) {
		e.activeParams = params
	}
}

func (e *Endpoint) SetDefaultSuggestion(suggestion string) {
	e.sig.L.Lock()
	defer e.sig.L.Unlock()
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

func (e *Endpoint) ActiveWithParams(params aitool.InvokeParams) {
	e.sig.L.Lock()
	defer e.sig.L.Unlock()

	if !utils.IsNil(params) {
		e.activeParams = params
	}
	e.sig.Broadcast()
}
