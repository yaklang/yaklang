package ssa

import (
	"github.com/jinzhu/gorm"
)

func NewProgramFromDatabase(db *gorm.DB, program string) *Program {
	prog := &Program{
		Name:     "",
		Packages: map[string]*Package{},
	}

	prog.Cache = NewDBCache(program)

	return prog
}
