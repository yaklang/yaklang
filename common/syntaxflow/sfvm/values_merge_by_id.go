package sfvm

import (
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/ssa"
)

func buildValueByID(values Values) map[int64]ValueOperator {
	res := make(map[int64]ValueOperator, len(values))
	for _, value := range values {
		if utils.IsNil(value) {
			continue
		}
		if idGetter, ok := value.(ssa.GetIdIF); ok {
			res[idGetter.GetId()] = value
		}
	}
	return res
}

func valueSetFromValues(values Values) *ValueSet {
	set := NewValueSet()
	for _, v := range values {
		if utils.IsNil(v) {
			continue
		}
		if idGetter, ok := v.(ssa.GetIdIF); ok {
			set.Add(idGetter.GetId(), v)
		}
	}
	return set
}

func intersectValuesByString(left Values, right Values) Values {
	rightByString := make(map[string]struct{}, len(right))
	for _, rv := range right {
		if utils.IsNil(rv) {
			continue
		}
		rightByString[rv.String()] = struct{}{}
	}
	var out []ValueOperator
	for _, lv := range left {
		if utils.IsNil(lv) {
			continue
		}
		if _, ok := rightByString[lv.String()]; ok {
			out = append(out, lv)
		}
	}
	return NewValues(out)
}

func mergeValuesByID(left Values, right Values, andMode bool) Values {
	leftEmpty := left.IsEmpty()
	rightEmpty := right.IsEmpty()
	if andMode {
		if leftEmpty && rightEmpty {
			return NewEmptyValues()
		}
		if leftEmpty {
			return right
		}
		if rightEmpty {
			return left
		}
	}

	leftSet := valueSetFromValues(left)
	rightSet := valueSetFromValues(right)
	leftByIDMap := buildValueByID(left)
	rightByIDMap := buildValueByID(right)
	leftByID := leftSet.List()
	rightByID := rightSet.List()

	// Fallback for non-id values: keep existing side in OR mode.
	if len(leftByID) == 0 || len(rightByID) == 0 {
		if andMode {
			return intersectValuesByString(left, right)
		}
		return MergeValues(left, right)
	}

	var out []ValueOperator
	if andMode {
		andSet := leftSet.And(rightSet)
		if andSet != nil {
			out = andSet.List()
		}
		if len(out) == 0 {
			out = intersectValuesByString(left, right)
		}
		if len(out) == 0 {
			// Program-like compare candidates may not share stable IDs/strings.
			// Keep non-empty side instead of dropping everything.
			if len(right) > 0 {
				return right
			}
			if len(left) > 0 {
				return left
			}
		}
	} else {
		orSet := leftSet.Or(rightSet)
		if orSet != nil {
			out = orSet.List()
		}
	}
	// Preserve provenance across logical ops:
	//   outValue.bits |= leftValue.bits
	//   outValue.bits |= rightValue.bits
	for _, value := range out {
		idGetter, ok := value.(ssa.GetIdIF)
		if !ok {
			continue
		}
		if leftValue, ok := leftByIDMap[idGetter.GetId()]; ok {
			MergeAnchor(leftValue, value)
		}
		if rightValue, ok := rightByIDMap[idGetter.GetId()]; ok {
			MergeAnchor(rightValue, value)
		}
	}
	return NewValues(out)
}
