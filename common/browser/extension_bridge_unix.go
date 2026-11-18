//go:build darwin || linux

package browser

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"syscall"
)

func listenExtensionBridgeUnix(name string) (string, net.Listener, error) {
	// Keep the address independent of the home directory length. Darwin's
	// sockaddr_un has room for at most 103 bytes and a terminating NUL.
	directory := fmt.Sprintf("/tmp/yak-be-%d", os.Geteuid())
	if err := os.Mkdir(directory, 0700); err != nil && !os.IsExist(err) {
		return "", nil, err
	}
	info, err := os.Lstat(directory)
	if err != nil {
		return "", nil, err
	}
	owner, ok := info.Sys().(*syscall.Stat_t)
	if !ok || owner.Uid != uint32(os.Geteuid()) || !info.IsDir() || info.Mode().Perm() != 0700 {
		return "", nil, fmt.Errorf("%s must be a private directory owned by the current user", directory)
	}
	// Each engine generates a new random instance ID. Existing sockets are
	// never reclaimed by another instance.
	address := filepath.Join(directory, name)
	listener, err := net.Listen("unix", address)
	if err != nil {
		return "", nil, err
	}
	if err := os.Chmod(address, 0600); err != nil {
		listener.Close()
		return "", nil, err
	}
	return directory, listener, nil
}
