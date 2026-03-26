package main

import (
	"sync"
	"sync/atomic"

	"github.com/yaklang/yaklang/common/utils"
)

type runtimeWaitGroup struct {
	waitGroup sync.WaitGroup
	count     atomic.Int64
}

func (wg *runtimeWaitGroup) Add(delta ...int) {
	if wg == nil {
		return
	}
	n := 1
	if len(delta) > 0 {
		n = delta[0]
	}
	if n < 0 && wg.count.Load()+int64(n) < 0 {
		n = -int(wg.count.Load())
	}
	wg.count.Add(int64(n))
	wg.waitGroup.Add(n)
}

func (wg *runtimeWaitGroup) Done() {
	if wg == nil {
		return
	}
	wg.Add(-1)
}

func (wg *runtimeWaitGroup) Wait() {
	if wg == nil {
		return
	}
	wg.waitGroup.Wait()
}

func stdlibSyncNewWaitGroup(args []uint64) int64 {
	if len(args) != 0 {
		return 0
	}
	return int64(uintptr(newStdlibShadow(&runtimeWaitGroup{})))
}

func stdlibSyncNewSizedWaitGroup(args []uint64) int64 {
	if len(args) != 1 {
		return 0
	}
	return int64(uintptr(newStdlibShadow(utils.NewSizedWaitGroup(int(int64(args[0]))))))
}

func stdlibSyncNewLock(args []uint64) int64 {
	if len(args) != 0 {
		return 0
	}
	return int64(uintptr(newStdlibShadow(new(sync.Mutex))))
}

func stdlibSyncNewMutex(args []uint64) int64 {
	if len(args) != 0 {
		return 0
	}
	return int64(uintptr(newStdlibShadow(new(sync.Mutex))))
}

func stdlibSyncNewRWMutex(args []uint64) int64 {
	if len(args) != 0 {
		return 0
	}
	return int64(uintptr(newStdlibShadow(new(sync.RWMutex))))
}
