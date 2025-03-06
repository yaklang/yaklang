package ssaapi

import (
	"github.com/yaklang/yaklang/common/yak/ssa"
)

// FromDatabase get program from database by program name
func FromDatabase(programName string) (*Program, error) {
	config, err := defaultConfig(WithProgramName(programName))
	if err != nil {
		return nil, err
	}
	return config.fromDatabase()
}

func (c *config) fromDatabase() (*Program, error) {
	// get program from database
	prog, err := ssa.GetProgram(c.ProgramName, ssa.Application)
	if err != nil {
		return nil, err
	}

	// all function and instruction will be lazy
	ret := NewProgram(prog, c)
	ret.comeFromDatabase = true
	ret.irProgram = prog.GetIrProgram()
	return ret, nil
}
