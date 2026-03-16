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

// valueIdentity is the logical identity key for slot-union:
// duplicates in a scope source list should share the same local slot set.
func valueIdentity(value ValueOperator) string {
	if id, ok := fetchId(value); ok {
		return "id:" + strconv.FormatInt(id, 10)
	}
	return fmt.Sprintf("%p", value)
}

// valuePointerKey is the physical identity key for restore bookkeeping:
// the same object can appear multiple times in the source list, but we only
// need to save/restore its bits once.
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

func snapshotOriginalAnchorBitsOnce(sourceValues Values) []anchorRestoreEntry {
	restores := make([]anchorRestoreEntry, 0, len(sourceValues))
	savedPointers := make(map[string]struct{}, len(sourceValues))
	for _, v := range sourceValues {
		if utils.IsNil(v) {
			continue
		}
		ptrKey := valuePointerKey(v)
		if _, ok := savedPointers[ptrKey]; ok {
			continue
		}
		savedPointers[ptrKey] = struct{}{}

		var cloned *utils.BitVector
		if bits := v.GetAnchorBitVector(); bits != nil && !bits.IsEmpty() {
			cloned = bits.Clone()
		}
		restores = append(restores, anchorRestoreEntry{value: v, bits: cloned})
	}
	return restores
}

func collectLocalSlotBitsByIdentity(sourceValues Values, base int) map[string]*utils.BitVector {
	localBitsByIdentity := make(map[string]*utils.BitVector, len(sourceValues))
	for idx, v := range sourceValues {
		if utils.IsNil(v) {
			continue
		}
		idKey := valueIdentity(v)
		localBits := localBitsByIdentity[idKey]
		if localBits == nil {
			localBits = utils.NewBitVector()
			localBitsByIdentity[idKey] = localBits
		}
		localBits.Set(base + idx)
	}
	return localBitsByIdentity
}

func applyScopedAnchorBits(restores []anchorRestoreEntry, localBitsByIdentity map[string]*utils.BitVector) {
	for _, restore := range restores {
		localBits := localBitsByIdentity[valueIdentity(restore.value)]
		if localBits == nil || localBits.IsEmpty() {
			restore.value.SetAnchorBitVector(restore.bits)
			continue
		}

		merged := localBits.Clone()
		if restore.bits != nil && !restore.bits.IsEmpty() {
			merged.Or(restore.bits)
		}
		restore.value.SetAnchorBitVector(merged)
	}
}

// assignLocalAnchorBitVector adds local anchor bits for each leaf in sourceValues so
// they map to the local slot index inside a scope range [base, base+len(sourceValues)).
//
// Values that appear multiple times (same identity) get a union of all their slot
// indices to keep mask alignment stable.
func assignLocalAnchorBitVector(sourceValues Values, base int) []anchorRestoreEntry {
	// 1) snapshotOriginalAnchorBitsOnce: restore bookkeeping by object pointer (save once)
	restores := snapshotOriginalAnchorBitsOnce(sourceValues)

	// 2) collectLocalSlotBitsByIdentity: collect local slot bits by logical identity (union duplicates)
	localBitsByIdentity := collectLocalSlotBitsByIdentity(sourceValues, base)

	// 3) applyScopedAnchorBits: apply (localBits OR originalBits) back to each distinct object pointer
	applyScopedAnchorBits(restores, localBitsByIdentity)
	return restores
}

// restoreAnchorBitVector restores anchor bits overwritten by assignLocalAnchorBitVector
// at anchor-scope start.
func restoreAnchorBitVector(entries []anchorRestoreEntry) {
	for _, e := range entries {
		if utils.IsNil(e.value) {
			continue
		}
		e.value.SetAnchorBitVector(e.bits)
	}
}

func buildSlotAnchorBitVectors(sourceValues Values, base int) []*utils.BitVector {
	slotBits := make([]*utils.BitVector, len(sourceValues))
	for idx, value := range sourceValues {
		if utils.IsNil(value) {
			continue
		}
		var bits *utils.BitVector
		if existed := value.GetAnchorBitVector(); existed != nil && !existed.IsEmpty() {
			bits = existed.Clone()
		} else {
			bits = utils.NewBitVector()
		}
		bits.Set(base + idx)
		slotBits[idx] = bits
	}
	return slotBits
}

// markMaskByBitVector projects anchor bits back to the current scope mask:
//
//	mask[i] = true  iff  (base+i) in bits
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

func mergeAnchorBits(dst ValueOperator, sourceBits *utils.BitVector) {
	if utils.IsNil(dst) || sourceBits == nil || sourceBits.IsEmpty() {
		return
	}
	dstBits := dst.GetAnchorBitVector()
	if dstBits == nil || dstBits.IsEmpty() {
		dst.SetAnchorBitVector(sourceBits.Clone())
		return
	}
	merged := dstBits.Clone()
	merged.Or(sourceBits)
	dst.SetAnchorBitVector(merged)
}

// MergeAnchor propagates provenance from source to each destination value:
//
//	for each d in dst: d.bits |= source.bits
func MergeAnchor(source ValueOperator, dst ...ValueOperator) {
	if utils.IsNil(source) {
		return
	}
	sourceBits := source.GetAnchorBitVector()
	if sourceBits == nil || sourceBits.IsEmpty() {
		return
	}
	for _, operator := range dst {
		mergeAnchorBits(operator, sourceBits)
	}
}
