package ssa

import (
	"fmt"
	"sort"

	"github.com/yaklang/yaklang/common/log"
	"golang.org/x/exp/slices"
)

type OffsetItem struct {
	variable *Variable // maybe nil
	value    Value
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
	for i := 0; i < len(prog.OffsetSortedSlice); i++ {
		offset := prog.OffsetSortedSlice[i]
		value := prog.OffsetMap[offset].GetValue()
		if value == nil {
		}
		fmt.Printf("%d: %s\n", offset, value.String())
	}
}

func (prog *Program) SetOffsetVariable(v *Variable, r *Range) {
	if r == nil {
		log.Errorf("SetOffsetVariable: range is nil")
		return
	}
	endOffset := r.GetEndOffset()
	prog.OffsetSortedSlice = InsertSortedIntSlice(prog.OffsetSortedSlice, endOffset)

	if _, ok := prog.OffsetMap[endOffset]; ok {
		prog.OffsetMap[endOffset].variable = v
	} else {
		prog.OffsetMap[endOffset] = &OffsetItem{variable: v, value: v.GetValue()}
	}
}

func (prog *Program) SetOffsetValue(v Value, r *Range) {
	if r == nil {
		log.Errorf("SetOffsetValue: range is nil")
		return
	}
	endOffset := r.GetEndOffset()
	prog.OffsetSortedSlice = InsertSortedIntSlice(prog.OffsetSortedSlice, endOffset)
	prog.OffsetMap[endOffset] = &OffsetItem{variable: nil, value: v}
}
