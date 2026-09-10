//go:build !windows

package engineendpoint

import (
	"context"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"sync"
	"syscall"
)

type ownedListener struct {
	net.Listener
	path string
	file os.FileInfo
	once sync.Once
	err  error
}

func (l *ownedListener) Close() error {
	l.once.Do(func() {
		l.err = l.Listener.Close()
		// Never unlink a replacement path, even if this instance's socket was moved.
		if current, err := os.Lstat(l.path); err == nil && current.Mode()&os.ModeSocket != 0 && os.SameFile(l.file, current) {
			if err := os.Remove(l.path); l.err == nil {
				l.err = err
			}
		}
	})
	return l.err
}

// PrepareListener checks the endpoint before expensive server/database startup.
// It creates only a missing immediate parent; existing directories are never chmodded.
// Listen repeats these checks because the filesystem can change after preflight.
func PrepareListener(transport, endpoint string) error {
	if err := Validate(transport, endpoint); err != nil {
		return err
	}
	if transport != "unix" {
		return fmt.Errorf("IPC listener requires unix transport")
	}
	parent := filepath.Dir(endpoint)
	if err := os.Mkdir(parent, 0700); err != nil && !os.IsExist(err) {
		return err
	}
	dir, err := os.Lstat(parent)
	if err != nil {
		return err
	}
	// Existing project directories may be shared (0755/0777). The CLI requires
	// authentication, and the socket itself is restricted to 0600 below. Do not
	// change permissions on an existing directory or require users to move it.
	if !dir.IsDir() || dir.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("%w: %q", ErrInvalidDirectory, parent)
	}
	// Refuse files, directories, links, live sockets AND stale sockets. No takeover.
	if _, err := os.Lstat(endpoint); !os.IsNotExist(err) {
		if err != nil {
			return err
		}
		return fmt.Errorf("endpoint already exists: %w", syscall.EADDRINUSE)
	}
	return nil
}

func Listen(transport, endpoint string) (net.Listener, error) {
	if err := PrepareListener(transport, endpoint); err != nil {
		return nil, err
	}
	listener, err := net.ListenUnix("unix", &net.UnixAddr{Name: endpoint, Net: "unix"})
	if err != nil {
		return nil, err
	}
	listener.SetUnlinkOnClose(false)
	file, err := os.Lstat(endpoint)
	if err != nil {
		listener.Close()
		return nil, err
	}
	owned := &ownedListener{Listener: listener, path: endpoint, file: file}
	if err := os.Chmod(endpoint, 0600); err != nil {
		owned.Close()
		return nil, err
	}
	return owned, nil
}

func DialContext(ctx context.Context, transport, endpoint string) (net.Conn, error) {
	if err := Validate(transport, endpoint); err != nil {
		return nil, err
	}
	if transport != "unix" {
		return nil, fmt.Errorf("IPC dial requires unix transport")
	}
	return (&net.Dialer{}).DialContext(ctx, "unix", endpoint)
}
