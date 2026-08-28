package ssa

import (
	"strings"

	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
)

// GetVariableFromDB restores a Variable's use-range offsets from the
// ir_offsets table. The returned variable has skipPersistOffsets=true to
// prevent AddRange/persistOffset from re-persisting offsets that already
// exist in the DB.
//
// Callers MUST call RestoreVariableFinish(v) — ideally via defer — to clear
// the flag so subsequent compile-time AddRange/Assign calls persist offsets
// normally. For callers that also call Assign, the recommended pattern is:
//
//	v := GetVariableFromDB(id, name, progName)
//	defer RestoreVariableFinish(v)
//	v.Assign(value)
//
// This ensures skipPersistOffsets is true during the Assign call (so
// persistAllOffsets is suppressed) and cleared afterward regardless of
// whether Assign succeeded or returned an error.
//
// For callers that do not call Assign, simply:
//
//	v := GetVariableFromDB(id, name, progName)
//	defer RestoreVariableFinish(v)
//	// use v ...
func GetVariableFromDB(id int64, name string, programName string) *Variable {
	v := NewVariable(0, name, false, nil).(*Variable)
	v.skipPersistOffsets = true
	offset := ssadb.GetOffsetByVariable(name, id, programName)
	for _, o := range offset {
		editor, start, end, err := o.GetStartAndEndPositions()
		if err != nil {
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

// RestoreVariableFinish clears the skipPersistOffsets flag on a variable
// restored by GetVariableFromDB. This MUST be called after the restore
// sequence to ensure subsequent AddRange/Assign calls persist offsets
// normally. Safe to call multiple times and on nil variables.
func RestoreVariableFinish(v *Variable) {
	if v == nil {
		return
	}
	v.skipPersistOffsets = false
}
