// Package engineendpoint provides opt-in, private IPC endpoints for the engine CLI.
// It deliberately does not change the historical lowtun socket API used elsewhere.
package engineendpoint

import (
	"errors"
	"fmt"
	"path/filepath"
	"runtime"
	"strings"
	"unicode/utf16"
	"unicode/utf8"
)

var ErrInvalidDirectory = errors.New("unix socket parent must be a directory, not a symbolic link")

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
		if len(endpoint) <= len(prefix) || !strings.EqualFold(endpoint[:len(prefix)], prefix) {
			return fmt.Errorf("npipe socket-path must be a full local named pipe path (\\\\.\\pipe\\name)")
		}
		name := endpoint[len(prefix):]
		// Win32 measures this limit in UTF-16 code units, not UTF-8 bytes.
		if !utf8.ValidString(endpoint) || strings.ContainsAny(name, `/\`+"\x00") || len(utf16.Encode([]rune(endpoint))) > 256 {
			return fmt.Errorf("npipe socket-path must be a full local named pipe path (\\\\.\\pipe\\name)")
		}
	default:
		return fmt.Errorf("unsupported transport %q; expected tcp, unix or npipe", transport)
	}
	return nil
}
