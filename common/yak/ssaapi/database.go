package ssaapi

import (
	"github.com/yaklang/yaklang/common/yak/ssa"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
)

func (c *config) fromDatabase() (*Program, error) {
	// get program from database
	// packages := ssadb.GetPackageFunction()

	// all function and instruction will be lazy
	db := ssadb.GetDB()
	ret := NewProgram(ssa.NewProgramFromDatabase(db, c.DatabaseProgramName), c)
	ret.comeFromDatabase = true
	return ret, nil
}
