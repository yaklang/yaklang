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

func Listen(transport, endpoint string) (net.Listener, error) {
	if err := Validate(transport, endpoint); err != nil {
		return nil, err
	}
	if transport != "unix" {
		return nil, fmt.Errorf("IPC listener requires unix transport")
	}
	parent := filepath.Dir(endpoint)
	if err := os.Mkdir(parent, 0700); err != nil && !os.IsExist(err) {
		return nil, err
	}
	dir, err := os.Lstat(parent)
	if err != nil {
		return nil, err
	}
	owner, ok := dir.Sys().(*syscall.Stat_t)
	if !dir.IsDir() || dir.Mode()&os.ModeSymlink != 0 || dir.Mode().Perm()&0077 != 0 || !ok || owner.Uid != uint32(os.Geteuid()) {
		return nil, fmt.Errorf("unix socket parent must be a private directory owned by the current user (mode 0700)")
	}
	// Refuse files, directories, links, live sockets AND stale sockets. No takeover.
	if _, err := os.Lstat(endpoint); !os.IsNotExist(err) {
		if err != nil {
			return nil, err
		}
		return nil, fmt.Errorf("endpoint already exists: %w", syscall.EADDRINUSE)
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
