package main

import (
	"github.com/yaklang/yaklang/common/yak/yaklib"
)

type runtimeWaitGroup = yaklib.WaitGroupProxy

func runtimeSyncNewWaitGroup() any {
	return yaklib.NewWaitGroup()
}

func runtimeSyncNewSizedWaitGroup(size int64) any {
	return yaklib.NewSizedWaitGroup(int(size))
}

func runtimeSyncNewLock() any {
	return yaklib.NewLock()
}

func runtimeSyncNewMutex() any {
	return yaklib.NewMutex()
}

func runtimeSyncNewRWMutex() any {
	return yaklib.NewRWMutex()
}

func runtimeSyncNewMap() any {
	return yaklib.NewMap()
}

func runtimeSyncNewOnce() any {
	return yaklib.NewOnce()
}

func runtimeSyncNewPool() any {
	return yaklib.NewPool()
}

func runtimeSyncNewCond() any {
	return yaklib.NewCond()
}
