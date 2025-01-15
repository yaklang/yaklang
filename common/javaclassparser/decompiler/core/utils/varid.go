package utils

import (
	"fmt"
	"github.com/samber/lo"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

type VariableId struct {
	parent   *VariableId
	children []*VariableId
}

func NewRootVariableId() *VariableId {
	return &VariableId{}
}

//	func (v *VariableId) Uid() string {
//		return fmt.Sprintf("var%d", v.Uid())
//	}
func (v *VariableId) Id() int {
	return v._id(utils.NewSet[*VariableId]()) - 1
}
func (v *VariableId) _id(set *utils.Set[*VariableId]) int {
	if set.Has(v) {
		log.Errorf("cycle detected in variable id")
		return 0
	}
	set.Add(v)
	if v.parent == nil {
		return 0
	}
	return v.parent._id(set) + 1
}
func (v *VariableId) String() string {
	return fmt.Sprintf("var%d", v.Id())
}

func (v *VariableId) Delete() {
	for _, child := range v.children {
		child.parent = nil
	}
	if v.parent != nil {
		v.parent.children = lo.Filter(v.parent.children, func(item *VariableId, index int) bool {
			return item != v
		})
		for _, child := range v.children {
			child.parent = v.parent
		}
	}
}
func (v *VariableId) Horizontal() *VariableId {
	newV := v.parent.Next()
	return newV
}
func (v *VariableId) Next() *VariableId {
	newV := &VariableId{
		parent: v,
	}
	v.children = append(v.children, newV)
	return newV
}
