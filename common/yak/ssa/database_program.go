package ssa

import (
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/omap"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
)

var programCachePool = omap.NewEmptyOrderedMap[string, *Program]()

func setProgramCachePool(program string, prog *Program) {
	programCachePool.Set(program, prog)
}

func deleteProgramCachePool(program string) {
	programCachePool.Delete(program)
}

func GetProgram(program string, kind ProgramKind) (*Program, error) {
	// check in memory
	if prog, ok := programCachePool.Get(program); ok {
		return prog, nil
	}

	// rebuild in database
	p, err := ssadb.GetProgram(program, string(kind))
	if err != nil {
		return nil, utils.Errorf("program %s have err: %v", program, err)
	}
	prog := NewProgramFromDB(p)
	return prog, nil
}
