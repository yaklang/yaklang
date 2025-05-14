package aibp

import (
	_ "embed"

	"github.com/yaklang/yaklang/common/aiforge"
)

//go:embed xss_prompts/init.txt
var xssInitPrompt string

//go:embed xss_prompts/persistent.txt
var xssPersistentPrompt string

func init() {
	cfg := aiforge.NewYakForgeBlueprintConfig("xss", xssInitPrompt, xssPersistentPrompt)
	cfg.WithToolKeywords("fs", "http")
	cfg.WithTools(
		"do_http",
		"http",
		"packet",
		"grep",
		"read_file_chunk",
		"read_file_lines",
		"cybersecurity-risk",
	)
	aiforge.RegisterYakAiForge(cfg)
}
