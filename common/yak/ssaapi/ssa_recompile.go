package ssaapi

import (
	"github.com/yaklang/yaklang/common/consts"

	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/ssa"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
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
	ssadb.UpdateProgram(irProg)
}

// 已弃用
// recompile from Profile SSAProgram
func (prog *Program) Recompile(inputOpt ...ssaconfig.Option) error {
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

	// 检测是否是增量编译的 program
	// 如果当前 program 是增量编译的，重编译时应该自动启用增量编译，使用当前 program 作为 base program
	if prog.IsIncrementalCompile() {
		log.Infof("检测到增量编译 program，自动启用增量编译，base program: %s", prog.Program.Name)
		opt = append(opt, WithBaseProgramName(prog.Program.Name))
		// 增量编译时，不设置 WithProgramName，让调用者通过 inputOpt 传入新的 program name
		// 这样可以确保每次重新编译都会创建一个新的 diff program，而不是覆盖现有的
	} else {
		// 非增量编译时，使用相同的 program name（重新编译会覆盖）
		opt = append(opt, WithProgramName(prog.Program.Name))
	}

	// append other options
	opt = append(opt, WithLanguage(prog.GetLanguage()))
	opt = append(opt, WithReCompile(true))
	opt = append(opt, inputOpt...)

	// parse
	newProg, err := ParseProject(opt...)
	_ = newProg

	return err
}
