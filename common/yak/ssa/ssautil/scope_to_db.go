package ssautil

import (
	"encoding/json"
	"fmt"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/omap"
	"strconv"
)

func (s *ScopedVersionedTable[T]) SetParentId(i int64) {
	s.parentId = i
}

func (s *ScopedVersionedTable[T]) SetScopeLevel(i int) {
	s.level = i
}

func (s *ScopedVersionedTable[T]) GetValues() *omap.OrderedMap[string, *omap.OrderedMap[string, VersionedIF[T]]] {
	return s.values
}

func (s *ScopedVersionedTable[T]) GetVariable() *omap.OrderedMap[T, []VersionedIF[T]] {
	return s.variable
}

func (s *ScopedVersionedTable[T]) GetCaptured() *omap.OrderedMap[string, VersionedIF[T]] {
	return s.captured
}

func (s *ScopedVersionedTable[T]) GetIncomingPhi() *omap.OrderedMap[string, VersionedIF[T]] {
	return s.incomingPhi
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

func (s *ScopedVersionedTable[T]) SaveToDatabase() error {
	if !s.ShouldSaveToDatabase() {
		return nil
	}
	params := make(map[string]any)

	// save to database
	params["level"] = s.level

	// ScopedVersionedTable.values
	if s.values.Len() > 0 {
		values, err := s.values.MarshalJSONWithKeyValueFetcher(nil, ssaValueMarshal)
		if err != nil {
			return utils.Wrap(err, "marshal scope.values error")
		}
		params["values"] = strconv.Quote(string(values))
	} else {
		params["values"] = "[]"
	}

	vars, err := s.variable.MarshalJSONWithKeyValueFetcher(ssaValueMarshal, nil)
	if err != nil {
		return utils.Wrap(err, "marshal scope.variable error")
	}
	params["variable"] = strconv.Quote(string(vars))

	captured, err := s.captured.MarshalJSONWithKeyValueFetcher(nil, ssaValueMarshal)
	if err != nil {
		return utils.Wrap(err, "marshal scope.captured error")
	}
	params["captured"] = strconv.Quote(string(captured))

	incomingPhi, err := s.incomingPhi.MarshalJSONWithKeyValueFetcher(nil, ssaValueMarshal)
	if err != nil {
		return utils.Wrap(err, "marshal scope.incomingPhi error")
	}
	params["incomingPhi"] = strconv.Quote(string(incomingPhi))

	params["spin"] = s.spin
	params["this"] = s.persistentId
	params["parent"] = s.parentId

	raw, err := json.Marshal(params)
	if err != nil {
		return err
	}
	s.persistentNode.ExtraInfo = string(raw)

	s.persistentNode.ProgramName = s.persistentProgramName
	if err := consts.GetGormProjectDatabase().Save(s.persistentNode).Error; err != nil {
		return utils.Error(err.Error())
	}
	return nil
}
