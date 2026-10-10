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

// FromDatabase 从数据库中按程序名加载已编译的 SSA 程序（导出名为 ssa.NewFromProgramName）
// 参数:
//   - programName: 已保存的程序名
//
// 返回值:
//   - SSA 程序对象
//   - 错误信息
//
// Example:
// ```
// // 加载此前编译并保存的程序（示意性示例，需要数据库中已有该程序）
// prog = ssa.NewFromProgramName("my-program")~
// result = prog.SyntaxFlowWithError("sink* as $sink")~
// dump(result)
// ```
func FromDatabase(programName string) (p *Program, err error) {
	if prog, ok := ProgramCache.Get(programName); ok && prog != nil {
		closedWrite := prog.Program != nil &&
			prog.Program.DatabaseKind == ssa.ProgramCacheDBWrite &&
			prog.Program.Cache != nil &&
			prog.Program.Cache.IsClosed()
		staleIR := false
		var irProg *ssadb.IrProgram
		if !closedWrite {
			loaded, getErr := ssadb.GetProgram(programName, ssadb.Application)
			if getErr == nil && loaded != nil {
				irProg = loaded
				// Long-lived processes (CI yak grpc, yaklang engine) cache
				// FromDatabase results for 10m. Recompile in another process
				// deletes IR rows and writes a new ir_programs.updated_at;
				// returning the cached Program would scan deleted instruction
				// IDs and produce 0 matches.
				if prog.irProgram != nil && !loaded.UpdatedAt.Equal(prog.irProgram.UpdatedAt) {
					staleIR = true
				}
			}
		}
		if closedWrite || staleIR {
			ProgramCache.Remove(programName)
		} else {
			if irProg != nil {
				prog.irProgram = irProg
				attachOverlayIfNeeded(prog, irProg, make(map[string]bool))
			}
			return prog, nil
		}
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

// buildOverlayForRow 严格按行上记录的增量类型组装合并视图，失败返回错误：
//   - 行是 overlay 链头且层名单完整（HasSavedOverlayLayers）：按配方逐层加载，
//     在内存中聚合为 ProgramOverLay；
//   - 行是差量层但配方缺失/损坏：加载其 base，重建两层视图兜底；
//   - 其他（普通全量编译、增量基座层）：返回 nil，无需视图。
// 这是视图组装的唯一决策点：查询路径（attachOverlayIfNeeded）与编译路径
// （loadBaseOverlayForDiffCompile）共用本函数，只是失败策略不同。
func buildOverlayForRow(prog *Program, irProg *ssadb.IrProgram, visited map[string]bool) (*ProgramOverLay, error) {
	if prog == nil || irProg == nil {
		return nil, nil
	}
	if irProg.HasSavedOverlayLayers() {
		overlay, err := loadOverlayFromDatabase(irProg.OverlayLayers, visited)
		if err != nil {
			return nil, utils.Wrapf(err, "failed to load overlay from database: %s", irProg.ProgramName)
		}
		return overlay, nil
	}
	if irProg.IsIncrementalKind() && !irProg.IsBaseProgramKind() {
		baseProgram, err := fromDatabaseWithVisited(irProg.BaseProgramName, visited)
		if err != nil {
			return nil, utils.Wrapf(err, "failed to load base program %s for diff program %s",
				irProg.BaseProgramName, irProg.ProgramName)
		}
		overlay := NewProgramOverLay(baseProgram, prog)
		if overlay == nil {
			return nil, utils.Errorf("failed to create overlay for diff program %s with base %s",
				irProg.ProgramName, irProg.BaseProgramName)
		}
		return overlay, nil
	}
	return nil, nil
}

// attachOverlayIfNeeded 查询路径的挂载入口：组装失败只告警并降级为裸 program
// （查询范围缩小但不出错）。编译路径请改用 loadBaseOverlayForDiffCompile，
// 那里的失败会直接中断编译——两种策略刻意不同：查询可以缩小范围，
// diff 基准不完整却不能继续编译。
func attachOverlayIfNeeded(prog *Program, irProg *ssadb.IrProgram, visited map[string]bool) {
	if prog == nil || irProg == nil || prog.GetOverlay() != nil {
		return
	}
	overlay, err := buildOverlayForRow(prog, irProg, visited)
	if err != nil {
		log.Warnf("failed to attach overlay, degraded to bare program: %v", err)
		return
	}
	prog.overlay = overlay
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

	// 行上的增量类型决定是否挂载合并视图（配方聚合 / 两层兜底 / 无操作）
	attachOverlayIfNeeded(ret, irProg, visited)

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

// NewProgramFromDB 从数据库加载程序并返回 SyntaxFlowQueryInstance 接口（导出名为 ssa.NewProgramFromDB）
// 如果程序有 overlay（已保存的 overlay 或增量编译的 diff program），返回 *ProgramOverLay，否则返回 *Program
// 参数:
//   - programName: 已保存的程序名
//
// 返回值:
//   - 可执行 SyntaxFlow 查询的程序实例
//   - 错误信息
//
// Example:
// ```
// // 加载已保存的程序并执行查询（示意性示例，需要数据库中已有该程序）
// prog = ssa.NewProgramFromDB("my-program")~
// result = prog.SyntaxFlowWithError("sink* as $sink")~
// dump(result)
// ```
func NewProgramFromDB(programName string) (SyntaxFlowQueryInstance, error) {
	program, err := FromDatabase(programName)
	if err != nil {
		return nil, err
	}
	if program == nil {
		return nil, utils.Errorf("program %s is nil", programName)
	}
	return program.AsSyntaxFlowQueryInstance(), nil
}
