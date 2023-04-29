//go:build windows
// +build windows

package routewrapper

import (
	"os/exec"
)

func NewRouteWrapper() (Routing, error) {
	pathToRouteCommand, err := exec.LookPath("route")
	if err != nil {
		return nil, err
	}
	return NewWindowsRouteWrapper(pathToRouteCommand)
}
