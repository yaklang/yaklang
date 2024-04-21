package ssautil

import (
	"encoding/json"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
	"strconv"
)

func (s *ScopedVersionedTable[T]) SaveToDatabase() error {
	if !s.ShouldSaveToDatabase() {
		return nil
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

	s.persistentNode.ProgramName = s.persistentProgramName
	if err := consts.GetGormProjectDatabase().Save(s.persistentNode).Error; err != nil {
		return utils.Error(err.Error())
	}
	return nil
}

func (s *ScopedVersionedTable[T]) SyncFromDatabase() error {
	if !s.ShouldSaveToDatabase() {
		return nil
	}

	var err error
	s.persistentNode, err = ssadb.GetIrScope(s.persistentId)
	if err != nil {
		return utils.Wrapf(err, "failed to get tree node")
	}

	// handle persistent id
	var params = make(map[string]any)
	if err := json.Unmarshal([]byte(s.persistentNode.ExtraInfo), &params); err != nil {
		return utils.Wrapf(err, "failed to unmarshal extra info")
	}

	// load to database
	s.level = utils.MapGetInt(params, "level")

	quotedValues := utils.MapGetString(params, "values")
	quotedVariable := utils.MapGetString(params, "variable")
	quotedCaptured := utils.MapGetString(params, "captured")
	quotedIncomingPhi := utils.MapGetString(params, "incomingPhi")
	s.spin = utils.MapGetBool(params, "spin")
	s.parentId = utils.MapGetInt64(params, "parent")

	values, err := strconv.Unquote(quotedValues)
	if err != nil {
		return utils.Wrapf(err, "unquote values error")
	}
	if err := s.values.UnmarshalJSON([]byte(values)); err != nil {
		return utils.Wrapf(err, "unmarshal values error")
	}

	variable, err := strconv.Unquote(quotedVariable)
	if err != nil {
		return utils.Wrapf(err, "unquote variable error")
	}
	if err := s.variable.UnmarshalJSON([]byte(variable)); err != nil {
		return utils.Wrapf(err, "unmarshal variable error")
	}

	captured, err := strconv.Unquote(quotedCaptured)
	if err != nil {
		return utils.Wrapf(err, "unquote captured error")
	}

	if err := s.captured.UnmarshalJSON([]byte(captured)); err != nil {
		return utils.Wrapf(err, "unmarshal captured error")
	}

	incomingPhi, err := strconv.Unquote(quotedIncomingPhi)
	if err != nil {
		return utils.Wrapf(err, "unquote incomingPhi error")
	}

	if err := s.incomingPhi.UnmarshalJSON([]byte(incomingPhi)); err != nil {
		return utils.Wrapf(err, "unmarshal incomingPhi error")
	}

	return nil
}
