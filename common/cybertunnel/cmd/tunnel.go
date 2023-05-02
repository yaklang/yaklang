package main

import (
	"os"
	"github.com/yaklang/yaklang/common/cybertunnel"
	"github.com/yaklang/yaklang/common/log"
)

func main() {
	err := cybertunnel.GetTunnelServerCommandCli().Run(os.Args)
	if err != nil {
		log.Errorf("cybertunnel failed: %s", err)
		return
	}
}
