package ssa

import (
	"sort"
	"sync"
	syncAtomic "sync/atomic"
	"time"

	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
)

var (
	_SSASaveIrCodeCPUCost  uint64
	_SSASaveIrCodeCPUCount uint64

	Site1 uint64
	Site2 uint64
	Site3 uint64

	_SSASaveIrCodeDBCost  uint64
	_SSASaveIrCodeDBCount uint64

	// _SSACacheToDatabaseCost  uint64
	// _SSACacheToDatabaseCount uint64

	_CostCallback []func()

	_compileFileHit = make(map[string]int64)

	InstructionMarshal      uint64
	InstructionMarshalCount uint64

	Instruction2IRcode      uint64
	Instruction2IRcodeCount uint64

	Value2IrCode      uint64
	Value2IrCodeCount uint64

	Function2IrCode   uint64
	BasicBlock2IrCode uint64

	SetExtraInfo uint64
	SaveValueOff uint64

	SaveDBWait uint64

	SetInstructionTime  uint64
	SetInstructionCount uint64

	GetInstructionTime  uint64
	GetInstructionCount uint64

	LoadInstructionTime  uint64
	LoadInstructionCount uint64

	FetchInstructionTime  uint64
	FetchInstructionCount uint64
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
	log.Errorf("FetchInstruction Time: %v, Count: %v, Avg: %v",
		time.Duration(syncAtomic.LoadUint64(&FetchInstructionTime)),
		syncAtomic.LoadUint64(&FetchInstructionCount),
		time.Duration(syncAtomic.LoadUint64(&FetchInstructionTime))/time.Duration(syncAtomic.LoadUint64(&FetchInstructionCount)),
	)

	log.Errorf("GetInstruction Time: %v, Count: %v, Avg: %v",
		time.Duration(syncAtomic.LoadUint64(&GetInstructionTime)),
		syncAtomic.LoadUint64(&GetInstructionCount),
		time.Duration(syncAtomic.LoadUint64(&GetInstructionTime))/time.Duration(syncAtomic.LoadUint64(&GetInstructionCount)),
	)
	log.Errorf("SetInstruction Time: %v, Count: %v, Avg: %v",
		time.Duration(syncAtomic.LoadUint64(&SetInstructionTime)),
		syncAtomic.LoadUint64(&SetInstructionCount),
		time.Duration(syncAtomic.LoadUint64(&SetInstructionTime))/time.Duration(syncAtomic.LoadUint64(&SetInstructionCount)),
	)

	if LoadInstructionCount != 0 {
		log.Errorf("LoadInstruction Time: %v, Count: %v, Avg: %v",
			time.Duration(syncAtomic.LoadUint64(&LoadInstructionTime)),
			syncAtomic.LoadUint64(&LoadInstructionCount),
			time.Duration(syncAtomic.LoadUint64(&LoadInstructionTime))/time.Duration(syncAtomic.LoadUint64(&LoadInstructionCount)),
		)
	} else {
		log.Errorf("LoadInstruction Count: 0")
	}

	log.Errorf("--------------------------------------------------")

	if _SSASaveIrCodeCPUCount != 0 {
		log.Errorf("SSA Database SaveIrCode CPU Time: %v, Count: %v, Avg: %v",
			GetSSASaveIrCodeCPUCost(),
			syncAtomic.LoadUint64(&_SSASaveIrCodeCPUCount),
			GetSSASaveIrCodeCPUCost()/time.Duration(syncAtomic.LoadUint64(&_SSASaveIrCodeCPUCount)),
		)
	} else {
		log.Errorf("SSA Database SaveIrCode CPU Count: 0")
	}
	if _SSASaveIrCodeDBCount != 0 {
		log.Errorf("SSA Database SaveIrCode DB  Cost: %v, Count: %v, Avg: %v",
			GetSSASaveIRcodeDBCast(),
			syncAtomic.LoadUint64(&_SSASaveIrCodeDBCount),
			GetSSASaveIRcodeDBCast()/time.Duration(syncAtomic.LoadUint64(&_SSASaveIrCodeDBCount)),
		)
	} else {
		log.Errorf("SSA Database SaveIrCode DB Count: 0")
	}
	log.Errorf("SSA Database SaveIndex Cost: %v", ssadb.GetSSAIndexCost())
	log.Errorf("SSA Database SaveSourceCode Cost: %v", ssadb.GetSSASourceCodeCost())
	log.Errorf("SSA Database SaveType Cost: %v", ssadb.GetSSASaveTypeCost())
	// log.Errorf("SSA Database CacheToDatabase Cost: %v", GetSSACacheToDatabaseCost())
	log.Errorf("SSA Database SaveDBWait Cost: %v", time.Duration(syncAtomic.LoadUint64(&SaveDBWait)))

	log.Errorf("--------------------------------------------------")
	if InstructionMarshalCount != 0 {
		log.Errorf("SSA database instruction marshal all Cost: %v, Count: %v, Avg: %v",
			time.Duration(syncAtomic.LoadUint64(&InstructionMarshal)),
			syncAtomic.LoadUint64(&InstructionMarshalCount),
			time.Duration(syncAtomic.LoadUint64(&InstructionMarshal))/time.Duration(syncAtomic.LoadUint64(&InstructionMarshalCount)),
		)
	} else {
		log.Errorf("SSA database instruction marshal all Count: 0")
	}

	if Instruction2IRcodeCount != 0 {
		log.Errorf("SSA Database Instruction2IRcode Cost: %v, Count: %v, Avg: %v",
			time.Duration(syncAtomic.LoadUint64(&Instruction2IRcode)),
			syncAtomic.LoadUint64(&Instruction2IRcodeCount),
			time.Duration(syncAtomic.LoadUint64(&Instruction2IRcode))/time.Duration(syncAtomic.LoadUint64(&Instruction2IRcodeCount)),
		)
	} else {
		log.Errorf("SSA Database Instruction2IRcode Count: 0")
	}

	if Value2IrCodeCount != 0 {
		log.Errorf("SSA Database Value2IrCode Cost: %v, Count: %v, Avg: %v",
			time.Duration(syncAtomic.LoadUint64(&Value2IrCode)),
			syncAtomic.LoadUint64(&Value2IrCodeCount),
			time.Duration(syncAtomic.LoadUint64(&Value2IrCode))/time.Duration(syncAtomic.LoadUint64(&Value2IrCodeCount)),
		)
	} else {
		log.Errorf("SSA Database Value2IrCode Count: 0")
	}
	log.Errorf("SSA Database Function2IrCode Cost: %v", time.Duration(syncAtomic.LoadUint64(&Function2IrCode)))
	log.Errorf("SSA Database BasicBlock2IrCode Cost: %v", time.Duration(syncAtomic.LoadUint64(&BasicBlock2IrCode)))

	log.Errorf("SSA Database SetExtraInfo Cost: %v", time.Duration(syncAtomic.LoadUint64(&SetExtraInfo)))
	log.Errorf("SSA Database SaveValueOff Cost: %v", time.Duration(syncAtomic.LoadUint64(&SaveValueOff)))

	log.Errorf("--------------------------------------------------")
	log.Errorf("Site1 Cost: %v", time.Duration(syncAtomic.LoadUint64(&Site1)))
	log.Errorf("Site2 Cost: %v", time.Duration(syncAtomic.LoadUint64(&Site2)))
	log.Errorf("Site3 Cost: %v", time.Duration(syncAtomic.LoadUint64(&Site3)))
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

// func GetSSACacheToDatabaseCost() time.Duration {
// 	return time.Duration(syncAtomic.LoadUint64(&_SSACacheToDatabaseCost))
// }
