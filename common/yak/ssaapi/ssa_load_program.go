package ssaapi

import (
	"time"

	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/ssa"
)

var ProgramCache = utils.NewLRUCache[*Program](10)

func SetProgramCache(program *Program, ttls ...time.Duration) {
	ttl := 10 * time.Minute
	if len(ttls) > 0 {
		ttl = ttls[0]
	}
	ProgramCache.SetWithTTL(program.GetProgramName(), program, ttl)
}

// FromDatabase get program from database by program name
func FromDatabase(programName string) (p *Program, err error) {

	if prog, ok := ProgramCache.Get(programName); ok && prog != nil {
		return prog, nil
	}
	defer func() {
		if err != nil {
			return
		}
		SetProgramCache(p)
	}()

	config, err := DefaultConfig(WithProgramName(programName))
	if err != nil {
		return nil, err
	}
	return config.fromDatabase()
}

func (c *Config) fromDatabase() (*Program, error) {
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
