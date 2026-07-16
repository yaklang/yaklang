package ssa

import (
	"sort"

	"github.com/yaklang/yaklang/common/utils/memedit"
	"golang.org/x/exp/slices"
)

type OffsetItem struct {
	variable    *Variable // maybe nil
	value       Value
	rangeLength int
}

func (item *OffsetItem) GetVariable() *Variable {
	return item.variable
}

func (item *OffsetItem) GetValue() Value {
	if item.value != nil {
		return item.value
	} else if item.variable != nil {
		return item.variable.GetValue()
	}
	return nil
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
	prog.offsets.showAll()
}

func (prog *Program) SetOffsetVariable(v *Variable, r *memedit.Range) {
	prog.offsets.setVariable(v, r)
}

func (prog *Program) ForceSetOffsetValue(v Value, r *memedit.Range) {
	prog.offsets.setValue(v, r, true)
}

func (prog *Program) SetOffsetValue(v Value, r *memedit.Range) {
	prog.offsets.setValue(v, r, false)
}

func (prog *Program) SetOffsetValueEx(v Value, r *memedit.Range, force bool) {
	prog.offsets.setValue(v, r, force)
}