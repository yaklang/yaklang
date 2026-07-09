package ssa

import (
	"fmt"
	"sort"

	"github.com/yaklang/yaklang/common/utils/memedit"
	"golang.org/x/exp/slices"
)

type OffsetItem struct {
	variable    *Variable // maybe nil
	value       Value
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
	prog.OffsetMap[endOffset] = &OffsetItem{
		variable:    v,
		value:       v.GetValue(),
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
		rangeLength: r.Len(),
	}
}
