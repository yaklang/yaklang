package ssa

import (
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
func GetProgram(program string, kind ProgramKind) (*Program, error) {
	// rebuild in database
	if p, err := ssadb.GetProgram(program, string(kind)); err == nil {
		return NewProgramFromDB(p), nil
	} else {
		return nil, utils.Errorf("program %s have err: %v", program, err)
	}
}

func NewProgramFromDB(p *ssadb.IrProgram) *Program {
	prog := NewProgram(p.ProgramName, true, ProgramKind(p.ProgramKind), nil, "")
	prog.irProgram = p
	prog.Language = p.Language
	prog.FileList = p.FileList
	prog.ExtraFile = p.ExtraFile
	// TODO: handler up and down stream
	return prog
}

func (prog *Program) UpdateToDatabase() {
	ir := prog.irProgram
	if ir == nil {
		ir = ssadb.CreateProgram(prog.Name, string(prog.ProgramKind), prog.Version)
		prog.irProgram = ir
	}
	ir.Language = prog.Language
	ir.ProgramKind = string(prog.ProgramKind)
	ir.ProgramName = prog.Name
	ir.Version = prog.Version
	ir.FileList = prog.FileList
	ir.ExtraFile = prog.ExtraFile
	ssadb.UpdateProgram(ir)
}
