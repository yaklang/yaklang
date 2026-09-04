//go:build darwin || linux

package mcp

import (
	"os"
	"sync"

	"golang.org/x/sys/unix"
)

// reserveMCPProtocolStdout duplicates the inherited stdout for the protocol
// writer, then points file descriptor 1 at stderr. Redirecting the descriptor
// (rather than only assigning os.Stdout) also contains writers cached by
// libraries or embedded runtimes before the MCP command starts.
func reserveMCPProtocolStdout() (*os.File, func() error, error) {
	stdoutFD := int(os.Stdout.Fd())
	protocolFD, err := unix.Dup(stdoutFD)
	if err != nil {
		return nil, nil, err
	}
	unix.CloseOnExec(protocolFD)

	protocolStdout := os.NewFile(uintptr(protocolFD), "mcp-protocol-stdout")
	if protocolStdout == nil {
		_ = unix.Close(protocolFD)
		return nil, nil, os.ErrInvalid
	}

	if err := unix.Dup2(int(os.Stderr.Fd()), stdoutFD); err != nil {
		_ = protocolStdout.Close()
		return nil, nil, err
	}

	var once sync.Once
	var restoreErr error
	restore := func() error {
		once.Do(func() {
			restoreErr = unix.Dup2(int(protocolStdout.Fd()), stdoutFD)
			if closeErr := protocolStdout.Close(); restoreErr == nil {
				restoreErr = closeErr
			}
		})
		return restoreErr
	}

	return protocolStdout, restore, nil
}
