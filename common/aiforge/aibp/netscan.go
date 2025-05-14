package aibp

import (
	_ "embed"

	"github.com/yaklang/yaklang/common/aiforge"
)

//go:embed netscan/init.txt
var netscanInitPrompt string

//go:embed netscan/persistent.txt
var netscanPersistentPrompt string

func init() {
	cfg := aiforge.NewYakForgeBlueprintConfig("netscan", netscanInitPrompt, netscanPersistentPrompt)
	cfg.WithTools("http", "pentest", "net", "fs", "dns", "codec", "risk", "tls")
	aiforge.RegisterYakAiForge(cfg)
}
