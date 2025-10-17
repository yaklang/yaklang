package aicommon

import (
	"context"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"time"
)

type Endpoint struct {
	id              string
	sig             *EndpointSignal
	reviewType      schema.EventType
	activeParams    aitool.InvokeParams
	reviewMaterials aitool.InvokeParams

	// seq and checkpoint for recovering
	seq        int64
	checkpoint *schema.AiCheckpoint
}

func (e *Endpoint) GetSeq() int64 {
	return e.seq
}

func (e *Endpoint) GetCheckpoint() *schema.AiCheckpoint {
	if e.checkpoint == nil {
		e.checkpoint = &schema.AiCheckpoint{
			Seq: e.seq,
		}
	}
	return e.checkpoint
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

func (e *Endpoint) GetId() string {
	return e.id
}
