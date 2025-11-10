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
	irProg, err := ssadb.GetProgram(c.GetProgramName(), ssa.Application)
	if err != nil {
		log.Errorf("irProg is nil, save config failed: %v", err)
		return
	}
	irProg.Description = c.GetProjectDescription()
	irProg.Language = c.GetLanguage()
	irProg.EngineVersion = consts.GetYakVersion()
	irProg.ConfigInput = c.JSON()
	irProg.PeepholeSize = c.GetCompilePeepholeSize()
	ssadb.UpdateProgram(irProg)
}

// recompile from Profile SSAProgram
func (prog *Program) Recompile(opts ...ssaconfig.Option) error {
	// get file system
	hasFS := false
	// recompile from info
	if prog.irProgram != nil {
		if configInfo := prog.irProgram.ConfigInput; configInfo != "" {
			opts = append(opts, ssaconfig.WithConfigJson(configInfo))
			hasFS = true
		}
		opts = append(opts, WithPeepholeSize(prog.irProgram.PeepholeSize))
	}
	//TODO: recompile from database

	// check file system
	if !hasFS {
		return utils.Errorf("该项目编译时引擎版本过旧，无法重新编译。")
		// return utils.Errorf("The project compilation engine version is too old to recompile.\n该项目编译时引擎版本过旧，无法重新编译。")
	}

	// append other options
	opts = append(opts, WithProgramName(prog.Program.Name))
	opts = append(opts, WithLanguage(prog.GetLanguage()))
	opts = append(opts, WithReCompile(true))

	// parse
	newProg, err := ParseProject(opts...)
	_ = newProg

	return err
}
