package ssaapi

import (
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/yak/ssa"
)

func (c *config) fromDatabase() (*Program, error) {
	// get program from database
	// packages := ssadb.GetPackageFunction()

	// all function and instruction will be lazy
	db := consts.GetGormProjectDatabase()
	ret := NewProgram(ssa.NewProgramFromDatabase(db, c.DatabaseProgramName), c)
	ret.comeFromDatabase = true
	return ret, nil
}
