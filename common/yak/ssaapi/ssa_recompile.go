package ssaapi

import "github.com/yaklang/yaklang/common/utils"

func (prog *Program) Recompile(opts ...Option) error {
	// get file system
	hasFS := false
	// recompile from info
	if prog.ssaProgram != nil && prog.ssaProgram.ConfigInput != "" {
		configInfo := prog.ssaProgram.ConfigInput
		opts = append(opts, WithConfigInfoRaw(configInfo))
		hasFS = true
	}
	// recompile from database
	if !hasFS {
		// handler
	}

	// check file system
	if !hasFS {
		return utils.Errorf("该项目编译时引擎版本过旧，无法重新编译。")
		// return utils.Errorf("The project compilation engine version is too old to recompile.\n该项目编译时引擎版本过旧，无法重新编译。")
	}

	// append other options
	opts = append(opts, WithProgramName(prog.Program.Name))
	opts = append(opts, WithSaveToProfile())
	opts = append(opts, WithReCompile(true))

	// parse
	newProg, err := ParseProject(opts...)
	_ = newProg

	return err
}
