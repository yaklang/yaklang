package ssaapi

import (
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/yak/ssa"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
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
	// packages := ssadb.GetPackageFunction()
	ssaProg := ssadb.CheckAndSwitchDB(c.ProgramName)
	if ssaProg == nil {
		log.Info("Program not found in profile database")
	}

	prog, err := ssa.GetProgram(c.ProgramName, ssa.Application)
	if err != nil {
		return nil, err
	}

	// all function and instruction will be lazy
	ret := NewProgram(prog, c)
	ret.comeFromDatabase = true
	ret.ssaProgram = ssaProg
	return ret, nil
}
