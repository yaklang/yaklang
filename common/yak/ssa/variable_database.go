package ssa

import (
	"strings"

	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
)

func GetVariableFromDB(id int64, name string, programName string) *Variable {
	v := NewVariable(0, name, false, nil).(*Variable)
	offset := ssadb.GetOffsetByVariable(name, id, programName)
	for _, o := range offset {
		editor, start, end, err := o.GetStartAndEndPositions()
		if err != nil {
			// Missing source hash/editor records are expected for some DB-only/synthetic values.
			// Keep signal low to avoid flooding logs during analysis output rendering.
			if strings.Contains(err.Error(), "record not found") {
				continue
			}
			log.Errorf("GetStartAndEndPositions failed: %v", err)
			continue
		}
		if editor == nil || start == nil || end == nil {
			continue
		}
		rng := editor.GetRangeByPosition(start, end)
		v.AddRange(rng, true)
	}
	return v
}
