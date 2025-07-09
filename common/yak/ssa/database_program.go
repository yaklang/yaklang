package ssa

import (
	"sync"

	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
)

func GetLibrary(program, version string) (*Program, error) {
	if p, err := ssadb.GetLibrary(program, version); err == nil {
		return NewProgramFromDB(p), nil
	} else {
		return nil, utils.Errorf("program %s have err: %v", program, err)
	}
}
func GetProgram(program string, kind ssadb.ProgramKind) (*Program, error) {
	// rebuild in database
	if p, err := ssadb.GetProgram(program, kind); err == nil {
		return NewProgramFromDB(p), nil
	} else {
		return nil, utils.Errorf("program %s have err: %v", program, err)
	}
}

func NewProgramFromDB(p *ssadb.IrProgram) *Program {
	prog := NewProgram(p.ProgramName, true, p.ProgramKind, nil, "", 0)
	prog.Cache.SaveToDatabase()
	prog.irProgram = p
	prog.Language = p.Language
	prog.FileList = p.FileList
	prog.ExtraFile = p.ExtraFile
	// TODO: handler up and down stream
	return prog
}

func (prog *Program) UpdateToDatabase() func() {
	wg := sync.WaitGroup{}
	wg.Add(1)
	go func() {
		defer wg.Done()
		ir := prog.irProgram
		if ir == nil {
			ir = ssadb.CreateProgram(prog.Name, prog.Version, prog.ProgramKind)
			prog.irProgram = ir
		}
		ir.Language = prog.Language
		ir.ProgramKind = prog.ProgramKind
		ir.ProgramName = prog.Name
		ir.Version = prog.Version
		ir.FileList = prog.FileList
		ir.ExtraFile = prog.ExtraFile
		ssadb.UpdateProgram(ir)
	}()
	return wg.Wait
}

func (p *Program) GetIrProgram() *ssadb.IrProgram {
	if p == nil || p.irProgram == nil {
		return nil
	}
	return p.irProgram
}
