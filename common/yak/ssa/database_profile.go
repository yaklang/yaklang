package ssa

import (
	"sort"
	"sync"
	syncAtomic "sync/atomic"
	"time"

	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
)

var (
	_SSASaveIrCodeCPUCost uint64

	Marshal1 uint64
	Marshal2 uint64
	Marshal3 uint64

	_SSASaveIrCodeDBCost    uint64
	_SSACacheToDatabaseCost uint64
	_CostCallback           []func()
	_compileFileHit         = make(map[string]int64)

	InstructoinMarshal uint64
	Instruction2IRcode uint64
	Value2IrCode       uint64
	Function2IrCode    uint64
	BasicBlock2IrCode  uint64
	SetExtraInfo       uint64
	SaveValueOff       uint64
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

func GetSSASaveIrCodeCPUCost() time.Duration {
	return time.Duration(syncAtomic.LoadUint64(&_SSASaveIrCodeCPUCost))
}

func GetSSASaveIRcodeDBCast() time.Duration {
	return time.Duration(syncAtomic.LoadUint64(&_SSASaveIrCodeDBCost))
}

type fileCounter struct {
	fileName string
	count    int64
}

func ShowDatabaseCacheCost() {
	log.Errorf("SSA Database SaveIrCode CPU Cost: %v", GetSSASaveIrCodeCPUCost())
	log.Errorf("SSA Database SaveIrCode DB  Cost: %v", GetSSASaveIRcodeDBCast())
	log.Errorf("SSA Database SaveIndex Cost: %v", ssadb.GetSSAIndexCost())
	log.Errorf("SSA Database SaveSourceCode Cost: %v", ssadb.GetSSASourceCodeCost())
	log.Errorf("SSA Database SaveType Cost: %v", ssadb.GetSSASaveTypeCost())
	log.Errorf("SSA Database CacheToDatabase Cost: %v", GetSSACacheToDatabaseCost())

	log.Errorf("--------------------------------------------------")
	log.Errorf("SSA database instruction marshal all cost: %v", time.Duration(syncAtomic.LoadUint64(&InstructoinMarshal)))

	log.Errorf("SSA Database Instruction2IRcode Cost: %v", time.Duration(syncAtomic.LoadUint64(&Instruction2IRcode)))
	log.Errorf("SSA Database Value2IrCode Cost: %v", time.Duration(syncAtomic.LoadUint64(&Value2IrCode)))
	log.Errorf("SSA Database Function2IrCode Cost: %v", time.Duration(syncAtomic.LoadUint64(&Function2IrCode)))
	log.Errorf("SSA Database BasicBlock2IrCode Cost: %v", time.Duration(syncAtomic.LoadUint64(&BasicBlock2IrCode)))
	log.Errorf("SSA Database SetExtraInfo Cost: %v", time.Duration(syncAtomic.LoadUint64(&SetExtraInfo)))
	log.Errorf("SSA Database SaveValueOff Cost: %v", time.Duration(syncAtomic.LoadUint64(&SaveValueOff)))

	log.Errorf("--------------------------------------------------")
	log.Errorf("SSA Database Marshal1 Cost: %v", time.Duration(syncAtomic.LoadUint64(&Marshal1)))
	log.Errorf("SSA Database Marshal2 Cost: %v", time.Duration(syncAtomic.LoadUint64(&Marshal2)))
	log.Errorf("SSA Database Marshal3 Cost: %v", time.Duration(syncAtomic.LoadUint64(&Marshal3)))
	log.Errorf("--------------------------------------------------")

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
