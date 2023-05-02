package main

import (
	"os"
	"yaklang.io/yaklang/common/cybertunnel"
	"yaklang.io/yaklang/common/log"
)

func main() {
	err := cybertunnel.GetTunnelServerCommandCli().Run(os.Args)
	if err != nil {
		log.Errorf("cybertunnel failed: %s", err)
		return
	}
}
