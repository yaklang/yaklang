package harness

import (
	"fmt"

	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
)

// CompileProject compiles a source code directory into SSA IR and persists it to database.
// Returns the program name that can be used by AI Agent's SSA tools.
func CompileProject(projectPath, language, programName string) (string, error) {
	if programName == "" {
		programName = fmt.Sprintf("eval-%s", language)
	}

	lang, err := ssaconfig.ValidateLanguage(language)
	if err != nil {
		return "", fmt.Errorf("invalid language %q: %w", language, err)
	}

	opts := []ssaconfig.Option{
		ssaapi.WithLanguage(lang),
		ssaapi.WithProgramName(programName),
		ssaapi.WithLocalFs(projectPath),
	}

	prog, err := ssaapi.ParseProject(opts...)
	if err != nil {
		return "", fmt.Errorf("SSA compile failed: %w", err)
	}
	if prog == nil {
		return "", fmt.Errorf("SSA compile returned nil program")
	}

	return programName, nil
}
