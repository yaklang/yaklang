package utils

import (
	"fmt"
)

type VariableId struct {
	parent *VariableId
}

func NewRootVariableId() *VariableId {
	return &VariableId{}
}

//	func (v *VariableId) Uid() string {
//		return fmt.Sprintf("var%d", v.Uid())
//	}
func (v *VariableId) Id() int {
	if v.parent == nil {
		return 0
	}
	return v.parent.Id() + 1
}
func (v *VariableId) String() string {
	return fmt.Sprintf("var%d", v.Id())
}

func (v *VariableId) Next() *VariableId {
	newV := &VariableId{
		parent: v,
	}
	return newV
}
