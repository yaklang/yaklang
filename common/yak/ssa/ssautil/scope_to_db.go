package ssautil

import (
	"encoding/json"
	"fmt"
	"sync/atomic"
	"time"
)

var (
	_SSAScopeTimeCost    uint64
	_SSAScopeSaveCounter uint64
)

func GetSSAScopeTimeCost() time.Duration {
	return time.Duration(atomic.LoadUint64(&_SSAScopeTimeCost))
}

func GetSSAScopeSaveCounter() uint64 {
	return atomic.LoadUint64(&_SSAScopeSaveCounter)
}

func (s *ScopedVersionedTable[T]) SetParentId(i int64) {
	s.parentId = i
}

func (s *ScopedVersionedTable[T]) SetScopeLevel(i int) {
	s.level = i
}

func ssaValueMarshal(raw any) ([]byte, error) {
	v, ok := raw.(SSAValue)
	if ok {
		return []byte(fmt.Sprint(v.GetId())), nil
	}
	hookedMarshal, ok := raw.(interface {
		MarshalJSONWithKeyValueFetcher(func(any) ([]byte, error), func(any) ([]byte, error)) ([]byte, error)
	})
	if ok {
		return hookedMarshal.MarshalJSONWithKeyValueFetcher(ssaValueMarshal, ssaValueMarshal)
	}
	return json.Marshal(raw)
}
