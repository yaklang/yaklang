package ssadb

import (
	"sync/atomic"
	"time"
)

var (
	_SSASaveTypeCost   uint64
	_SSAVariableCost   uint64
	_SSASourceCodeCost uint64
	_SSAIndexCost      uint64
)

func GetSSAIndexCost() time.Duration {
	return time.Duration(atomic.LoadUint64(&_SSAIndexCost))
}

func GetSSASaveTypeCost() time.Duration {
	return time.Duration(atomic.LoadUint64(&_SSASaveTypeCost))
}

func GetSSASourceCodeCost() time.Duration {
	return time.Duration(atomic.LoadUint64(&_SSASourceCodeCost))
}
