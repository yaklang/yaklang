package xmlrecord

import (
	"context"
	"encoding/xml"
	"io"

	"github.com/yaklang/yaklang/common/sca/core/budget"
)

// destPlan charges heap objects encoding/xml allocates into out before Decode.
// Each destination slot is a field path, not a global local-name maximum.
type destPlan struct {
	slices     []destSlot
	once       []destSlot
	properties []destSlot
}

type pathElem struct {
	Space, Local string
}

type destSlot struct {
	path        []pathElem
	store, heap int64
}

func pathMatch(stack []xml.Name, path []pathElem) bool {
	if len(path) == 1 && path[0].Local == "*" {
		return len(stack) == 1
	}
	if len(stack) != len(path)+1 {
		return false
	}
	for i, p := range path {
		el := stack[1+i]
		if p.Local != el.Local {
			return false
		}
		if p.Space != "" && p.Space != el.Space {
			return false
		}
	}
	return true
}

// pointerCopies is the number of pointee allocations encoding/xml will make
// for a pointer field. A single-value parent overwrites on repeated tags.
// A pointer inside each slice element is allocated once per parent instance
// that actually contains the child, bounded by min(parentHits, childHits).
func pointerCopies(path []pathElem, hits func([]pathElem) int64) int64 {
	n := hits(path)
	if n <= 0 {
		return 0
	}
	if len(path) <= 1 {
		return 1
	}
	parent := hits(path[:len(path)-1])
	if parent <= 0 {
		return 1
	}
	if n < parent {
		return n
	}
	return parent
}

func chargeDestination(ctx context.Context, plan *destPlan, hits func([]pathElem) int64, props int64) error {
	var need int64
	add := func(n int64) error {
		var err error
		need, err = budget.SizeAdd(need, n)
		return err
	}
	for _, s := range plan.slices {
		n := hits(s.path)
		if n <= 0 {
			continue
		}
		capn := xmlSliceCap(n)
		store, err := budget.SizeMul(int(capn), s.store)
		if err != nil {
			return err
		}
		heap, err := budget.SizeMul(int(n), s.heap)
		if err != nil {
			return err
		}
		if err := add(store); err != nil {
			return err
		}
		if err := add(heap); err != nil {
			return err
		}
	}
	for _, s := range plan.once {
		n := pointerCopies(s.path, hits)
		if n <= 0 {
			continue
		}
		heap, err := budget.SizeMul(int(n), s.heap)
		if err != nil {
			return err
		}
		if err := add(heap); err != nil {
			return err
		}
	}
	if props > 0 {
		for _, s := range plan.properties {
			g, err := budget.SizeMul(int(props), s.heap)
			if err != nil {
				return err
			}
			if err := add(g); err != nil {
				return err
			}
		}
	}
	if need == 0 {
		return nil
	}
	return budget.From(ctx).Add(1, need)
}

// xmlSliceCap is capacity after n encoding/xml Grow(1) calls from an empty
// slice, using Go 1.22 nextslicecap. malloc size-class rounding is not charged.
func xmlSliceCap(n int64) int64 {
	if n <= 0 {
		return 0
	}
	var capn, length int64
	for i := int64(0); i < n; i++ {
		need := length + 1
		if need > capn {
			capn = nextSliceCap(need, capn)
		}
		length++
	}
	return capn
}

func nextSliceCap(newLen, oldCap int64) int64 {
	if oldCap == 0 {
		return newLen
	}
	doublecap := oldCap + oldCap
	if newLen > doublecap {
		return newLen
	}
	const threshold = 256
	if oldCap < threshold {
		return doublecap
	}
	newcap := oldCap
	for {
		newcap += (newcap + 3*threshold) >> 2
		if newcap <= 0 {
			return newLen
		}
		if newcap >= newLen {
			return newcap
		}
	}
}

// countFixedStarts holds only the bounded element stack and one counter per
// fixed schema slot. Unknown element names cannot grow a path-index map.
func countFixedStarts(ctx context.Context, d *xml.Decoder, l budget.Limits, plan *destPlan) (func([]pathElem) int64, int64, error) {
	st := budget.From(ctx)
	slots := len(plan.slices) + 2*len(plan.once)
	counterBytes, err := budget.SizeMul(slots, 8)
	if err != nil {
		return nil, 0, err
	}
	need, err := budget.SizeAdd(counterBytes, 2*budget.SizeSlice, budget.SizeObject)
	if err != nil {
		return nil, 0, err
	}
	if err = st.Working(need); err != nil {
		return nil, 0, err
	}
	var stack []xml.Name
	counts := make([]int64, slots)
	guard := tokens{ctx: ctx, d: d, l: l}
	var props int64
	inProps := 0
	for {
		tok, err := guard.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, 0, err
		}
		switch v := tok.(type) {
		case xml.StartElement:
			if len(stack) == cap(stack) {
				capacity := min(l.MaxSyntaxDepth, max(2, cap(stack)*2))
				n, err := budget.SizeMul(capacity, 2*budget.SizeString)
				if err != nil {
					return nil, 0, err
				}
				if err := st.Working(n + budget.SizeSlice); err != nil {
					return nil, 0, err
				}
				grown := make([]xml.Name, len(stack), capacity)
				copy(grown, stack)
				stack = grown
			}
			stack = append(stack, v.Name) // Token checked depth before this append.
			for i, slot := range plan.slices {
				if pathMatch(stack, slot.path) {
					counts[i]++
				}
			}
			for i, slot := range plan.once {
				if pathMatch(stack, slot.path) {
					counts[len(plan.slices)+i]++
				}
				if len(slot.path) > 1 && pathMatch(stack, slot.path[:len(slot.path)-1]) {
					counts[len(plan.slices)+len(plan.once)+i]++
				}
			}
			if inProps > 0 {
				props++
				if err := st.Result(budget.SizeOfString(v.Name.Local)); err != nil {
					return nil, 0, err
				}
			}
			if v.Name.Local == "properties" {
				inProps++
			}
		case xml.EndElement:
			if v.Name.Local == "properties" && inProps > 0 {
				inProps--
			}
			if len(stack) > 0 {
				stack = stack[:len(stack)-1]
			}
		}
	}
	return func(path []pathElem) int64 {
		equal := func(a, b []pathElem) bool {
			if len(a) != len(b) {
				return false
			}
			for i := range a {
				if a[i] != b[i] {
					return false
				}
			}
			return true
		}
		for i, slot := range plan.slices {
			if equal(path, slot.path) {
				return counts[i]
			}
		}
		for i, slot := range plan.once {
			if equal(path, slot.path) {
				return counts[len(plan.slices)+i]
			}
		}
		for i, slot := range plan.once {
			if len(slot.path) > 1 && equal(path, slot.path[:len(slot.path)-1]) {
				return counts[len(plan.slices)+len(plan.once)+i]
			}
		}
		return 0
	}, props, nil
}
