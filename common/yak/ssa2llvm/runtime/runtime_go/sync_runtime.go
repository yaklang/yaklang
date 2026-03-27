package main

import (
	"github.com/yaklang/yaklang/common/yak/yaklib"
)

type runtimeWaitGroup = yaklib.WaitGroupProxy

func stdlibSyncNewWaitGroup(args []uint64) int64 {
	if len(args) != 0 {
		return 0
	}
	return int64(uintptr(newStdlibShadow(yaklib.NewWaitGroup())))
}

func stdlibSyncNewSizedWaitGroup(args []uint64) int64 {
	if len(args) != 1 {
		return 0
	}
	return int64(uintptr(newStdlibShadow(yaklib.NewSizedWaitGroup(int(int64(args[0]))))))
}

func stdlibSyncNewLock(args []uint64) int64 {
	if len(args) != 0 {
		return 0
	}
	return int64(uintptr(newStdlibShadow(yaklib.NewLock())))
}

func stdlibSyncNewMutex(args []uint64) int64 {
	if len(args) != 0 {
		return 0
	}
	return int64(uintptr(newStdlibShadow(yaklib.NewMutex())))
}

func stdlibSyncNewRWMutex(args []uint64) int64 {
	if len(args) != 0 {
		return 0
	}
	return int64(uintptr(newStdlibShadow(yaklib.NewRWMutex())))
}

func stdlibSyncNewMap(args []uint64) int64 {
	if len(args) != 0 {
		return 0
	}
	return int64(uintptr(newStdlibShadow(yaklib.NewMap())))
}

func stdlibSyncNewOnce(args []uint64) int64 {
	if len(args) != 0 {
		return 0
	}
	return int64(uintptr(newStdlibShadow(yaklib.NewOnce())))
}

func stdlibSyncNewPool(args []uint64) int64 {
	if len(args) != 0 {
		return 0
	}
	return int64(uintptr(newStdlibShadow(yaklib.NewPool())))
}

func stdlibSyncNewCond(args []uint64) int64 {
	if len(args) != 0 {
		return 0
	}
	return int64(uintptr(newStdlibShadow(yaklib.NewCond())))
}
