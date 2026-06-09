//go:build ssa2llvm_pruned_runtime

package main

import "sync"

type runtimeWaitGroup = sync.WaitGroup

func runtimeSyncNewWaitGroup() any {
	return &sync.WaitGroup{}
}

func runtimeSyncNewSizedWaitGroup(size int64) any {
	return &sync.WaitGroup{}
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
