//go:build !darwin && !linux

package browser

import (
	"errors"
	"net"
)

func listenExtensionBridgeUnix(string) (string, net.Listener, error) {
	return "", nil, errors.New("browser bridge Unix sockets require macOS or Linux")
}
