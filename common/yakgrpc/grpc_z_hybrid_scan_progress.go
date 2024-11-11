package yakgrpc

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"github.com/yaklang/yaklang/common/utils"
)

type HybridScanTaskManager struct {
	taskId       string
	isPaused     *utils.AtomicBool
	ctx          context.Context
	cancel       context.CancelFunc
	resumeSignal *sync.Cond
	waitCount    int64
	//resumeLock   *sync.Mutex
}

func (h *HybridScanTaskManager) IsPaused() bool {
	return h.isPaused.IsSet()
}

func (h *HybridScanTaskManager) IsStop() bool {
	select {
	case <-h.ctx.Done():
		return true
	default:
		return false
	}
}

func (h *HybridScanTaskManager) TaskId() string {
	return h.taskId
}

func (h *HybridScanTaskManager) Stop() {
	h.cancel()
}

func (h *HybridScanTaskManager) WaitCount() int64 {
	return h.waitCount
}

func (h *HybridScanTaskManager) Checkpoint(hs ...func()) {
	if !h.isPaused.IsSet() {
		return
	}
	for _, handle := range hs {
		handle()
	}
	h.resumeSignal.L.Lock()
	atomic.AddInt64(&h.waitCount, 1)
	h.resumeSignal.Wait()
	atomic.AddInt64(&h.waitCount, -1)
	h.resumeSignal.L.Unlock()
}

func (h *HybridScanTaskManager) Pause() {
	h.isPaused.Set()
}

func (h *HybridScanTaskManager) PauseEffect() {
	h.Pause()
	for {
		select {
		case <-h.ctx.Done():
			return
		default:
			if h.WaitCount() > 0 {
				return
			}
			time.Sleep(100 * time.Millisecond)
		}
	}
}

func (h *HybridScanTaskManager) Context() context.Context {
	return h.ctx
}

func (h *HybridScanTaskManager) Resume() { // close pause task
	h.isPaused.UnSet()
	//h.isPaused.UnSet()
	//h.resumeLock.Lock()
	//go func() {
	//	defer h.resumeLock.Unlock()
	//
	//	count := 0
	//	for {
	//		if count > 5 {
	//			return
	//		}
	//
	//		if h.waitCount > 0 {
	//			count = 0
	//			h.resumeSignal.Broadcast()
	//			time.Sleep(200 * time.Millisecond)
	//		} else {
	//			count++
	//		}
	//	}
	//}()
}

var hybrisScanManager = new(sync.Map)

func CreateHybridTask(id string, ctx context.Context) (*HybridScanTaskManager, error) {
	_, ok := hybrisScanManager.Load(id)
	if ok {
		return nil, utils.Errorf("task id %s already exists", id)
	}
	if ctx == nil {
		ctx = context.Background()
	}
	var rootctx, cancel = context.WithCancel(ctx)
	m := &HybridScanTaskManager{
		isPaused:     utils.NewAtomicBool(),
		ctx:          rootctx,
		cancel:       cancel,
		resumeSignal: sync.NewCond(&sync.Mutex{}),
		//resumeLock:   new(sync.Mutex),
		taskId: id,
	}
	hybrisScanManager.Store(id, m)
	return m, nil
}

func GetHybridTask(id string) (*HybridScanTaskManager, error) {
	raw, ok := hybrisScanManager.Load(id)
	if !ok {
		return nil, utils.Errorf("task id %s not exists", id)
	}
	if ins, ok := raw.(*HybridScanTaskManager); ok {
		return ins, nil
	} else {
		return nil, utils.Errorf("task id %s not exists(typeof %T err)", id, raw)
	}
}

func RemoveHybridTask(id string) {
	r, err := GetHybridTask(id)
	if err != nil {
		return
	}
	r.Stop()
	hybrisScanManager.Delete(id)
}
