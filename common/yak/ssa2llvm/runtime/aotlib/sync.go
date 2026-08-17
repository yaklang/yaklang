package aotlib

import "sync"

// waitGroupProxy mirrors common/yak/yaklib.WaitGroupProxy's variadic Add
// (default delta 1) so `wg.Add()` and `wg.Add(1)` both work under AOT.
type waitGroupProxy struct {
	sync.WaitGroup
}

func (wg *waitGroupProxy) Add(deltas ...int) {
	delta := 1
	if len(deltas) > 0 {
		delta = deltas[0]
	}
	wg.WaitGroup.Add(delta)
}

func (wg *waitGroupProxy) Done() {
	wg.WaitGroup.Done()
}

func (wg *waitGroupProxy) Wait() {
	wg.WaitGroup.Wait()
}

// sizedWaitGroup mirrors common/utils.SizedWaitGroup: Add blocks while the
// concurrency limit is reached, Done releases a slot, Wait blocks until all
// added tasks finish.
type sizedWaitGroup struct {
	limit chan struct{}
	wg    sync.WaitGroup
}

func (s *sizedWaitGroup) Add(deltas ...int) {
	n := 1
	if len(deltas) > 0 {
		n = deltas[0]
	}
	for i := 0; i < n; i++ {
		s.limit <- struct{}{}
		s.wg.Add(1)
	}
}

func (s *sizedWaitGroup) Done() {
	<-s.limit
	s.wg.Done()
}

func (s *sizedWaitGroup) Wait() {
	s.wg.Wait()
}

func NewWaitGroup() *waitGroupProxy { return new(waitGroupProxy) }
func NewSizedWaitGroup(size int) *sizedWaitGroup {
	limit := size
	if limit <= 0 {
		limit = 1
	}
	return &sizedWaitGroup{limit: make(chan struct{}, limit)}
}
func NewMutex() *sync.Mutex     { return new(sync.Mutex) }
func NewLock() *sync.Mutex      { return new(sync.Mutex) }
func NewRWMutex() *sync.RWMutex { return new(sync.RWMutex) }
func NewMap() *sync.Map         { return new(sync.Map) }
func NewPool() *sync.Pool       { return new(sync.Pool) }
func NewOnce() *sync.Once       { return new(sync.Once) }
func NewCond() *sync.Cond       { return sync.NewCond(new(sync.Mutex)) }

// SyncExports mirrors the sync module's export table (the AOT-supported
// subset). Entries match common/yak/yaklib.SyncExport signatures.
var SyncExports = map[string]any{
	"NewWaitGroup":      NewWaitGroup,
	"NewSizedWaitGroup": NewSizedWaitGroup,
	"NewMutex":          NewMutex,
	"NewLock":           NewLock,
	"NewRWMutex":        NewRWMutex,
	"NewMap":            NewMap,
	"NewPool":           NewPool,
	"NewOnce":           NewOnce,
	"NewCond":           NewCond,
}
