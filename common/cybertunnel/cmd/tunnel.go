package main

import (
	"os"
	"yaklang/common/cybertunnel"
	"yaklang/common/log"
)

func main() {
	err := cybertunnel.GetTunnelServerCommandCli().Run(os.Args)
	if err != nil {
		log.Errorf("cybertunnel failed: %s", err)
		return
	}
}
