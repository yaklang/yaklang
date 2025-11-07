//go:build !windows
// +build !windows

/* SPDX-License-Identifier: MIT
 *
 * yaklang.io modified
 */

package lowtun

import (
	"net"
	"os"

	"github.com/yaklang/yaklang/common/utils"
)

// ListenSocket creates a Unix domain socket listener at the specified path.
// It will automatically remove any existing socket file before listening.
func ListenSocket(socketPath string) (net.Listener, error) {
	// Remove existing socket file if it exists
	if err := os.RemoveAll(socketPath); err != nil {
		return nil, utils.Errorf("failed to remove existing socket: %v", err)
	}

	// Create Unix domain socket listener
	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		return nil, utils.Errorf("failed to listen on socket %s: %v", socketPath, err)
	}

	return listener, nil
}

// DialSocket connects to a Unix domain socket at the specified path.
func DialSocket(socketPath string) (net.Conn, error) {
	conn, err := net.Dial("unix", socketPath)
	if err != nil {
		return nil, utils.Errorf("failed to dial socket %s: %v", socketPath, err)
	}
	return conn, nil
}
