package ssautil

import (
	"encoding/json"
	"fmt"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
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

//func (s *ScopedVersionedTable[T]) GetValues() *omap.OrderedMap[string, *omap.OrderedMap[string, VersionedIF[T]]] {
//	return s.values
//}
//
//func (s *ScopedVersionedTable[T]) GetVariable() *omap.OrderedMap[T, []VersionedIF[T]] {
//	return s.variable
//}
//
//func (s *ScopedVersionedTable[T]) GetCaptured() *omap.OrderedMap[string, VersionedIF[T]] {
//	return s.captured
//}
//
//func (s *ScopedVersionedTable[T]) GetIncomingPhi() *omap.OrderedMap[string, VersionedIF[T]] {
//	return s.incomingPhi
//}

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

func (s *ScopedVersionedTable[T]) SaveToDatabase() error {
	return s._SaveToDatabase()
}

func (s *ScopedVersionedTable[T]) _SaveToDatabase() error {
	if !s.ShouldSaveToDatabase() {
		return nil
	}
	params := make(map[string]any)

	// save to database
	params["level"] = s.level

	start := time.Now()
	defer func() {
		atomic.AddUint64(&_SSAScopeTimeCost, uint64(time.Now().Sub(start).Nanoseconds()))
		atomic.AddUint64(&_SSAScopeSaveCounter, 1)
	}()

	// ScopedVersionedTable.values
	//if s.values.Len() > 0 {
	//	values, err := s.values.MarshalJSONWithKeyValueFetcher(nil, ssaValueMarshal)
	//	if err != nil {
	//		log.Warnf("marshal scope.values error: %v", err)
	//		params["values"] = "[]"
	//	} else {
	//		params["values"] = strconv.Quote(string(values))
	//	}
	//
	//} else {
	//	params["values"] = "[]"
	//}

	//vars, err := s.variable.MarshalJSONWithKeyValueFetcher(ssaValueMarshal, nil)
	//if err != nil {
	//	return utils.Wrap(err, "marshal scope.variable error")
	//}
	//params["variable"] = strconv.Quote(string(vars))
	//
	//captured, err := s.captured.MarshalJSONWithKeyValueFetcher(nil, ssaValueMarshal)
	//if err != nil {
	//	return utils.Wrap(err, "marshal scope.captured error")
	//}
	//params["captured"] = strconv.Quote(string(captured))
	//
	//incomingPhi, err := s.incomingPhi.MarshalJSONWithKeyValueFetcher(nil, ssaValueMarshal)
	//if err != nil {
	//	return utils.Wrap(err, "marshal scope.incomingPhi error")
	//}
	//params["incomingPhi"] = strconv.Quote(string(incomingPhi))

	params["spin"] = s.spin
	params["this"] = s.persistentId
	params["parent"] = s.parentId

	raw, err := json.Marshal(params)
	if err != nil {
		return err
	}
	s.persistentNode.ExtraInfo = string(raw)

	s.persistentNode.ProgramName = s.persistentProgramName
	if err := ssadb.GetDB().Save(s.persistentNode).Error; err != nil {
		return utils.Error(err.Error())
	}
	return nil
}
