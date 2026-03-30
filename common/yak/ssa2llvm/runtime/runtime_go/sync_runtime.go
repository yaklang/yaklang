package main

import (
	"github.com/yaklang/yaklang/common/yak/yaklib"
)

type runtimeWaitGroup = yaklib.WaitGroupProxy

func stdlibSyncNewWaitGroup() any {
	return yaklib.NewWaitGroup()
}

func stdlibSyncNewSizedWaitGroup(size int64) any {
	return yaklib.NewSizedWaitGroup(int(size))
}

func stdlibSyncNewLock() any {
	return yaklib.NewLock()
}

func stdlibSyncNewMutex() any {
	return yaklib.NewMutex()
}

func stdlibSyncNewRWMutex() any {
	return yaklib.NewRWMutex()
}

func stdlibSyncNewMap() any {
	return yaklib.NewMap()
}

func stdlibSyncNewOnce() any {
	return yaklib.NewOnce()
}

func stdlibSyncNewPool() any {
	return yaklib.NewPool()
}

func stdlibSyncNewCond() any {
	return yaklib.NewCond()
}
