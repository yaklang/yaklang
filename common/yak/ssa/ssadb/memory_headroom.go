package ssadb

import "github.com/yaklang/yaklang/common/yak/ssa/ssautil"

// indexSortMemoryFloorBytes is the free memory the host must still have before
// an index build is allowed to sort in memory. One index build for a large
// project needs a few hundred megabytes; the floor leaves room for the compile
// program that is still resident.
const indexSortMemoryFloorBytes int64 = 6 << 30

// memoryHeadroomForIndexSort reports whether the host can afford an in-memory
// index sort. An unknown host reports no headroom, so the caller keeps the
// safer on-disk sorter.
func memoryHeadroomForIndexSort() bool {
	return ssautil.HasMemoryHeadroom(indexSortMemoryFloorBytes)
}
