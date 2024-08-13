package ssaapi

import (
	"github.com/yaklang/yaklang/common/yak/ssa"
)

func (c *config) fromDatabase() (*Program, error) {
	// get program from database
	// packages := ssadb.GetPackageFunction()
	prog, err := ssa.GetProgram(c.ProgramName, ssa.Application)
	if err != nil {
		return nil, err
	}

	// all function and instruction will be lazy
	ret := NewProgram(prog, c)
	ret.comeFromDatabase = true
	return ret, nil
}
