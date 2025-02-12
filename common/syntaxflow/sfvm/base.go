package sfvm

import (
	"regexp"
)

var BinOpRegexp = regexp.MustCompile(`(?i)([A-Za-z_-]+)\[([\w\S+=*/]{1,3})]`)

type ValueSet struct {
	m map[int64]ValueOperator
}

func NewValueSet() *ValueSet {
	return &ValueSet{
		m: make(map[int64]ValueOperator),
	}
}

func (v *ValueSet) Add(id int64, value ValueOperator) {
	if v.m == nil {
		v.m = make(map[int64]ValueOperator)
	}
	v.m[id] = value
}

func (v *ValueSet) Has(id int64) bool {
	_, ok := v.m[id]
	return ok
}

func (v *ValueSet) List() []ValueOperator {
	var res []ValueOperator
	for _, v := range v.m {
		res = append(res, v)
	}
	return res
}

func (v *ValueSet) And(other *ValueSet) *ValueSet {
	if v == nil || other == nil {
		return nil
	}
	res := NewValueSet()
	for id, vo := range v.m {
		if other.Has(id) {
			res.Add(id, vo)
		}
	}
	return res
}
