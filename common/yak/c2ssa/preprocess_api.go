package c2ssa

import (
	fi "github.com/yaklang/yaklang/common/utils/filesys/filesys_interface"
	"github.com/yaklang/yaklang/common/yak/c2ssa/preprocess"
)

type (
	MacroTables        = preprocess.MacroTables
	FunctionMacro      = preprocess.FunctionMacro
	PreprocessConfig   = preprocess.PreprocessConfig
	CPreprocessProject = preprocess.CPreprocessProject
)

// DefaultPreprocessConfig returns sensible defaults for project preprocessing.
func DefaultPreprocessConfig() PreprocessConfig {
	return preprocess.DefaultConfig()
}

// BuildCPreprocessProject constructs a preprocessor project from fs and config.
func BuildCPreprocessProject(fs fi.FileSystem, config PreprocessConfig) *CPreprocessProject {
	return preprocess.BuildProject(fs, config)
}

// PreprocessCSource preprocesses a single source string without filesystem include context.
func PreprocessCSource(src string) (string, error) {
	return preprocess.ExpandFunctionMacros(src)
}

// ExpandFunctionMacros expands function-like and object-like macros in src.
func ExpandFunctionMacros(src string) (string, error) {
	return preprocess.ExpandFunctionMacros(src)
}
