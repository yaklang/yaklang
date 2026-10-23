package ssa

import (
	"fmt"
	"sort"

	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/memedit"
	"golang.org/x/exp/slices"
)

type OffsetItem struct {
	variable    *Variable // maybe nil
	value       Value
	valueID     int64
	program     *Program
	rangeLength int
}

// OffsetRLock/OffsetRUnlock expose the offset-map read lock so cross-package
// readers (e.g. ssaapi) can hold it while inspecting OffsetMap/
// OffsetSortedSlice, which are otherwise unsafe under concurrent scan-time lazy
// reloads. Write-path mutations stay in-package (SetOffsetValue etc.) so no
// public write-lock wrapper is needed.
func (prog *Program) OffsetRLock()   { prog.offsetMu.RLock() }
func (prog *Program) OffsetRUnlock() { prog.offsetMu.RUnlock() }

func (item *OffsetItem) GetVariable() *Variable {
	return item.variable
}

func (item *OffsetItem) GetValue() Value {
	if item == nil {
		return nil
	}
	if !utils.IsNil(item.value) {
		return item.value
	}
	if item.variable != nil {
		if value := item.variable.GetValue(); !utils.IsNil(value) {
			return value
		}
	}
	if item.valueID <= 0 || item.program == nil || item.program.Cache == nil || item.program.Cache.instructions == nil {
		return nil
	}
	inst := item.program.Cache.instructions.reloadPersistedInstruction(item.valueID)
	value, ok := ToValue(inst)
	if !ok || utils.IsNil(value) {
		return nil
	}
	return value
}

func InsertSortedIntSlice(ts []int, t int) []int {
	i, found := sort.Find(len(ts), func(i int) int {
		return t - ts[i]
	})

	// if found, return the original slice
	if found {
		return ts
	}
	return slices.Insert(ts, i, t)
}

func (prog *Program) ShowOffsetMap() {
	prog.offsetMu.RLock()
	defer prog.offsetMu.RUnlock()
	for i := 0; i < len(prog.OffsetSortedSlice); i++ {
		offset := prog.OffsetSortedSlice[i]
		value := prog.OffsetMap[offset].GetValue()
		if value == nil {
		}
		fmt.Printf("%d: %s\n", offset, value.String())
	}
}

func (prog *Program) SetOffsetVariable(v *Variable, r *memedit.Range) {
	if r == nil {
		return
	}
	endOffset := r.GetEndOffset()

	prog.offsetMu.Lock()
	defer prog.offsetMu.Unlock()
	// If it already exists, then the trust range is smaller
	if item, ok := prog.OffsetMap[endOffset]; ok && item.rangeLength <= r.Len() {
		return
	}

	prog.OffsetSortedSlice = InsertSortedIntSlice(prog.OffsetSortedSlice, endOffset)
	value := v.GetValue()
	prog.OffsetMap[endOffset] = &OffsetItem{
		variable:    v,
		value:       value,
		valueID:     offsetValueID(value),
		program:     prog,
		rangeLength: r.Len(),
	}
}

func (prog *Program) ForceSetOffsetValue(v Value, r *memedit.Range) {
	prog.SetOffsetValueEx(v, r, true)
}

func (prog *Program) SetOffsetValue(v Value, r *memedit.Range) {
	prog.SetOffsetValueEx(v, r, false)
}

func (prog *Program) SetOffsetValueEx(v Value, r *memedit.Range, force bool) {
	if r == nil {
		return
	}
	endOffset := r.GetEndOffset()

	prog.offsetMu.Lock()
	defer prog.offsetMu.Unlock()
	// If it already exists, then the trust range is smaller
	if item, ok := prog.OffsetMap[endOffset]; !force && ok && item.rangeLength <= r.Len() {
		return
	}

	prog.OffsetSortedSlice = InsertSortedIntSlice(prog.OffsetSortedSlice, endOffset)
	prog.OffsetMap[endOffset] = &OffsetItem{
		variable:    nil,
		value:       v,
		valueID:     offsetValueID(v),
		program:     prog,
		rangeLength: r.Len(),
	}
}

func offsetValueID(value Value) int64 {
	if utils.IsNil(value) {
		return 0
	}
	return value.GetId()
}
