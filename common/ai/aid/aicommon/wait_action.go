package aicommon

import (
	"context"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/utils"
)

type WaitableAction struct {
	ctx     context.Context
	name    string
	params  aitool.InvokeParams
	barrier *utils.CondBarrier
}

func NewWaitableAction(ctx context.Context, name string) *WaitableAction {
	return &WaitableAction{
		ctx:     ctx,
		name:    name,
		params:  make(aitool.InvokeParams),
		barrier: utils.NewCondBarrierContext(ctx),
	}
}

func (w *WaitableAction) Set(key string, value interface{}) {
	w.params[key] = value
	w.barrier.CreateBarrier(key).Done()
}

func (w *WaitableAction) Name() string {
	return w.name
}

func (w *WaitableAction) WaitAnyToString(key string) string {
	err := w.barrier.Wait(key)
	if err != nil {
		return ""
	}
	return w.params.GetAnyToString(key)
}

func (w *WaitableAction) WaitObject(key string) aitool.InvokeParams {
	err := w.barrier.Wait(key)
	if err != nil {
		return nil
	}
	return w.params.GetObject(key)
}

func (w *WaitableAction) WaitInt(key string) int64 {
	err := w.barrier.Wait(key)
	if err != nil {
		return 0
	}
	return w.params.GetInt(key)
}

func (w *WaitableAction) WaitBool(key string) bool {
	err := w.barrier.Wait(key)
	if err != nil {
		return false
	}
	return w.params.GetBool(key)
}

func (w *WaitableAction) WaitString(key string) string {
	err := w.barrier.Wait(key)
	if err != nil {
		return ""
	}
	return w.params.GetString(key)
}

func (w *WaitableAction) WaitObjectArray(key string) []aitool.InvokeParams {
	err := w.barrier.Wait(key)
	if err != nil {
		return nil
	}
	return w.params.GetObjectArray(key)
}

func (w *WaitableAction) WaitStringSlice(key string) []string {
	err := w.barrier.Wait(key)
	if err != nil {
		return nil
	}
	return w.params.GetStringSlice(key)
}

func (w *WaitableAction) WaitFloat(key string) float64 {
	err := w.barrier.Wait(key)
	if err != nil {
		return 0
	}
	return w.params.GetFloat(key)
}
