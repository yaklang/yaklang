package aicommon

import (
	"context"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/utils"
	"sync"
)

type WaitableAction struct {
	ctx     context.Context
	name    string
	params  aitool.InvokeParams
	barrier *utils.CondBarrier
	mu      sync.Mutex
}

func NewWaitableAction(ctx context.Context, name string) *WaitableAction {
	return &WaitableAction{
		ctx:     ctx,
		name:    name,
		params:  make(aitool.InvokeParams),
		barrier: utils.NewCondBarrierContext(ctx),
		mu:      sync.Mutex{},
	}
}

func (w *WaitableAction) Set(key string, value interface{}) {
	w.mu.Lock()
	w.params[key] = value
	w.mu.Unlock()
	w.barrier.CreateBarrier(key).Done()
}

func (w *WaitableAction) Name() string {
	return w.name
}

func (w *WaitableAction) SetName(i string) {
	w.name = i
}

func (w *WaitableAction) WaitAnyToString(key string) string {
	err := w.barrier.Wait(key)
	if err != nil {
		return ""
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	val := w.params.GetAnyToString(key)
	return val
}

func (w *WaitableAction) WaitObject(key string) aitool.InvokeParams {
	err := w.barrier.Wait(key)
	if err != nil {
		return nil
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	val := w.params.GetObject(key)
	return val
}

func (w *WaitableAction) WaitInt(key string) int64 {
	err := w.barrier.Wait(key)
	if err != nil {
		return 0
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	val := w.params.GetInt(key)
	return val
}

func (w *WaitableAction) WaitBool(key string) bool {
	err := w.barrier.Wait(key)
	if err != nil {
		return false
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	val := w.params.GetBool(key)
	return val
}

func (w *WaitableAction) WaitString(key string) string {
	err := w.barrier.Wait(key)
	if err != nil {
		return ""
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	val := w.params.GetString(key)
	return val
}

func (w *WaitableAction) WaitObjectArray(key string) []aitool.InvokeParams {
	err := w.barrier.Wait(key)
	if err != nil {
		return nil
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	val := w.params.GetObjectArray(key)
	return val
}

func (w *WaitableAction) WaitStringSlice(key string) []string {
	err := w.barrier.Wait(key)
	if err != nil {
		return nil
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	val := w.params.GetStringSlice(key)
	return val
}

func (w *WaitableAction) WaitFloat(key string) float64 {
	err := w.barrier.Wait(key)
	if err != nil {
		return 0
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	val := w.params.GetFloat(key)
	return val
}
