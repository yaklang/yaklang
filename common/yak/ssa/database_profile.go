package ssa

import (
	syncAtomic "sync/atomic"
	"time"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
	"github.com/yaklang/yaklang/common/yak/ssa/ssautil"
)

var (
	_SSASaveIrCodeCost uint64
	_CostCallback      []func()
)

func RegisterCostCallback(f func()) {
	_CostCallback = append(_CostCallback, f)
}

func GetSSASaveIrCodeCost() time.Duration {
	return time.Duration(syncAtomic.LoadUint64(&_SSASaveIrCodeCost))
}

func ShowDatabaseCacheCost() {
	log.Infof("SSA Database SaveIrCode Cost: %v", GetSSASaveIrCodeCost())
	log.Infof("SSA Database SaveVariable Cost: %v", ssadb.GetSSAVariableCost())
	log.Infof("SSA Database SaveIndex Cost: %v", ssadb.GetSSAIndexCost())
	log.Infof("SSA Database SaveSourceCode Cost: %v", ssadb.GetSSASourceCodeCost())
	log.Infof("SSA Database SaveType Cost: %v", ssadb.GetSSASaveTypeCost())
	log.Infof("SSA Database SaveScope Cost: %v Count: %v", ssautil.GetSSAScopeTimeCost(), ssautil.GetSSAScopeSaveCounter())
	log.Infof("SSA Database CacheToDatabase Cost: %v", GetSSACacheToDatabaseCost())
	log.Infof("SSA DB Cache DEBUG Cost: %v", GetSSACacheIterationCost())
	for _, cb := range _CostCallback {
		cb()
	}
}
