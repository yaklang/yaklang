package ssautil

import (
	"encoding/json"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
	"strconv"
)

func (s *ScopedVersionedTable[T]) SaveToDatabase() error {
	if s.persistentId <= 0 {
		return utils.Error("persistentNode should not be nil")
	}

	params := make(map[string]any)

	// save to database
	params["level"] = s.level

	// ScopedVersionedTable.values
	values, err := s.values.MarshalJSON()
	if err != nil {
		return utils.Wrap(err, "marshal scope.values error")
	}
	params["values"] = strconv.Quote(string(values))

	vars, err := s.variable.MarshalJSON()
	if err != nil {
		return utils.Wrap(err, "marshal scope.variable error")
	}
	params["variable"] = strconv.Quote(string(vars))

	captured, err := s.captured.MarshalJSON()
	if err != nil {
		return utils.Wrap(err, "marshal scope.captured error")
	}
	params["captured"] = strconv.Quote(string(captured))

	incomingPhi, err := s.incomingPhi.MarshalJSON()
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

	if err := consts.GetGormProjectDatabase().Save(s.persistentNode).Error; err != nil {
		return utils.Error(err.Error())
	}
	return nil
}

func (s *ScopedVersionedTable[T]) SyncFromDatabase() error {
	if s.persistentId <= 0 {
		return utils.Error("persistentId should be greater than 0")
	}

	var err error
	s.persistentNode, err = ssadb.GetTreeNode(s.persistentId)
	if err != nil {
		return utils.Wrapf(err, "failed to get tree node")
	}

	// handle persistent id
	var params = make(map[string]any)
	if err := json.Unmarshal([]byte(s.persistentNode.ExtraInfo), &params); err != nil {
		return utils.Wrapf(err, "failed to unmarshal extra info")
	}

	// load to database

	return nil
}
