package main

import (
	"github.com/yaklang/yaklang/common/cybertunnel"
	"github.com/yaklang/yaklang/common/log"
	"os"
)

func main() {
	err := cybertunnel.GetTunnelServerCommandCli().Run(os.Args)
	if err != nil {
		log.Errorf("cybertunnel failed: %s", err)
		return
	}
}
