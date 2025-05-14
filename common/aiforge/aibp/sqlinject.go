package aibp

import (
	_ "embed"

	"github.com/yaklang/yaklang/common/aiforge"
)

//go:embed sqlinject_prompts/init.txt
var sqlInjectInitPrompt string

//go:embed sqlinject_prompts/persistent.txt
var sqlInjectExecutePrompt string

func init() {
	cfg := aiforge.NewYakForgeBlueprintConfig("sqlinject", sqlInjectInitPrompt, sqlInjectExecutePrompt)
	cfg.WithToolKeywords("fs", "http")
	cfg.WithTools(
		"do_http",
		"grep",
		"read_file_chunk",
		"read_file_lines",
		"cybersecurity-risk",
	)
	aiforge.RegisterYakAiForge(cfg)
}
