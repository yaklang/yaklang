//go:build windows

package engineendpoint

import (
	"context"
	"fmt"
	"net"

	"github.com/Microsoft/go-winio"
	"golang.org/x/sys/windows"
)

func privatePipeSecurityDescriptor() (string, error) {
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		return "", err
	}
	return "D:P(A;;GA;;;SY)(A;;GA;;;" + user.User.Sid.String() + ")", nil
}

func Listen(transport, endpoint string) (net.Listener, error) {
	if err := Validate(transport, endpoint); err != nil {
		return nil, err
	}
	if transport != "npipe" {
		return nil, fmt.Errorf("IPC listener requires npipe transport")
	}
	sddl, err := privatePipeSecurityDescriptor()
	if err != nil {
		return nil, err
	}
	// go-winio creates the first pipe with FILE_CREATE, refusing to take over an existing pipe.
	return winio.ListenPipe(endpoint, &winio.PipeConfig{
		SecurityDescriptor: sddl, InputBufferSize: 4096, OutputBufferSize: 4096,
	})
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
