package sfvm

import (
	"fmt"
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

// assignLocalAnchorBitVector overwrites anchor bits for each leaf in sourceValues so
// they map to the local slot index inside the current condition scope.
//
// Values that appear multiple times (same identity) get a union of all their slot
// indices to keep mask alignment stable.
func assignLocalAnchorBitVector(sourceValues Values) []anchorRestoreEntry {
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
		key := valueIdentity(v)
		if _, ok := restores[key]; !ok {
			var cloned *utils.BitVector
			if bits := v.GetAnchorBitVector(); bits != nil && !bits.IsEmpty() {
				cloned = bits.Clone()
			}
			restores[key] = anchorRestoreEntry{value: v, bits: cloned}
		}
		group[key] = append(group[key], entry{value: v, index: idx})
	}

	out := make([]anchorRestoreEntry, 0, len(restores))
	for _, restore := range restores {
		out = append(out, restore)
	}

	for _, entries := range group {
		if len(entries) == 0 {
			continue
		}
		bits := utils.NewBitVector()
		for _, e := range entries {
			bits.Set(e.index)
		}
		for _, e := range entries {
			e.value.SetAnchorBitVector(bits)
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

func markMaskByBitVector(mask []bool, bits *utils.BitVector) bool {
	if bits == nil || bits.IsEmpty() {
		return false
	}
	matched := false
	bits.ForEach(func(index int) {
		if index >= 0 && index < len(mask) {
			mask[index] = true
			matched = true
		}
	})
	return matched
}

func buildValueIdentityIndex(source Values) map[string][]int {
	indexByIdentity := make(map[string][]int, len(source))
	for idx, value := range source {
		if utils.IsNil(value) {
			continue
		}
		key := valueIdentity(value)
		indexByIdentity[key] = append(indexByIdentity[key], idx)
	}
	return indexByIdentity
}

func markMaskByValueIdentity(mask []bool, indexByIdentity map[string][]int, value ValueOperator) bool {
	if utils.IsNil(value) || len(indexByIdentity) == 0 {
		return false
	}
	indexes, ok := indexByIdentity[valueIdentity(value)]
	if !ok || len(indexes) == 0 {
		return false
	}
	for _, index := range indexes {
		if index >= 0 && index < len(mask) {
			mask[index] = true
		}
	}
	return true
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
