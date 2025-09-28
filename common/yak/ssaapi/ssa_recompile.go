package ssaapi

import (
	"github.com/yaklang/yaklang/common/consts"

	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/ssa"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
)

// save to Profile SSAProgram
func SaveConfig(c *Config, prog *Program) {
	if c.databaseKind != ssa.ProgramCacheMemory {
		irProg, err := ssadb.GetProgram(c.ProgramName, ssa.Application)
		if err != nil {
			log.Errorf("irProg is nil, save config failed: %v", err)
			return
		}
		irProg.Description = c.ProgramDescription
		irProg.Language = string(c.language)
		irProg.EngineVersion = consts.GetYakVersion()
		irProg.ConfigInput = c.info
		irProg.PeepholeSize = c.GetCompilePeepholeSize()
		ssadb.UpdateProgram(irProg)
	} else {
		// only memory
		if c.ProgramName != "" {
			SetProgramCache(prog, c.programSaveTTL)
		}
	}
}

// recompile from Profile SSAProgram
func (prog *Program) Recompile(opts ...Option) error {
	// get file system
	hasFS := false
	// recompile from info
	if prog.irProgram != nil {
		if configInfo := prog.irProgram.ConfigInput; configInfo != "" {
			opts = append(opts, WithConfigInfoRaw(configInfo))
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
	opts = append(opts, WithRawLanguage(prog.GetLanguage()))
	opts = append(opts, WithReCompile(true))

	// parse
	newProg, err := ParseProject(opts...)
	_ = newProg

	return err
}
