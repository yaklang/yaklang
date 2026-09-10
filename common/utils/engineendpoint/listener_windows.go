//go:build windows

package engineendpoint

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"strings"
	"syscall"

	"github.com/Microsoft/go-winio"
	"golang.org/x/sys/windows"
)

func privatePipeSecurityDescriptor() (string, error) {
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		return "", err
	}
	// Pin IPC to desktop (medium) integrity so elevated startup does not depend
	// on implicit object-label behavior. The same user's ordinary client can
	// connect; setting this label needs no elevation or SeSecurityPrivilege.
	return "D:P(A;;GA;;;SY)(A;;GA;;;" + user.User.Sid.String() + ")S:(ML;;NW;;;ME)", nil
}

// Windows has no filesystem parent to prepare. Detect an occupied name without
// connecting to its server; only Listen's FILE_CREATE reserves ownership.
func PrepareListener(transport, endpoint string) error {
	if err := Validate(transport, endpoint); err != nil {
		return err
	}
	if transport != "npipe" {
		return fmt.Errorf("IPC listener requires npipe transport")
	}
	if namedPipeExists(endpoint) {
		return &os.PathError{Op: "listen", Path: endpoint, Err: syscall.EADDRINUSE}
	}
	return nil
}

func Listen(transport, endpoint string) (net.Listener, error) {
	if err := PrepareListener(transport, endpoint); err != nil {
		return nil, err
	}
	sddl, err := privatePipeSecurityDescriptor()
	if err != nil {
		return nil, err
	}
	// go-winio creates the first pipe with FILE_CREATE, refusing to take over an existing pipe.
	listener, err := winio.ListenPipe(endpoint, &winio.PipeConfig{
		SecurityDescriptor: sddl, InputBufferSize: 4096, OutputBufferSize: 4096,
	})
	// FILE_CREATE reports ERROR_ACCESS_DENIED for an existing pipe, too.
	// Recheck the name after a denied bind: never connect to, close or replace
	// the other server merely to distinguish occupancy from a policy denial.
	if errors.Is(err, os.ErrExist) || (errors.Is(err, windows.ERROR_ACCESS_DENIED) && namedPipeExists(endpoint)) {
		return nil, &os.PathError{Op: "listen", Path: endpoint, Err: syscall.EADDRINUSE}
	}
	return listener, err
}

func namedPipeExists(endpoint string) bool {
	path, err := windows.UTF16PtrFromString(endpoint)
	if err != nil {
		return false
	}
	var data windows.Win32finddata
	handle, err := windows.FindFirstFile(path, &data)
	if err != nil {
		return false
	}
	defer windows.FindClose(handle)
	// FindFirstFile understands wildcards; only an exact name is evidence.
	return strings.EqualFold(windows.UTF16ToString(data.FileName[:]), endpoint[len(`\\.\pipe\`):])
}

func DialContext(ctx context.Context, transport, endpoint string) (net.Conn, error) {
	if err := Validate(transport, endpoint); err != nil {
		return nil, err
	}
	if transport != "npipe" {
		return nil, fmt.Errorf("IPC dial requires npipe transport")
	}
	return winio.DialPipeContext(ctx, endpoint)
}
