package ssa

import "github.com/jinzhu/gorm"

type LazyInstruction struct {
	Instruction
	Value
	id int64
}

func NewLazyInstruction(db *gorm.DB, id int64) *LazyInstruction {
	return &LazyInstruction{
		id: id,
	}
}
