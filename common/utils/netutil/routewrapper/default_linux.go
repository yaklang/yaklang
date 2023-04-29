//go:build linux
// +build linux

package routewrapper

import (
	"os/exec"
)

func NewRouteWrapper() (Routing, error) {
	pathToIpCommand, err := exec.LookPath("ip")
	if err != nil {
		return nil, err
	}
	return NewLinuxRouteWrapper(pathToIpCommand)
}
