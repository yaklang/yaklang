package taskstack

import _ "embed"

//go:embed prompts/select-tool.txt
var __prompt_SELECT_TOOL string

//go:embed prompts/generate-tasklist.txt
var __prompt_GENERATE_TASKLIST string

//go:embed prompts/execute-tool.txt
var __prompt_EXECUTE_TOOL string
