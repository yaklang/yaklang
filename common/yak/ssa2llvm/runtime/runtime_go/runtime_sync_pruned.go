//go:build ssa2llvm_pruned_runtime

package main

import "sync"

type runtimeWaitGroup = sync.WaitGroup

type runtimeSizedWaitGroup struct {
	limit chan struct{}
	wg    sync.WaitGroup
}

func runtimeSyncNewWaitGroup() any {
	return &sync.WaitGroup{}
}

func runtimeSyncNewSizedWaitGroup(size int64) any {
	if size <= 0 {
		return &runtimeSizedWaitGroup{}
	}
	return &runtimeSizedWaitGroup{
		limit: make(chan struct{}, int(size)),
	}
}

func runtimeSyncNewLock() any {
	return &sync.Mutex{}
}

func runtimeSyncNewMutex() any {
	return &sync.Mutex{}
}

func runtimeSyncNewRWMutex() any {
	return &sync.RWMutex{}
}

func runtimeSyncNewMap() any {
	return &sync.Map{}
}

func runtimeSyncNewOnce() any {
	return &sync.Once{}
}

func runtimeSyncNewPool() any {
	return &sync.Pool{}
}

func runtimeSyncNewCond() any {
	return sync.NewCond(&sync.Mutex{})
}

func (s *runtimeSizedWaitGroup) Add(delta ...int) {
	n := 1
	if len(delta) > 0 {
		n = delta[0]
	}
	if n < 0 {
		for i := 0; i < -n; i++ {
			s.Done()
		}
		return
	}
	for i := 0; i < n; i++ {
		if s.limit != nil {
			s.limit <- struct{}{}
		}
		s.wg.Add(1)
	}
}

func (s *runtimeSizedWaitGroup) Done() {
	if s.limit != nil {
		<-s.limit
	}
	s.wg.Done()
}

func (s *runtimeSizedWaitGroup) Wait() {
	s.wg.Wait()
}
