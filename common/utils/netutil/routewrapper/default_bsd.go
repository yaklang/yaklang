//go:build darwin || freebsd
// +build darwin freebsd

package routewrapper

import (
	"os/exec"
)

func NewRouteWrapper() (Routing, error) {
	pathToNetstatCommand, err := exec.LookPath("netstat")
	if err != nil {
		return nil, err
	}
	pathToRouteCommand, err := exec.LookPath("route")
	if err != nil {
		return nil, err
	}
	return NewBSDRouteWrapper(pathToNetstatCommand, pathToRouteCommand)
}
