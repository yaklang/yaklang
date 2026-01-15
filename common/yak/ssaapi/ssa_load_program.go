package ssaapi

import (
	"time"

	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/ssa"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
)

var ProgramCache = utils.NewLRUCache[*Program](10)

func SetProgramCache(program *Program, ttls ...time.Duration) {
	ttl := 10 * time.Minute
	if len(ttls) > 0 {
		ttl = ttls[0]
	}
	ProgramCache.SetWithTTL(program.GetProgramName(), program, ttl)
}

func FromDatabase(programName string) (p *Program, err error) {
	if prog, ok := ProgramCache.Get(programName); ok && prog != nil {
		irProg, err := ssadb.GetProgram(programName, ssadb.Application)
		if err == nil && irProg != nil && irProg.IsOverlay && len(irProg.OverlayLayers) > 0 {
			if prog.GetOverlay() == nil {
				overlay, err := loadOverlayFromDatabase(irProg.OverlayLayers, make(map[string]bool))
				if err != nil {
					log.Warnf("failed to load overlay from cache: %v", err)
				} else {
					prog.overlay = overlay
				}
			}
		}
		return prog, nil
	}
	defer func() {
		if err != nil {
			return
		}
		if p != nil {
			SetProgramCache(p)
		}
	}()

	return fromDatabase(programName)
}

func fromDatabase(name string) (*Program, error) {
	return fromDatabaseWithVisited(name, make(map[string]bool))
}

func fromDatabaseWithVisited(name string, visited map[string]bool) (*Program, error) {
	if visited[name] {
		prog, err := ssa.GetProgram(name, ssa.Application)
		if err != nil {
			return nil, err
		}
		ret := NewProgram(prog, nil)
		ret.comeFromDatabase = true
		ret.enableDatabase = true
		ret.irProgram = prog.GetIrProgram()
		return ret, nil
	}

	visited[name] = true

	irProg, err := ssadb.GetProgram(name, ssadb.Application)
	if err != nil {
		return nil, err
	}

	prog, err := ssa.GetProgram(name, ssa.Application)
	if err != nil {
		return nil, err
	}

	ret := NewProgram(prog, nil)
	ret.comeFromDatabase = true
	ret.enableDatabase = true
	ret.irProgram = irProg

	// 如果这是一个 overlay（已保存的 overlay），直接加载
	if irProg != nil && irProg.IsOverlay && len(irProg.OverlayLayers) > 0 {
		overlay, err := loadOverlayFromDatabase(irProg.OverlayLayers, visited)
		if err != nil {
			log.Warnf("failed to load overlay from database: %v", err)
		} else {
			ret.overlay = overlay
		}
		return ret, nil
	}

	// 如果这是一个差量 program（增量编译但不是 base program），需要聚合生成 ProgramOverLay
	// 问题1：当一个 program 被从数据库中拿出来时，如果它是一个差量的，就必须要聚合生成 ProgramOverLay
	if ret.IsIncrementalCompile() && !ret.IsBaseProgram() {
		// 加载 base program
		baseProgramName := ret.GetBaseProgramName()
		baseProgram, err := fromDatabaseWithVisited(baseProgramName, visited)
		if err != nil {
			log.Warnf("failed to load base program %s for diff program %s: %v", baseProgramName, name, err)
			// 如果加载失败，仍然返回当前 program，但不设置 overlay
			return ret, nil
		}

		// 创建 ProgramOverLay：base program 作为 Layer1，当前 diff program 作为 Layer2
		overlay := NewProgramOverLay(baseProgram, ret)
		if overlay == nil {
			log.Warnf("failed to create overlay for diff program %s with base %s", name, prog.BaseProgramName)
		} else {
			ret.overlay = overlay
		}
	}

	return ret, nil
}

func loadOverlayFromDatabase(layerNames []string, visited map[string]bool) (*ProgramOverLay, error) {
	if len(layerNames) < 2 {
		return nil, utils.Errorf("overlay requires at least 2 layers, got %d", len(layerNames))
	}

	if visited == nil {
		visited = make(map[string]bool)
	}

	layerPrograms := make([]*Program, 0, len(layerNames))
	for _, layerName := range layerNames {
		if layerName == "" {
			continue
		}
		layerProg, err := fromDatabaseWithVisited(layerName, visited)
		if err != nil {
			return nil, utils.Wrapf(err, "failed to load layer program: %s", layerName)
		}
		layerPrograms = append(layerPrograms, layerProg)
	}

	if len(layerPrograms) < 2 {
		return nil, utils.Errorf("failed to load enough layer programs: expected at least 2, got %d", len(layerPrograms))
	}

	overlay := NewProgramOverLay(layerPrograms...)
	if overlay == nil {
		return nil, utils.Errorf("failed to create overlay from layer programs")
	}

	return overlay, nil
}

func fromDatabaseIRProgram(irprog *ssadb.IrProgram) (*Program, error) {
	prog := ssa.NewProgramFromDB(irprog)
	ret := NewProgram(prog, nil)
	ret.comeFromDatabase = true
	ret.enableDatabase = true
	ret.irProgram = irprog
	return ret, nil
}

func LoadProgramRegexp(match string) []*Program {
	programs := []*Program{}

	var irprogram []*ssadb.IrProgram
	ssadb.GetDB().Model(&ssadb.IrProgram{}).
		Where("program_name REGEXP ?  OR program_name = ? ", match, match).
		Where("program_kind = ?", "application").
		Find(&irprogram)

	for _, irp := range irprogram {
		p, err := fromDatabaseIRProgram(irp)
		if err != nil {
			log.Errorf("load program %s from database fail: %v", irp.ProgramName, err)
			continue
		}
		programs = append(programs, p)
	}

	return programs
}
