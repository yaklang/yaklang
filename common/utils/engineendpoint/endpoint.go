// Package engineendpoint provides opt-in, private IPC endpoints for the engine CLI.
// It deliberately does not change the historical lowtun socket API used elsewhere.
package engineendpoint

import (
	"fmt"
	"path/filepath"
	"runtime"
	"strings"
)

// Validate must run before database initialization or any filesystem mutation.
func Validate(transport, endpoint string) error {
	switch transport {
	case "tcp":
		if endpoint != "" {
			return fmt.Errorf("socket-path requires an explicit unix or npipe transport")
		}
	case "unix":
		if runtime.GOOS == "windows" {
			return fmt.Errorf("unix transport is not supported on Windows; use npipe")
		}
		// 103 bytes fits both Linux and Darwin sockaddr_un, leaving room for NUL.
		if !filepath.IsAbs(endpoint) || strings.ContainsRune(endpoint, 0) || len(endpoint) > 103 {
			return fmt.Errorf("unix socket-path must be an absolute path of at most 103 bytes")
		}
	case "npipe":
		if runtime.GOOS != "windows" {
			return fmt.Errorf("npipe transport is only supported on Windows; use unix")
		}
		const prefix = `\\.\pipe\`
		name := strings.TrimPrefix(endpoint, prefix)
		if name == endpoint || name == "" || strings.ContainsAny(name, `/\`+"\x00") || len(endpoint) > 256 {
			return fmt.Errorf("npipe socket-path must be a full local named pipe path (\\\\.\\pipe\\name)")
		}
	default:
		return fmt.Errorf("unsupported transport %q; expected tcp, unix or npipe", transport)
	}
	return nil
}
