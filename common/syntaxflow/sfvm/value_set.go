package sfvm

import "sort"

type ValueSet struct {
	m map[int64]ValueOperator
}

type valueSetListSort struct {
	ids    []int64
	values []ValueOperator
}

func (s valueSetListSort) Len() int { return len(s.ids) }
func (s valueSetListSort) Less(i, j int) bool {
	return s.ids[i] < s.ids[j]
}
func (s valueSetListSort) Swap(i, j int) {
	s.ids[i], s.ids[j] = s.ids[j], s.ids[i]
	s.values[i], s.values[j] = s.values[j], s.values[i]
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
	if v == nil || len(v.m) == 0 {
		return nil
	}

	ids := make([]int64, len(v.m))
	res := make([]ValueOperator, len(v.m))
	idx := 0
	for id, value := range v.m {
		ids[idx] = id
		res[idx] = value
		idx++
	}
	sort.Sort(valueSetListSort{ids: ids, values: res})
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

func (v *ValueSet) Or(other *ValueSet) *ValueSet {
	if v == nil && other == nil {
		return nil
	}
	res := NewValueSet()
	if v != nil {
		for id, vo := range v.m {
			res.Add(id, vo)
		}
	}
	if other != nil {
		for id, vo := range other.m {
			res.Add(id, vo)
		}
	}
	return res
}
