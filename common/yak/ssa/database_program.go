package ssa

import (
	"context"
	"fmt"
	"sync"

	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
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
	prog := NewProgram(context.Background(), p.ProgramName, ProgramCacheDBRead, p.ProgramKind, nil, "", 0)
	prog.irProgram = p
	prog.Language = ssaconfig.Language(p.Language)
	prog.FileList = p.FileList
	prog.LineCount = p.LineCount
	prog.ExtraFile = p.ExtraFile
	// 恢复增量编译信息（如果存在）
	if p.BaseProgramName != "" {
		prog.BaseProgramName = p.BaseProgramName
	}
	if len(p.FileHashMap) > 0 {
		// 将 StringMap 转换为 map[string]int
		prog.FileHashMap = make(map[string]int)
		for filePath, hashStr := range p.FileHashMap {
			var hash int
			if _, err := fmt.Sscanf(hashStr, "%d", &hash); err == nil {
				prog.FileHashMap[filePath] = hash
			}
		}
	}
	// TODO: handler up and down stream
	return prog
}

func (prog *Program) UpdateToDatabase() func() {
	wg := &sync.WaitGroup{}
	prog.UpdateToDatabaseWithWG(wg)
	return wg.Wait
}

func (prog *Program) UpdateToDatabaseWithWG(wg *sync.WaitGroup) {
	wg.Add(1)
	go func() {
		defer wg.Done()
		ir := prog.irProgram
		if ir == nil {
			existingIr, err := ssadb.GetProgram(prog.Name, prog.ProgramKind)
			if err == nil && existingIr != nil {
				ir = existingIr
				prog.irProgram = ir
			} else {
				ir = ssadb.CreateProgram(prog.Name, prog.Version, prog.ProgramKind)
				prog.irProgram = ir
			}
		}
		ir.Language = prog.Language
		ir.ProgramKind = prog.ProgramKind
		ir.ProgramName = prog.Name
		ir.Version = prog.Version
		ir.ProjectID = prog.ProjectID
		ir.FileList = prog.FileList
		ir.LineCount = prog.LineCount
		ir.ExtraFile = prog.ExtraFile
		// 同步增量编译信息（如果存在）
		if prog.BaseProgramName != "" {
			ir.BaseProgramName = prog.BaseProgramName
		}
		if len(prog.FileHashMap) > 0 {
			// 将 fileHashMap 转换为 StringMap 格式（int -> string）
			fileHashMapStr := make(ssadb.StringMap)
			for filePath, hash := range prog.FileHashMap {
				fileHashMapStr[filePath] = fmt.Sprintf("%d", hash)
			}
			ir.FileHashMap = fileHashMapStr
		}
		// 如果启用了增量编译（有 BaseProgramName 或 FileHashMap 不为 nil），设置 IsOverlay = true
		// FileHashMap 不为 nil 表示启用了增量编译（即使为空 map，也表示这是增量编译流程的一部分）
		if prog.BaseProgramName != "" || prog.FileHashMap != nil {
			ir.IsOverlay = true
		}
		ssadb.UpdateProgram(ir)
	}()
}

func (p *Program) GetIrProgram() *ssadb.IrProgram {
	if p == nil || p.irProgram == nil {
		return nil
	}
	return p.irProgram
}
