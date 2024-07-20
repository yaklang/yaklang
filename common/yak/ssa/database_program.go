package ssa

import (
	"github.com/samber/lo"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
)

// var programCachePool = omap.NewEmptyOrderedMap[string, *Program]()

// func setProgramCachePool(program string, prog *Program) {
// 	programCachePool.Set(program, prog)
// }

// func deleteProgramCachePool(program string) {
// 	programCachePool.Delete(program)
// }

func GetProgram(program string, kind ProgramKind) (*Program, error) {
	// rebuild in database
	p, err := ssadb.GetProgram(program, string(kind))
	if err != nil {
		return nil, utils.Errorf("program %s have err: %v", program, err)
	}
	prog := NewProgramFromDB(p)
	return prog, nil
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

func updateToDatabase(prog *Program) {
	ir := prog.irProgram
	if ir == nil {
		ir = ssadb.CreateProgram(prog.Name, string(prog.ProgramKind), prog.Version)
	}
	ir.Language = prog.Language
	ir.ProgramKind = string(prog.ProgramKind)
	ir.ProgramName = prog.Name
	ir.Version = prog.Version
	ir.UpStream = append(ir.UpStream, lo.Keys(prog.UpStream)...)
	ir.DownStream = append(ir.DownStream, lo.Keys(prog.DownStream)...)
	ir.FileList = prog.FileList
	ir.ExtraFile = prog.ExtraFile
	ssadb.UpdateProgram(ir)
}
