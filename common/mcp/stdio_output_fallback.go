//go:build !darwin && !linux && !windows

package mcp

import (
	"os"
	"sync"
)

func reserveMCPProtocolStdout() (*os.File, func() error, error) {
	protocolStdout := os.Stdout
	os.Stdout = os.Stderr

	var once sync.Once
	restore := func() error {
		once.Do(func() {
			os.Stdout = protocolStdout
		})
		return nil
	}
	return protocolStdout, restore, nil
}
