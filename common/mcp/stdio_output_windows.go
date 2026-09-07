//go:build windows

package mcp

import (
	"os"
	"sync"

	"golang.org/x/sys/windows"
)

func reserveMCPProtocolStdout() (*os.File, func() error, error) {
	originalStdout := os.Stdout
	originalHandle, err := windows.GetStdHandle(windows.STD_OUTPUT_HANDLE)
	if err != nil {
		return nil, nil, err
	}
	stderrHandle, err := windows.GetStdHandle(windows.STD_ERROR_HANDLE)
	if err != nil {
		return nil, nil, err
	}

	// Keep the protocol pipe on a private, non-inheritable handle. SetStdHandle
	// only changes the process standard-handle slot; the duplicate remains bound
	// to the caller's original stdout and is owned exclusively by the MCP writer.
	var protocolHandle windows.Handle
	currentProcess := windows.CurrentProcess()
	if err := windows.DuplicateHandle(
		currentProcess,
		originalHandle,
		currentProcess,
		&protocolHandle,
		0,
		false,
		windows.DUPLICATE_SAME_ACCESS,
	); err != nil {
		return nil, nil, err
	}
	protocolStdout := os.NewFile(uintptr(protocolHandle), "mcp-protocol-stdout")
	if protocolStdout == nil {
		_ = windows.CloseHandle(protocolHandle)
		return nil, nil, os.ErrInvalid
	}

	if err := windows.SetStdHandle(windows.STD_OUTPUT_HANDLE, stderrHandle); err != nil {
		_ = protocolStdout.Close()
		return nil, nil, err
	}
	os.Stdout = os.Stderr

	var once sync.Once
	var restoreErr error
	restore := func() error {
		once.Do(func() {
			restoreErr = windows.SetStdHandle(windows.STD_OUTPUT_HANDLE, originalHandle)
			os.Stdout = originalStdout
			if closeErr := protocolStdout.Close(); restoreErr == nil {
				restoreErr = closeErr
			}
		})
		return restoreErr
	}
	return protocolStdout, restore, nil
}
