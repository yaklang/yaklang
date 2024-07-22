package ssa

import (
	syncAtomic "sync/atomic"
	"time"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
)

var (
	_SSASaveIrCodeCost      uint64
	_SSACacheToDatabaseCost uint64
	_CostCallback           []func()
)

func RegisterCostCallback(f func()) {
	_CostCallback = append(_CostCallback, f)
}

func GetSSASaveIrCodeCost() time.Duration {
	return time.Duration(syncAtomic.LoadUint64(&_SSASaveIrCodeCost))
}

func ShowDatabaseCacheCost() {
	log.Infof("SSA Database SaveIrCode Cost: %v", GetSSASaveIrCodeCost())
	log.Infof("SSA Database SaveIndex Cost: %v", ssadb.GetSSAIndexCost())
	log.Infof("SSA Database SaveSourceCode Cost: %v", ssadb.GetSSASourceCodeCost())
	log.Infof("SSA Database SaveType Cost: %v", ssadb.GetSSASaveTypeCost())
	log.Infof("SSA Database CacheToDatabase Cost: %v", GetSSACacheToDatabaseCost())
	for _, cb := range _CostCallback {
		cb()
	}
}

func GetSSACacheToDatabaseCost() time.Duration {
	return time.Duration(syncAtomic.LoadUint64(&_SSACacheToDatabaseCost))
}
