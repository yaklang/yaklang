package sfvm

import (
	"fmt"
	"reflect"
	"strconv"

	"github.com/yaklang/yaklang/common/utils"
)

type anchorRestoreEntry struct {
	value ValueOperator
	bits  *utils.BitVector
}

func valueIdentity(value ValueOperator) string {
	if id, ok := fetchId(value); ok {
		return "id:" + strconv.FormatInt(id, 10)
	}
	return fmt.Sprintf("%p", value)
}

func valuePointerKey(value ValueOperator) string {
	if utils.IsNil(value) {
		return ""
	}
	v := reflect.ValueOf(value)
	switch v.Kind() {
	case reflect.Pointer, reflect.Map, reflect.Slice, reflect.Func, reflect.Chan, reflect.UnsafePointer:
		return fmt.Sprintf("%T:%d", value, v.Pointer())
	default:
		// Fallback: stable-enough within a single execution.
		return fmt.Sprintf("%T:%s", value, valueIdentity(value))
	}
}

// assignLocalAnchorBitVector adds local anchor bits for each leaf in sourceValues so
// they map to the local slot index inside a scope range [base, base+len(sourceValues)).
//
// Values that appear multiple times (same identity) get a union of all their slot
// indices to keep mask alignment stable.
func assignLocalAnchorBitVector(sourceValues Values, base int) []anchorRestoreEntry {
	type entry struct {
		value ValueOperator
		index int
	}
	group := make(map[string][]entry, len(sourceValues))
	restores := make(map[string]anchorRestoreEntry, len(sourceValues))
	for idx, v := range sourceValues {
		if utils.IsNil(v) {
			continue
		}
		idKey := valueIdentity(v)
		ptrKey := valuePointerKey(v)
		if _, ok := restores[ptrKey]; !ok {
			var cloned *utils.BitVector
			if bits := v.GetAnchorBitVector(); bits != nil && !bits.IsEmpty() {
				cloned = bits.Clone()
			}
			restores[ptrKey] = anchorRestoreEntry{value: v, bits: cloned}
		}
		group[idKey] = append(group[idKey], entry{value: v, index: idx})
	}

	out := make([]anchorRestoreEntry, 0, len(restores))
	for _, restore := range restores {
		out = append(out, restore)
	}

	for _, entries := range group {
		if len(entries) == 0 {
			continue
		}
		local := utils.NewBitVector()
		for _, e := range entries {
			local.Set(base + e.index)
		}
		for _, e := range entries {
			restore := restores[valuePointerKey(e.value)]
			merged := local.Clone()
			if restore.bits != nil && !restore.bits.IsEmpty() {
				merged.Or(restore.bits)
			}
			e.value.SetAnchorBitVector(merged)
		}
	}

	return out
}

func restoreAnchorBitVector(entries []anchorRestoreEntry) {
	for _, e := range entries {
		if utils.IsNil(e.value) {
			continue
		}
		e.value.SetAnchorBitVector(e.bits)
	}
}

func markMaskByBitVector(mask []bool, bits *utils.BitVector, base int) bool {
	if bits == nil || bits.IsEmpty() {
		return false
	}
	matched := false
	end := base + len(mask)
	bits.ForEach(func(index int) {
		if index >= base && index < end {
			mask[index-base] = true
			matched = true
		}
	})
	return matched
}

func mergeAnchorBitVector(dst ValueOperator, src ValueOperator) {
	if utils.IsNil(dst) || utils.IsNil(src) {
		return
	}
	srcBits := src.GetAnchorBitVector()
	if srcBits == nil || srcBits.IsEmpty() {
		return
	}
	dstBits := dst.GetAnchorBitVector()
	if dstBits == nil || dstBits.IsEmpty() {
		dst.SetAnchorBitVector(srcBits)
		return
	}
	merged := dstBits.Clone()
	merged.Or(srcBits)
	dst.SetAnchorBitVector(merged)
}

func mergeAnchorBitVectorToResult(result Values, source ValueOperator) {
	if result.IsEmpty() || utils.IsNil(source) {
		return
	}
	sourceBits := source.GetAnchorBitVector()
	if sourceBits == nil || sourceBits.IsEmpty() {
		return
	}
	for _, operator := range result {
		if utils.IsNil(operator) {
			continue
		}
		existing := operator.GetAnchorBitVector()
		if existing == nil || existing.IsEmpty() {
			operator.SetAnchorBitVector(sourceBits)
			continue
		}
		merged := existing.Clone()
		merged.Or(sourceBits)
		operator.SetAnchorBitVector(merged)
	}
}

// MergeAnchorBitVectorToResult is exported for callers outside sfvm (e.g. ssaapi native calls).
func MergeAnchorBitVectorToResult(result Values, source ValueOperator) {
	mergeAnchorBitVectorToResult(result, source)
}
