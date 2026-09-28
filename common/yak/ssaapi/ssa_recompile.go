package ssaapi

import (
	"time"

	"github.com/yaklang/yaklang/common/consts"

	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/ssa"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
	"github.com/yaklang/yaklang/common/yak/ssaproject"
)

// save to Profile SSAProgram
func SaveConfig(c *Config, prog *Program) {
	if c.databaseKind == ssa.ProgramCacheMemory || c.EnableCache {
		if c.GetProgramName() != "" {
			log.Errorf("Compile program cache to memory: %s", c.GetProgramName())
			SetProgramCache(prog)
		}
		return
	}
	irProg, err := ssadb.GetProgram(c.GetLatestProgramName(), ssa.Application)
	if err != nil {
		log.Errorf("irProg is nil, save config failed: %v", err)
		return
	}
	irProg.Description = c.GetProjectDescription()
	irProg.Language = c.GetLanguage()
	irProg.EngineVersion = consts.GetYakVersion()
	irProg.ConfigInput = c.JSON()
	irProg.PeepholeSize = c.GetCompilePeepholeSize()
	// 如果启用了增量编译，设置 IsOverlay = true
	if c.GetEnableIncrementalCompile() {
		irProg.IsOverlay = true
	}
	if err := ssadb.UpdateProgramWithError(irProg); err != nil {
		log.Errorf("save config update program failed: name=%s err=%v", irProg.ProgramName, err)
	}
}

// recompileProgramLayer 对单个 program 层执行重编译（使用该层自己的 ConfigInput，不会触发 overlay 其它层）。
func recompileProgramLayer(prog *Program, inputOpt ...ssaconfig.Option) error {
	if prog == nil {
		return utils.Error("program is nil")
	}
	opt := make([]ssaconfig.Option, 0)
	// get file system
	hasFS := false
	// recompile from info
	if prog.irProgram != nil {
		if configInfo := prog.irProgram.ConfigInput; configInfo != "" {
			opt = append(opt, ssaconfig.WithConfigJson(configInfo)) // this json as first option
			hasFS = true
		}
		opt = append(opt, WithPeepholeSize(prog.irProgram.PeepholeSize))
	}
	//TODO: recompile from database

	// check file system
	if !hasFS {
		return utils.Errorf("该项目编译时引擎版本过旧，无法重新编译。")
		// return utils.Errorf("The project compilation engine version is too old to recompile.\n该项目编译时引擎版本过旧，无法重新编译。")
	}

	layerName := prog.GetProgramName()
	if layerName == "" && prog.Program != nil {
		layerName = prog.Program.Name
	}

	// 检测是否是增量编译的 program
	// 如果当前 program 是增量编译的，重编译时应该自动启用增量编译，使用当前层 program 作为 base program
	if prog.IsIncrementalCompile() {
		log.Infof("检测到增量编译 program，自动启用增量编译，base program: %s", layerName)
		opt = append(opt, WithBaseProgramName(layerName))
		// 增量编译时，不设置 WithProgramName，让调用者通过 inputOpt 传入新的 program name
		// 这样可以确保每次重新编译都会创建一个新的 diff program，而不是覆盖现有的
	} else if projectConfigEnablesIncrementalCompile(prog) {
		// 非增量 program，但所属项目当前配置已启用增量编译（例如首次全量编译后才开启该选项）：
		// 以当前 program 为 base 做增量重编译，新 diff program 会被标记 IsOverlay，
		// 之后的重编译将自动走上面的增量分支
		log.Infof("项目配置已启用增量编译，program %s 转为增量重编译，base program: %s", layerName, layerName)
		opt = append(opt, WithBaseProgramName(layerName))
	} else {
		// 非增量编译时，使用相同的 program name（重新编译会覆盖）
		opt = append(opt, WithProgramName(layerName))
	}

	// append other options
	opt = append(opt, WithLanguage(prog.GetLanguage()))
	opt = append(opt, WithReCompile(true))
	// 增量重编译会创建新的 diff program：若调用者未显式指定新程序名，
	// 自动生成时间戳程序名（与"SSA 项目编译"插件一致），避免复用
	// ConfigInput 中的旧 program 名导致与 base/历史 program 同名冲突。
	if hasBaseProgramName(opt) && !hasProgramNameOption(inputOpt) {
		opt = append(opt, WithSetProgramName(layerName+"("+time.Now().Format("2006-01-02 15:04:05")+")"))
	}
	opt = append(opt, inputOpt...)

	// parse
	newProg, err := ParseProject(opt...)
	_ = newProg

	return err
}

// projectConfigEnablesIncrementalCompile 通过 program 的 project_id 读取所属 SSA 项目的
// 当前配置，判断项目是否启用了增量编译。
// 用于支持"首次全量编译后，才在项目中开启增量编译"的场景：重编译时应转为
// 以当前 program 为 base 的增量编译，而不是继续同名全量覆盖。
func projectConfigEnablesIncrementalCompile(prog *Program) bool {
	if prog == nil || prog.irProgram == nil || prog.irProgram.ProjectID == 0 {
		return false
	}
	project, err := ssaproject.LoadSSAProjectByID(uint(prog.irProgram.ProjectID))
	if err != nil || project == nil {
		return false
	}
	config, err := project.GetConfig()
	if err != nil || config == nil {
		return false
	}
	return config.GetEnableIncrementalCompile()
}

// hasBaseProgramName 检查 options 中是否设置了 WithBaseProgramName
// （在临时 config 上重放 options，检查副作用）。
func hasBaseProgramName(opts []ssaconfig.Option) bool {
	cfg := &ssaconfig.Config{Mode: ssaconfig.ModeAll}
	for _, o := range opts {
		if o == nil {
			continue
		}
		_ = o(cfg)
	}
	return cfg.GetBaseProgramName() != ""
}

// hasProgramNameOption 检查调用者 inputOpt 中是否显式传入了新的 program 名
// （ssaconfig.WithSetProgramName / ssaapi.WithProgramName）。
func hasProgramNameOption(inputOpt []ssaconfig.Option) bool {
	for _, o := range inputOpt {
		if o == nil {
			continue
		}
		cfg := &ssaconfig.Config{Mode: ssaconfig.ModeAll}
		_ = o(cfg)
		if cfg.GetLatestProgramName() != "" {
			return true
		}
	}
	return false
}

// 已弃用
// recompile from Profile SSAProgram
func (prog *Program) Recompile(inputOpt ...ssaconfig.Option) error {
	return recompileProgramLayer(prog, inputOpt...)
}

// Recompile 仅重编译 overlay 的当前层（最上层），不会重编译底层/父层 program。
func (o *ProgramOverLay) Recompile(inputOpt ...ssaconfig.Option) error {
	layerProg := o.topProgram()
	if layerProg == nil {
		return utils.Error("overlay program has no current layer")
	}
	return recompileProgramLayer(layerProg, inputOpt...)
}
