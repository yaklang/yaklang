package ssa

import (
	"sort"
	"sync"
	syncAtomic "sync/atomic"
	"time"

	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
)

var (
	_SSASaveIrCodeCost      uint64
	_SSACacheToDatabaseCost uint64
	_CostCallback           []func()
	_compileFileHit         = make(map[string]int64)
)

var _compileFileHitMutex = new(sync.Mutex)

func HitCompileFile(fileName string) {
	_compileFileHitMutex.Lock()
	defer _compileFileHitMutex.Unlock()

	if _, ok := _compileFileHit[fileName]; !ok {
		_compileFileHit[fileName] = 0
	}
	_compileFileHit[fileName]++
}

func RegisterCostCallback(f func()) {
	_CostCallback = append(_CostCallback, f)
}

func GetSSASaveIrCodeCost() time.Duration {
	return time.Duration(syncAtomic.LoadUint64(&_SSASaveIrCodeCost))
}

type fileCounter struct {
	fileName string
	count    int64
}

func ShowDatabaseCacheCost() {
	log.Infof("SSA Database SaveIrCode Cost: %v", GetSSASaveIrCodeCost())
	log.Infof("SSA Database SaveIndex Cost: %v", ssadb.GetSSAIndexCost())
	log.Infof("SSA Database SaveSourceCode Cost: %v", ssadb.GetSSASourceCodeCost())
	log.Infof("SSA Database SaveType Cost: %v", ssadb.GetSSASaveTypeCost())
	log.Infof("SSA Database CacheToDatabase Cost: %v", GetSSACacheToDatabaseCost())

	// _compileFileHit         = make(map[string]int64)
	var li []fileCounter
	for fileName, count := range _compileFileHit {
		if count > 1 {
			li = append(li, fileCounter{fileName, count})
		}
	}
	sort.Slice(li, func(i, j int) bool {
		// 降序
		return li[i].count > li[j].count
	})
	for _, k := range li {
		log.Infof("file %v include count: %v", k.fileName, k.count)
	}

	for _, cb := range _CostCallback {
		cb()
	}
}

func GetSSACacheToDatabaseCost() time.Duration {
	return time.Duration(syncAtomic.LoadUint64(&_SSACacheToDatabaseCost))
}
