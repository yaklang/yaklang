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
		return utils.Errorf("no config info found")
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
