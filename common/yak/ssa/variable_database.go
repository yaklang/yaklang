package ssa

import (
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
)

func GetVariableFromDB(id int64, name string) *Variable {
	v := NewVariable(0, name, false, nil).(*Variable)
	offset := ssadb.GetOffsetByVariable(name, id)
	for _, o := range offset {
		editor, start, end, err := o.GetStartAndEndPositions()
		if err != nil {
			log.Errorf("GetStartAndEndPositions failed: %v", err)
			continue
		}
		rng := editor.GetRangeByPosition(start, end)
		v.AddRange(rng, true)
	}
	return v
}
