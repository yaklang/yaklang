package ssaapi

import (
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
)

// save to Profile SSAProgram
func (c *config) SaveProfile() {
	if c.toProfile {
		prog := &schema.SSAProgram{
			Name:          c.ProgramName,
			Description:   c.ProgramDescription,
			DBPath:        consts.GetSSADataBasePathDefault(consts.GetDefaultYakitBaseDir()),
			Language:      string(c.language),
			EngineVersion: consts.GetYakVersion(),
			ConfigInput:   c.info,
			PeepholeSize:  c.peepholeSize,
		}
		ssadb.SaveSSAProgram(prog)
	}
}

// recompile from Profile SSAProgram
func (prog *Program) Recompile(opts ...Option) error {
	// get file system
	hasFS := false
	// recompile from info
	if prog.ssaProgram != nil {
		if prog.ssaProgram.ConfigInput != "" {
			configInfo := prog.ssaProgram.ConfigInput
			opts = append(opts, WithConfigInfoRaw(configInfo))
			hasFS = true
		}
		opts = append(opts, WithPeepholeSize(prog.ssaProgram.PeepholeSize))
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
	opts = append(opts, WithSaveToProfile())
	opts = append(opts, WithReCompile(true))

	// parse
	newProg, err := ParseProject(opts...)
	_ = newProg

	return err
}
